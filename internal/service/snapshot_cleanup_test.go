package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// snapshotBuild 描述 SNAPSHOT 的一个构建：构建号、jar 时间戳、创建时间。
type snapshotBuild struct {
	buildNum   int
	ts         string // Maven 时间戳，形如 "20260603.033633"
	createdAt  time.Time
}

// newSnapshotCleanupFixture 构建一个 Maven 本地仓库 + 一组 SNAPSHOT 构建（每个含 jar + 该 jar 的 .sha1 checksum 行）。
// 返回 repository_id、cleanup 任务和 DB。builds 按给定顺序写入 DB。
func newSnapshotCleanupFixture(t *testing.T, builds []snapshotBuild) (uint, *gorm.DB, *MavenSnapshotCleanup) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "maven-local", Type: model.RepoTypeLocal, PackageType: "maven"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	mk := func(remotePath, filename, kind string) model.Artifact {
		prefix := "file"
		if kind == runtime.KindChecksum {
			prefix = "checksum"
		}
		return model.Artifact{
			RepositoryID: repo.ID,
			Format:       "maven",
			Kind:         kind,
			IdentityKey:  prefix + "/" + remotePath,
			Name:         "com.example:lib",
			Version:      "1.0-SNAPSHOT",
			Path:         "com/example/lib/1.0-SNAPSHOT",
			Filename:     filename,
			RemotePath:   remotePath,
			CreatedAt:    time.Now().Add(-10 * 24 * time.Hour), // placeholder，下方覆盖
		}
	}

	var artifacts []model.Artifact
	for _, b := range builds {
		jarPath := fmt.Sprintf("com/example/lib/1.0-SNAPSHOT/lib-1.0-%s-%d.jar", b.ts, b.buildNum)
		jar := mk(jarPath, fmt.Sprintf("lib-1.0-%s-%d.jar", b.ts, b.buildNum), runtime.KindArtifact)
		jar.CreatedAt = b.createdAt
		sha := mk(jarPath+".sha1", fmt.Sprintf("lib-1.0-%s-%d.jar.sha1", b.ts, b.buildNum), runtime.KindChecksum)
		sha.CreatedAt = b.createdAt
		artifacts = append(artifacts, jar, sha)
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	store := storage.NewMetadataStoreWithArtifactService(db, nil)
	task := NewMavenSnapshotCleanup(db, repository.NewRepositoryRepository(db), store, nil)
	return repo.ID, db, task
}

// buildAt 生成一个创建于指定时间点、给定构建号的构建。
func buildAt(n int, createdAt time.Time) snapshotBuild {
	ts := createdAt.UTC().Format("20060102.150405")
	return snapshotBuild{buildNum: n, ts: ts, createdAt: createdAt.UTC()}
}

// TestMavenSnapshotCleanupDeletesChecksumWithArtifact 回归测试：
// 清理一个因超过 keep_last 且超过 max_age_days 而应移除的旧 SNAPSHOT 构建时，
// 该构建的 .sha1 checksum 行必须与其 jar 一并删除，不能留下孤儿 checksum。
func TestMavenSnapshotCleanupDeletesChecksumWithArtifact(t *testing.T) {
	now := time.Now().UTC()
	// 7 个构建：最近 5 个在 90 天内（保留），最早 2 个超出 90 天且排在 keep_last=5 之外（应删除）。
	// 删除的两个是 build 1 和 build 2（最旧）。
	var builds []snapshotBuild
	for i := 2; i >= 1; i-- {
		builds = append(builds, buildAt(i, now.Add(-time.Duration(180+i*30)*24*time.Hour)))
	}
	for i := 7; i >= 3; i-- {
		builds = append(builds, buildAt(i, now.Add(-time.Duration(i)*24*time.Hour)))
	}

	_, db, task := newSnapshotCleanupFixture(t, builds)

	deleted, err := task.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	// 期望删除 build 1 和 build 2：各自 jar + checksum = 4 行。
	if deleted != 4 {
		t.Fatalf("deleted = %d, want 4 (2 old jars + their 2 checksums)", deleted)
	}

	var remaining []model.Artifact
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("query remaining: %v", err)
	}
	wantRemaining := 2*7 - 4 // 14 行，删 4 行 = 10
	if len(remaining) != wantRemaining {
		t.Fatalf("remaining artifacts = %d, want %d; got: %+v", len(remaining), wantRemaining, remaining)
	}
	for _, a := range remaining {
		if filenameContainsDeletedBuild(a.Filename) {
			t.Fatalf("deleted build artifact %q was not cleaned up", a.Filename)
		}
	}
}

func filenameContainsDeletedBuild(filename string) bool {
	// 被删除的是 build 1 与 build 2（构建号 1、2）
	return endsWithBuildNum(filename, 1) || endsWithBuildNum(filename, 2)
}

func endsWithBuildNum(filename string, n int) bool {
	suffixJar := fmt.Sprintf("-%d.jar", n)
	suffixSha := fmt.Sprintf("-%d.jar.sha1", n)
	return len(filename) > len(suffixJar) &&
		(filename[len(filename)-len(suffixJar):] == suffixJar ||
			len(filename) > len(suffixSha) && filename[len(filename)-len(suffixSha):] == suffixSha)
}

// TestMavenSnapshotCleanupKeepsUnparseableBasePom 确保修复没有破坏保留语义：
// 无法解析为时间戳构建的 baseVersion .pom 必须保留（哪怕它属于一个整体无保留价值的版本目录）。
func TestMavenSnapshotCleanupKeepsUnparseableBasePom(t *testing.T) {
	// 复用会出现删除的 7-build 组，证明 baseVersion 的 .pom（不可解析为时间戳构建）不被误删。
	now := time.Now().UTC()
	var builds []snapshotBuild
	for i := 2; i >= 1; i-- {
		builds = append(builds, buildAt(i, now.Add(-time.Duration(180+i*30)*24*time.Hour)))
	}
	for i := 7; i >= 3; i-- {
		builds = append(builds, buildAt(i, now.Add(-time.Duration(i)*24*time.Hour)))
	}

	repoID, db, task := newSnapshotCleanupFixture(t, builds)

	pom := model.Artifact{
		RepositoryID: repoID,
		Format:       "maven",
		Kind:         runtime.KindArtifact,
		IdentityKey:  "file/com/example/lib/1.0-SNAPSHOT/lib-1.0-SNAPSHOT.pom",
		Name:         "com.example:lib",
		Version:      "1.0-SNAPSHOT",
		Path:         "com/example/lib/1.0-SNAPSHOT",
		Filename:     "lib-1.0-SNAPSHOT.pom",
		RemotePath:   "com/example/lib/1.0-SNAPSHOT/lib-1.0-SNAPSHOT.pom",
	}
	if err := db.Create(&pom).Error; err != nil {
		t.Fatalf("create pom: %v", err)
	}

	deleted, err := task.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	// 期望删除 build 1、2（jar+checksum=4 行），base .pom 必须保留。
	if deleted != 4 {
		t.Fatalf("deleted = %d, want 4 (2 old jars + 2 checksums); pom must survive anyway", deleted)
	}

	var pomCount int64
	if err := db.Model(&model.Artifact{}).Where("filename = ?", "lib-1.0-SNAPSHOT.pom").Count(&pomCount).Error; err != nil {
		t.Fatalf("count pom: %v", err)
	}
	if pomCount != 1 {
		t.Fatalf("pom count = %d, want 1 (base pom must be kept)", pomCount)
	}
}