package database

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

// migrateLegacyPackagesToComponents moves packages/package_versions/package_files into components/assets/blobs.
func migrateLegacyPackagesToComponents() error {
	if !DB.Migrator().HasTable("packages") {
		return nil
	}

	var count int64
	if err := DB.Model(&model.Component{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return dropLegacyPackageTables()
	}

	type legacyPackage struct {
		ID             uint
		Name           string
		DisplayName    string
		Type           string
		Description    string
		RepositoryID   uint
		RepositoryType string
		License        string
		DownloadCount  int64
		CreatedBy      uint
	}

	type legacyVersion struct {
		ID              uint
		PackageID       uint
		Version         string
		Status          string
		PublishedAt     string
		PublishedBy     uint
		Metadata        string
		License         string
		DownloadCount   int64
		SizeBytes       int64
		ChecksumMD5     string
		ChecksumSHA256  string
		FilesDownloaded bool
	}

	type legacyFile struct {
		ID             uint
		VersionID      uint
		Filename       string
		FileType       string
		StoragePath    string
		SizeBytes      int64
		ChecksumSHA256 string
		ChecksumMD5    string
		DownloadCount  int64
		DownloadURL    string
	}

	type legacyDep struct {
		ID                   uint
		VersionID            uint
		DepName              string
		DepVersionConstraint string
		DepType              string
		PackageType          string
		IsOptional           bool
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var packages []legacyPackage
		if err := tx.Table("packages").Find(&packages).Error; err != nil {
			return fmt.Errorf("read legacy packages: %w", err)
		}
		pkgByID := make(map[uint]legacyPackage, len(packages))
		for _, p := range packages {
			pkgByID[p.ID] = p
		}

		var versions []legacyVersion
		if err := tx.Table("package_versions").Find(&versions).Error; err != nil {
			return fmt.Errorf("read legacy package_versions: %w", err)
		}

		blobByRef := make(map[string]uint)

		for _, ver := range versions {
			pkg, ok := pkgByID[ver.PackageID]
			if !ok {
				continue
			}

			status := model.StatusPublished
			switch ver.Status {
			case string(model.StatusDeprecated):
				status = model.StatusDeprecated
			case string(model.StatusYanked):
				status = model.StatusYanked
			case string(model.StatusDraft):
				status = model.StatusDraft
			}

			comp := model.Component{
				BaseModel: model.BaseModel{ID: ver.ID},
				RepositoryID:  pkg.RepositoryID,
				Format:          model.PackageType(pkg.Type),
				Namespace:       "",
				Name:            pkg.Name,
				Version:         ver.Version,
				DisplayName:     pkg.DisplayName,
				Description:     pkg.Description,
				Status:          status,
				PublishedBy:     ver.PublishedBy,
				Metadata:        ver.Metadata,
				License:         firstNonEmpty(ver.License, pkg.License),
				DownloadCount:   ver.DownloadCount,
				SizeBytes:       ver.SizeBytes,
				FilesDownloaded: ver.FilesDownloaded,
				CreatedBy:       pkg.CreatedBy,
			}
			if err := tx.Session(&gorm.Session{SkipHooks: true}).Create(&comp).Error; err != nil {
				return fmt.Errorf("migrate component id=%d: %w", ver.ID, err)
			}
		}

		var files []legacyFile
		if err := tx.Table("package_files").Find(&files).Error; err != nil {
			return fmt.Errorf("read legacy package_files: %w", err)
		}

		for _, f := range files {
			ref := f.StoragePath
			if ref == "" {
				ref = fmt.Sprintf("legacy/%d/%s", f.VersionID, f.Filename)
			}
			blobID, ok := blobByRef[ref]
			if !ok {
				blob := model.Blob{
					Ref:       ref,
					SHA256:    f.ChecksumSHA256,
					MD5:       f.ChecksumMD5,
					SizeBytes: f.SizeBytes,
				}
				if err := tx.Create(&blob).Error; err != nil {
					return fmt.Errorf("migrate blob ref=%s: %w", ref, err)
				}
				blobID = blob.ID
				blobByRef[ref] = blobID
			}

			asset := model.Asset{
				ComponentID:   f.VersionID,
				Path:          f.Filename,
				FileName:      f.Filename,
				Kind:          model.AssetKind(f.FileType),
				BlobID:        blobID,
				DownloadCount: f.DownloadCount,
				DownloadURL:   f.DownloadURL,
			}
			if asset.Kind == "" {
				asset.Kind = model.AssetKindPrimary
			}
			if err := tx.Session(&gorm.Session{SkipHooks: true}).Create(&asset).Error; err != nil {
				return fmt.Errorf("migrate asset file=%s: %w", f.Filename, err)
			}
		}

		if tx.Migrator().HasTable("package_dependencies") {
			var deps []legacyDep
			if err := tx.Table("package_dependencies").Find(&deps).Error; err != nil {
				return fmt.Errorf("read legacy package_dependencies: %w", err)
			}
			for _, d := range deps {
				dep := model.ComponentDependency{
					ComponentID:          d.VersionID,
					DepName:              d.DepName,
					DepVersionConstraint: d.DepVersionConstraint,
					DepType:              d.DepType,
					PackageType:          d.PackageType,
					IsOptional:           d.IsOptional,
				}
				if err := tx.Create(&dep).Error; err != nil {
					return fmt.Errorf("migrate dependency: %w", err)
				}
			}
		}

		if tx.Migrator().HasColumn(&model.ScanResult{}, "version_id") {
			if err := tx.Exec("UPDATE scan_results SET component_id = version_id WHERE component_id = 0 OR component_id IS NULL").Error; err != nil {
				return fmt.Errorf("migrate scan_results: %w", err)
			}
		}

		return dropLegacyPackageTablesTx(tx)
	})
}

func dropLegacyPackageTables() error {
	return dropLegacyPackageTablesTx(DB)
}

func dropLegacyPackageTablesTx(tx *gorm.DB) error {
	tables := []string{"package_dependencies", "package_files", "package_versions", "packages"}
	for _, table := range tables {
		if tx.Migrator().HasTable(table) {
			if err := tx.Migrator().DropTable(table); err != nil {
				return fmt.Errorf("drop table %s: %w", table, err)
			}
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
