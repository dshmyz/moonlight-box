package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type SearchRequest struct {
	Query      string
	Type       string
	Name       string
	Version    string
	Repository string
	Sort       string
	Page       int
	PageSize   int
}

// SearchEntry groups artifacts by name for browse/search (one row per name in a repo).
type SearchEntry struct {
	ID             uint      `json:"id"`
	RepositoryID   uint      `json:"repository_id"`
	Format         string    `json:"format"`
	Namespace      string    `json:"namespace,omitempty"`
	Name           string    `json:"name"`
	DisplayName    string    `json:"display_name"`
	Description    string    `json:"description,omitempty"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	VersionCount   int       `json:"version_count"`
	DownloadCount  int64     `json:"download_count"`
	RepositoryName string    `json:"repository_name,omitempty"`
	License        string    `json:"license,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SearchResult struct {
	List         []SearchEntry `json:"list"`
	Total        int64         `json:"total"`
	Page         int           `json:"page"`
	PageSize     int           `json:"page_size"`
	SearchTimeMs int64         `json:"search_time_ms"`
	RawCount     int           `json:"raw_count"`
}

type PackageSearchService struct {
	db             *gorm.DB
	cache          *cache.MemoryCache
	hasPackagesTbl bool
}

func NewPackageSearchService(db *gorm.DB) *PackageSearchService {
	return &PackageSearchService{
		db:             db,
		cache:          cache.NewMemoryCache(), // 默认缓存
		hasPackagesTbl: db.Migrator().HasTable(&model.Package{}),
	}
}

// InvalidateCache 清除所有搜索缓存（当 artifact 变更时调用）
func (s *PackageSearchService) InvalidateCache() {
	s.cache.Clear()
}

// generateCacheKey 生成缓存键
func (s *PackageSearchService) generateCacheKey(req *SearchRequest) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d",
		req.Query, req.Type, req.Name, req.Version, req.Repository, req.Page, req.PageSize)
	hash := sha256.Sum256([]byte(data))
	return base64.URLEncoding.EncodeToString(hash[:])
}

// rawArtifact 原始 SQL 查询结果行，避免 GORM JSONB scan 问题
type rawArtifact struct {
	ID           uint      `gorm:"column:id"`
	RepositoryID uint      `gorm:"column:repository_id"`
	Format       string    `gorm:"column:format"`
	Name         string    `gorm:"column:name"`
	Version      string    `gorm:"column:version"`
	Attributes   string    `gorm:"column:attributes"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// Search 从 packages 聚合表搜索包。
// packages 表存在时始终使用快速路径，避免列表接口在大 artifacts 表上做全量聚合。
func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	// 尝试从缓存获取结果
	cacheKey := s.generateCacheKey(req)
	if cached, ok := s.cache.Get(cacheKey); ok {
		if result, ok := cached.(*SearchResult); ok {
			return result, nil
		}
	}

	pkgResult, pkgErr := s.searchFromPackages(ctx, req)
	if pkgErr == nil && pkgResult != nil {
		s.cache.Set(cacheKey, pkgResult, 5*time.Minute)
		return pkgResult, nil
	}

	util.WithFields(logrus.Fields{
		util.LogKeyModule: "package-search",
		"reason":          pkgErr.Error(),
	}).Warn("Falling back to artifacts aggregation because packages table is unavailable")

	artifactResult, err := s.searchFromArtifacts(ctx, req)
	if err != nil {
		return nil, err
	}
	result := artifactResult
	if artifactResult.Total == 0 && pkgErr == nil && pkgResult != nil {
		result = pkgResult
	}

	// 缓存结果（5分钟）
	s.cache.Set(cacheKey, result, 5*time.Minute)

	return result, nil
}

// searchFromPackages 从 packages 表查询（快速路径）
func (s *PackageSearchService) searchFromPackages(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

	// 检查 packages 表是否存在（启动时缓存结果，避免每次请求查询）
	if !s.hasPackagesTbl {
		return nil, fmt.Errorf("packages table not exists")
	}

	query := s.db.WithContext(ctx).Model(&model.Package{})
	query = query.Where(searchablePackageSQL("packages"))

	// 构建查询条件
	if req.Type != "" {
		types := util.ExpandPackageTypeAliases(req.Type)
		query = query.Where("format IN ?", types)
	}
	if req.Repository != "" {
		query = query.Where("repository_id = (SELECT id FROM repositories WHERE name = ? LIMIT 1)", req.Repository)
	}
	if req.Query != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(req.Query)+"%")
	}
	if req.Name != "" {
		query = query.Where("name = ?", req.Name)
	}

	// 查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 排序
	switch req.Sort {
	case "name":
		query = query.Order("name ASC")
	case "download_count":
		query = query.Order("download_count DESC")
	case "updated_at":
		query = query.Order("updated_at DESC")
	default:
		query = query.Order("updated_at DESC")
	}

	// 分页
	offset := (req.Page - 1) * req.PageSize
	var packages []model.Package
	if err := query.Offset(offset).Limit(req.PageSize).Find(&packages).Error; err != nil {
		return nil, err
	}

	// 查询仓库名称
	repoIDs := make(map[uint]bool)
	for _, pkg := range packages {
		repoIDs[pkg.RepositoryID] = true
	}
	repoIDList := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		repoIDList = append(repoIDList, id)
	}

	repoNameMap := make(map[uint]string)
	if len(repoIDList) > 0 {
		type repoRow struct {
			ID   uint
			Name string
		}
		var repoRows []repoRow
		s.db.Model(&model.Repository{}).Select("id, name").Where("id IN ?", repoIDList).Find(&repoRows)
		for _, r := range repoRows {
			repoNameMap[r.ID] = r.Name
		}
	}

	// 构建结果
	list := make([]SearchEntry, len(packages))
	for i, pkg := range packages {
		list[i] = SearchEntry{
			ID:             pkg.ID,
			RepositoryID:   pkg.RepositoryID,
			Format:         pkg.Format,
			Name:           pkg.Name,
			DisplayName:    pkg.DisplayName,
			Description:    pkg.Description,
			LatestVersion:  pkg.LatestVersion,
			VersionCount:   pkg.VersionCount,
			DownloadCount:  pkg.DownloadCount,
			RepositoryName: repoNameMap[pkg.RepositoryID],
			License:        pkg.License,
			UpdatedAt:      pkg.UpdatedAt,
		}
	}

	return &SearchResult{
		List:         list,
		Total:        total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SearchTimeMs: time.Since(start).Milliseconds(),
		RawCount:     len(packages),
	}, nil
}

// searchFromArtifacts 从 artifacts 表聚合查询（慢速路径，回退方案）
func (s *PackageSearchService) searchFromArtifacts(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

	// 构建基础查询条件
	var conditions []string
	var args []interface{}

	if req.Type != "" {
		types := util.ExpandPackageTypeAliases(req.Type)
		placeholders := make([]string, len(types))
		for i, t := range types {
			placeholders[i] = "?"
			args = append(args, t)
		}
		conditions = append(conditions, fmt.Sprintf("format IN (%s)", strings.Join(placeholders, ",")))
	}
	if req.Repository != "" {
		conditions = append(conditions, "repository_id = (SELECT id FROM repositories WHERE name = ? LIMIT 1)")
		args = append(args, req.Repository)
	}
	if req.Query != "" {
		conditions = append(conditions, "LOWER(name) LIKE ?")
		args = append(args, "%"+strings.ToLower(req.Query)+"%")
	}
	if req.Name != "" {
		conditions = append(conditions, "name = ?")
		args = append(args, req.Name)
	}
	conditions = append(conditions, searchableArtifactSQL("artifacts"))

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 步骤1：查询所有匹配的 artifacts（用于聚合）
	// 使用索引优化查询，按时间倒序
	query := "SELECT id, repository_id, format, name, version, attributes, created_at, updated_at FROM artifacts" +
		whereClause + " ORDER BY updated_at DESC"

	var rawRows []rawArtifact
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rawRows).Error; err != nil {
		return nil, err
	}

	// 步骤2：在内存中聚合（按 repository_id + format + name 分组）
	type groupKey struct {
		repositoryID uint
		format       string
		name         string
	}
	groups := make(map[groupKey]*groupAcc)

	for _, row := range rawRows {
		name := row.Name
		if name == "" {
			continue
		}

		if req.Version != "" {
			ver := row.Version
			if ver != req.Version {
				continue
			}
		}

		key := groupKey{row.RepositoryID, row.Format, name}
		acc, ok := groups[key]
		if !ok {
			acc = &groupAcc{name: name, format: row.Format, repositoryID: row.RepositoryID}
			groups[key] = acc
		}
		acc.versionCount++
		if row.UpdatedAt.After(acc.latestTime) {
			acc.latestTime = row.UpdatedAt
		}
		if acc.firstID == 0 {
			acc.firstID = row.ID
		}
		if acc.license == "" {
			acc.license = extractField(row.Attributes, "license")
		}
		if acc.description == "" {
			acc.description = extractField(row.Attributes, "description")
		}
	}

	// 步骤3：转换为切片并排序
	var allEntries []groupKey
	for key := range groups {
		allEntries = append(allEntries, key)
	}

	switch req.Sort {
	case "name":
		sort.Slice(allEntries, func(i, j int) bool {
			return groups[allEntries[i]].name < groups[allEntries[j]].name
		})
	case "updated_at":
		sort.Slice(allEntries, func(i, j int) bool {
			return groups[allEntries[i]].latestTime.After(groups[allEntries[j]].latestTime)
		})
	default:
		sort.Slice(allEntries, func(i, j int) bool {
			return groups[allEntries[i]].latestTime.After(groups[allEntries[j]].latestTime)
		})
	}

	total := int64(len(allEntries))

	// 步骤4：分页
	offset := (req.Page - 1) * req.PageSize
	if offset > len(allEntries) {
		offset = len(allEntries)
	}
	end := offset + req.PageSize
	if end > len(allEntries) {
		end = len(allEntries)
	}
	pagedKeys := allEntries[offset:end]

	// 步骤5：查询仓库名称
	repoIDs := make(map[uint]bool)
	for _, k := range pagedKeys {
		repoIDs[k.repositoryID] = true
	}
	repoIDList := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		repoIDList = append(repoIDList, id)
	}

	repoNameMap := make(map[uint]string)
	if len(repoIDList) > 0 {
		type repoRow struct {
			ID   uint
			Name string
		}
		var repoRows []repoRow
		s.db.Model(&model.Repository{}).Select("id, name").Where("id IN ?", repoIDList).Find(&repoRows)
		for _, r := range repoRows {
			repoNameMap[r.ID] = r.Name
		}
	}

	// 步骤6：构建结果
	list := make([]SearchEntry, len(pagedKeys))
	for i, k := range pagedKeys {
		acc := groups[k]
		list[i] = SearchEntry{
			ID:             acc.firstID,
			RepositoryID:   acc.repositoryID,
			Format:         acc.format,
			Name:           acc.name,
			Description:    acc.description,
			VersionCount:   acc.versionCount,
			UpdatedAt:      acc.latestTime,
			RepositoryName: repoNameMap[acc.repositoryID],
			License:        acc.license,
		}
	}

	return &SearchResult{
		List:         list,
		Total:        total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SearchTimeMs: time.Since(start).Milliseconds(),
		RawCount:     len(rawRows),
	}, nil
}

type groupAcc struct {
	name         string
	format       string
	repositoryID uint
	versionCount int
	latestTime   time.Time
	firstID      uint
	license      string
	description  string
}

func searchableArtifactSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return "(" + prefix + "kind IS NULL OR " + prefix + "kind NOT IN ('metadata', 'checksum', 'directory'))" +
		" AND NOT (" + prefix + "format = 'yum' AND (" +
		prefix + "remote_path LIKE 'repodata/%' OR " +
		prefix + "remote_path LIKE '%/repodata/%' OR " +
		prefix + "path = 'repodata' OR " +
		prefix + "path LIKE '%/repodata' OR " +
		prefix + "filename = 'repomd.xml'))"
}

func searchablePackageSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	name := prefix + "name"
	format := prefix + "format"
	versionCount := prefix + "version_count"

	return "(" + name + " IS NOT NULL AND " + name + " != '' AND " + versionCount + " > 0)" +
		" AND NOT (" + format + " = 'yum' AND (" +
		name + " = 'repomd.xml' OR " +
		name + " LIKE '%.xml' OR " +
		name + " LIKE '%.xml.gz' OR " +
		name + " LIKE '%.xml.xz' OR " +
		name + " LIKE '%.xml.bz2' OR " +
		name + " LIKE '%.sqlite' OR " +
		name + " LIKE '%.sqlite.gz' OR " +
		name + " LIKE '%.sqlite.xz' OR " +
		name + " LIKE '%.sqlite.bz2'))" +
		" AND NOT (" + format + " = 'apt' AND (" +
		name + " IN ('Packages', 'Packages.gz', 'Packages.xz', 'Packages.bz2', 'Release', 'InRelease') OR " +
		name + " LIKE '%/Packages' OR " +
		name + " LIKE '%/Packages.gz' OR " +
		name + " LIKE '%/Packages.xz' OR " +
		name + " LIKE '%/Packages.bz2'))"
}

// extractField 从 JSON 字符串中提取单个字段值。
func extractField(coordsJSON, key string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(coordsJSON), &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}
