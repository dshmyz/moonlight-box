package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newPublishedAtTestService 建立带 PackageVersion 表的内存库与 ArtifactService。
func newPublishedAtTestService(t *testing.T) (*ArtifactService, *model.Repository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := model.Repository{Name: "maven-repo", Type: model.RepoTypeProxy, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	return NewArtifactService(db), &repo
}

// saveSnapshotFile 保存一个 Maven 快照文件行，publishedAt 为空时不写属性。
func saveSnapshotFile(t *testing.T, svc *ArtifactService, repoID uint, version, filename, publishedAt string) {
	t.Helper()
	attrs := map[string]string{}
	if publishedAt != "" {
		attrs["published_at"] = publishedAt
	}
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repoID),
		Format:       "maven",
		Kind:         runtime.KindArtifact,
		Name:         "com.example:lib",
		Version:      version,
		Filename:     filename,
		RemotePath:   "com/example/lib/" + version + "/" + filename,
		Attributes:   attrs,
		Properties:   attrs,
		BlobRefs:     []runtime.BlobRef{{Digest: "sha256:" + filename, Size: 100}},
	})); err != nil {
		t.Fatalf("save artifact %s: %v", filename, err)
	}
}

func loadVersionPublishedAt(t *testing.T, svc *ArtifactService, repoID uint, version string) *time.Time {
	t.Helper()
	if err := svc.RefreshPackageVersionSummary(context.Background(), repoID, "maven", "com.example:lib", version); err != nil {
		t.Fatalf("refresh summary: %v", err)
	}
	db := svc.db
	var pv model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", repoID, "maven", "com.example:lib", version).First(&pv).Error; err != nil {
		t.Fatalf("load package version: %v", err)
	}
	return pv.PublishedAt
}

// SNAPSHOT 版本发布时间 = 所有文件行 published_at 的最大值（最新构建时间）。
// 先保存新构建、后保存旧构建：现有实现按 updated_at DESC 取首个，
// 若实现不是取 max 而是取首个，此顺序下会错误返回旧构建时间。
func TestSnapshotPublishedAtTakesLatestBuild(t *testing.T) {
	svc, repo := newPublishedAtTestService(t)
	saveSnapshotFile(t, svc, repo.ID, "1.0-SNAPSHOT", "lib-1.0-20260605.090000-3.jar", "2026-06-05T09:00:00Z")
	saveSnapshotFile(t, svc, repo.ID, "1.0-SNAPSHOT", "lib-1.0-20260604.090000-2.jar", "2026-06-04T09:00:00Z")

	got := loadVersionPublishedAt(t, svc, repo.ID, "1.0-SNAPSHOT")
	if got == nil {
		t.Fatal("published_at is nil, want latest build time")
	}
	if want := "2026-06-05T09:00:00Z"; got.UTC().Format(time.RFC3339) != want {
		t.Fatalf("published_at = %v, want %s", got.UTC().Format(time.RFC3339), want)
	}
}

// metadata 行（KindVersion，代理同步写入 lastUpdated）晚于最新构建时，
// 文件行的真实构建时间仍优先，不被 metadata 时间污染。
func TestSnapshotPublishedAtPrefersFileRowOverMetadataRow(t *testing.T) {
	svc, repo := newPublishedAtTestService(t)
	saveSnapshotFile(t, svc, repo.ID, "1.0-SNAPSHOT", "lib-1.0-20260604.090000-2.jar", "2026-06-04T09:00:00Z")
	// 模拟代理同步的版本行：updated_at 更新、published_at 晚于文件行
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "maven",
		Kind:         runtime.KindVersion,
		Name:         "com.example:lib",
		Version:      "1.0-SNAPSHOT",
		Attributes:   map[string]string{"published_at": "2026-06-06T12:00:00Z"},
		Properties:   map[string]string{"published_at": "2026-06-06T12:00:00Z"},
	})); err != nil {
		t.Fatalf("save version row: %v", err)
	}

	got := loadVersionPublishedAt(t, svc, repo.ID, "1.0-SNAPSHOT")
	if got == nil {
		t.Fatal("published_at is nil, want file-row build time")
	}
	if want := "2026-06-04T09:00:00Z"; got.UTC().Format(time.RFC3339) != want {
		t.Fatalf("published_at = %v, want file-row time %s", got.UTC().Format(time.RFC3339), want)
	}
}

// 无文件行时（release 版本或仅 metadata），回退 metadata 行 published_at——
// 锁住既有行为不变。
func TestPublishedAtFallsBackToMetadataRow(t *testing.T) {
	svc, repo := newPublishedAtTestService(t)
	if err := svc.Save(context.Background(), runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: fmt.Sprint(repo.ID),
		Format:       "maven",
		Kind:         runtime.KindVersion,
		Name:         "com.example:lib",
		Version:      "1.2.3",
		Attributes:   map[string]string{"published_at": "2026-01-01T00:00:00Z"},
		Properties:   map[string]string{"published_at": "2026-01-01T00:00:00Z"},
	})); err != nil {
		t.Fatalf("save version row: %v", err)
	}

	got := loadVersionPublishedAt(t, svc, repo.ID, "1.2.3")
	if got == nil {
		t.Fatal("published_at is nil, want metadata time fallback")
	}
	if want := "2026-01-01T00:00:00Z"; got.UTC().Format(time.RFC3339) != want {
		t.Fatalf("published_at = %v, want %s", got.UTC().Format(time.RFC3339), want)
	}
}

// 追加新构建后时间前进；随后重复 refresh（重复同步同一批文件）时间保持不变。
func TestSnapshotPublishedAtAdvancesAndStable(t *testing.T) {
	svc, repo := newPublishedAtTestService(t)
	saveSnapshotFile(t, svc, repo.ID, "1.0-SNAPSHOT", "lib-1.0-20260604.090000-2.jar", "2026-06-04T09:00:00Z")

	first := loadVersionPublishedAt(t, svc, repo.ID, "1.0-SNAPSHOT")
	if first == nil || first.UTC().Format(time.RFC3339) != "2026-06-04T09:00:00Z" {
		t.Fatalf("initial published_at = %v, want 2026-06-04T09:00:00Z", first)
	}

	// 重复同步同一批文件（同一行重新 Save）+ 上游 metadata lastUpdated 变化
	// ——都不能改变发布时间
	saveSnapshotFile(t, svc, repo.ID, "1.0-SNAPSHOT", "lib-1.0-20260604.090000-2.jar", "2026-06-04T09:00:00Z")
	stable := loadVersionPublishedAt(t, svc, repo.ID, "1.0-SNAPSHOT")
	if stable.UTC().Format(time.RFC3339) != "2026-06-04T09:00:00Z" {
		t.Fatalf("after re-sync published_at = %v, want unchanged", stable)
	}

	// 上游出现新构建 → 时间前进
	saveSnapshotFile(t, svc, repo.ID, "1.0-SNAPSHOT", "lib-1.0-20260605.090000-3.jar", "2026-06-05T09:00:00Z")
	advanced := loadVersionPublishedAt(t, svc, repo.ID, "1.0-SNAPSHOT")
	if advanced.UTC().Format(time.RFC3339) != "2026-06-05T09:00:00Z" {
		t.Fatalf("after new build published_at = %v, want 2026-06-05T09:00:00Z", advanced)
	}
}
