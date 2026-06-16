package service

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/proxy"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRepositoryServiceGetUsesRepositoryCache(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryMember{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{
		Name:        "npm-proxy",
		DisplayName: "cached-name",
		Type:        model.RepoTypeProxy,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	cache := proxy.NewRepositoryCache(repoRepo, groupRepo, time.Hour)
	if _, err := cache.GetByNameContext(context.Background(), repo.Name); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	if err := db.Model(&model.Repository{}).Where("id = ?", repo.ID).Update("display_name", "db-name").Error; err != nil {
		t.Fatalf("update db: %v", err)
	}

	svc := NewRepositoryService(repoRepo, groupRepo, db)
	svc.SetRepoCache(cache)
	got, err := svc.Get(repo.Name)
	if err != nil {
		t.Fatalf("get repo: %v", err)
	}
	if got.DisplayName != "cached-name" {
		t.Fatalf("DisplayName = %q, want cached-name from RepositoryCache", got.DisplayName)
	}
}

func TestRepositoryServiceGetByIDUsesRepositoryCache(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryMember{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{
		Name:        "maven-proxy",
		DisplayName: "cached-name",
		Type:        model.RepoTypeProxy,
		PackageType: "maven",
		Enabled:     true,
	}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	cache := proxy.NewRepositoryCache(repoRepo, groupRepo, time.Hour)
	if _, err := cache.GetByIDContext(context.Background(), repo.ID); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	if err := db.Model(&model.Repository{}).Where("id = ?", repo.ID).Update("display_name", "db-name").Error; err != nil {
		t.Fatalf("update db: %v", err)
	}

	svc := NewRepositoryService(repoRepo, groupRepo, db)
	svc.SetRepoCache(cache)
	got, err := svc.GetByID(repo.ID)
	if err != nil {
		t.Fatalf("get repo: %v", err)
	}
	if got.DisplayName != "cached-name" {
		t.Fatalf("DisplayName = %q, want cached-name from RepositoryCache", got.DisplayName)
	}
}
