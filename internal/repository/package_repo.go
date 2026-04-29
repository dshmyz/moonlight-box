package repository

import (
	"context"
	"errors"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/util"

	"gorm.io/gorm"
)

type PackageRepository struct {
	db *gorm.DB
}

func NewPackageRepository(db *gorm.DB) *PackageRepository {
	return &PackageRepository{db: db}
}

func (r *PackageRepository) CreateOrUpdate(ctx context.Context, pkg *model.Package, ver *model.PackageVersion) (*model.Package, *model.PackageVersion, error) {
	// 查找或创建包
	var existing model.Package
	result := r.db.Where("name = ? AND type = ?", pkg.Name, pkg.Type).First(&existing)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil, result.Error
	}

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// 创建新包
		if err := r.db.Create(pkg).Error; err != nil {
			return nil, nil, err
		}
	} else {
		pkg.ID = existing.ID
	}

	// 创建版本（仅在 ver 不为 nil 时）
	var createdVer *model.PackageVersion
	if ver != nil {
		ver.PackageID = pkg.ID
		if err := r.db.Create(ver).Error; err != nil {
			return nil, nil, err
		}
		createdVer = ver
	}

	// 重新加载包及其版本
	var updatedPkg model.Package
	r.db.Preload("Versions").First(&updatedPkg, pkg.ID)
	return &updatedPkg, createdVer, nil
}

func (r *PackageRepository) FindByNameAndType(name string, pkgType model.PackageType) (*model.Package, error) {
	var pkg model.Package
	result := r.db.Preload("Versions").Where("name = ? AND type = ?", name, pkgType).First(&pkg)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &pkg, nil
}

func (r *PackageRepository) DeleteByNameAndVersion(name string, version string) error {
	var pkg model.Package
	result := r.db.Where("name = ? AND type = ?", name, model.PackageTypeNPM).First(&pkg)
	if result.Error != nil {
		return result.Error
	}
	return r.db.Where("package_id = ? AND version = ?", pkg.ID, version).Delete(&model.PackageVersion{}).Error
}

func (r *PackageRepository) ListVersions(name string, pkgType model.PackageType) ([]string, error) {
	var versions []string
	var pkg model.Package
	result := r.db.Where("name = ? AND type = ?", name, pkgType).First(&pkg)
	if result.Error != nil {
		return nil, result.Error
	}

	var pkgVersions []model.PackageVersion
	r.db.Where("package_id = ?", pkg.ID).Find(&pkgVersions)
	for _, v := range pkgVersions {
		versions = append(versions, v.Version)
	}
	return versions, nil
}

func (r *PackageRepository) List(page, pageSize int, pkgType string, keyword string) ([]model.Package, int64, error) {
	var packages []model.Package
	var total int64

	query := r.db.Model(&model.Package{}).Preload("Versions")

	if pkgType != "" {
		query = query.Where("type = ?", pkgType)
	}

	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&packages)

	return packages, total, result.Error
}

func (r *PackageRepository) FindVersionByID(id uint) (*model.PackageVersion, error) {
	var ver model.PackageVersion
	result := r.db.Preload("Package").First(&ver, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &ver, nil
}

func (r *PackageRepository) UpdatePackageVersion(ver *model.PackageVersion) error {
	return r.db.Model(&model.PackageVersion{}).Where("id = ?", ver.ID).Updates(map[string]interface{}{
		"status": ver.Status,
	}).Error
}

func (r *PackageRepository) DeleteVersion(id uint) error {
	return r.db.Delete(&model.PackageVersion{}, id).Error
}
