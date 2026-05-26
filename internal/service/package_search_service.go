package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/gorm"
)

type SearchRequest struct {
	Query      string
	Type       string
	Name       string
	Version    string
	Repository string
	Scope      string
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
	db *gorm.DB
}

func NewPackageSearchService(db *gorm.DB) *PackageSearchService {
	return &PackageSearchService{db: db}
}

// rawArtifact 原始 SQL 查询结果行，避免 GORM JSONB scan 问题
type rawArtifact struct {
	ID           uint      `gorm:"column:id"`
	RepositoryID uint      `gorm:"column:repository_id"`
	Format       string    `gorm:"column:format"`
	Coordinates  string    `gorm:"column:coordinates"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// Search 从 artifacts 表搜索包，按 name 聚合。
// 使用原始 SQL 读取 coordinates 列，手动解析 JSON，绕过 GORM JSONB 扫描器的兼容问题。
func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

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

	query := "SELECT id, repository_id, format, coordinates, created_at, updated_at FROM artifacts"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	var rawRows []rawArtifact
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rawRows).Error; err != nil {
		return nil, err
	}

	// 聚合
	type groupKey struct {
		repositoryID uint
		format       string
		name         string
	}
	groups := make(map[groupKey]*groupAcc)
	var orderedKeys []groupKey

	for _, row := range rawRows {
		name := extractName("", row.Coordinates)
		if name == "" {
			continue
		}

		if req.Query != "" {
			if !strings.Contains(strings.ToLower(name), strings.ToLower(req.Query)) {
				continue
			}
		}
		if req.Name != "" && name != req.Name {
			continue
		}
		if req.Version != "" {
			ver := extractField(row.Coordinates, "version")
			if ver != req.Version {
				continue
			}
		}

		key := groupKey{row.RepositoryID, row.Format, name}
		acc, ok := groups[key]
		if !ok {
			acc = &groupAcc{name: name, format: row.Format, repositoryID: row.RepositoryID}
			groups[key] = acc
			orderedKeys = append(orderedKeys, key)
		}
		acc.versionCount++
		if row.UpdatedAt.After(acc.latestTime) {
			acc.latestTime = row.UpdatedAt
		}
		if acc.firstID == 0 {
			acc.firstID = row.ID
		}
	}

	// 排序
	switch req.Sort {
	case "name":
		sort.Slice(orderedKeys, func(i, j int) bool {
			return groups[orderedKeys[i]].name < groups[orderedKeys[j]].name
		})
	default:
		sort.Slice(orderedKeys, func(i, j int) bool {
			return groups[orderedKeys[i]].latestTime.After(groups[orderedKeys[j]].latestTime)
		})
	}

	total := int64(len(orderedKeys))

	// 分页
	offset := (req.Page - 1) * req.PageSize
	if offset > len(orderedKeys) {
		offset = len(orderedKeys)
	}
	end := offset + req.PageSize
	if end > len(orderedKeys) {
		end = len(orderedKeys)
	}
	pagedKeys := orderedKeys[offset:end]

	// 收集 repo ID
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

	list := make([]SearchEntry, len(pagedKeys))
	for i, k := range pagedKeys {
		acc := groups[k]
		list[i] = SearchEntry{
			ID:             acc.firstID,
			RepositoryID:   acc.repositoryID,
			Format:         acc.format,
			Name:           acc.name,
			VersionCount:   acc.versionCount,
			UpdatedAt:      acc.latestTime,
			RepositoryName: repoNameMap[acc.repositoryID],
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
}

// extractName 从原始 coordinates JSON 中提取包名
// extractName 从原始 coordinates JSON 中提取包名。首选 "name" 键，旧数据回退拼装。
func extractName(_, coordsJSON string) string {
	return extractField(coordsJSON, "name")
}

// extractField 从原始 JSON 字符串中提取单个字段值
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
