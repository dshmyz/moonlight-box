package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/util"
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
}

type PackageSearchService struct {
	db *gorm.DB
}

func NewPackageSearchService(db *gorm.DB) *PackageSearchService {
	return &PackageSearchService{db: db}
}

// Search 从 artifacts 表搜索包，按 name 聚合
func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

	db := s.db.WithContext(ctx).Model(&model.Artifact{})

	if req.Type != "" {
		db = db.Where("format IN ?", util.ExpandPackageTypeAliases(req.Type))
	}
	if req.Repository != "" {
		db = db.Where("repository_id IN (?)",
			s.db.Model(&model.Repository{}).Select("id").Where("name = ?", req.Repository))
	}

	var artifacts []model.Artifact
	if err := db.Order("created_at DESC").Find(&artifacts).Error; err != nil {
		return nil, err
	}

	// 在 Go 中按 name 聚合
	type groupKey struct {
		repositoryID uint
		format       string
		name         string
	}
	groups := make(map[groupKey]*groupAcc)
	var orderedKeys []groupKey

	for _, a := range artifacts {
		name := coordinateStr(a.Coordinates, "name")
		if name == "" {
			name = coordinateStr(a.Coordinates, "package")
		}
		if name == "" {
			continue
		}

		// 过滤
		if req.Query != "" {
			if req.Scope == "all" {
				if !strings.Contains(strings.ToLower(name), strings.ToLower(req.Query)) {
					continue
				}
			} else {
				if !strings.Contains(strings.ToLower(name), strings.ToLower(req.Query)) {
					continue
				}
			}
		}
		if req.Name != "" && name != req.Name {
			continue
		}
		if req.Version != "" {
			ver := coordinateStr(a.Coordinates, "version")
			if ver != req.Version {
				continue
			}
		}

		key := groupKey{a.RepositoryID, a.Format, name}
		acc, ok := groups[key]
		if !ok {
			acc = &groupAcc{name: name, format: a.Format, repositoryID: a.RepositoryID}
			groups[key] = acc
			orderedKeys = append(orderedKeys, key)
		}
		acc.versionCount++
		if a.UpdatedAt.After(acc.latestTime) {
			acc.latestTime = a.UpdatedAt
		}
		if acc.firstID == 0 {
			acc.firstID = a.ID
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

	// 收集 repo IDs
	repoIDs := make(map[uint]bool)
	for _, k := range pagedKeys {
		repoIDs[k.repositoryID] = true
	}
	repoIDList := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		repoIDList = append(repoIDList, id)
	}

	// 批量查仓库名
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

func coordinateStr(coords model.JSONB, key string) string {
	if coords == nil {
		return ""
	}
	v, ok := coords[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}
