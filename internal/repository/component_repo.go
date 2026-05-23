package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/util"

	"gorm.io/gorm"
)

type ComponentRepository struct {
	db *gorm.DB
}

func NewComponentRepository(db *gorm.DB) *ComponentRepository {
	return &ComponentRepository{db: db}
}

func retryOnLocked(fn func() error, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isSQLiteLockedError(err) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}

func isSQLiteLockedError(err error) bool {
	return err != nil && (err.Error() == "database is locked" ||
		errors.Is(err, gorm.ErrRecordNotFound) == false && 
		(err.Error() == "SQLITE_BUSY" || 
		 len(err.Error()) > 15 && err.Error()[:15] == "database is locked"))
}

func (r *ComponentRepository) DB() *gorm.DB {
	return r.db
}

// StoreComponentAsset persists component + asset (+ blob). Component must include RepositoryID, Format, Name, Version.
func (r *ComponentRepository) StoreComponentAsset(ctx context.Context, comp *model.Component, asset *model.Asset) (*model.Component, *model.Asset, error) {
	if err := validateComponentForStore(comp); err != nil {
		return nil, nil, err
	}
	if asset != nil {
		if err := validateAssetForStore(asset); err != nil {
			return nil, nil, fmt.Errorf("asset validation failed: %w", err)
		}
		comp.FilesDownloaded = true
	}

	var savedComp *model.Component
	var savedAsset *model.Asset

	retryErr := retryOnLocked(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			savedComp, err = r.findOrCreateComponent(tx, comp)
			if err != nil {
				return err
			}
			if asset == nil {
				return nil
			}
			savedAsset, err = r.findOrCreateAsset(tx, asset, savedComp.ID)
			if err != nil {
				return err
			}
			totalSize, err := r.recalculateComponentSize(tx, savedComp.ID)
			if err != nil {
				return err
			}
			savedComp.SizeBytes = totalSize
			return nil
		})
	}, 10)

	if retryErr != nil {
		return nil, nil, retryErr
	}
	return savedComp, savedAsset, nil
}

func (r *ComponentRepository) CreateOrUpdate(ctx context.Context, comp *model.Component) (*model.Component, error) {
	if comp.Name == "" {
		return nil, fmt.Errorf("component name cannot be empty")
	}
	comp.FilesDownloaded = true
	saved, _, err := r.StoreComponentAsset(ctx, comp, nil)
	return saved, err
}

func (r *ComponentRepository) CreateOrUpdateMetadata(ctx context.Context, comp *model.Component) (*model.Component, error) {
	if err := validateComponentForStore(comp); err != nil {
		return nil, err
	}
	prepareComponent(comp)

	var saved *model.Component
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		saved, err = r.findOrCreateComponentForMetadata(tx, comp)
		return err
	})
	return saved, err
}

func (r *ComponentRepository) UpsertComponentDependencies(ctx context.Context, componentID uint, deps []model.ComponentDependency) error {
	if componentID == 0 {
		return fmt.Errorf("componentID cannot be 0")
	}

	return retryOnLocked(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("component_id = ?", componentID).Delete(&model.ComponentDependency{}).Error; err != nil {
				return err
			}
			if len(deps) == 0 {
				return nil
			}

			cleaned := make([]model.ComponentDependency, 0, len(deps))
			seen := make(map[string]struct{}, len(deps))
			for _, dep := range deps {
				if dep.DepName == "" || dep.DepVersionConstraint == "" {
					continue
				}
				dep.ComponentID = componentID
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

func (r *ComponentRepository) FindByRepoNameAndType(repositoryID uint, name string, format model.PackageType) (*ComponentAggregate, error) {
	return r.FindByRepoNameAndTypeContext(context.Background(), repositoryID, name, format)
}

func (r *ComponentRepository) ListVersionsByRepo(repositoryID uint, name string, format model.PackageType) ([]string, error) {
	return r.ListVersionsByRepoContext(context.Background(), repositoryID, name, format)
}

func (r *ComponentRepository) FindByRepoNameAndTypeContext(ctx context.Context, repositoryID uint, name string, format model.PackageType) (*ComponentAggregate, error) {
	query := r.db.WithContext(ctx).Model(&model.Component{}).
		Where("name = ? AND format = ?", name, format)
	if repositoryID > 0 {
		query = query.Where("repository_id = ?", repositoryID)
	}

	var components []model.Component
	if err := query.Preload("Assets").Preload("Assets.Blob").Preload("Dependencies").
		Order("published_at DESC").Find(&components).Error; err != nil {
		return nil, err
	}
	if len(components) == 0 {
		return nil, util.ErrPackageNotFound
	}

	agg := &ComponentAggregate{
		RepositoryID: components[0].RepositoryID,
		Format:       components[0].Format,
		Namespace:    components[0].Namespace,
		Name:         components[0].Name,
		DisplayName:  components[0].DisplayName,
		Description:  components[0].Description,
		Components:   components,
	}
	return agg, nil
}

// ComponentAggregate groups all versions (components) sharing the same name in a repository.
type ComponentAggregate struct {
	RepositoryID uint
	Format       model.PackageType
	Namespace    string
	Name         string
	DisplayName  string
	Description  string
	Components   []model.Component
}

func (r *ComponentRepository) LookupIDByRepoNameAndType(ctx context.Context, repositoryID uint, name string, format model.PackageType) (uint, error) {
	var comp model.Component
	query := r.db.WithContext(ctx).Select("id").Where("name = ? AND format = ?", name, format)
	if repositoryID > 0 {
		query = query.Where("repository_id = ?", repositoryID)
	}
	result := query.Order("published_at DESC").First(&comp)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return 0, util.ErrPackageNotFound
		}
		return 0, result.Error
	}
	return comp.ID, nil
}

func (r *ComponentRepository) FindComponentByRepoNameVersionContext(ctx context.Context, repositoryID uint, name, version string, format model.PackageType) (*model.Component, error) {
	return r.FindComponentByCoordinates(ctx, repositoryID, format, "", name, version)
}

func (r *ComponentRepository) FindComponentByCoordinates(ctx context.Context, repositoryID uint, format model.PackageType, namespace, name, version string) (*model.Component, error) {
	var comp model.Component
	query := r.db.WithContext(ctx).Preload("Assets").Preload("Assets.Blob").Preload("Dependencies").
		Where("repository_id = ? AND format = ? AND namespace = ? AND name = ? AND version = ?",
			repositoryID, format, namespace, name, version)
	result := query.First(&comp)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &comp, nil
}

func (r *ComponentRepository) FindAssetByComponentAndFilenameContext(ctx context.Context, componentID uint, filename string) (*model.Asset, error) {
	var asset model.Asset
	result := r.db.WithContext(ctx).Preload("Blob").
		Where("component_id = ? AND file_name = ?", componentID, filename).First(&asset)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &asset, nil
}

func (r *ComponentRepository) DeleteByRepoNameAndVersionContext(ctx context.Context, repositoryID uint, name, version string, format model.PackageType) error {
	comp, err := r.FindComponentByCoordinates(ctx, repositoryID, format, "", name, version)
	if err != nil {
		return err
	}
	return r.DeleteComponentContext(ctx, comp.ID)
}

func (r *ComponentRepository) ListVersionsByRepoContext(ctx context.Context, repositoryID uint, name string, format model.PackageType) ([]string, error) {
	agg, err := r.FindByRepoNameAndTypeContext(ctx, repositoryID, name, format)
	if err != nil {
		return nil, err
	}
	versions := make([]string, len(agg.Components))
	for i, c := range agg.Components {
		versions[i] = c.Version
	}
	return versions, nil
}

func (r *ComponentRepository) FindComponentByIDContext(ctx context.Context, id uint) (*model.Component, error) {
	var comp model.Component
	result := r.db.WithContext(ctx).Preload("Assets").Preload("Assets.Blob").Preload("Dependencies").First(&comp, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrPackageNotFound
		}
		return nil, result.Error
	}
	return &comp, nil
}

func (r *ComponentRepository) UpdateComponentContext(ctx context.Context, comp *model.Component) error {
	return r.db.WithContext(ctx).Model(&model.Component{}).Where("id = ?", comp.ID).Updates(map[string]interface{}{
		"status": comp.Status,
	}).Error
}

func (r *ComponentRepository) DeleteComponentContext(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var assetIDs []uint
		if err := tx.Model(&model.Asset{}).Where("component_id = ?", id).Pluck("id", &assetIDs).Error; err != nil {
			return err
		}
		if len(assetIDs) > 0 {
			if err := tx.Where("component_id = ?", id).Delete(&model.Asset{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("component_id = ?", id).Delete(&model.ComponentDependency{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Component{}, id).Error
	})
}

// DeleteCatalogEntry deletes all components sharing coordinates of the catalog entry id (any version under same name).
func (r *ComponentRepository) DeleteCatalogEntryContext(ctx context.Context, catalogID uint) error {
	var anchor model.Component
	if err := r.db.WithContext(ctx).First(&anchor, catalogID).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint
		if err := tx.Model(&model.Component{}).
			Where("repository_id = ? AND format = ? AND namespace = ? AND name = ?",
				anchor.RepositoryID, anchor.Format, anchor.Namespace, anchor.Name).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := r.deleteComponentTx(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ComponentRepository) deleteComponentTx(tx *gorm.DB, id uint) error {
	if err := tx.Where("component_id = ?", id).Delete(&model.Asset{}).Error; err != nil {
		return err
	}
	if err := tx.Where("component_id = ?", id).Delete(&model.ComponentDependency{}).Error; err != nil {
		return err
	}
	return tx.Delete(&model.Component{}, id).Error
}

func (r *ComponentRepository) IncrementDownloadCountByAmount(ctx context.Context, componentID, assetID uint, amount int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if assetID > 0 {
			if err := tx.Model(&model.Asset{}).Where("id = ?", assetID).
				UpdateColumn("download_count", gorm.Expr("download_count + ?", amount)).Error; err != nil {
				return err
			}
		}
		if componentID > 0 {
			if err := tx.Model(&model.Component{}).Where("id = ?", componentID).
				UpdateColumn("download_count", gorm.Expr("download_count + ?", amount)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ComponentRepository) ListContext(ctx context.Context, page, pageSize int, format, keyword string) ([]model.Component, int64, error) {
	var components []model.Component
	var total int64
	query := r.db.WithContext(ctx).Model(&model.Component{})
	if format != "" {
		query = query.Where("format = ?", format)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Preload("Assets").Preload("Assets.Blob").
		Offset(offset).Limit(pageSize).Order("updated_at DESC").Find(&components).Error; err != nil {
		return nil, 0, err
	}
	return components, total, nil
}

func (r *ComponentRepository) FindAssetsByFilenameContext(ctx context.Context, filename string) ([]model.Asset, error) {
	var assets []model.Asset
	err := r.db.WithContext(ctx).Preload("Blob").Where("file_name = ?", filename).Find(&assets).Error
	return assets, err
}

func validateComponentForStore(comp *model.Component) error {
	if comp == nil {
		return fmt.Errorf("component cannot be nil")
	}
	if comp.Name == "" {
		return fmt.Errorf("component name cannot be empty")
	}
	if comp.Format == "" {
		return fmt.Errorf("component format cannot be empty for %q", comp.Name)
	}
	if comp.Version == "" {
		return fmt.Errorf("component version cannot be empty")
	}
	return nil
}

func validateAssetForStore(asset *model.Asset) error {
	if asset.FileName == "" {
		return fmt.Errorf("asset file name cannot be empty")
	}
	return nil
}

func prepareComponent(comp *model.Component) {
	if comp.DisplayName == "" {
		comp.DisplayName = util.GenerateDisplayName(comp.Name, string(comp.Format))
	}
	if comp.Status == "" {
		comp.Status = model.StatusPublished
	}
}

func (r *ComponentRepository) findOrCreateComponent(tx *gorm.DB, comp *model.Component) (*model.Component, error) {
	prepareComponent(comp)
	var existing model.Component
	result := tx.Where(
		"repository_id = ? AND format = ? AND namespace = ? AND name = ? AND version = ?",
		comp.RepositoryID, comp.Format, comp.Namespace, comp.Name, comp.Version,
	).First(&existing)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := tx.Create(comp).Error; err != nil {
			return nil, err
		}
		return comp, nil
	}
	comp.ID = existing.ID
	return comp, nil
}

func (r *ComponentRepository) findOrCreateComponentForMetadata(tx *gorm.DB, comp *model.Component) (*model.Component, error) {
	var existing model.Component
	result := tx.Where(
		"repository_id = ? AND format = ? AND namespace = ? AND name = ? AND version = ?",
		comp.RepositoryID, comp.Format, comp.Namespace, comp.Name, comp.Version,
	).First(&existing)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			prepareComponent(comp)
			if err := tx.Create(comp).Error; err != nil {
				return nil, err
			}
			return comp, nil
		}
		return nil, result.Error
	}
	comp.ID = existing.ID
	if err := tx.Model(&existing).Updates(comp).Error; err != nil {
		return nil, err
	}
	return comp, nil
}

func (r *ComponentRepository) findOrCreateBlob(tx *gorm.DB, ref, sha256, md5 string, size int64) (*model.Blob, error) {
	if ref == "" {
		return nil, fmt.Errorf("blob ref cannot be empty")
	}
	var blob model.Blob
	if sha256 != "" {
		if err := tx.Where("sha256 = ?", sha256).First(&blob).Error; err == nil {
			return &blob, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	result := tx.Where("ref = ?", ref).First(&blob)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		blob = model.Blob{Ref: ref, SHA256: sha256, MD5: md5, SizeBytes: size}
		if err := tx.Create(&blob).Error; err != nil {
			return nil, err
		}
		return &blob, nil
	}
	return &blob, nil
}

func (r *ComponentRepository) findOrCreateAsset(tx *gorm.DB, asset *model.Asset, componentID uint) (*model.Asset, error) {
	asset.ComponentID = componentID
	if asset.Path == "" {
		asset.Path = asset.FileName
	}

	blobRef := asset.Path
	if asset.Blob.Ref != "" {
		blobRef = asset.Blob.Ref
	}
	blob, err := r.findOrCreateBlob(tx, blobRef, asset.Blob.SHA256, asset.Blob.MD5, asset.Blob.SizeBytes)
	if err != nil {
		return nil, err
	}
	asset.BlobID = blob.ID

	var existing model.Asset
	result := tx.Where("component_id = ? AND path = ?", componentID, asset.Path).First(&existing)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := tx.Create(asset).Error; err != nil {
			return nil, err
		}
		return asset, nil
	}
	asset.ID = existing.ID
	if err := tx.Model(&existing).Updates(map[string]interface{}{
		"blob_id":      asset.BlobID,
		"kind":         asset.Kind,
		"download_url": asset.DownloadURL,
	}).Error; err != nil {
		return nil, err
	}
	return asset, nil
}

func (r *ComponentRepository) recalculateComponentSize(tx *gorm.DB, componentID uint) (int64, error) {
	var total int64
	if err := tx.Model(&model.Asset{}).Joins("JOIN blobs ON blobs.id = assets.blob_id").
		Where("assets.component_id = ?", componentID).
		Select("COALESCE(SUM(blobs.size_bytes), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	if err := tx.Model(&model.Component{}).Where("id = ?", componentID).Update("size_bytes", total).Error; err != nil {
		return 0, err
	}
	return total, nil
}
