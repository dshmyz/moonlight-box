package adapter

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
)

func setupGoAdapter(t *testing.T) *GoAdapter {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(gormlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	db.AutoMigrate(
		&model.Package{},
		&model.PackageVersion{},
		&model.Repository{},
		&model.StorageBackend{},
	)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	pkgRepo := repository.NewPackageRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)

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

	auditSvc := service.NewAuditService()

	adapter := NewGoAdapter(pkgRepo, repoRepo, storageSvc, auditSvc)
	return adapter
}

func TestGoAdapter_Type(t *testing.T) {
	adapter := setupGoAdapter(t)
	if adapter.Type() != GoType {
		t.Errorf("expected GoType, got %v", adapter.Type())
	}
}

func TestGoAdapter_RoutePrefix(t *testing.T) {
	adapter := setupGoAdapter(t)
	if adapter.RoutePrefix() != "/go" {
		t.Errorf("expected /go, got %v", adapter.RoutePrefix())
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
