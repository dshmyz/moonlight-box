package repository

import (
	"context"
	"testing"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupComponentRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Blob{}, &model.Component{}, &model.Asset{}, &model.ComponentDependency{}))
	return db
}

func TestComponentRepositoryStoresSameNamePerRepository(t *testing.T) {
	db := setupComponentRepoTestDB(t)
	repo := NewComponentRepository(db)

	for _, repoID := range []uint{10, 20} {
		_, _, err := repo.StoreComponentAsset(context.Background(), &model.Component{
			Name:         "shared-lib",
			Format:       model.PackageTypeNPM,
			Version:      "1.0.0",
			RepositoryID: repoID,
			Status:       model.StatusPublished,
		}, &model.Asset{
			FileName: "package.tgz",
			Kind:     model.AssetKindPrimary,
			Path:     "npm/repo/package.tgz",
			Blob:     model.Blob{Ref: "npm/repo/package.tgz", SizeBytes: int64(repoID)},
		})
		require.NoError(t, err)
	}

	agg10, err := repo.FindByRepoNameAndTypeContext(context.Background(), 10, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, uint(10), agg10.RepositoryID)

	agg20, err := repo.FindByRepoNameAndTypeContext(context.Background(), 20, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, uint(20), agg20.RepositoryID)
	require.NotEqual(t, agg10.Components[0].ID, agg20.Components[0].ID)

	var count int64
	require.NoError(t, db.Model(&model.Component{}).Where("name = ? AND format = ?", "shared-lib", model.PackageTypeNPM).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestComponentRepositoryListVersionsByRepo(t *testing.T) {
	db := setupComponentRepoTestDB(t)
	repo := NewComponentRepository(db)

	_, _, err := repo.StoreComponentAsset(context.Background(), &model.Component{
		Name: "shared-lib", Format: model.PackageTypeNPM, Version: "1.0.0",
		RepositoryID: 10, Status: model.StatusPublished,
	}, &model.Asset{FileName: "a.tgz", Kind: model.AssetKindPrimary, Path: "a", Blob: model.Blob{Ref: "a", SizeBytes: 1}})
	require.NoError(t, err)

	_, _, err = repo.StoreComponentAsset(context.Background(), &model.Component{
		Name: "shared-lib", Format: model.PackageTypeNPM, Version: "2.0.0",
		RepositoryID: 20, Status: model.StatusPublished,
	}, &model.Asset{FileName: "b.tgz", Kind: model.AssetKindPrimary, Path: "b", Blob: model.Blob{Ref: "b", SizeBytes: 1}})
	require.NoError(t, err)

	versions10, err := repo.ListVersionsByRepoContext(context.Background(), 10, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, []string{"1.0.0"}, versions10)

	versions20, err := repo.ListVersionsByRepoContext(context.Background(), 20, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, []string{"2.0.0"}, versions20)
}

func TestComponentRepositoryUpsertComponentDependencies(t *testing.T) {
	db := setupComponentRepoTestDB(t)
	repo := NewComponentRepository(db)

	comp, _, err := repo.StoreComponentAsset(context.Background(), &model.Component{
		Name: "dep-lib", Format: model.PackageTypeNPM, Version: "1.0.0",
		RepositoryID: 1, Status: model.StatusPublished,
	}, &model.Asset{
		FileName: "dep-lib-1.0.0.tgz", Kind: model.AssetKindPrimary, Path: "dep.tgz",
		Blob: model.Blob{Ref: "dep.tgz", SizeBytes: 10},
	})
	require.NoError(t, err)

	initial := []model.ComponentDependency{
		{DepName: "lodash", DepVersionConstraint: "^4.17.21", DepType: "direct", PackageType: "npm"},
		{DepName: "axios", DepVersionConstraint: "^1.7.0", DepType: "direct", PackageType: "npm"},
	}
	require.NoError(t, repo.UpsertComponentDependencies(context.Background(), comp.ID, initial))

	var deps []model.ComponentDependency
	require.NoError(t, db.Where("component_id = ?", comp.ID).Order("dep_name asc").Find(&deps).Error)
	require.Len(t, deps, 2)

	replaced := []model.ComponentDependency{
		{DepName: "react", DepVersionConstraint: "^18.3.0", DepType: "direct", PackageType: "npm"},
	}
	require.NoError(t, repo.UpsertComponentDependencies(context.Background(), comp.ID, replaced))

	deps = nil
	require.NoError(t, db.Where("component_id = ?", comp.ID).Find(&deps).Error)
	require.Len(t, deps, 1)
	require.Equal(t, "react", deps[0].DepName)
}
