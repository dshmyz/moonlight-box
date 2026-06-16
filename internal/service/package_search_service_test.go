package service

import (
	"context"
	"fmt"
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

func TestSearchDoesNotFallBackWhenPackagesTableIsEmptyButArtifactsExist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
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
	if got.Total != 0 || len(got.List) != 0 {
		t.Fatalf("expected empty packages result without artifact fallback, got total=%d list=%#v", got.Total, got.List)
	}
}

func TestSearchUsesPackagesTableAndIgnoresMetadataArtifacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
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
	if err := db.Create(&model.Package{
		RepositoryID:  repo.ID,
		Format:        "yum",
		Name:          "nginx",
		VersionCount:  1,
		LatestVersion: "1.20.1",
		UpdatedAt:     time.Now(),
	}).Error; err != nil {
		t.Fatalf("create package: %v", err)
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
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
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
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
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

func TestSearchDoesNotFallBackToArtifactsWhenPackagesTableHasRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	now := time.Now()
	if err := db.Create(&model.Package{
		RepositoryID:  repo.ID,
		Format:        "npm",
		Name:          "left-pad",
		VersionCount:  1,
		LatestVersion: "1.0.0",
		UpdatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "version",
		Name:         "ghost-only-in-artifacts",
		Version:      "1.0.0",
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Query:    "ghost-only-in-artifacts",
		Type:     "npm",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 0 || len(got.List) != 0 {
		t.Fatalf("expected packages fast path empty result without artifact fallback, got total=%d list=%#v", got.Total, got.List)
	}
}

func TestSearchFromPackagesExcludesAptCompressedIndexRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "apt-proxy", Type: model.RepoTypeProxy, PackageType: "apt"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	now := time.Now()
	rows := []model.Package{
		{RepositoryID: repo.ID, Format: "apt", Name: "dists/bookworm/main/binary-amd64/Packages.xz", VersionCount: 1, UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "apt", Name: "nginx", VersionCount: 1, LatestVersion: "1.22.1", UpdatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create packages: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "apt",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 {
		t.Fatalf("expected only apt package row, got total=%d list=%#v", got.Total, got.List)
	}
	if got.List[0].Name != "nginx" {
		t.Fatalf("Name = %q, want nginx", got.List[0].Name)
	}
}

func TestSearchFromPackagesIncludesOnlyGoRowsWithValidVersionOrCachedFile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "go-proxy", Type: model.RepoTypeProxy, PackageType: "go"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	now := time.Now()
	rows := []model.Package{
		{RepositoryID: repo.ID, Format: "go", Name: "example.com", VersionCount: 1, LatestVersion: "v1.0.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "go", Name: "github.com/probe", VersionCount: 1, LatestVersion: "v1.0.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "go", Name: "github.com/gin-gonic/gin", VersionCount: 1, LatestVersion: "v1.10.0", UpdatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create packages: %v", err)
	}
	artifacts := []model.Artifact{
		{RepositoryID: repo.ID, Format: "go", Kind: "version", IdentityKey: "version/example.com/v1.0.0", Name: "example.com", Version: "v1.0.0"},
		{RepositoryID: repo.ID, Format: "go", Kind: "file", IdentityKey: "file/github.com/gin-gonic/gin/@v/v1.10.0.mod", Name: "github.com/gin-gonic/gin", Version: "v1.10.0", RemotePath: "github.com/gin-gonic/gin/@v/v1.10.0.mod"},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}
	if err := db.Create(&model.ArtifactBlob{ArtifactID: artifacts[1].ID, BlobID: 1, Position: 0}).Error; err != nil {
		t.Fatalf("create artifact blob: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "go",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 2 || len(got.List) != 2 {
		t.Fatalf("expected only valid go rows, got total=%d list=%#v", got.Total, got.List)
	}
	names := []string{got.List[0].Name, got.List[1].Name}
	want := []string{"example.com", "github.com/gin-gonic/gin"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
}
