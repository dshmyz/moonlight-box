package service

import (
	"context"
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

type SearchResult struct {
	List         []model.ComponentCatalogEntry `json:"list"`
	Total        int64                         `json:"total"`
	Page         int                           `json:"page"`
	PageSize     int                           `json:"page_size"`
	SearchTimeMs int64                         `json:"search_time_ms"`
}

type PackageSearchService struct {
	db *gorm.DB
}

func NewPackageSearchService(db *gorm.DB) *PackageSearchService {
	return &PackageSearchService{db: db}
}

func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

	subQuery := s.db.WithContext(ctx).Model(&model.Component{}).
		Select(`MIN(id) as id, repository_id, format, namespace, name,
			MAX(display_name) as display_name,
			MAX(description) as description,
			MAX(updated_at) as updated_at,
			SUM(download_count) as download_count,
			COUNT(*) as version_count`)

	if req.Query != "" {
		switch req.Scope {
		case "description":
			subQuery = subQuery.Where("description LIKE ?", "%"+req.Query+"%")
		case "all":
			subQuery = subQuery.Where("name LIKE ? OR description LIKE ?", "%"+req.Query+"%", "%"+req.Query+"%")
		default:
			subQuery = subQuery.Where("name LIKE ?", "%"+req.Query+"%")
		}
	}

	if req.Type != "" {
		subQuery = subQuery.Where("format IN ?", util.ExpandPackageTypeAliases(req.Type))
	}
	if req.Repository != "" {
		subQuery = subQuery.Where("repository_id IN (?)",
			s.db.Model(&model.Repository{}).Select("id").Where("name = ?", req.Repository))
	}
	if req.Name != "" {
		subQuery = subQuery.Where("name = ?", req.Name)
	}
	if req.Version != "" {
		subQuery = subQuery.Where("version = ?", req.Version)
	}

	subQuery = subQuery.Group("repository_id, format, namespace, name")

	var total int64
	if err := s.db.Table("(?) as grouped", subQuery).Count(&total).Error; err != nil {
		return nil, err
	}

	order := "download_count DESC"
	switch req.Sort {
	case "name":
		order = "name ASC"
	case "updated_at":
		order = "updated_at DESC"
	}

	type row struct {
		ID            uint
		RepositoryID  uint
		Format        model.PackageType
		Namespace     string
		Name          string
		DisplayName   string
		Description   string
		UpdatedAt     time.Time
		DownloadCount int64
		VersionCount  int
	}

	var rows []row
	offset := (req.Page - 1) * req.PageSize
	if err := s.db.Table("(?) as grouped", subQuery).
		Order(order).Offset(offset).Limit(req.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}

	list := make([]model.ComponentCatalogEntry, len(rows))
	repoIDs := make([]uint, 0, len(rows))
	for i, r := range rows {
		list[i] = model.ComponentCatalogEntry{
			ID:            r.ID,
			RepositoryID:  r.RepositoryID,
			Format:        r.Format,
			Namespace:     r.Namespace,
			Name:          r.Name,
			DisplayName:   r.DisplayName,
			Description:   r.Description,
			DownloadCount: r.DownloadCount,
			VersionCount:  r.VersionCount,
			UpdatedAt:     r.UpdatedAt,
		}
		if r.RepositoryID > 0 {
			repoIDs = append(repoIDs, r.RepositoryID)
		}
	}

	if len(repoIDs) > 0 {
		type RepoName struct {
			ID   uint
			Name string
		}
		var names []RepoName
		s.db.Model(&model.Repository{}).Select("id, name").Where("id IN ?", repoIDs).Find(&names)
		m := make(map[uint]string, len(names))
		for _, n := range names {
			m[n.ID] = n.Name
		}
		for i := range list {
			list[i].RepositoryName = m[list[i].RepositoryID]
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
