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

	if ver != nil && file != nil {
		ver.FilesDownloaded = true
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

				// 更新版本总大小：计算该版本所有文件的大小之和
				var totalSize int64
				tx.Model(&model.PackageFile{}).Where("version_id = ?", ver.ID).Select("SUM(size_bytes)").Scan(&totalSize)
				tx.Model(&model.PackageVersion{}).Where("id = ?", ver.ID).Update("size_bytes", totalSize)
				ver.SizeBytes = totalSize
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
// 注意：此方法假设文件已存储，会自动设置 FilesDownloaded = true
func (r *PackageRepository) CreateOrUpdate(ctx context.Context, pkg *model.Package, ver *model.PackageVersion) (*model.Package, *model.PackageVersion, error) {
	if ver != nil {
		ver.FilesDownloaded = true
	}
	p, v, _, err := r.StorePackageFile(ctx, pkg, ver, nil)
	return p, v, err
}

// CreateOrUpdateMetadata 创建或更新包版本的元数据（不涉及文件存储）
func (r *PackageRepository) CreateOrUpdateMetadata(ctx context.Context, pkg *model.Package, ver *model.PackageVersion) (*model.Package, *model.PackageVersion, error) {
	if pkg.DisplayName == "" {
		pkg.DisplayName = util.GenerateDisplayName(pkg.Name, string(pkg.Type))
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var existingPkg model.Package
		result := tx.Where("name = ? AND type = ?", pkg.Name, pkg.Type).First(&existingPkg)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				if err := tx.Create(pkg).Error; err != nil {
					return err
				}
				existingPkg = *pkg
			} else {
				return result.Error
			}
		}
		pkg.ID = existingPkg.ID

		var existingVer model.PackageVersion
		verResult := tx.Where("package_id = ? AND version = ?", existingPkg.ID, ver.Version).First(&existingVer)
		if verResult.Error != nil {
			if errors.Is(verResult.Error, gorm.ErrRecordNotFound) {
				ver.PackageID = existingPkg.ID
				if err := tx.Create(ver).Error; err != nil {
					return err
				}
			} else {
				return verResult.Error
			}
		} else {
			ver.ID = existingVer.ID
			ver.PackageID = existingPkg.ID
			if err := tx.Model(&existingVer).Updates(ver).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return pkg, ver, nil
}

func (r *PackageRepository) FindByNameAndType(name string, pkgType model.PackageType) (*model.Package, error) {
	var pkg model.Package
	result := r.db.Where("name = ? AND type = ?", name, pkgType).First(&pkg)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}

	var versions []model.PackageVersion
	if err := r.db.Where("package_id = ?", pkg.ID).Find(&versions).Error; err != nil {
		return nil, err
	}

	if len(versions) > 0 {
		versionIDs := make([]uint, len(versions))
		for i, v := range versions {
			versionIDs[i] = v.ID
		}

		var files []model.PackageFile
		if err := r.db.Where("version_id IN ?", versionIDs).Find(&files).Error; err != nil {
			return nil, err
		}

		fileMap := make(map[uint][]model.PackageFile)
		for _, f := range files {
			fileMap[f.VersionID] = append(fileMap[f.VersionID], f)
		}

		for i := range versions {
			versions[i].Files = fileMap[versions[i].ID]
		}
	}

	pkg.Versions = versions
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

func (r *PackageRepository) DeleteByNameAndVersion(name string, version string, pkgType model.PackageType) error {
	var pkg model.Package
	result := r.db.Where("name = ? AND type = ?", name, pkgType).First(&pkg)
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

	query := r.db.Model(&model.Package{})

	if pkgType != "" {
		query = query.Where("type = ?", pkgType)
	}

	if keyword != "" {
		query = query.Where("name LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&packages)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	if len(packages) == 0 {
		return packages, total, nil
	}

	packageIDs := make([]uint, len(packages))
	for i, p := range packages {
		packageIDs[i] = p.ID
	}

	var versions []model.PackageVersion
	if err := r.db.Where("package_id IN ?", packageIDs).Find(&versions).Error; err != nil {
		return nil, 0, err
	}

	if len(versions) > 0 {
		versionIDs := make([]uint, len(versions))
		for i, v := range versions {
			versionIDs[i] = v.ID
		}

		var files []model.PackageFile
		if err := r.db.Where("version_id IN ?", versionIDs).Find(&files).Error; err != nil {
			return nil, 0, err
		}

		fileMap := make(map[uint][]model.PackageFile)
		for _, f := range files {
			fileMap[f.VersionID] = append(fileMap[f.VersionID], f)
		}

		for i := range versions {
			versions[i].Files = fileMap[versions[i].ID]
		}
	}

	packageVersionsMap := make(map[uint][]model.PackageVersion)
	for _, v := range versions {
		packageVersionsMap[v.PackageID] = append(packageVersionsMap[v.PackageID], v)
	}

	for i := range packages {
		packages[i].Versions = packageVersionsMap[packages[i].ID]
	}

	return packages, total, nil
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
	return r.IncrementDownloadCountByAmount(context.Background(), pkgID, versionID, fileID, 1)
}

func (r *PackageRepository) IncrementDownloadCountByAmount(ctx context.Context, pkgID uint, versionID uint, fileID uint, amount int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if fileID > 0 {
			if err := tx.Model(&model.PackageFile{}).Where("id = ?", fileID).UpdateColumn("download_count", gorm.Expr("download_count + ?", amount)).Error; err != nil {
				return err
			}
		}
		if versionID > 0 {
			if err := tx.Model(&model.PackageVersion{}).Where("id = ?", versionID).UpdateColumn("download_count", gorm.Expr("download_count + ?", amount)).Error; err != nil {
				return err
			}
		}
		if pkgID > 0 {
			if err := tx.Model(&model.Package{}).Where("id = ?", pkgID).UpdateColumn("download_count", gorm.Expr("download_count + ?", amount)).Error; err != nil {
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
