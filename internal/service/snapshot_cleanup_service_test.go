package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSnapshotCleanupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}, &model.ArtifactBlob{}, &model.Repository{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestMavenSnapshotCleanup_TimestampedVersions 验证时间戳版本（1.0-20250101.010000-1）的行
// 能被清理任务选中，并与基础版本（1.0-SNAPSHOT）归到同一 GAV 组、按同一保留策略清理。
// 修复前：SQL 只按 version LIKE '%-SNAPSHOT' 过滤，时间戳版本行整批漏掉，永不清理。
func TestMavenSnapshotCleanup_TimestampedVersions(t *testing.T) {
	db := newSnapshotCleanupTestDB(t)
	repo := model.Repository{Name: "snapshots", Type: "local", PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	const name = "com.example:my-lib"
	const base = "1.0"
	// 7 个构建：版本形式混合（基础版本/时间戳版本交替），时间戳都在保留窗口之外
	for n := 1; n <= 7; n++ {
		ts := fmt.Sprintf("2025010%d.010000", n)
		version := base + "-SNAPSHOT"
		if n%2 == 1 {
			version = fmt.Sprintf("%s-%s-%d", base, ts, n)
		}
		jar := fmt.Sprintf("my-lib-%s-%s-%d.jar", base, ts, n)
		sha1 := jar + ".sha1"
		// jar + checksum 一对：同一个构建，kind 不同
		for _, f := range []struct {
			kind     string
			filename string
		}{
			{"artifact", jar},
			{"checksum", sha1},
		} {
			if err := db.Create(&model.Artifact{
				RepositoryID: repo.ID,
				Format:       "maven",
				Kind:         f.kind,
				Name:         name,
				Version:      version,
				Filename:     f.filename,
				RemotePath:   "snapshots/1.0-SNAPSHOT/" + f.filename,
			}).Error; err != nil {
				t.Fatalf("create artifact: %v", err)
			}
		}
	}

	cleanup := NewMavenSnapshotCleanup(db, nil, storage.NewMetadataStore(db), nil)
	deleted, err := cleanup.cleanupRepo(context.Background(), repo.ID, "snapshots", 2, 90, false)
	if err != nil {
		t.Fatalf("cleanupRepo: %v", err)
	}
	// 保留最新 2 个构建（7、6），删除 5 个构建 = 5 jar + 5 sha1
	if deleted != 10 {
		t.Fatalf("deleted = %d, want 10（时间戳版本行必须被选中并参与清理）", deleted)
	}

	var remain []model.Artifact
	if err := db.Where("repository_id = ?", repo.ID).Find(&remain).Error; err != nil {
		t.Fatalf("query remain: %v", err)
	}
	if len(remain) != 4 {
		t.Fatalf("remaining rows = %d, want 4（最新 2 个构建 × jar/sha1）", len(remain))
	}
	for _, a := range remain {
		if !strings.Contains(a.Filename, "20250106") && !strings.Contains(a.Filename, "20250107") {
			t.Errorf("remaining filename = %s, want build 6/7 的产物", a.Filename)
		}
	}
}

// TestMavenSnapshotCleanup_DryRun 验证 dry-run 模式：计算出将删除的行但实际不删除。
func TestMavenSnapshotCleanup_DryRun(t *testing.T) {
	db := newSnapshotCleanupTestDB(t)
	repo := model.Repository{Name: "snapshots", Type: "local", PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	const name = "com.example:my-lib"
	for n := 1; n <= 4; n++ {
		ts := fmt.Sprintf("2025010%d.010000", n)
		jar := fmt.Sprintf("my-lib-1.0-%s-%d.jar", ts, n)
		for _, f := range []struct {
			kind     string
			filename string
		}{
			{"artifact", jar},
			{"checksum", jar + ".sha1"},
		} {
			if err := db.Create(&model.Artifact{
				RepositoryID: repo.ID, Format: "maven", Kind: f.kind, Name: name,
				Version:    "1.0-SNAPSHOT",
				Filename:   f.filename,
				RemotePath: "snapshots/1.0-SNAPSHOT/" + f.filename,
			}).Error; err != nil {
				t.Fatalf("create artifact: %v", err)
			}
		}
	}

	cleanup := NewMavenSnapshotCleanup(db, nil, storage.NewMetadataStore(db), nil)
	// dryRun=true：返回将删除数，但数据库行必须原样保留
	deleted, err := cleanup.cleanupRepo(context.Background(), repo.ID, "snapshots", 2, 90, true)
	if err != nil {
		t.Fatalf("cleanupRepo(dry-run): %v", err)
	}
	if deleted != 4 {
		t.Fatalf("dry-run 将删除 = %d, want 4（保留最新 2 构建，删 2 构建 × jar/sha1）", deleted)
	}
	var remain int64
	db.Model(&model.Artifact{}).Where("repository_id = ?", repo.ID).Count(&remain)
	if remain != 8 {
		t.Fatalf("dry-run 后仍应保留 %d 行, got %d", 8, remain)
	}
}

// TestMavenSnapshotCleanup_UniqueLayoutTimestampedVersion 验证唯一快照布局：
// version 列与目录都是时间戳形式（1.0-20250101.010000-1）的行必须被选中并清理。
// 这是线上"通过时间戳 URL 直接拉取缓存"产生的行——既不在 -SNAPSHOT/ 目录下、
// version 也不是 1.0-SNAPSHOT，仅路径锚定会整批漏掉。
func TestMavenSnapshotCleanup_UniqueLayoutTimestampedVersion(t *testing.T) {
	db := newSnapshotCleanupTestDB(t)
	repo := model.Repository{Name: "snapshots", Type: "local", PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// 唯一快照布局：版本/目录都是时间戳形式
	uniq := model.Artifact{
		RepositoryID: repo.ID, Format: "maven", Kind: "artifact", Name: "com.example:uniq-lib",
		Version:    "1.0-20250101.010000-1",
		Filename:   "uniq-lib-1.0-20250101.010000-1.jar",
		RemotePath: "snapshots/1.0-20250101.010000-1/uniq-lib-1.0-20250101.010000-1.jar",
	}
	if err := db.Create(&uniq).Error; err != nil {
		t.Fatalf("create unique-layout artifact: %v", err)
	}
	// 常规布局：基础版本行
	base := model.Artifact{
		RepositoryID: repo.ID, Format: "maven", Kind: "artifact", Name: "com.example:my-lib",
		Version:    "1.0-SNAPSHOT",
		Filename:   "my-lib-1.0-20250101.010000-1.jar",
		RemotePath: "snapshots/1.0-SNAPSHOT/my-lib-1.0-20250101.010000-1.jar",
	}
	if err := db.Create(&base).Error; err != nil {
		t.Fatalf("create base artifact: %v", err)
	}

	cleanup := NewMavenSnapshotCleanup(db, nil, storage.NewMetadataStore(db), nil)
	deleted, err := cleanup.cleanupRepo(context.Background(), repo.ID, "snapshots", 0, 90, false)
	if err != nil {
		t.Fatalf("cleanupRepo: %v", err)
	}
	// keepLast=0 + 超期：两种布局都应被清理
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2（唯一快照布局行 + 常规布局行都必须被清理）", deleted)
	}
}
