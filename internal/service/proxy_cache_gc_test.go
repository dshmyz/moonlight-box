package service

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestProxyMetadataCacheGCRemovesStaleNoBlobRowsOnly 验证 GC 只删除
// proxy 仓库中无 blob 引用且过期的元数据行：
// 有 blob（已下载内容）、未过期、hosted 仓库的行都保留。
func TestProxyMetadataCacheGCRemovesStaleNoBlobRowsOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	proxy := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	hosted := model.Repository{Name: "npm-local", Type: model.RepoTypeLocal, PackageType: "npm"}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&hosted).Error; err != nil {
		t.Fatal(err)
	}

	old := time.Now().AddDate(0, 0, -40)
	recent := time.Now().AddDate(0, 0, -5)

	// 应删除：proxy 过期 + 无 blob
	stale := model.Artifact{RepositoryID: proxy.ID, Format: "npm", Kind: "version", Name: "pkg", Version: "1.0.0", IdentityKey: "version/pkg/1.0.0", UpdatedAt: old}
	// 应保留：proxy 过期但有 blob（已下载内容）
	downloaded := model.Artifact{RepositoryID: proxy.ID, Format: "npm", Kind: "artifact", Name: "pkg", Version: "1.0.0", IdentityKey: "file/pkg/-/pkg-1.0.0.tgz", UpdatedAt: old}
	// 应保留：proxy 无 blob 但未过期
	fresh := model.Artifact{RepositoryID: proxy.ID, Format: "npm", Kind: "version", Name: "pkg", Version: "2.0.0", IdentityKey: "version/pkg/2.0.0", UpdatedAt: recent}
	// 应保留：hosted 过期无 blob（GC 绝不触碰 hosted）
	hostedArt := model.Artifact{RepositoryID: hosted.ID, Format: "npm", Kind: "version", Name: "hpkg", Version: "1.0.0", IdentityKey: "version/hpkg/1.0.0", UpdatedAt: old}

	for _, a := range []*model.Artifact{&stale, &downloaded, &fresh, &hostedArt} {
		if err := db.Create(a).Error; err != nil {
			t.Fatalf("create artifact: %v", err)
		}
	}
	blob := model.Blob{Algorithm: "sha256", Digest: "abc123", StoragePath: "/blobs/x", Size: 1}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ArtifactBlob{ArtifactID: downloaded.ID, BlobID: blob.ID, Position: 0}).Error; err != nil {
		t.Fatal(err)
	}

	gc := NewProxyMetadataCacheGC(db, NewArtifactService(db), nil) // 默认 enabled=true, maxAgeDays=30
	deleted, err := gc.Run(context.Background())
	if err != nil {
		t.Fatalf("gc run: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	var remain int64
	if err := db.Model(&model.Artifact{}).Count(&remain).Error; err != nil {
		t.Fatal(err)
	}
	if remain != 3 {
		t.Fatalf("remaining rows = %d, want 3 (downloaded + fresh + hosted)", remain)
	}
	var c int64
	if err := db.Model(&model.Artifact{}).Where("id = ?", stale.ID).Count(&c).Error; err != nil {
		t.Fatal(err)
	}
	if c != 0 {
		t.Fatalf("stale no-blob proxy row not deleted")
	}
}

// TestProxyMetadataCacheGCDisabled 验证 enabled=false 时不删除任何行。
func TestProxyMetadataCacheGCDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	proxy := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}
	old := model.Artifact{RepositoryID: proxy.ID, Format: "npm", Kind: "version", Name: "pkg", Version: "1.0.0", IdentityKey: "version/pkg/1.0.0", UpdatedAt: time.Now().AddDate(0, 0, -40)}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}

	gc := NewProxyMetadataCacheGC(db, nil, nil) // disabled 测试在 nil 检查前短路
	gc.enabled = false
	deleted, err := gc.Run(context.Background())
	if err != nil {
		t.Fatalf("gc run: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0 when disabled", deleted)
	}
	var remain int64
	if err := db.Model(&model.Artifact{}).Count(&remain).Error; err != nil {
		t.Fatal(err)
	}
	if remain != 1 {
		t.Fatalf("remaining rows = %d, want 1 when disabled", remain)
	}
}
