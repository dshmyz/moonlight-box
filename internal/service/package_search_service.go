package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/database/dialect"
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

	// packageVersions 表存在性在启动后通常不变，用 sync.Once 缓存避免每次请求执行系统表查询。
	packageVersionsTableOnce  sync.Once
	packageVersionsTableReady bool
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
	// 注意：必须包含所有影响结果集和顺序的请求字段，否则不同输入会命中同一缓存条目，
	// 导致结果污染（例如不同 Sort 复用同一份按某顺序排序的结果）。
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d|%d",
		req.Query, req.Type, req.Name, req.Version, req.Repository, req.Sort, req.Page, req.PageSize)
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

	if pkgErr != nil {
		util.WithFields(logrus.Fields{
			util.LogKeyModule: "package-search",
			"reason":          pkgErr.Error(),
		}).Warn("Falling back to artifacts aggregation because packages table is unavailable")
	}

	artifactResult, err := s.searchFromArtifacts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search artifacts fallback failed: %w", err)
	}

	// 缓存结果（5分钟）
	s.cache.Set(cacheKey, artifactResult, 5*time.Minute)

	return artifactResult, nil
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

	// 版本筛选：packages 表只有 latest_version，无法匹配历史版本。
	// 用 IN 子查询下推到 artifacts 表，先查出匹配版本的 (repo_id, format, name) 集合，
	// 再过滤 packages，避免 EXISTS 对每行 packages 重复执行相关子查询。
	// 注意：含 [ 的 glob 字符类（如 [12].0.0）SQL LIKE 不支持，
	// 需回退到 searchFromArtifacts 内存 filepath.Match 精确过滤。
	if req.Version != "" {
		if strings.Contains(req.Version, "[") {
			return nil, fmt.Errorf("version pattern with char class requires artifacts fallback")
		}
		verCond, verArgs := versionToSQLCondition("a.version", req.Version)
		if verCond != "" {
			subQuery := "packages.repository_id || '|' || packages.format || '|' || packages.name IN (" +
				"SELECT a.repository_id || '|' || a.format || '|' || a.name FROM artifacts a " +
				"WHERE a.version != '' AND " + verCond + " AND " +
				"(a.kind IS NULL OR a.kind NOT IN ('metadata','checksum','directory')))"
			query = query.Where(subQuery, verArgs...)
		}
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
	if !strings.Contains(req.Version, "[") {
		return s.searchFromArtifactsGrouped(ctx, req)
	}

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
	// version 下推到 SQL，走 idx_artifact_version 索引，避免全量加载后内存逐行 Match。
	// 含 [ 的字符类 SQL LIKE 不支持，跳过 SQL 过滤，在内存用 filepath.Match 精过滤。
	if req.Version != "" && !strings.Contains(req.Version, "[") {
		verCond, verArgs := versionToSQLCondition("version", req.Version)
		if verCond != "" {
			conditions = append(conditions, verCond)
			args = append(args, verArgs...)
		}
	}
	conditions = append(conditions, searchableArtifactSQL("artifacts"))

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 对包含字符类的版本模式，数据库无法用跨方言 SQL 完整表达 filepath.Match。
	// 逐批扫描并立即聚合，既保持原有匹配语义，也不会截断较旧的 artifact。
	type groupKey struct {
		repositoryID uint
		format       string
		name         string
	}
	groups := make(map[groupKey]*groupAcc)
	const artifactBatchSize = 1000
	rawCount := 0
	lastID := uint(0)
	for {
		batchArgs := append([]interface{}{}, args...)
		batchArgs = append(batchArgs, lastID, artifactBatchSize)
		batchWhere := whereClause + " AND artifacts.id > ?"
		batchQuery := "SELECT id, repository_id, format, name, version, attributes, created_at, updated_at FROM artifacts" +
			batchWhere + " ORDER BY id ASC LIMIT ?"
		var rows []rawArtifact
		if err := s.db.WithContext(ctx).Raw(batchQuery, batchArgs...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		rawCount += len(rows)
		lastID = rows[len(rows)-1].ID

		for _, row := range rows {
			if row.Name == "" {
				continue
			}
			matched, _ := filepath.Match(req.Version, row.Version)
			if !matched {
				continue
			}
			key := groupKey{row.RepositoryID, row.Format, row.Name}
			acc, ok := groups[key]
			if !ok {
				acc = &groupAcc{name: row.Name, format: row.Format, repositoryID: row.RepositoryID}
				groups[key] = acc
			}
			acc.versionCount++
			if row.UpdatedAt.After(acc.latestTime) {
				acc.latestTime = row.UpdatedAt
			}
			if acc.firstID == 0 {
				acc.firstID = row.ID
			}
			// license/description 取最新非空：遇到更晚 updated_at 的行时覆盖已有值，
			// 与 searchFromArtifactsGrouped 的 SQL 子查询（ORDER BY updated_at DESC LIMIT 1）对齐。
			if license := extractField(row.Attributes, "license"); license != "" {
				if acc.license == "" || row.UpdatedAt.After(acc.latestLicenseTime) {
					acc.license = license
					acc.latestLicenseTime = row.UpdatedAt
				}
			}
			if description := extractField(row.Attributes, "description"); description != "" {
				if acc.description == "" || row.UpdatedAt.After(acc.latestDescriptionTime) {
					acc.description = description
					acc.latestDescriptionTime = row.UpdatedAt
				}
			}
		}
		if len(rows) < artifactBatchSize {
			break
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
		RawCount:     rawCount,
	}, nil
}

type groupedArtifact struct {
	ID           uint   `gorm:"column:id"`
	RepositoryID uint   `gorm:"column:repository_id"`
	Format       string `gorm:"column:format"`
	Name         string `gorm:"column:name"`
	License      string `gorm:"column:license"`
	Description  string `gorm:"column:description"`
	VersionCount int    `gorm:"column:version_count"`
	UpdatedAt    string `gorm:"column:updated_at"`
}

// searchFromArtifactsGrouped lets the database group artifact rows before it
// counts and paginates packages. This prevents a package with many versions
// from hiding older packages behind an arbitrary artifact-row scan limit.
func (s *PackageSearchService) searchFromArtifactsGrouped(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()
	conditions, args := artifactSearchConditions(req)
	conditions = append(conditions, searchableArtifactSQL("artifacts"))
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	var rawCount int64
	if err := s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM artifacts"+whereClause, args...).Scan(&rawCount).Error; err != nil {
		return nil, err
	}

	// 对 JSONB 字段 attributes 不能用 MAX()：不同方言行为不一致（SQLite 按字典序取最大字符串，
	// PostgreSQL 可能报错），且取到的不是最新版本的 license/description。
	// 改用相关子查询按 updated_at DESC 取最新行的对应字段，与 ArtifactService.RebuildPackages 对齐。
	dialectName := s.db.Dialector.Name()
	licenseExpr := dialect.JSONTextExpr(dialectName, "a2.attributes", "license")
	descriptionExpr := dialect.JSONTextExpr(dialectName, "a3.attributes", "description")
	groupedQuery := "SELECT MIN(id) AS id, repository_id, format, name, " +
		"COUNT(*) AS version_count, MAX(updated_at) AS updated_at, " +
		"COALESCE((SELECT " + licenseExpr + " FROM artifacts a2 WHERE a2.repository_id = artifacts.repository_id " +
		"AND a2.format = artifacts.format AND a2.name = artifacts.name " +
		"AND " + licenseExpr + " IS NOT NULL AND " + licenseExpr + " != '' " +
		"ORDER BY a2.updated_at DESC LIMIT 1), '') AS license, " +
		"COALESCE((SELECT " + descriptionExpr + " FROM artifacts a3 WHERE a3.repository_id = artifacts.repository_id " +
		"AND a3.format = artifacts.format AND a3.name = artifacts.name " +
		"AND " + descriptionExpr + " IS NOT NULL AND " + descriptionExpr + " != '' " +
		"ORDER BY a3.updated_at DESC LIMIT 1), '') AS description " +
		"FROM artifacts" + whereClause + " GROUP BY repository_id, format, name"

	var total int64
	if err := s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM ("+groupedQuery+") grouped", args...).Scan(&total).Error; err != nil {
		return nil, err
	}

	orderBy := "updated_at DESC"
	if req.Sort == "name" {
		orderBy = "name ASC"
	}
	offset := (req.Page - 1) * req.PageSize
	pageArgs := append(append([]interface{}{}, args...), req.PageSize, offset)
	var rows []groupedArtifact
	pageQuery := "SELECT id, repository_id, format, name, license, description, version_count, updated_at FROM (" +
		groupedQuery + ") grouped ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	if err := s.db.WithContext(ctx).Raw(pageQuery, pageArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	repoIDs := make(map[uint]bool)
	for _, row := range rows {
		repoIDs[row.RepositoryID] = true
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
		if err := s.db.WithContext(ctx).Model(&model.Repository{}).Select("id, name").Where("id IN ?", repoIDList).Find(&repoRows).Error; err != nil {
			return nil, err
		}
		for _, row := range repoRows {
			repoNameMap[row.ID] = row.Name
		}
	}

	list := make([]SearchEntry, len(rows))
	for i, row := range rows {
		updatedAt, err := parseGroupedArtifactTime(row.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse grouped artifact updated_at %q: %w", row.UpdatedAt, err)
		}
		list[i] = SearchEntry{
			ID:             row.ID,
			RepositoryID:   row.RepositoryID,
			Format:         row.Format,
			Name:           row.Name,
			Description:    row.Description,
			VersionCount:   row.VersionCount,
			UpdatedAt:      updatedAt,
			RepositoryName: repoNameMap[row.RepositoryID],
			License:        row.License,
		}
	}

	return &SearchResult{
		List:         list,
		Total:        total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SearchTimeMs: time.Since(start).Milliseconds(),
		RawCount:     int(rawCount),
	}, nil
}

func parseGroupedArtifactTime(value string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format")
}

func artifactSearchConditions(req *SearchRequest) ([]string, []interface{}) {
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
	if req.Version != "" {
		verCond, verArgs := versionToSQLCondition("version", req.Version)
		if verCond != "" {
			conditions = append(conditions, verCond)
			args = append(args, verArgs...)
		}
	}
	return conditions, args
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
	// latestLicenseTime / latestDescriptionTime 记录当前已采纳的 license/description
	// 来自哪一行的 updated_at；用于在内存聚合中复刻 grouped 路径
	// "ORDER BY updated_at DESC LIMIT 1 取最新非空字段"的语义，
	// 保证 glob 路径和 grouped 路径返回一致的元数据。
	latestLicenseTime     time.Time
	latestDescriptionTime time.Time
}

func searchableArtifactSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	idExpr := prefix + "id"
	return "(" + prefix + "kind IS NULL OR " + prefix + "kind NOT IN ('metadata', 'checksum', 'directory'))" +
		" AND (" + prefix + "format != 'go' OR " + prefix + "kind = 'version' OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = " + idExpr + "))" +
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
		" AND (" + format + " != 'go' OR EXISTS (" +
		"SELECT 1 FROM artifacts go_artifacts " +
		"WHERE go_artifacts.repository_id = " + prefix + "repository_id " +
		"AND go_artifacts.format = " + format + " " +
		"AND go_artifacts.name = " + name + " " +
		"AND go_artifacts.version != '' " +
		"AND (go_artifacts.kind = 'version' OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = go_artifacts.id))))" +
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

// versionToSQLCondition 将版本过滤条件转换为 SQL 片段和参数。
// - 无通配符：精确匹配，走 idx_artifact_version 索引，最快
// - 前缀通配（如 1.2.*）：转 LIKE '1.2.%'，走索引前缀
// - 其他通配（如 1.*.0）：转 LIKE，可能全表扫描
// 返回 (SQL片段, 参数)。pattern 为空时返回 ("", nil) 表示不过滤。
func versionToSQLCondition(column string, pattern string) (string, []interface{}) {
	if pattern == "" {
		return "", nil
	}
	// filepath.Match 的通配符：* ? [ ]，SQL LIKE 的通配符：% _
	// 不含通配符时用精确匹配，走索引
	if !strings.ContainsAny(pattern, "*?[") {
		return column + " = ?", []interface{}{pattern}
	}
	// 转换 glob 到 SQL LIKE：
	// % 和 _ 需要转义，* → %，? → _，[...] 不转（SQL LIKE 不支持字符类，退化为普通字符）
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '%', '_':
			// 转义 LIKE 的特殊字符
			b.WriteByte('\\')
			b.WriteRune(r)
		case '*':
			b.WriteByte('%')
		case '?':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return column + " LIKE ? ESCAPE '\\'", []interface{}{b.String()}
}

// ========== List: 关键字 → 包信息 + 版本列表 ==========

// ListRequest 关键字查询请求，一次返回包概要 + 完整版本列表。
type ListRequest struct {
	Query           string // 关键字：先子串匹配包名，未命中再子串匹配版本号
	Type            string // 包格式过滤（可选）
	Repository      string // 仓库名过滤（可选）
	Version         string // 版本号过滤（可选，支持 glob 通配符）
	FilesDownloaded *bool  // 版本级过滤：只返回 files_downloaded 匹配的版本；nil 表示不过滤
	Page            int
	PageSize        int
}

// ListVersionEntry 版本条目，字段对齐 PackageVersionHandler 返回的版本结构。
type ListVersionEntry struct {
	ID              uint       `json:"id"`
	RepositoryID    uint       `json:"repository_id"`
	Version         string     `json:"version"`
	Name            string     `json:"name"`
	Namespace       string     `json:"namespace,omitempty"`
	IdentityKey     string     `json:"identity_key,omitempty"`
	Status          string     `json:"status"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	SizeBytes       int64      `json:"size_bytes"`
	ChecksumSHA256  string     `json:"checksum_sha256,omitempty"`
	FileCount       int        `json:"file_count"`
	FilesDownloaded bool       `json:"files_downloaded"`
	DownloadCount   int64      `json:"download_count"`
	License         string     `json:"license,omitempty"`
}

// ListPackageEntry 包条目，内嵌完整版本列表。
type ListPackageEntry struct {
	ID             uint               `json:"id"`
	RepositoryID   uint               `json:"repository_id"`
	Format         string             `json:"format"`
	Namespace      string             `json:"namespace,omitempty"`
	Name           string             `json:"name"`
	DisplayName    string             `json:"display_name,omitempty"`
	Description    string             `json:"description,omitempty"`
	LatestVersion  string             `json:"latest_version,omitempty"`
	VersionCount   int                `json:"version_count"`
	DownloadCount  int64              `json:"download_count"`
	RepositoryName string             `json:"repository_name,omitempty"`
	License        string             `json:"license,omitempty"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Versions       []ListVersionEntry `json:"versions"`
}

// ListResult List 响应。
type ListResult struct {
	Packages     []ListPackageEntry `json:"packages"`
	Total        int64              `json:"total"`
	Page         int                `json:"page"`
	PageSize     int                `json:"page_size"`
	SearchTimeMs int64              `json:"search_time_ms"`
}

// List 根据关键字一次性返回包信息 + 版本列表。
//
// 匹配策略（方案 X）：
//  1. 先用 q 子串匹配包名，命中的包返回全部版本（再按 version 参数过滤）。
//  2. 包名未命中时，用 q 子串匹配版本号，命中的包只返回匹配版本。
//  3. 同一个包只出现一次（包名命中优先，版本列表完整）。
//
// 查询策略（批量查询，避免 N+1）：
//   - 包名匹配：Search 拿包列表（1 次 SQL）→ 批量查所有命中包的版本（1 次 SQL）
//   - 版本号匹配：查匹配版本的包标识（1 次 SQL）→ 批量查这些包的匹配版本（1 次 SQL）
func (s *PackageSearchService) List(ctx context.Context, req *ListRequest) (*ListResult, error) {
	start := time.Now()

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	// 第一步：按包名子串匹配
	nameSearchReq := &SearchRequest{
		Query:      req.Query,
		Type:       req.Type,
		Repository: req.Repository,
		Sort:       "updated_at",
		Page:       req.Page,
		PageSize:   req.PageSize,
	}
	nameResult, nameErr := s.Search(ctx, nameSearchReq)

	// 组装包名匹配结果 + 批量加载版本列表
	packages := make([]ListPackageEntry, 0)
	var total int64

	if nameErr == nil && nameResult != nil && len(nameResult.List) > 0 {
		total += nameResult.Total

		// 批量加载所有命中包的版本列表（1 次 SQL 替代 N 次）
		pkgKeys := make([]packageKeyTuple, 0, len(nameResult.List))
		for _, entry := range nameResult.List {
			pkgKeys = append(pkgKeys, packageKeyTuple{
				RepositoryID: entry.RepositoryID,
				Format:       entry.Format,
				Name:         entry.Name,
			})
		}
		versionMap, err := s.batchLoadVersions(ctx, pkgKeys, req.Version, req.FilesDownloaded)
		if err != nil {
			return nil, fmt.Errorf("batch load versions: %w", err)
		}

		for _, entry := range nameResult.List {
			key := fmt.Sprintf("%d|%s|%s", entry.RepositoryID, entry.Format, entry.Name)
			pkgEntry := ListPackageEntry{
				ID:             entry.ID,
				RepositoryID:   entry.RepositoryID,
				Format:         entry.Format,
				Namespace:      entry.Namespace,
				Name:           entry.Name,
				DisplayName:    entry.DisplayName,
				Description:    entry.Description,
				LatestVersion:  entry.LatestVersion,
				VersionCount:   entry.VersionCount,
				DownloadCount:  entry.DownloadCount,
				RepositoryName: entry.RepositoryName,
				License:        entry.License,
				UpdatedAt:      entry.UpdatedAt,
				Versions:       versionMap[key],
			}
			packages = append(packages, pkgEntry)
		}
	}

	// 第二步：按版本号子串匹配（包名匹配未命中或出错时）
	if nameErr != nil || nameResult == nil || nameResult.Total == 0 {
		versionMatchedPackages, versionMatchTotal, err := s.listByVersionMatch(ctx, req)
		if err != nil {
			return nil, err
		}

		// 合并版本号匹配的结果（去重：包名匹配已经包含的包不再重复加入）
		if len(versionMatchedPackages) > 0 {
			seen := make(map[string]bool)
			for _, p := range packages {
				seen[fmt.Sprintf("%d|%s|%s", p.RepositoryID, p.Format, p.Name)] = true
			}
			for _, p := range versionMatchedPackages {
				key := fmt.Sprintf("%d|%s|%s", p.RepositoryID, p.Format, p.Name)
				if !seen[key] {
					packages = append(packages, p)
					seen[key] = true
				}
			}
			total += versionMatchTotal
		}
	}

	// 如果两步都没有命中，返回空结果
	if len(packages) == 0 {
		return &ListResult{
			Packages:     []ListPackageEntry{},
			Total:        0,
			Page:         req.Page,
			PageSize:     req.PageSize,
			SearchTimeMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return &ListResult{
		Packages:     packages,
		Total:        total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SearchTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// packageKeyTuple 标识一个包（仓库 + 格式 + 名字）。
type packageKeyTuple struct {
	RepositoryID uint
	Format       string
	Name         string
	LatestAt     string `gorm:"column:latest_at"`
}

// lookupByVersionMatch 用 q 子串匹配版本号，返回命中版本的包列表。
// 仅在包名匹配未命中时调用。使用批量查询，避免 N+1。
func (s *PackageSearchService) listByVersionMatch(ctx context.Context, req *ListRequest) ([]ListPackageEntry, int64, error) {
	if req.Query == "" {
		return nil, 0, nil
	}

	// 查找版本号子串匹配 q 的包（去重）
	baseConditions, baseArgs := artifactSearchConditions(&SearchRequest{
		Type:       req.Type,
		Repository: req.Repository,
	})
	baseConditions = append(baseConditions, searchableArtifactSQL("artifacts"))
	baseConditions = append(baseConditions, "version != ''")
	baseConditions = append(baseConditions, "LOWER(version) LIKE ?")
	baseArgs = append(baseArgs, "%"+strings.ToLower(req.Query)+"%")

	// 如果指定了 version glob 过滤，叠加条件
	if req.Version != "" && !strings.Contains(req.Version, "[") {
		verCond, verArgs := versionToSQLCondition("version", req.Version)
		if verCond != "" {
			baseConditions = append(baseConditions, verCond)
			baseArgs = append(baseArgs, verArgs...)
		}
	}

	whereClause := " WHERE " + strings.Join(baseConditions, " AND ")

	groupedQuery := "SELECT repository_id, format, name, MAX(updated_at) AS latest_at FROM artifacts" +
		whereClause + " GROUP BY repository_id, format, name"

	// 先查总数（去重后的包数）
	countQuery := "SELECT COUNT(*) FROM (" + groupedQuery + ") grouped_packages"
	var total int64
	if err := s.db.WithContext(ctx).Raw(countQuery, baseArgs...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	// 分页查去重后的包标识
	offset := (req.Page - 1) * req.PageSize
	pageQuery := "SELECT repository_id, format, name, latest_at FROM (" + groupedQuery + ") grouped_packages ORDER BY latest_at DESC LIMIT ? OFFSET ?"
	pageArgs := append(append([]interface{}{}, baseArgs...), req.PageSize, offset)

	var pkgRows []packageKeyTuple
	if err := s.db.WithContext(ctx).Raw(pageQuery, pageArgs...).Scan(&pkgRows).Error; err != nil {
		return nil, 0, err
	}

	// 批量查仓库名
	repoIDs := make(map[uint]bool)
	for _, r := range pkgRows {
		repoIDs[r.RepositoryID] = true
	}
	repoNameMap := s.batchRepoNames(ctx, repoIDs)

	// 批量加载这些包的版本列表（1 次 SQL），再在内存按 q 子串过滤版本号
	versionMap, err := s.batchLoadVersions(ctx, pkgRows, req.Version, req.FilesDownloaded)
	if err != nil {
		return nil, 0, fmt.Errorf("batch load versions: %w", err)
	}

	// 组装结果：只保留版本号子串匹配 q 的版本
	packages := make([]ListPackageEntry, 0, len(pkgRows))
	qLower := strings.ToLower(req.Query)
	for _, r := range pkgRows {
		key := fmt.Sprintf("%d|%s|%s", r.RepositoryID, r.Format, r.Name)
		allVersions := versionMap[key]
		filtered := make([]ListVersionEntry, 0, len(allVersions))
		for _, v := range allVersions {
			if strings.Contains(strings.ToLower(v.Version), qLower) {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			continue
		}

		pkgEntry := ListPackageEntry{
			RepositoryID:   r.RepositoryID,
			Format:         r.Format,
			Name:           r.Name,
			RepositoryName: repoNameMap[r.RepositoryID],
			Versions:       filtered,
			VersionCount:   len(filtered),
			UpdatedAt:      parseSQLTime(r.LatestAt),
		}
		if len(filtered) > 0 {
			pkgEntry.LatestVersion = filtered[0].Version
		}
		packages = append(packages, pkgEntry)
	}

	return packages, total, nil
}

// batchLoadVersions 批量加载多个包的版本列表，返回按 "repoID|format|name" 索引的 map。
// 优先从 package_versions 读模型表查询，回退到 artifacts 聚合。
// versionFilter 支持空字符串（不过滤）或 glob 通配符。
// filesDownloaded 非 nil 时只返回 files_downloaded 匹配的版本。
func (s *PackageSearchService) batchLoadVersions(ctx context.Context, keys []packageKeyTuple, versionFilter string, filesDownloaded *bool) (map[string][]ListVersionEntry, error) {
	if len(keys) == 0 {
		return make(map[string][]ListVersionEntry), nil
	}
	if s.hasPackageVersionsTable() {
		return s.batchLoadVersionsFromReadModel(ctx, keys, versionFilter, filesDownloaded)
	}
	return s.batchLoadVersionsFromArtifacts(ctx, keys, versionFilter, filesDownloaded)
}

func (s *PackageSearchService) hasPackageVersionsTable() bool {
	s.packageVersionsTableOnce.Do(func() {
		s.packageVersionsTableReady = s.db.Migrator().HasTable(&model.PackageVersion{})
	})
	return s.packageVersionsTableReady
}

// batchLoadVersionsFromReadModel 从 package_versions 表批量查询版本列表。
// 使用 OR 组合多个 (repository_id, format, package_name) 条件，1 次 SQL 拿全。
func (s *PackageSearchService) batchLoadVersionsFromReadModel(ctx context.Context, keys []packageKeyTuple, versionFilter string, filesDownloaded *bool) (map[string][]ListVersionEntry, error) {
	result := make(map[string][]ListVersionEntry)
	if len(keys) == 0 {
		return result, nil
	}

	db := s.db.WithContext(ctx).Model(&model.PackageVersion{})
	// 用 OR 组合多个包标识条件
	orConditions := make([]string, 0, len(keys))
	orArgs := make([]interface{}, 0, len(keys)*3)
	for _, k := range keys {
		orConditions = append(orConditions, "(repository_id = ? AND format = ? AND package_name = ?)")
		orArgs = append(orArgs, k.RepositoryID, k.Format, k.Name)
	}
	db = db.Where(strings.Join(orConditions, " OR "), orArgs...)

	// 版本过滤：含 [ 的 glob 退化为内存过滤
	if versionFilter != "" && !strings.Contains(versionFilter, "[") {
		verCond, verArgs := versionToSQLCondition("version", versionFilter)
		if verCond != "" {
			db = db.Where(verCond, verArgs...)
		}
	}
	// files_downloaded 过滤（SQL 层，走索引更高效）
	if filesDownloaded != nil {
		db = db.Where("files_downloaded = ?", *filesDownloaded)
	}

	var summaries []model.PackageVersion
	if err := db.Order("latest_artifact_at DESC").Find(&summaries).Error; err != nil {
		return nil, err
	}

	for _, s := range summaries {
		key := fmt.Sprintf("%d|%s|%s", s.RepositoryID, s.Format, s.PackageName)
		// 含 [ 的 glob 在内存过滤
		if versionFilter != "" && strings.Contains(versionFilter, "[") {
			matched, _ := filepath.Match(versionFilter, s.Version)
			if !matched {
				continue
			}
		}
		entry := ListVersionEntry{
			ID:              s.ID,
			RepositoryID:    s.RepositoryID,
			Version:         s.Version,
			Name:            s.PackageName,
			Namespace:       s.Namespace,
			Status:          s.Status,
			PublishedAt:     s.PublishedAt,
			SizeBytes:       s.SizeBytes,
			ChecksumSHA256:  s.ChecksumSHA256,
			FileCount:       s.FileCount,
			FilesDownloaded: s.FilesDownloaded,
			DownloadCount:   s.DownloadCount,
			License:         s.License,
		}
		if entry.Status == "" {
			entry.Status = "published"
		}
		result[key] = append(result[key], entry)
	}
	return result, nil
}

// batchLoadVersionsFromArtifacts 从 artifacts 表批量聚合版本列表（回退路径）。
func (s *PackageSearchService) batchLoadVersionsFromArtifacts(ctx context.Context, keys []packageKeyTuple, versionFilter string, filesDownloaded *bool) (map[string][]ListVersionEntry, error) {
	result := make(map[string][]ListVersionEntry)
	if len(keys) == 0 {
		return result, nil
	}

	db := s.db.WithContext(ctx).Model(&model.Artifact{}).
		Where("version != ''").
		Where("(kind IS NULL OR kind NOT IN ?)", []string{"metadata", "checksum", "directory"}).
		Where("(format != 'go' OR kind = 'version' OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))").
		Where("NOT (format = 'yum' AND (remote_path LIKE 'repodata/%' OR remote_path LIKE '%/repodata/%' OR path = 'repodata' OR path LIKE '%/repodata' OR filename = 'repomd.xml'))")

	// 用 OR 组合多个包标识条件
	orConditions := make([]string, 0, len(keys))
	orArgs := make([]interface{}, 0, len(keys)*3)
	for _, k := range keys {
		orConditions = append(orConditions, "(repository_id = ? AND format = ? AND name = ?)")
		orArgs = append(orArgs, k.RepositoryID, k.Format, k.Name)
	}
	db = db.Where(strings.Join(orConditions, " OR "), orArgs...)

	// 版本过滤：含 [ 的 glob 退化为内存过滤
	if versionFilter != "" && !strings.Contains(versionFilter, "[") {
		verCond, verArgs := versionToSQLCondition("version", versionFilter)
		if verCond != "" {
			db = db.Where(verCond, verArgs...)
		}
	}
	// files_downloaded 过滤：true 要求有 blob，false 要求无 blob
	if filesDownloaded != nil {
		if *filesDownloaded {
			db = db.Where("EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id)")
		} else {
			db = db.Where("NOT EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id)")
		}
	}

	var artifacts []model.Artifact
	if err := db.Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, err
	}
	downloadedArtifactIDs := make(map[uint]bool)
	if len(artifacts) > 0 {
		artifactIDs := make([]uint, 0, len(artifacts))
		for _, a := range artifacts {
			artifactIDs = append(artifactIDs, a.ID)
		}
		var downloadedRows []struct {
			ArtifactID uint
		}
		if err := s.db.WithContext(ctx).Model(&model.ArtifactBlob{}).
			Select("DISTINCT artifact_id").
			Where("artifact_id IN ?", artifactIDs).
			Find(&downloadedRows).Error; err != nil {
			return nil, err
		}
		for _, row := range downloadedRows {
			downloadedArtifactIDs[row.ArtifactID] = true
		}
	}

	// 按 "repoID|format|name|version" 分组聚合
	type versionGroup struct {
		id              uint
		latestAt        time.Time
		publishedAtStr  string
		name            string
		namespace       string
		identityKey     string
		sizeBytes       int64
		sha256          string
		fileCount       int
		repoID          uint
		license         string
		filesDownloaded bool
	}
	verGroups := make(map[string]*versionGroup)
	var verOrder []string
	for _, a := range artifacts {
		// 含 [ 的 glob 在内存过滤
		if versionFilter != "" && strings.Contains(versionFilter, "[") {
			matched, _ := filepath.Match(versionFilter, a.Version)
			if !matched {
				continue
			}
		}

		groupKey := fmt.Sprintf("%d|%s|%s|%s", a.RepositoryID, a.Format, a.Name, a.Version)
		vp, ok := verGroups[groupKey]
		if !ok {
			vp = &versionGroup{repoID: a.RepositoryID}
			verGroups[groupKey] = vp
			verOrder = append(verOrder, groupKey)
		}
		if vp.id == 0 {
			vp.id = a.ID
		}
		if a.CreatedAt.After(vp.latestAt) {
			vp.latestAt = a.CreatedAt
			vp.id = a.ID
		}
		if vp.sizeBytes == 0 && a.SizeBytes > 0 {
			vp.sizeBytes = a.SizeBytes
		}
		if vp.sha256 == "" {
			vp.sha256 = jsonbString(a.Checksums, "sha256")
		}
		if vp.name == "" {
			vp.name = a.Name
		}
		if vp.namespace == "" {
			vp.namespace = a.Namespace
		}
		if vp.identityKey == "" {
			vp.identityKey = a.IdentityKey
		}
		if vp.publishedAtStr == "" {
			vp.publishedAtStr = jsonbString(a.Attributes, "published_at")
		}
		if vp.license == "" {
			vp.license = jsonbString(a.Attributes, "license")
		}
		if downloadedArtifactIDs[a.ID] {
			vp.filesDownloaded = true
		}
		vp.fileCount++
	}

	// 按 groupKey 输出，拆出包级 key 和 version
	for _, groupKey := range verOrder {
		vp := verGroups[groupKey]
		// groupKey 格式: "repoID|format|name|version"，拆出包级 key 和 version
		lastSep := strings.LastIndex(groupKey, "|")
		if lastSep < 0 {
			continue
		}
		pkgKey := groupKey[:lastSep]
		version := groupKey[lastSep+1:]

		publishedAt := vp.latestAt
		if vp.publishedAtStr != "" {
			if t, err := time.Parse(time.RFC3339, vp.publishedAtStr); err == nil {
				publishedAt = t
			}
		}
		if publishedAt.IsZero() {
			publishedAt = time.Now()
		}

		result[pkgKey] = append(result[pkgKey], ListVersionEntry{
			ID:              vp.id,
			RepositoryID:    vp.repoID,
			Version:         version,
			Name:            vp.name,
			Namespace:       vp.namespace,
			IdentityKey:     vp.identityKey,
			Status:          "published",
			PublishedAt:     &publishedAt,
			SizeBytes:       vp.sizeBytes,
			ChecksumSHA256:  vp.sha256,
			FileCount:       vp.fileCount,
			FilesDownloaded: vp.filesDownloaded,
			License:         vp.license,
		})
	}
	return result, nil
}

// batchRepoNames 批量查询仓库名称。
func (s *PackageSearchService) batchRepoNames(ctx context.Context, repoIDs map[uint]bool) map[uint]string {
	result := make(map[uint]string)
	if len(repoIDs) == 0 {
		return result
	}
	ids := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		ids = append(ids, id)
	}
	type repoRow struct {
		ID   uint
		Name string
	}
	var rows []repoRow
	if err := s.db.WithContext(ctx).Model(&model.Repository{}).Select("id, name").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return result
	}
	for _, r := range rows {
		result[r.ID] = r.Name
	}
	return result
}

// jsonbString 从 model.JSONB 中提取字符串字段。
func jsonbString(data model.JSONB, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return s
}

func parseSQLTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
