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

func TestArtifactServiceSaveSyncsPackageVersionSummaryForMavenFiles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	files := []struct {
		filename string
		size     int64
		sha256   string
		license  string
	}{
		{filename: "app-1.0.0.jar", size: 100, sha256: "jar-sha"},
		{filename: "app-1.0.0.pom", size: 20, sha256: "pom-sha", license: "Apache-2.0"},
		{filename: "app-1.0.0-sources.jar", size: 30, sha256: "sources-sha"},
	}
	for _, file := range files {
		attrs := map[string]string{}
		if file.license != "" {
			attrs["license"] = file.license
		}
		if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: fmt.Sprint(repo.ID),
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			Name:         "com.example:app",
			Version:      "1.0.0",
			Path:         "com/example/app/1.0.0",
			Filename:     file.filename,
			RemotePath:   "com/example/app/1.0.0/" + file.filename,
			SizeBytes:    file.size,
			Checksums:    map[string]string{"sha256": file.sha256},
			Attributes:   attrs,
		})); err != nil {
			t.Fatalf("save %s: %v", file.filename, err)
		}
	}

	var versions []model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ?", repo.ID, "maven", "com.example:app").
		Find(&versions).Error; err != nil {
		t.Fatalf("query package versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 package version row, got %d: %#v", len(versions), versions)
	}
	v := versions[0]
	if v.Version != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", v.Version)
	}
	if v.FileCount != 3 {
		t.Fatalf("FileCount = %d, want 3", v.FileCount)
	}
	if v.SizeBytes != 150 {
		t.Fatalf("SizeBytes = %d, want 150", v.SizeBytes)
	}
	if v.ChecksumSHA256 == "" {
		t.Fatal("ChecksumSHA256 should be populated from artifacts")
	}
	if v.License != "Apache-2.0" {
		t.Fatalf("License = %q, want Apache-2.0", v.License)
	}
}

func TestArtifactServicePackageVersionSummaryUsesArtifactStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "maven",
		Kind:         runtime.KindArtifact,
		Name:         "com.example:app",
		Version:      "1.0.0",
		Path:         "com/example/app/1.0.0",
		Filename:     "app-1.0.0.jar",
		RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
		Properties:   map[string]string{"status": "deprecated"},
	})); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	var version model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", 1, "maven", "com.example:app", "1.0.0").
		First(&version).Error; err != nil {
		t.Fatalf("query package version: %v", err)
	}
	if version.Status != "deprecated" {
		t.Fatalf("Status = %q, want deprecated", version.Status)
	}
}

func TestArtifactServiceUpdatePackageVersionStatusUpdatesAllArtifacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	artifacts := []model.Artifact{
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  "artifact/com.example:app/1.0.0/app-1.0.0.jar",
			Name:         "com.example:app",
			Version:      "1.0.0",
			Filename:     "app-1.0.0.jar",
			RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  "artifact/com.example:app/1.0.0/app-1.0.0.pom",
			Name:         "com.example:app",
			Version:      "1.0.0",
			Filename:     "app-1.0.0.pom",
			RemotePath:   "com/example/app/1.0.0/app-1.0.0.pom",
		},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}
	svc := NewArtifactService(db)

	if err := svc.UpdatePackageVersionStatus(context.Background(), 1, "maven", "com.example:app", "1.0.0", "deprecated", "bad release"); err != nil {
		t.Fatalf("update package version status: %v", err)
	}

	var updated []model.Artifact
	if err := db.Where("repository_id = ? AND format = ? AND name = ? AND version = ?", 1, "maven", "com.example:app", "1.0.0").
		Order("filename").Find(&updated).Error; err != nil {
		t.Fatalf("query artifacts: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(updated))
	}
	for _, artifact := range updated {
		if artifact.Metadata["status"] != "deprecated" {
			t.Fatalf("%s status = %#v, want deprecated", artifact.Filename, artifact.Metadata["status"])
		}
		if artifact.Metadata["deprecation_reason"] != "bad release" {
			t.Fatalf("%s deprecation_reason = %#v, want bad release", artifact.Filename, artifact.Metadata["deprecation_reason"])
		}
	}

	var version model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", 1, "maven", "com.example:app", "1.0.0").
		First(&version).Error; err != nil {
		t.Fatalf("query package version: %v", err)
	}
	if version.Status != "deprecated" {
		t.Fatalf("summary status = %q, want deprecated", version.Status)
	}
}

func TestArtifactServiceDeleteRemovesPackageVersionWhenLastArtifactDeleted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	svc := NewArtifactService(db)
	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "maven",
		Kind:         runtime.KindArtifact,
		Name:         "com.example:app",
		Version:      "1.0.0",
		Path:         "com/example/app/1.0.0",
		Filename:     "app-1.0.0.jar",
		RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
	})
	if err := svc.Save(context.Background(), artifact); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	if err := svc.Delete(context.Background(), runtime.ArtifactKey{
		RepositoryID: "1",
		Format:       "maven",
		RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
	}); err != nil {
		t.Fatalf("delete artifact: %v", err)
	}

	var count int64
	if err := db.Model(&model.PackageVersion{}).Where("format = ? AND package_name = ? AND version = ?", "maven", "com.example:app", "1.0.0").Count(&count).Error; err != nil {
		t.Fatalf("count package versions: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected package version summary to be removed, got %d rows", count)
	}
}

func TestArtifactServiceRebuildPackageVersionsFromArtifacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	artifacts := []model.Artifact{
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  "artifact/com.example:app/1.0.0/com/example/app/1.0.0/app-1.0.0.jar",
			Name:         "com.example:app",
			Version:      "1.0.0",
			Filename:     "app-1.0.0.jar",
			RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
			SizeBytes:    100,
			Checksums:    model.JSONB{"sha256": "jar-sha"},
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  "artifact/com.example:app/1.0.0/com/example/app/1.0.0/app-1.0.0.pom",
			Name:         "com.example:app",
			Version:      "1.0.0",
			Filename:     "app-1.0.0.pom",
			RemotePath:   "com/example/app/1.0.0/app-1.0.0.pom",
			SizeBytes:    20,
			Attributes:   model.JSONB{"license": "MIT"},
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  "artifact/com.example:app/2.0.0/com/example/app/2.0.0/app-2.0.0.jar",
			Name:         "com.example:app",
			Version:      "2.0.0",
			Filename:     "app-2.0.0.jar",
			RemotePath:   "com/example/app/2.0.0/app-2.0.0.jar",
			SizeBytes:    200,
		},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}
	if err := db.Create(&model.PackageVersion{
		RepositoryID: 1,
		Format:       "maven",
		PackageName:  "stale",
		Version:      "0.0.1",
		Status:       "published",
	}).Error; err != nil {
		t.Fatalf("create stale package version: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.RebuildPackageVersions(context.Background()); err != nil {
		t.Fatalf("rebuild package versions: %v", err)
	}

	var versions []model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ?", 1, "maven", "com.example:app").
		Order("version").Find(&versions).Error; err != nil {
		t.Fatalf("query package versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 rebuilt package versions, got %d: %#v", len(versions), versions)
	}
	if versions[0].Version != "1.0.0" || versions[0].FileCount != 2 || versions[0].SizeBytes != 120 || versions[0].License != "MIT" {
		t.Fatalf("unexpected 1.0.0 summary: %#v", versions[0])
	}
	if versions[1].Version != "2.0.0" || versions[1].FileCount != 1 || versions[1].SizeBytes != 200 {
		t.Fatalf("unexpected 2.0.0 summary: %#v", versions[1])
	}
	var staleCount int64
	if err := db.Model(&model.PackageVersion{}).Where("package_name = ?", "stale").Count(&staleCount).Error; err != nil {
		t.Fatalf("count stale rows: %v", err)
	}
	if staleCount != 0 {
		t.Fatalf("expected stale package version rows to be removed, got %d", staleCount)
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
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}, &model.ArtifactBlob{}, &model.Blob{}); err != nil {
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

func TestArtifactServiceAggregatesGoVersionMetadataEvenForRootModule(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}, &model.ArtifactBlob{}, &model.Blob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "go",
		Kind:         runtime.KindVersion,
		Name:         "example.com",
		Version:      "v1.0.0",
	})); err != nil {
		t.Fatalf("save root module version: %v", err)
	}

	var names []string
	if err := db.Model(&model.Package{}).Where("format = ?", "go").Order("name").Pluck("name", &names).Error; err != nil {
		t.Fatalf("query package names: %v", err)
	}
	if len(names) != 1 || names[0] != "example.com" {
		t.Fatalf("go package names = %#v, want only example.com", names)
	}
}

func TestArtifactServiceDoesNotAggregateUncachedGoModuleFileAsPackage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}, &model.ArtifactBlob{}, &model.Blob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "go",
		Kind:         runtime.KindFile,
		Name:         "github.com/gin-gonic/gin",
		Version:      "v1.10.0",
		Path:         "github.com/gin-gonic/gin/@v",
		Filename:     "v1.10.0.mod",
		RemotePath:   "github.com/gin-gonic/gin/@v/v1.10.0.mod",
	})); err != nil {
		t.Fatalf("save uncached module file: %v", err)
	}

	var count int64
	if err := db.Model(&model.Package{}).Where("format = ?", "go").Count(&count).Error; err != nil {
		t.Fatalf("count packages: %v", err)
	}
	if count != 0 {
		t.Fatalf("uncached go module file should not be aggregated, got %d rows", count)
	}

	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "go",
		Kind:         runtime.KindFile,
		Name:         "github.com/gin-gonic/gin",
		Version:      "v1.10.0",
		Path:         "github.com/gin-gonic/gin/@v",
		Filename:     "v1.10.0.mod",
		RemotePath:   "github.com/gin-gonic/gin/@v/v1.10.0.mod",
		BlobRefs:     []runtime.BlobRef{{BlobID: 1, Size: 123}},
	})); err != nil {
		t.Fatalf("save cached module file: %v", err)
	}

	var names []string
	if err := db.Model(&model.Package{}).Where("format = ?", "go").Order("name").Pluck("name", &names).Error; err != nil {
		t.Fatalf("query package names: %v", err)
	}
	if len(names) != 1 || names[0] != "github.com/gin-gonic/gin" {
		t.Fatalf("go package names = %#v, want only github.com/gin-gonic/gin", names)
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

func TestArtifactServiceRebuildPackagesUsesGoVersionOrCachedFileOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "go-proxy", Type: model.RepoTypeProxy, PackageType: "go"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	rows := []model.Artifact{
		{RepositoryID: repo.ID, Format: "go", Kind: runtime.KindVersion, IdentityKey: "version/example.com/v1.0.0", Name: "example.com", Version: "v1.0.0"},
		{RepositoryID: repo.ID, Format: "go", Kind: runtime.KindFile, IdentityKey: "file/github.com/probe/@v/v1.0.0.mod", Name: "github.com/probe", Version: "v1.0.0", RemotePath: "github.com/probe/@v/v1.0.0.mod"},
		{RepositoryID: repo.ID, Format: "go", Kind: runtime.KindFile, IdentityKey: "file/github.com/gin-gonic/gin/@v/v1.10.0.mod", Name: "github.com/gin-gonic/gin", Version: "v1.10.0", RemotePath: "github.com/gin-gonic/gin/@v/v1.10.0.mod"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}
	if err := db.Create(&model.ArtifactBlob{ArtifactID: rows[2].ID, BlobID: 1, Position: 0}).Error; err != nil {
		t.Fatalf("create artifact blob: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.RebuildPackages(context.Background()); err != nil {
		t.Fatalf("rebuild packages: %v", err)
	}

	var names []string
	if err := db.Model(&model.Package{}).Where("format = ?", "go").Order("name").Pluck("name", &names).Error; err != nil {
		t.Fatalf("query package names: %v", err)
	}
	want := []string{"example.com", "github.com/gin-gonic/gin"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("go package names = %#v, want %#v", names, want)
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

// TestArtifactServiceRepublishSameVersionIsIdempotent 验证同仓库重新发布
// 相同包+版本不会产生重复的 artifact 行或 packages 行：
// SaveBatch 按 (repository_id, identity_key) upsert，DB 唯一索引兜底。
func TestArtifactServiceRepublishSameVersionIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "npm-local", Type: model.RepoTypeLocal, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	svc := NewArtifactService(db)
	// 用 KindVersion（会被聚合进 packages 表）构造，模拟真实发布里驱动
	// packages 聚合的 file/version 制品。
	spec := runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "npm",
		Kind:         runtime.KindVersion,
		Name:         "left-pad",
		Version:      "1.0.0",
		Attributes:   map[string]string{"license": "MIT", "description": "String left pad"},
	}

	// 同一包+版本发布两次（模拟重复 publish）
	for i := 0; i < 2; i++ {
		if err := svc.SaveBatch(context.Background(), []*runtime.Artifact{
			runtime.NewArtifact(spec),
		}); err != nil {
			t.Fatalf("republish #%d: %v", i+1, err)
		}
	}

	// artifacts 表只有一行
	var artCount int64
	if err := db.Model(&model.Artifact{}).Where("repository_id = ?", repo.ID).Count(&artCount).Error; err != nil {
		t.Fatalf("count artifacts: %v", err)
	}
	if artCount != 1 {
		t.Fatalf("artifact rows = %d, want 1 (no duplicate on republish)", artCount)
	}

	// packages 表只有一行
	var pkgCount int64
	if err := db.Model(&model.Package{}).Where("repository_id = ?", repo.ID).Count(&pkgCount).Error; err != nil {
		t.Fatalf("count packages: %v", err)
	}
	if pkgCount != 1 {
		t.Fatalf("package rows = %d, want 1 (no duplicate on republish)", pkgCount)
	}
}

// TestArtifactServiceSaveVirtualRepoCreatesNoPackage 验证虚拟（group）仓库
// 不会创建独立的 packages/package_versions 记录：虚拟仓库只是路由层。
func TestArtifactServiceSaveVirtualRepoCreatesNoPackage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	local := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	group := model.Repository{Name: "maven-group", Type: model.RepoTypeVirtual, PackageType: "maven"}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("create local repo: %v", err)
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group repo: %v", err)
	}

	svc := NewArtifactService(db)
	for _, repo := range []model.Repository{local, group} {
		if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: fmt.Sprint(repo.ID),
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			Name:         "com.example:app",
			Version:      "1.0.0",
			Path:         "com/example/app/1.0.0",
			Filename:     "app-1.0.0.jar",
			RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
		})); err != nil {
			t.Fatalf("save to %s: %v", repo.Name, err)
		}
	}

	// 本地仓库有 packages 行
	var localPkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", local.ID, "maven", "com.example:app").First(&localPkg).Error; err != nil {
		t.Fatalf("load package for local repo: %v", err)
	}

	// 虚拟仓库没有 packages 行
	var groupPkgCount int64
	if err := db.Model(&model.Package{}).Where("repository_id = ?", group.ID).Count(&groupPkgCount).Error; err != nil {
		t.Fatalf("count group packages: %v", err)
	}
	if groupPkgCount != 0 {
		t.Fatalf("virtual repo package rows = %d, want 0", groupPkgCount)
	}

	// 虚拟仓库没有 package_versions 行
	var groupVerCount int64
	if err := db.Model(&model.PackageVersion{}).Where("repository_id = ?", group.ID).Count(&groupVerCount).Error; err != nil {
		t.Fatalf("count group package_versions: %v", err)
	}
	if groupVerCount != 0 {
		t.Fatalf("virtual repo package_versions rows = %d, want 0", groupVerCount)
	}
}

// TestArtifactServiceRebuildPackagesExcludesVirtualRepo 验证 RebuildPackages
// 不聚合虚拟仓库的 artifact，且会清掉历史遗留的虚拟仓库 packages 行。
func TestArtifactServiceRebuildPackagesExcludesVirtualRepo(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	local := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	group := model.Repository{Name: "maven-group", Type: model.RepoTypeVirtual, PackageType: "maven"}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("create local repo: %v", err)
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group repo: %v", err)
	}

	// 两个仓库都有 artifact（虚拟仓库属于历史遗留脏数据）
	for _, repo := range []model.Repository{local, group} {
		if err := db.Create(&model.Artifact{
			RepositoryID: repo.ID,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			IdentityKey:  fmt.Sprintf("file/%d/com.example:app/1.0.0/app-1.0.0.jar", repo.ID),
			Name:         "com.example:app",
			Version:      "1.0.0",
			Path:         "com/example/app/1.0.0",
			Filename:     "app-1.0.0.jar",
			RemotePath:   "com/example/app/1.0.0/app-1.0.0.jar",
		}).Error; err != nil {
			t.Fatalf("create artifact for %s: %v", repo.Name, err)
		}
	}

	// 预置一条虚拟仓库的 packages 脏数据（模拟历史遗留）
	stale := model.Package{
		RepositoryID:  group.ID,
		Format:        "maven",
		Name:          "com.example:app",
		LatestVersion: "1.0.0",
		VersionCount:  1,
		DownloadCount: 42,
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("create stale package: %v", err)
	}

	svc := NewArtifactService(db)
	if err := svc.RebuildPackages(context.Background()); err != nil {
		t.Fatalf("rebuild packages: %v", err)
	}

	// 本地仓库有行
	var localPkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", local.ID, "maven", "com.example:app").First(&localPkg).Error; err != nil {
		t.Fatalf("load package for local repo: %v", err)
	}

	// 虚拟仓库脏行被清除
	var groupPkgCount int64
	if err := db.Model(&model.Package{}).Where("repository_id = ?", group.ID).Count(&groupPkgCount).Error; err != nil {
		t.Fatalf("count group packages: %v", err)
	}
	if groupPkgCount != 0 {
		t.Fatalf("virtual repo package rows after rebuild = %d, want 0", groupPkgCount)
	}
}
