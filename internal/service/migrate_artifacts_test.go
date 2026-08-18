package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newMigrateTestDB 打开内存 SQLite 并迁移迁移功能涉及的表。
func newMigrateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryMember{}, &model.Artifact{}, &model.Blob{},
		&model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func createMigrateRepo(t *testing.T, db *gorm.DB, name string, typ model.RepositoryType, pkgType string) uint {
	t.Helper()
	repo := model.Repository{Name: name, Type: typ, PackageType: pkgType}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo %s: %v", name, err)
	}
	return repo.ID
}

// seedMigrateContent 插入一个 artifact 及其 packages/package_versions 投影行。
func seedMigrateContent(t *testing.T, db *gorm.DB, repoID uint, format, name, version, identityKey string) {
	t.Helper()
	artifact := model.Artifact{
		RepositoryID: repoID,
		Format:       format,
		Kind:         "artifact",
		Name:         name,
		Version:      version,
		IdentityKey:  identityKey,
		RemotePath:   identityKey,
		Filename:     name + "-" + version + ".jar",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := db.Create(&model.Package{RepositoryID: repoID, Format: format, Name: name}).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	if err := db.Create(&model.PackageVersion{
		RepositoryID:     repoID,
		Format:           format,
		PackageName:      name,
		Version:          version,
		LatestArtifactAt: artifact.CreatedAt,
	}).Error; err != nil {
		t.Fatalf("create package version: %v", err)
	}
}

func TestMigrateArtifactsToRepoMovesAllRows(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	seedMigrateContent(t, db, source, "maven", "com.example:foo", "1.0-SNAPSHOT",
		"file/com/example/foo/1.0-SNAPSHOT/foo-1.0-20260101.120000-1.jar")

	svc := NewArtifactService(db)
	result, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", result.Conflicts)
	}
	if result.MovedArtifacts != 1 || result.MovedPackages != 1 || result.MovedVersions != 1 {
		t.Fatalf("moved = (%d,%d,%d), want (1,1,1)",
			result.MovedArtifacts, result.MovedPackages, result.MovedVersions)
	}

	var count int64
	db.Model(&model.Artifact{}).Where("repository_id = ?", source).Count(&count)
	if count != 0 {
		t.Fatalf("source still has %d artifacts, want 0", count)
	}
	db.Model(&model.Artifact{}).Where("repository_id = ?", target).Count(&count)
	if count != 1 {
		t.Fatalf("target has %d artifacts, want 1", count)
	}
	db.Model(&model.Package{}).Where("repository_id = ?", target).Count(&count)
	if count != 1 {
		t.Fatalf("target has %d packages, want 1", count)
	}
	db.Model(&model.PackageVersion{}).Where("repository_id = ?", target).Count(&count)
	if count != 1 {
		t.Fatalf("target has %d package_versions, want 1", count)
	}
}

func TestMigrateArtifactsToRepoEmptySource(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")

	svc := NewArtifactService(db)
	result, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(result.Conflicts) != 0 || result.MovedArtifacts != 0 || result.MovedPackages != 0 || result.MovedVersions != 0 {
		t.Fatalf("expected all zero, got %+v", result)
	}
}

func TestMigrateArtifactsToRepoPackageConflict(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	// 两侧同一包名、不同版本：packages(format+name) 冲突，versions 不冲突
	seedMigrateContent(t, db, source, "maven", "com.example:foo", "1.0-SNAPSHOT",
		"file/foo-1.0-SNAPSHOT.jar")
	seedMigrateContent(t, db, target, "maven", "com.example:foo", "2.0-SNAPSHOT",
		"file/foo-2.0-SNAPSHOT.jar")

	svc := NewArtifactService(db)
	result, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(result.Conflicts) == 0 {
		t.Fatalf("expected package conflict, got none")
	}
	seenPackage := false
	for _, c := range result.Conflicts {
		if c.Kind == "package" && c.Name == "com.example:foo" {
			seenPackage = true
		}
		if c.Kind == "version" {
			t.Fatalf("unexpected version conflict %+v", c)
		}
	}
	if !seenPackage {
		t.Fatalf("conflict list missing package: %+v", result.Conflicts)
	}

	// 冲突时不执行迁移
	var count int64
	db.Model(&model.Artifact{}).Where("repository_id = ?", source).Count(&count)
	if count != 1 {
		t.Fatalf("migration ran despite conflict: source has %d artifacts, want 1", count)
	}
}

func TestMigrateArtifactsToRepoVersionConflict(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	// 两侧同一包名+同一版本：version 冲突
	seedMigrateContent(t, db, source, "maven", "com.example:foo", "1.0-SNAPSHOT",
		"file/com/example/foo/1.0-SNAPSHOT/foo-1.0-20260101.120000-1.jar")
	seedMigrateContent(t, db, target, "maven", "com.example:foo", "1.0-SNAPSHOT",
		"file/com/example/foo/1.0-SNAPSHOT/foo-1.0-20260102.120000-1.jar")

	svc := NewArtifactService(db)
	result, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seenVersion := false
	for _, c := range result.Conflicts {
		if c.Kind == "version" && c.Name == "com.example:foo" && c.Version == "1.0-SNAPSHOT" {
			seenVersion = true
		}
	}
	if !seenVersion {
		t.Fatalf("expected version conflict, got %+v", result.Conflicts)
	}
}

func TestMigrateArtifactsToRepoArtifactConflict(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	// 目标侧只有 artifact 行（无投影），identity_key 与源重叠 → artifact 冲突
	seedMigrateContent(t, db, source, "maven", "com.example:shared", "1.0-SNAPSHOT",
		"file/com/example/shared-1.0.jar")
	db.Create(&model.Artifact{
		RepositoryID: target,
		Format:       "maven",
		Kind:         "artifact",
		Name:         "com.example:shared",
		Version:      "2.0-SNAPSHOT",
		IdentityKey:  "file/com/example/shared-1.0.jar",
		RemotePath:   "file/com/example/shared-1.0.jar",
	})

	svc := NewArtifactService(db)
	result, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seenArtifact := false
	for _, c := range result.Conflicts {
		if c.Kind == "artifact" && c.Name == "file/com/example/shared-1.0.jar" {
			seenArtifact = true
		}
	}
	if !seenArtifact {
		t.Fatalf("expected artifact conflict, got %+v", result.Conflicts)
	}
	if result.TotalConflicts != 1 {
		t.Fatalf("TotalConflicts = %d, want 1", result.TotalConflicts)
	}
	if result.MovedArtifacts != 0 {
		t.Fatalf("migration ran despite conflict")
	}
}

// TestMigrateArtifactsToRepoConflictSampleCapped 验证冲突数超过采样上限时仍报告完整总数、
// 只返回采样列表（配合 handler 的前 20 条展示），且不会把全量冲突物化。
func TestMigrateArtifactsToRepoConflictSampleCapped(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	// 25 个包发生了 packages 级冲突（两侧同 format+name、不同版本，避免 version 冲突）
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("com.example:pkg-%02d", i)
		seedMigrateContent(t, db, source, "maven", name, "1.0-SNAPSHOT",
			fmt.Sprintf("file/%s-1.0-SNAPSHOT.jar", name))
		seedMigrateContent(t, db, target, "maven", name, "2.0-SNAPSHOT",
			fmt.Sprintf("file/%s-2.0-SNAPSHOT.jar", name))
	}

	svc := NewArtifactService(db)
	result, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if result.TotalConflicts != 25 {
		t.Fatalf("TotalConflicts = %d, want 25", result.TotalConflicts)
	}
	if len(result.Conflicts) > maxConflictSamples {
		t.Fatalf("Conflicts sample size = %d, want <= %d", len(result.Conflicts), maxConflictSamples)
	}
	// 采样仍应命中源数据（抽样正确性）
	if len(result.Conflicts) != maxConflictSamples {
		t.Fatalf("Conflicts sample size = %d, want %d", len(result.Conflicts), maxConflictSamples)
	}
}

func TestMigrateArtifactsToRepoRollsBackOnFailure(t *testing.T) {
	db := newMigrateTestDB(t)
	source := createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	target := createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	seedMigrateContent(t, db, source, "maven", "com.example:foo", "1.0-SNAPSHOT",
		"file/foo-1.0-SNAPSHOT.jar")

	// 让 packages 的 UPDATE 失败，验证 artifacts 的 UPDATE 随事务回滚
	cbName := "test-fail-packages"
	db.Callback().Update().Before("gorm:update").Register(cbName, func(gdb *gorm.DB) {
		if gdb.Statement.Table == "packages" {
			gdb.AddError(errors.New("injected failure"))
		}
	})
	defer db.Callback().Update().Before("gorm:update").Remove(cbName)

	svc := NewArtifactService(db)
	_, err := svc.MigrateArtifactsToRepo(context.Background(), source, target)
	if err == nil {
		t.Fatalf("expected error from injected failure")
	}

	// 回滚后 source 数据原样
	var count int64
	db.Model(&model.Artifact{}).Where("repository_id = ?", source).Count(&count)
	if count != 1 {
		t.Fatalf("rollback failed: source has %d artifacts, want 1", count)
	}
	db.Model(&model.Package{}).Where("repository_id = ?", source).Count(&count)
	if count != 1 {
		t.Fatalf("rollback failed: source has %d packages, want 1", count)
	}
}

// --- RepositoryService.MigrateCacheToRepo 校验 ---

func newMigrateRepoSvc(t *testing.T, db *gorm.DB) *RepositoryService {
	t.Helper()
	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	svc := NewRepositoryService(repoRepo, groupRepo, db)
	svc.SetArtifactService(NewArtifactService(db))
	return svc
}

func TestMigrateCacheToRepoValidation(t *testing.T) {
	db := newMigrateTestDB(t)
	createMigrateRepo(t, db, "maven-snapshots", model.RepoTypeProxy, "maven")
	createMigrateRepo(t, db, "maven-local-copy", model.RepoTypeLocal, "maven")
	createMigrateRepo(t, db, "npm-local", model.RepoTypeLocal, "npm")
	svc := newMigrateRepoSvc(t, db)

	// source 是 proxy、target 是 local → 通过（无内容，moved=0）
	result, err := svc.MigrateCacheToRepo(context.Background(), "maven-snapshots", "maven-local-copy")
	if err != nil {
		t.Fatalf("valid migration failed: %v", err)
	}
	if len(result.Conflicts) != 0 || result.MovedArtifacts != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	// source 不是 proxy
	if _, err := svc.MigrateCacheToRepo(context.Background(), "maven-local-copy", "maven-snapshots"); err == nil {
		t.Fatalf("expected error when source is not proxy")
	}

	// target 不是 local
	if _, err := svc.MigrateCacheToRepo(context.Background(), "maven-snapshots", "npm-local"); err == nil {
		t.Fatalf("expected error when target is not local")
	}

	// format 不一致
	createMigrateRepo(t, db, "maven-proxy-other", model.RepoTypeProxy, "npm")
	if _, err := svc.MigrateCacheToRepo(context.Background(), "maven-proxy-other", "maven-local-copy"); err == nil {
		t.Fatalf("expected error on format mismatch")
	}

	// source == target
	if _, err := svc.MigrateCacheToRepo(context.Background(), "maven-snapshots", "maven-snapshots"); err == nil {
		t.Fatalf("expected error when source == target")
	}

	// 目标不存在
	if _, err := svc.MigrateCacheToRepo(context.Background(), "maven-snapshots", "no-such-repo"); err == nil {
		t.Fatalf("expected error when target missing")
	}
}
