package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	handler "github.com/moonlight-box/registry/internal/api/http"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPackageVersionsRouteIsPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	// 创建测试所需的表（避免日志噪音）
	db.Exec("CREATE TABLE IF NOT EXISTS packages (id INTEGER PRIMARY KEY, name TEXT, type TEXT)")
	db.Exec("CREATE TABLE IF NOT EXISTS artifact_blobs (artifact_id INTEGER, blob_id INTEGER)")
	db.Exec("CREATE TABLE IF NOT EXISTS blobs (id INTEGER PRIMARY KEY, size INTEGER)")
	if err := db.Create(&model.Artifact{
		RepositoryID: 1,
		Format:       "npm",
		Coordinates: model.JSONB{
			"name":    "lodash",
			"version": "1.0.0",
		},
	}).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	ctx := &RouterContext{}
	ctx.Handlers.Search = handler.NewPackageSearchHandler(service.NewPackageSearchService(db))
	ctx.Handlers.PackageVersion = handler.NewPackageVersionHandler(db)

	r := gin.New()
	api := r.Group("/api/v1")
	ctx.setupPackagePublicRoutes(api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/packages/npm/versions?name=lodash", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected public versions route status 200, got %d body %s", w.Code, w.Body.String())
	}
}
