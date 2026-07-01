package service

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDownloadCountBatcherUpdatesPackageVersionCounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Repository{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := db.Create(&model.Package{
		RepositoryID: repo.ID,
		Format:       "maven",
		Name:         "com.example:lib",
	}).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	if err := db.Create(&model.PackageVersion{
		RepositoryID:     repo.ID,
		Format:           "maven",
		PackageName:      "com.example:lib",
		Version:          "1.0.0",
		Status:           "published",
		LatestArtifactAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create package version: %v", err)
	}

	batcher := NewDownloadCountBatcher(db, time.Hour)
	batcher.Increment(repo.ID, "maven", "com.example:lib", "1.0.0")
	batcher.Increment(repo.ID, "maven", "com.example:lib", "1.0.0")
	batcher.flush()
	batcher.Stop()

	var pkg model.Package
	if err := db.Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "maven", "com.example:lib").First(&pkg).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if pkg.DownloadCount != 2 {
		t.Fatalf("package DownloadCount = %d, want 2", pkg.DownloadCount)
	}
	var version model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", repo.ID, "maven", "com.example:lib", "1.0.0").First(&version).Error; err != nil {
		t.Fatalf("load package version: %v", err)
	}
	if version.DownloadCount != 2 {
		t.Fatalf("package version DownloadCount = %d, want 2", version.DownloadCount)
	}
}
