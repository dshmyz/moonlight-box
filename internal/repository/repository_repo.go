package repository

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
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
	var repo model.Repository
	err := r.db.Where("name = ?", name).
		Preload("Members").
		Preload("Members.MemberRepo").
		First(&repo).Error
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// FindByID 根据ID查找仓库
func (r *RepositoryRepository) FindByID(id uint) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.Preload("Members").
		Preload("Members.MemberRepo").
		First(&repo, id).Error
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// List 列出仓库，支持过滤
func (r *RepositoryRepository) List(filter map[string]interface{}) ([]model.Repository, error) {
	var repos []model.Repository
	query := r.db.Model(&model.Repository{})

	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if repoType, ok := filter["type"]; ok {
		query = query.Where("type = ?", repoType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}

	err := query.Order("created_at DESC").Find(&repos).Error
	return repos, err
}

// Update 更新仓库
func (r *RepositoryRepository) Update(name string, updates map[string]interface{}) error {
	return r.db.Model(&model.Repository{}).Where("name = ?", name).Updates(updates).Error
}

// Delete 删除仓库
func (r *RepositoryRepository) Delete(name string) error {
	repo, err := r.FindByName(name)
	if err != nil {
		return err
	}
	return r.db.Delete(repo).Error
}

// FindByPackageType 根据包类型查找所有启用的仓库，按代理优先级排序
func (r *RepositoryRepository) FindByPackageType(pkgType string) ([]model.Repository, error) {
	var repos []model.Repository
	err := r.db.Where("package_type = ? AND enabled = ?", pkgType, true).
		Order("proxy_priority ASC").Find(&repos).Error
	return repos, err
}

// FindVirtualByPackageType 查找指定包类型的虚拟仓库
func (r *RepositoryRepository) FindVirtualByPackageType(pkgType string) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.Where("type = ? AND package_type = ? AND enabled = ?",
		model.RepoTypeVirtual, pkgType, true).First(&repo).Error
	if err != nil {
		return nil, fmt.Errorf("virtual repository not found for package type: %s", pkgType)
	}
	return &repo, nil
}
