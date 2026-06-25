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
			if acc.license == "" {
				acc.license = extractField(row.Attributes, "license")
			}
			if acc.description == "" {
				acc.description = extractField(row.Attributes, "description")
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
	Attributes   string `gorm:"column:attributes"`
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

	groupedQuery := "SELECT MIN(id) AS id, repository_id, format, name, " +
		"MAX(attributes) AS attributes, COUNT(*) AS version_count, MAX(updated_at) AS updated_at " +
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
	pageQuery := "SELECT id, repository_id, format, name, attributes, version_count, updated_at FROM (" +
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
			Description:    extractField(row.Attributes, "description"),
			VersionCount:   row.VersionCount,
			UpdatedAt:      updatedAt,
			RepositoryName: repoNameMap[row.RepositoryID],
			License:        extractField(row.Attributes, "license"),
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
