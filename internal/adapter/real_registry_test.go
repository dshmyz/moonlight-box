package adapter

import (
	"context"
	"github.com/moonlight-box/registry/internal/cache"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGoAdapter(t *testing.T) *GoAdapter {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	db.AutoMigrate(
		&model.Component{},
		&model.Component{},
		&model.ComponentDependency{},
		&model.Repository{},
		&model.StorageBackend{},
	)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	compRepo := repository.NewComponentRepository(db)

	storageSvc, err := service.NewStorageService(storageBackendRepo, "", 0)
	if err != nil {
		t.Fatalf("failed to create storage service: %v", err)
	}

	backend := &model.StorageBackend{
		Name:      "test-local",
		Type:      model.StorageTypeLocal,
		IsDefault: true,
		Status:    model.StatusActive,
		IsActive:  true,
		Config: model.StorageBackendConfig{
			Local: &model.LocalConfig{
				BasePath:  "/tmp/test-storage-go",
				MaxSizeGB: 1,
			},
		},
	}
	db.Create(backend)

	storageSvc.RefreshBackends()

	compCache := cache.NewComponentCache(compRepo, 5*time.Minute)

	adapter := NewGoAdapter(storageSvc, compCache)
	return adapter
}

func TestGoAdapter_Type(t *testing.T) {
	adapter := setupGoAdapter(t)
	if adapter.Type() != GoType {
		t.Errorf("expected GoType, got %v", adapter.Type())
	}
}

func TestGoAdapter_ListVersions_Empty(t *testing.T) {
	adapter := setupGoAdapter(t)
	ctx := context.Background()

	versions, err := adapter.ListVersions(ctx, "nonexistent-module")
	if err == nil && len(versions) > 0 {
		t.Errorf("expected empty versions or error for nonexistent module")
	}
}
