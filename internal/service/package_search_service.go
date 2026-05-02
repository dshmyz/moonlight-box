package service

import (
	"context"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

// SearchRequest 包搜索请求参数
type SearchRequest struct {
	Query    string
	Type     string
	Scope    string
	Sort     string
	Page     int
	PageSize int
}

// SearchResult 包搜索结果
type SearchResult struct {
	List         []model.Package `json:"list"`
	Total        int64           `json:"total"`
	Page         int             `json:"page"`
	PageSize     int             `json:"page_size"`
	SearchTimeMs int64           `json:"search_time_ms"`
}

// PackageSearchService 包搜索服务
type PackageSearchService struct {
	db *gorm.DB
}

// NewPackageSearchService 创建包搜索服务实例
func NewPackageSearchService(db *gorm.DB) *PackageSearchService {
	return &PackageSearchService{db: db}
}

// Search 执行包搜索
func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

	query := s.db.Model(&model.Package{})

	// 根据 scope 构建搜索条件
	if req.Query != "" {
		switch req.Scope {
		case "name":
			query = query.Where("name LIKE ?", "%"+req.Query+"%")
		case "description":
			query = query.Where("description LIKE ?", "%"+req.Query+"%")
		case "all":
			query = query.Where("name LIKE ? OR description LIKE ?",
				"%"+req.Query+"%", "%"+req.Query+"%")
		default:
			query = query.Where("name LIKE ?", "%"+req.Query+"%")
		}
	}

	// 包类型过滤
	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}

	// 排序
	switch req.Sort {
	case "downloads":
		query = query.Order("download_count DESC")
	case "name":
		query = query.Order("name ASC")
	case "updated_at":
		query = query.Order("updated_at DESC")
	default:
		query = query.Order("download_count DESC")
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var packages []model.Package
	offset := (req.Page - 1) * req.PageSize
	if err := query.Offset(offset).Limit(req.PageSize).Find(&packages).Error; err != nil {
		return nil, err
	}

	// 填充仓库名称
	for i := range packages {
		if packages[i].RepositoryID > 0 {
			var repo model.Repository
			if err := s.db.Select("name").First(&repo, packages[i].RepositoryID).Error; err == nil {
				packages[i].RepositoryName = repo.Name
			}
		}
	}

	return &SearchResult{
		List:         packages,
		Total:        total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SearchTimeMs: time.Since(start).Milliseconds(),
	}, nil
}
