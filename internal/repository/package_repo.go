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

func (r *PackageRepository) DB() *gorm.DB {
	return r.db
}

// StorePackageFile 存储包文件，自动处理 Package、PackageVersion、PackageFile 的创建
func (r *PackageRepository) StorePackageFile(ctx context.Context, pkg *model.Package, ver *model.PackageVersion, file *model.PackageFile) (*model.Package, *model.PackageVersion, *model.PackageFile, error) {
	if pkg.DisplayName == "" {
		pkg.DisplayName = util.GenerateDisplayName(pkg.Name, string(pkg.Type))
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var existingPkg model.Package
		result := tx.Where("name = ? AND type = ?", pkg.Name, pkg.Type).First(&existingPkg)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			if err := tx.Create(pkg).Error; err != nil {
				return err
			}
		} else {
			pkg.ID = existingPkg.ID
			if pkg.RepositoryID > 0 && pkg.RepositoryID != existingPkg.RepositoryID {
				if err := tx.Model(&existingPkg).Updates(map[string]interface{}{
					"repository_id":   pkg.RepositoryID,
					"repository_type": pkg.RepositoryType,
				}).Error; err != nil {
					return err
				}
			}
		}

		if ver != nil {
			ver.PackageID = pkg.ID

			var existingVer model.PackageVersion
			verResult := tx.Where("package_id = ? AND version = ?", pkg.ID, ver.Version).First(&existingVer)
			if verResult.Error != nil && !errors.Is(verResult.Error, gorm.ErrRecordNotFound) {
				return verResult.Error
			}

			if errors.Is(verResult.Error, gorm.ErrRecordNotFound) {
				if err := tx.Create(ver).Error; err != nil {
					return err
				}
			} else {
				ver.ID = existingVer.ID
			}

			if file != nil {
				file.VersionID = ver.ID

				var existingFile model.PackageFile
				fileResult := tx.Where("version_id = ? AND filename = ?", ver.ID, file.Filename).First(&existingFile)
				if fileResult.Error != nil && !errors.Is(fileResult.Error, gorm.ErrRecordNotFound) {
					return fileResult.Error
				}

				if errors.Is(fileResult.Error, gorm.ErrRecordNotFound) {
					if err := tx.Create(file).Error; err != nil {
						return err
					}
				} else {
					file.ID = existingFile.ID
					if err := tx.Model(file).Updates(map[string]interface{}{
						"storage_path":    file.StoragePath,
						"size_bytes":      file.SizeBytes,
						"checksum_sha256": file.ChecksumSHA256,
						"checksum_md5":    file.ChecksumMD5,
					}).Error; err != nil {
						return err
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, nil, err
	}

	return pkg, ver, file, nil
}

// StorePackageFileAndIncrementDownload 存储包文件并增加下载计数
func (r *PackageRepository) StorePackageFileAndIncrementDownload(ctx context.Context, pkg *model.Package, ver *model.PackageVersion, file *model.PackageFile) (*model.Package, *model.PackageVersion, *model.PackageFile, error) {
	storedPkg, storedVer, storedFile, err := r.StorePackageFile(ctx, pkg, ver, file)
	if err != nil {
		return nil, nil, nil, err
	}

	if storedPkg != nil && storedVer != nil && storedFile != nil {
		r.IncrementDownloadCount(storedPkg.ID, storedVer.ID, storedFile.ID)
	}

	return storedPkg, storedVer, storedFile, nil
}

// CreateOrUpdate 兼容旧 API，内部调用 StorePackageFile
func (r *PackageRepository) CreateOrUpdate(ctx context.Context, pkg *model.Package, ver *model.PackageVersion) (*model.Package, *model.PackageVersion, error) {
	p, v, _, err := r.StorePackageFile(ctx, pkg, ver, nil)
	return p, v, err
}

func (r *PackageRepository) FindByNameAndType(name string, pkgType model.PackageType) (*model.Package, error) {
	var pkg model.Package
	result := r.db.Preload("Versions").Preload("Versions.Files").Where("name = ? AND type = ?", name, pkgType).First(&pkg)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &pkg, nil
}

func (r *PackageRepository) FindVersionByPackageAndVersion(pkgID uint, version string) (*model.PackageVersion, error) {
	var ver model.PackageVersion
	result := r.db.Where("package_id = ? AND version = ?", pkgID, version).First(&ver)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &ver, nil
}

func (r *PackageRepository) FindFileByVersionAndFilename(versionID uint, filename string) (*model.PackageFile, error) {
	var file model.PackageFile
	result := r.db.Where("version_id = ? AND filename = ?", versionID, filename).First(&file)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &file, nil
}

func (r *PackageRepository) DeleteByNameAndVersion(name string, version string) error {
	var pkg model.Package
	result := r.db.Where("name = ? AND type = ?", name, model.PackageTypeNPM).First(&pkg)
	if result.Error != nil {
		return result.Error
	}

	var ver model.PackageVersion
	if err := r.db.Where("package_id = ? AND version = ?", pkg.ID, version).First(&ver).Error; err != nil {
		return err
	}

	if err := r.db.Where("version_id = ?", ver.ID).Delete(&model.PackageFile{}).Error; err != nil {
		return err
	}

	return r.db.Delete(&ver).Error
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

	query := r.db.Model(&model.Package{}).Preload("Versions").Preload("Versions.Files")

	if pkgType != "" {
		query = query.Where("type = ?", pkgType)
	}

	if keyword != "" {
		query = query.Where("name LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&packages)

	return packages, total, result.Error
}

func (r *PackageRepository) FindVersionByID(id uint) (*model.PackageVersion, error) {
	var ver model.PackageVersion
	result := r.db.Preload("Package").Preload("Files").First(&ver, id)
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
	if err := r.db.Where("version_id = ?", id).Delete(&model.PackageFile{}).Error; err != nil {
		return err
	}
	return r.db.Delete(&model.PackageVersion{}, id).Error
}

func (r *PackageRepository) IncrementDownloadCount(pkgID uint, versionID uint, fileID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if fileID > 0 {
			if err := tx.Model(&model.PackageFile{}).Where("id = ?", fileID).UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error; err != nil {
				return err
			}
		}
		if versionID > 0 {
			if err := tx.Model(&model.PackageVersion{}).Where("id = ?", versionID).UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error; err != nil {
				return err
			}
		}
		if pkgID > 0 {
			if err := tx.Model(&model.Package{}).Where("id = ?", pkgID).UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *PackageRepository) FindFilesByFilename(filename string) ([]model.PackageFile, error) {
	var files []model.PackageFile
	err := r.db.Where("filename = ?", filename).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}
