package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

func retryOnLocked(fn func() error, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "database is locked") {
			time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
			continue
		}
		return err
	}
	return err
}

// StorePackageFile 存储包文件，自动处理 Package、PackageVersion、PackageFile 的创建
func (r *PackageRepository) StorePackageFile(ctx context.Context, pkg *model.Package, ver *model.PackageVersion, file *model.PackageFile) (*model.Package, *model.PackageVersion, *model.PackageFile, error) {
	if err := validatePackageForStore(pkg); err != nil {
		return nil, nil, nil, err
	}
	if err := validateVersionForStore(ver); err != nil {
		return nil, nil, nil, fmt.Errorf("version validation failed: %w", err)
	}
	if err := validateFileForStore(file); err != nil {
		return nil, nil, nil, fmt.Errorf("file validation failed: %w", err)
	}

	preparePackage(pkg)
	if ver != nil && file != nil {
		ver.FilesDownloaded = true
	}

	var savedPkg *model.Package
	var savedVer *model.PackageVersion
	var savedFile *model.PackageFile

	var err error
	retryErr := retryOnLocked(func() error {
		err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			savedPkg, err = r.findOrCreatePackage(tx, pkg)
			if err != nil {
				return err
			}

			if ver != nil {
				savedVer, err = r.findOrCreateVersion(tx, ver, savedPkg.ID)
				if err != nil {
					return err
				}

				if file != nil {
					savedFile, err = r.findOrCreateFile(tx, file, savedVer.ID)
					if err != nil {
						return err
					}

					totalSize, err := r.recalculateVersionSize(tx, savedVer.ID)
					if err != nil {
						return err
					}
					savedVer.SizeBytes = totalSize
				}
			}

			return nil
		})
		return err
	}, 10)

	if retryErr != nil {
		return nil, nil, nil, retryErr
	}

	return savedPkg, savedVer, savedFile, nil
}

// CreateOrUpdate 兼容旧 API，内部调用 StorePackageFile
func (r *PackageRepository) CreateOrUpdate(ctx context.Context, pkg *model.Package, ver *model.PackageVersion) (*model.Package, *model.PackageVersion, error) {
	if pkg.Name == "" {
		return nil, nil, fmt.Errorf("package name cannot be empty")
	}
	if ver != nil {
		ver.FilesDownloaded = true
	}
	p, v, _, err := r.StorePackageFile(ctx, pkg, ver, nil)
	return p, v, err
}

// CreateOrUpdateMetadata 创建或更新包版本的元数据（不涉及文件存储）
func (r *PackageRepository) CreateOrUpdateMetadata(ctx context.Context, pkg *model.Package, ver *model.PackageVersion) (*model.Package, *model.PackageVersion, error) {
	if err := validatePackageForStore(pkg); err != nil {
		return nil, nil, err
	}
	if err := validateVersionForStore(ver); err != nil {
		return nil, nil, fmt.Errorf("version validation failed: %w", err)
	}

	preparePackage(pkg)

	var savedPkg *model.Package
	var savedVer *model.PackageVersion

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		savedPkg, err = r.findOrCreatePackage(tx, pkg)
		if err != nil {
			return err
		}

		savedVer, err = r.findOrCreateVersionForMetadata(tx, ver, savedPkg.ID)
		return err
	})

	if err != nil {
		return nil, nil, err
	}

	return savedPkg, savedVer, nil
}

// UpsertVersionDependencies 覆盖写入某个版本的依赖列表（先清空再写入）
func (r *PackageRepository) UpsertVersionDependencies(ctx context.Context, versionID uint, deps []model.PackageDependency) error {
	if versionID == 0 {
		return fmt.Errorf("versionID cannot be 0")
	}

	return retryOnLocked(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("version_id = ?", versionID).Delete(&model.PackageDependency{}).Error; err != nil {
				return err
			}

			if len(deps) == 0 {
				return nil
			}

			cleaned := make([]model.PackageDependency, 0, len(deps))
			seen := make(map[string]struct{}, len(deps))
			for _, dep := range deps {
				if dep.DepName == "" || dep.DepVersionConstraint == "" {
					continue
				}
				dep.VersionID = versionID
				if dep.DepType == "" {
					dep.DepType = "direct"
				}
				if dep.PackageType == "" {
					dep.PackageType = "generic"
				}

				key := dep.DepName + "|" + dep.DepVersionConstraint + "|" + dep.DepType + "|" + dep.PackageType
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				cleaned = append(cleaned, dep)
			}

			if len(cleaned) == 0 {
				return nil
			}
			return tx.Create(&cleaned).Error
		})
	}, 10)
}

func (r *PackageRepository) FindByNameAndType(name string, pkgType model.PackageType) (*model.Package, error) {
	return r.FindByRepoNameAndType(0, name, pkgType)
}

func (r *PackageRepository) FindByRepoNameAndType(repositoryID uint, name string, pkgType model.PackageType) (*model.Package, error) {
	return r.FindByRepoNameAndTypeContext(context.Background(), repositoryID, name, pkgType)
}

func (r *PackageRepository) FindByRepoNameAndTypeContext(ctx context.Context, repositoryID uint, name string, pkgType model.PackageType) (*model.Package, error) {
	var pkg model.Package
	query := r.db.WithContext(ctx).Preload("Versions").Preload("Versions.Files").Preload("Versions.Dependencies").
		Where("name = ? AND type = ?", name, pkgType)
	if repositoryID > 0 {
		query = query.Where("repository_id = ?", repositoryID)
	}

	result := query.First(&pkg)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}

	return &pkg, nil
}

func (r *PackageRepository) FindVersionByPackageAndVersion(pkgID uint, version string) (*model.PackageVersion, error) {
	return r.FindVersionByPackageAndVersionContext(context.Background(), pkgID, version)
}

func (r *PackageRepository) FindVersionByPackageAndVersionContext(ctx context.Context, pkgID uint, version string) (*model.PackageVersion, error) {
	var ver model.PackageVersion
	result := r.db.WithContext(ctx).Where("package_id = ? AND version = ?", pkgID, version).First(&ver)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &ver, nil
}

func (r *PackageRepository) FindFileByVersionAndFilename(versionID uint, filename string) (*model.PackageFile, error) {
	return r.FindFileByVersionAndFilenameContext(context.Background(), versionID, filename)
}

func (r *PackageRepository) FindFileByVersionAndFilenameContext(ctx context.Context, versionID uint, filename string) (*model.PackageFile, error) {
	var file model.PackageFile
	result := r.db.WithContext(ctx).Where("version_id = ? AND filename = ?", versionID, filename).First(&file)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &file, nil
}

func (r *PackageRepository) DeleteByNameAndVersion(name string, version string, pkgType model.PackageType) error {
	return r.DeleteByRepoNameAndVersion(0, name, version, pkgType)
}

func (r *PackageRepository) DeleteByRepoNameAndVersion(repositoryID uint, name string, version string, pkgType model.PackageType) error {
	return r.DeleteByRepoNameAndVersionContext(context.Background(), repositoryID, name, version, pkgType)
}

func (r *PackageRepository) DeleteByRepoNameAndVersionContext(ctx context.Context, repositoryID uint, name string, version string, pkgType model.PackageType) error {
	var pkg model.Package
	query := r.db.WithContext(ctx).Where("name = ? AND type = ?", name, pkgType)
	if repositoryID > 0 {
		query = query.Where("repository_id = ?", repositoryID)
	}
	result := query.First(&pkg)
	if result.Error != nil {
		return result.Error
	}

	var ver model.PackageVersion
	if err := r.db.WithContext(ctx).Where("package_id = ? AND version = ?", pkg.ID, version).First(&ver).Error; err != nil {
		return err
	}

	if err := r.db.WithContext(ctx).Where("version_id = ?", ver.ID).Delete(&model.PackageFile{}).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).Delete(&ver).Error
}

func (r *PackageRepository) ListVersions(name string, pkgType model.PackageType) ([]string, error) {
	return r.ListVersionsByRepo(0, name, pkgType)
}

func (r *PackageRepository) ListVersionsByRepo(repositoryID uint, name string, pkgType model.PackageType) ([]string, error) {
	return r.ListVersionsByRepoContext(context.Background(), repositoryID, name, pkgType)
}

func (r *PackageRepository) ListVersionsByRepoContext(ctx context.Context, repositoryID uint, name string, pkgType model.PackageType) ([]string, error) {
	var versions []string
	var pkg model.Package
	query := r.db.WithContext(ctx).Where("name = ? AND type = ?", name, pkgType)
	if repositoryID > 0 {
		query = query.Where("repository_id = ?", repositoryID)
	}
	result := query.First(&pkg)
	if result.Error != nil {
		return nil, result.Error
	}

	var pkgVersions []model.PackageVersion
	if err := r.db.WithContext(ctx).Where("package_id = ?", pkg.ID).Find(&pkgVersions).Error; err != nil {
		return nil, err
	}
	for _, v := range pkgVersions {
		versions = append(versions, v.Version)
	}
	return versions, nil
}

func (r *PackageRepository) List(page, pageSize int, pkgType string, keyword string) ([]model.Package, int64, error) {
	return r.ListContext(context.Background(), page, pageSize, pkgType, keyword)
}

func (r *PackageRepository) ListContext(ctx context.Context, page, pageSize int, pkgType string, keyword string) ([]model.Package, int64, error) {
	var packages []model.Package
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Package{})

	if pkgType != "" {
		query = query.Where("type = ?", pkgType)
	}

	if keyword != "" {
		query = query.Where("name LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

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
	if err := r.db.WithContext(ctx).Where("package_id IN ?", packageIDs).Order("published_at DESC").Find(&versions).Error; err != nil {
		return nil, 0, err
	}

	if len(versions) > 0 {
		versionIDs := make([]uint, len(versions))
		for i, v := range versions {
			versionIDs[i] = v.ID
		}

		var files []model.PackageFile
		if err := r.db.WithContext(ctx).Where("version_id IN ?", versionIDs).Find(&files).Error; err != nil {
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
	return r.FindVersionByIDContext(context.Background(), id)
}

func (r *PackageRepository) FindVersionByIDContext(ctx context.Context, id uint) (*model.PackageVersion, error) {
	var ver model.PackageVersion
	result := r.db.WithContext(ctx).Preload("Package").Preload("Files").First(&ver, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &ver, nil
}

func (r *PackageRepository) UpdatePackageVersion(ver *model.PackageVersion) error {
	return r.UpdatePackageVersionContext(context.Background(), ver)
}

func (r *PackageRepository) UpdatePackageVersionContext(ctx context.Context, ver *model.PackageVersion) error {
	return r.db.WithContext(ctx).Model(&model.PackageVersion{}).Where("id = ?", ver.ID).Updates(map[string]interface{}{
		"status": ver.Status,
	}).Error
}

func (r *PackageRepository) DeleteVersion(id uint) error {
	return r.DeleteVersionContext(context.Background(), id)
}

func (r *PackageRepository) DeleteVersionContext(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Where("version_id = ?", id).Delete(&model.PackageFile{}).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Delete(&model.PackageVersion{}, id).Error
}

func (r *PackageRepository) IncrementDownloadCount(pkgID uint, versionID uint, fileID uint) error {
	return r.IncrementDownloadCountByAmount(context.Background(), pkgID, versionID, fileID, 1)
}

func (r *PackageRepository) IncrementDownloadCountByAmount(ctx context.Context, pkgID uint, versionID uint, fileID uint, amount int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	return r.FindFilesByFilenameContext(context.Background(), filename)
}

func (r *PackageRepository) FindFilesByFilenameContext(ctx context.Context, filename string) ([]model.PackageFile, error) {
	var files []model.PackageFile
	err := r.db.WithContext(ctx).Where("filename = ?", filename).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}

// 验证方法

func validatePackageForStore(pkg *model.Package) error {
	if pkg == nil {
		return fmt.Errorf("package cannot be nil")
	}
	if pkg.Name == "" {
		return fmt.Errorf("package name cannot be empty")
	}
	if pkg.Type == "" {
		return fmt.Errorf("package type cannot be empty for package %q", pkg.Name)
	}
	return nil
}

func validateVersionForStore(ver *model.PackageVersion) error {
	if ver == nil {
		return nil
	}
	if ver.Version == "" {
		return fmt.Errorf("package version cannot be empty")
	}
	return nil
}

func validateFileForStore(file *model.PackageFile) error {
	if file == nil {
		return nil
	}
	if file.Filename == "" {
		return fmt.Errorf("package filename cannot be empty")
	}
	return nil
}

// 准备方法

func preparePackage(pkg *model.Package) {
	if pkg.DisplayName == "" {
		pkg.DisplayName = util.GenerateDisplayName(pkg.Name, string(pkg.Type))
	}
}

// findOrCreate 模式

func (r *PackageRepository) findOrCreatePackage(tx *gorm.DB, pkg *model.Package) (*model.Package, error) {
	var existingPkg model.Package
	query := tx.Where("name = ? AND type = ?", pkg.Name, pkg.Type)
	if pkg.RepositoryID > 0 {
		query = query.Where("repository_id = ?", pkg.RepositoryID)
	}
	result := query.First(&existingPkg)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := tx.Create(pkg).Error; err != nil {
			return nil, err
		}
		return pkg, nil
	}

	pkg.ID = existingPkg.ID
	if pkg.RepositoryType != "" && pkg.RepositoryType != existingPkg.RepositoryType {
		if err := tx.Model(&existingPkg).Update("repository_type", pkg.RepositoryType).Error; err != nil {
			return nil, err
		}
	}
	return pkg, nil
}

func (r *PackageRepository) findOrCreateVersion(tx *gorm.DB, ver *model.PackageVersion, packageID uint) (*model.PackageVersion, error) {
	ver.PackageID = packageID

	var existingVer model.PackageVersion
	result := tx.Where("package_id = ? AND version = ?", packageID, ver.Version).First(&existingVer)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := tx.Create(ver).Error; err != nil {
			return nil, err
		}
		return ver, nil
	}

	ver.ID = existingVer.ID
	return ver, nil
}

func (r *PackageRepository) findOrCreateVersionForMetadata(tx *gorm.DB, ver *model.PackageVersion, packageID uint) (*model.PackageVersion, error) {
	var existingVer model.PackageVersion
	result := tx.Where("package_id = ? AND version = ?", packageID, ver.Version).First(&existingVer)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			ver.PackageID = packageID
			if err := tx.Create(ver).Error; err != nil {
				return nil, err
			}
			return ver, nil
		}
		return nil, result.Error
	}

	ver.ID = existingVer.ID
	ver.PackageID = packageID
	if err := tx.Model(&existingVer).Updates(ver).Error; err != nil {
		return nil, err
	}
	return ver, nil
}

func (r *PackageRepository) findOrCreateFile(tx *gorm.DB, file *model.PackageFile, versionID uint) (*model.PackageFile, error) {
	file.VersionID = versionID

	var existingFile model.PackageFile
	result := tx.Where("version_id = ? AND filename = ?", versionID, file.Filename).First(&existingFile)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := tx.Create(file).Error; err != nil {
			return nil, err
		}
		return file, nil
	}

	file.ID = existingFile.ID
	if err := tx.Model(file).Updates(map[string]interface{}{
		"storage_path":    file.StoragePath,
		"size_bytes":      file.SizeBytes,
		"checksum_sha256": file.ChecksumSHA256,
		"checksum_md5":    file.ChecksumMD5,
	}).Error; err != nil {
		return nil, err
	}
	return file, nil
}

func (r *PackageRepository) recalculateVersionSize(tx *gorm.DB, versionID uint) (int64, error) {
	var totalSize int64
	if err := tx.Model(&model.PackageFile{}).Where("version_id = ?", versionID).Select("SUM(size_bytes)").Scan(&totalSize).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&model.PackageVersion{}).Where("id = ?", versionID).Update("size_bytes", totalSize).Error; err != nil {
		return 0, err
	}
	return totalSize, nil
}
