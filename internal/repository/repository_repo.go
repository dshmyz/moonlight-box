package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/gorm"
)

// RepositoryRepository 仓库数据访问层
type RepositoryRepository struct {
	db *gorm.DB
}

func NewRepositoryRepository(db *gorm.DB) *RepositoryRepository {
	return &RepositoryRepository{db: db}
}

// Create 创建仓库
func (r *RepositoryRepository) Create(repo *model.Repository) error {
	return r.db.Create(repo).Error
}

// FindByName 根据名称查找仓库
func (r *RepositoryRepository) FindByName(name string) (*model.Repository, error) {
	return r.FindByNameContext(context.Background(), name)
}

// FindByNameContext 根据名称查找仓库
func (r *RepositoryRepository) FindByNameContext(ctx context.Context, name string) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.WithContext(ctx).Where("name = ?", name).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Members.MemberRepo").
		First(&repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, util.ErrRepoNotFound
		}
		return nil, err
	}
	return &repo, nil
}

// FindByIDContext 根据ID查找仓库
func (r *RepositoryRepository) FindByIDContext(ctx context.Context, id uint) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.WithContext(ctx).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Members.MemberRepo").
		First(&repo, id).Error
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// List 列出仓库，支持过滤（不分页）
func (r *RepositoryRepository) List(filter map[string]interface{}) ([]model.Repository, error) {
	return r.ListContext(context.Background(), filter, 0, 0)
}

// ListContext 列出仓库，支持过滤和分页（page=0 或 pageSize=0 表示不分页）
func (r *RepositoryRepository) ListContext(ctx context.Context, filter map[string]interface{}, page, pageSize int) ([]model.Repository, error) {
	var repos []model.Repository
	query := r.db.WithContext(ctx).Model(&model.Repository{})

	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if repoType, ok := filter["type"]; ok {
		query = query.Where("type = ?", repoType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}
	if publicVisible, ok := filter["public_visible"]; ok {
		query = query.Where("public_visible = ?", publicVisible)
	}
	if keyword, ok := filter["keyword"]; ok {
		keywordStr := fmt.Sprintf("%%%s%%", keyword)
		query = query.Where("name LIKE ? OR display_name LIKE ? OR description LIKE ?", keywordStr, keywordStr, keywordStr)
	}
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	err := query.Order("created_at DESC").Find(&repos).Error
	return repos, err
}

// CountContext 统计符合条件的仓库数量
func (r *RepositoryRepository) CountContext(ctx context.Context, filter map[string]interface{}) (int64, error) {
	var total int64
	query := r.db.WithContext(ctx).Model(&model.Repository{})

	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if repoType, ok := filter["type"]; ok {
		query = query.Where("type = ?", repoType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}
	if publicVisible, ok := filter["public_visible"]; ok {
		query = query.Where("public_visible = ?", publicVisible)
	}
	if keyword, ok := filter["keyword"]; ok {
		keywordStr := fmt.Sprintf("%%%s%%", keyword)
		query = query.Where("name LIKE ? OR display_name LIKE ? OR description LIKE ?", keywordStr, keywordStr, keywordStr)
	}
	err := query.Count(&total).Error
	return total, err
}

// Update 更新仓库指定字段，使用 struct+Select 确保 serializer 正确生效
func (r *RepositoryRepository) Update(name string, repo *model.Repository, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&model.Repository{}).Where("name = ?", name).Select(fields).Updates(repo).Error
}

// Delete 删除仓库
func (r *RepositoryRepository) Delete(name string) error {
	repo, err := r.FindByName(name)
	if err != nil {
		return err
	}
	return r.db.Delete(repo).Error
}

// FindVirtualByPackageTypeContext 查找指定包类型的虚拟仓库
func (r *RepositoryRepository) FindVirtualByPackageTypeContext(ctx context.Context, pkgType string) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.WithContext(ctx).Where("type = ? AND package_type = ? AND enabled = ?",
		model.RepoTypeVirtual, pkgType, true).First(&repo).Error
	if err != nil {
		return nil, fmt.Errorf("virtual repository not found for package type: %s", pkgType)
	}
	return &repo, nil
}
