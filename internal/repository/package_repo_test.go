package repository

import (
	"context"
	"testing"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPackageRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Package{}, &model.PackageVersion{}, &model.PackageFile{}))
	return db
}

func TestPackageRepositoryStoresSamePackageNamePerRepository(t *testing.T) {
	db := setupPackageRepoTestDB(t)
	repo := NewPackageRepository(db)

	for _, repoID := range []uint{10, 20} {
		_, _, _, err := repo.StorePackageFile(context.Background(), &model.Package{
			Name:           "shared-lib",
			Type:           model.PackageTypeNPM,
			RepositoryID:   repoID,
			RepositoryType: model.RepoTypeLocal,
		}, &model.PackageVersion{
			Version: "1.0.0",
			Status:  model.StatusPublished,
		}, &model.PackageFile{
			Filename:    "package.tgz",
			FileType:    model.FileTypePrimary,
			StoragePath: "npm/repo/package.tgz",
			SizeBytes:   int64(repoID),
		})
		require.NoError(t, err)
	}

	pkg10, err := repo.FindByRepoNameAndType(10, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, uint(10), pkg10.RepositoryID)

	pkg20, err := repo.FindByRepoNameAndType(20, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, uint(20), pkg20.RepositoryID)
	require.NotEqual(t, pkg10.ID, pkg20.ID)

	var count int64
	require.NoError(t, db.Model(&model.Package{}).Where("name = ? AND type = ?", "shared-lib", model.PackageTypeNPM).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestPackageRepositoryListVersionsByRepo(t *testing.T) {
	db := setupPackageRepoTestDB(t)
	repo := NewPackageRepository(db)

	_, _, _, err := repo.StorePackageFile(context.Background(), &model.Package{
		Name:           "shared-lib",
		Type:           model.PackageTypeNPM,
		RepositoryID:   10,
		RepositoryType: model.RepoTypeLocal,
	}, &model.PackageVersion{Version: "1.0.0", Status: model.StatusPublished}, &model.PackageFile{Filename: "a.tgz", FileType: model.FileTypePrimary, StoragePath: "a", SizeBytes: 1})
	require.NoError(t, err)

	_, _, _, err = repo.StorePackageFile(context.Background(), &model.Package{
		Name:           "shared-lib",
		Type:           model.PackageTypeNPM,
		RepositoryID:   20,
		RepositoryType: model.RepoTypeLocal,
	}, &model.PackageVersion{Version: "2.0.0", Status: model.StatusPublished}, &model.PackageFile{Filename: "b.tgz", FileType: model.FileTypePrimary, StoragePath: "b", SizeBytes: 1})
	require.NoError(t, err)

	versions10, err := repo.ListVersionsByRepo(10, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, []string{"1.0.0"}, versions10)

	versions20, err := repo.ListVersionsByRepo(20, "shared-lib", model.PackageTypeNPM)
	require.NoError(t, err)
	require.Equal(t, []string{"2.0.0"}, versions20)
}
