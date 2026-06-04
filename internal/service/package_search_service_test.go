package service

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	_ = util.InitLogger(&util.LoggerConfig{Level: "error", Format: "console", Output: "stdout"})
}

func TestSearchFallsBackWhenPackagesTableIsEmptyButArtifactsExist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "version",
		Coordinates: model.JSONB{
			"name":    "left-pad",
			"version": "1.0.0",
		},
		UpdatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "npm",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 {
		t.Fatalf("expected artifact fallback result, got total=%d len=%d", got.Total, len(got.List))
	}
	if got.List[0].Name != "left-pad" {
		t.Fatalf("Name = %q", got.List[0].Name)
	}
}
