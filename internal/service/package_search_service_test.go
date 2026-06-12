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
		Name:         "left-pad",
		Version:      "1.0.0",
		UpdatedAt:    time.Now(),
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

func TestSearchFallbackExcludesMetadataArtifacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "yum-proxy", Type: model.RepoTypeProxy, PackageType: "yum"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "yum",
		Kind:         "metadata",
		Name:         "abc123-primary.xml.xz",
		Filename:     "abc123-primary.xml.xz",
		RemotePath:   "repodata/abc123-primary.xml.xz",
		UpdatedAt:    time.Now(),
	}).Error; err != nil {
		t.Fatalf("create metadata artifact: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "yum",
		Kind:         "file",
		Name:         "nginx",
		Version:      "1.20.1",
		Filename:     "nginx-1.20.1-1.el8.x86_64.rpm",
		RemotePath:   "Packages/nginx-1.20.1-1.el8.x86_64.rpm",
		UpdatedAt:    time.Now(),
	}).Error; err != nil {
		t.Fatalf("create rpm artifact: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "yum",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 {
		t.Fatalf("expected only rpm package, got total=%d list=%#v", got.Total, got.List)
	}
	if got.List[0].Name != "nginx" {
		t.Fatalf("Name = %q, want nginx", got.List[0].Name)
	}
}

func TestSearchFromPackagesExcludesRowsWithOnlyMetadataArtifacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "yum-proxy", Type: model.RepoTypeProxy, PackageType: "yum"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	now := time.Now()
	if err := db.Create(&model.Package{
		RepositoryID:  repo.ID,
		Format:        "yum",
		Name:          "abc123-primary.xml.xz",
		VersionCount:  1,
		LatestVersion: "",
		UpdatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("create polluted package: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "yum",
		Kind:         "metadata",
		Name:         "abc123-primary.xml.xz",
		Filename:     "abc123-primary.xml.xz",
		RemotePath:   "repodata/abc123-primary.xml.xz",
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create metadata artifact: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "yum",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 0 || len(got.List) != 0 {
		t.Fatalf("expected polluted package row to be hidden, got total=%d list=%#v", got.Total, got.List)
	}
}

func TestSearchFromPackagesExcludesLegacyYumRepodataRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "yum-proxy", Type: model.RepoTypeProxy, PackageType: "yum"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	now := time.Now()
	if err := db.Create(&model.Package{
		RepositoryID: repo.ID,
		Format:       "yum",
		Name:         "repomd.xml",
		VersionCount: 1,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create polluted package: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "yum",
		Kind:         "artifact",
		Name:         "repomd.xml",
		Filename:     "repomd.xml",
		Path:         "repodata",
		RemotePath:   "repodata/repomd.xml",
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create legacy repodata artifact: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "yum",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 0 || len(got.List) != 0 {
		t.Fatalf("expected legacy repodata package row to be hidden, got total=%d list=%#v", got.Total, got.List)
	}
}
