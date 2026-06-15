package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestArtifactServiceSaveSyncsPackageFieldsFromAttributes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	err = svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "npm",
		Kind:         runtime.KindVersion,
		Name:         "left-pad",
		Version:      "1.0.0",
		Attributes: map[string]string{
			"license":     "MIT",
			"description": "String left pad",
		},
	}))
	if err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "npm", "left-pad").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.License != "MIT" {
		t.Fatalf("License = %q, want MIT", pkg.License)
	}
	if pkg.Description != "String left pad" {
		t.Fatalf("Description = %q, want String left pad", pkg.Description)
	}
}

func TestArtifactServiceSaveRefreshesExistingPackageLicenseFromAttributes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "pypi-proxy", Type: model.RepoTypeProxy, PackageType: "pypi"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "pypi",
		Kind:         runtime.KindVersion,
		Name:         "requests",
		Version:      "2.28.0",
	})); err != nil {
		t.Fatalf("save initial artifact: %v", err)
	}
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "pypi",
		Kind:         runtime.KindVersion,
		Name:         "requests",
		Version:      "2.31.0",
		Attributes: map[string]string{
			"license":     "Apache-2.0",
			"description": "Python HTTP for Humans.",
		},
	})); err != nil {
		t.Fatalf("save metadata-rich artifact: %v", err)
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "pypi", "requests").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.License != "Apache-2.0" {
		t.Fatalf("License = %q, want Apache-2.0", pkg.License)
	}
	if pkg.Description != "Python HTTP for Humans." {
		t.Fatalf("Description = %q, want Python HTTP for Humans.", pkg.Description)
	}
}

func TestArtifactServiceCountsDistinctVersionsForMavenPackage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	for _, filename := range []string{"app-1.0.0.jar", "app-1.0.0.pom", "app-1.0.0-sources.jar"} {
		if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: fmt.Sprint(repo.ID),
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			Name:         "com.example:app",
			Version:      "1.0.0",
			Path:         "com/example/app/1.0.0",
			Filename:     filename,
			RemotePath:   "com/example/app/1.0.0/" + filename,
		})); err != nil {
			t.Fatalf("save %s: %v", filename, err)
		}
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "maven", "com.example:app").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.VersionCount != 1 {
		t.Fatalf("VersionCount = %d, want 1 distinct version", pkg.VersionCount)
	}
}

func TestArtifactServiceDoesNotAggregateYumRepodataAsPackage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "yum-proxy", Type: model.RepoTypeProxy, PackageType: "yum"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "yum",
		Kind:         runtime.KindArtifact,
		Name:         "repomd.xml",
		Path:         "repodata",
		Filename:     "repomd.xml",
		RemotePath:   "repodata/repomd.xml",
	})); err != nil {
		t.Fatalf("save repodata artifact: %v", err)
	}

	var count int64
	if err := db.Model(&model.Package{}).Where("format = ? AND name = ?", "yum", "repomd.xml").Count(&count).Error; err != nil {
		t.Fatalf("count package: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no package aggregate for repodata, got %d", count)
	}
}

func TestArtifactServiceDoesNotAggregateDirectoryAsPackage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}, &model.Package{}, &model.ArtifactBlob{}, &model.Blob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "generic",
		Kind:         runtime.KindDirectory,
		Name:         "docs",
		Filename:     "docs",
		RemotePath:   "docs",
	})); err != nil {
		t.Fatalf("save directory: %v", err)
	}

	var count int64
	if err := db.Model(&model.Package{}).Count(&count).Error; err != nil {
		t.Fatalf("count packages: %v", err)
	}
	if count != 0 {
		t.Fatalf("directory should not be aggregated into packages, got %d rows", count)
	}
}

func TestArtifactServiceDeleteKeepsDistinctMavenVersionCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	for _, filename := range []string{"app-1.0.0.jar", "app-1.0.0.pom"} {
		if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: fmt.Sprint(repo.ID),
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			Name:         "com.example:app",
			Version:      "1.0.0",
			Path:         "com/example/app/1.0.0",
			Filename:     filename,
			RemotePath:   "com/example/app/1.0.0/" + filename,
		})); err != nil {
			t.Fatalf("save %s: %v", filename, err)
		}
	}

	if err := svc.Delete(context.Background(), runtime.ArtifactKey{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "maven",
		RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
	}); err != nil {
		t.Fatalf("delete jar: %v", err)
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "maven", "com.example:app").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.VersionCount != 1 {
		t.Fatalf("VersionCount after deleting one file = %d, want 1", pkg.VersionCount)
	}
}

func TestArtifactServiceRebuildPackagesRestoresLicenseFromAttributes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         runtime.KindVersion,
		IdentityKey:  "version/left-pad/1.0.0",
		Name:         "left-pad",
		Version:      "1.0.0",
		Attributes: model.JSONB{
			"license":     "MIT",
			"description": "String left pad",
		},
	}).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.RebuildPackages(context.Background()); err != nil {
		t.Fatalf("rebuild packages: %v", err)
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "npm", "left-pad").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.License != "MIT" {
		t.Fatalf("License = %q, want MIT", pkg.License)
	}
	if pkg.Description != "String left pad" {
		t.Fatalf("Description = %q, want String left pad", pkg.Description)
	}
}

func TestArtifactServiceRebuildPackagesCountsDistinctVersions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	for _, filename := range []string{"app-1.0.0.jar", "app-1.0.0.pom"} {
		if err := db.Create(&model.Artifact{
			RepositoryID: repo.ID,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  "file/com/example/app/1.0.0/" + filename,
			Name:         "com.example:app",
			Version:      "1.0.0",
			Path:         "com/example/app/1.0.0",
			Filename:     filename,
			RemotePath:   "com/example/app/1.0.0/" + filename,
		}).Error; err != nil {
			t.Fatalf("create artifact %s: %v", filename, err)
		}
	}

	svc := NewArtifactService(db)
	if err := svc.RebuildPackages(context.Background()); err != nil {
		t.Fatalf("rebuild packages: %v", err)
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "maven", "com.example:app").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.VersionCount != 1 {
		t.Fatalf("VersionCount = %d, want 1 distinct version", pkg.VersionCount)
	}
}

func TestArtifactServiceRebuildPackagesRollsBackOnInsertFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	existing := model.Package{
		RepositoryID:  repo.ID,
		Format:        "npm",
		Name:          "left-pad",
		LatestVersion: "0.9.0",
		VersionCount:  1,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing package: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         runtime.KindArtifact,
		IdentityKey:  "file/left-pad/-/left-pad-1.0.0.tgz",
		Name:         "left-pad",
		Version:      "1.0.0",
		RemotePath:   "left-pad/-/left-pad-1.0.0.tgz",
		Filename:     "left-pad-1.0.0.tgz",
	}).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_package_rebuild BEFORE INSERT ON packages BEGIN SELECT RAISE(ABORT, 'forced rebuild failure'); END;`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.RebuildPackages(context.Background()); err == nil {
		t.Fatal("RebuildPackages error = nil, want forced failure")
	}

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "npm", "left-pad").First(&pkg).Error; err != nil {
		t.Fatalf("load package after failed rebuild: %v", err)
	}
	if pkg.LatestVersion != "0.9.0" || pkg.VersionCount != 1 {
		t.Fatalf("package after failed rebuild = latest %q count %d, want preserved latest 0.9.0 count 1", pkg.LatestVersion, pkg.VersionCount)
	}
}
