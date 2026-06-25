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

// TestSearchFromPackagesWithVersionFilterUsesFastPath 验证带 version 参数时走 packages 快速路径（EXISTS 子查询），
// 不回退到 artifacts 全量聚合慢路径，且正确过滤出包含匹配版本的包。
func TestSearchFromPackagesWithVersionFilterUsesFastPath(t *testing.T) {
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
	// 两个包：left-pad 有 1.0.0 和 1.2.0，right-pad 只有 2.0.0
	packages := []model.Package{
		{RepositoryID: repo.ID, Format: "npm", Name: "left-pad", VersionCount: 2, LatestVersion: "1.2.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Name: "right-pad", VersionCount: 1, LatestVersion: "2.0.0", UpdatedAt: now},
	}
	if err := db.Create(&packages).Error; err != nil {
		t.Fatalf("create packages: %v", err)
	}
	artifacts := []model.Artifact{
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "left-pad", Version: "1.0.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "left-pad", Version: "1.2.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "right-pad", Version: "2.0.0", UpdatedAt: now},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	svc := NewPackageSearchService(db)

	// 精确版本匹配
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "npm",
		Version:  "1.0.0",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search with exact version failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 || got.List[0].Name != "left-pad" {
		t.Fatalf("exact version: expected only left-pad, got total=%d list=%#v", got.Total, got.List)
	}

	// 通配符版本匹配：1.* 应匹配 left-pad（有 1.0.0 和 1.2.0），不匹配 right-pad（只有 2.0.0）
	got, err = svc.Search(context.Background(), &SearchRequest{
		Type:     "npm",
		Version:  "1.*",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search with glob version failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 || got.List[0].Name != "left-pad" {
		t.Fatalf("glob version 1.*: expected only left-pad, got total=%d list=%#v", got.Total, got.List)
	}
}

// TestSearchFromPackagesWithVersionFilterNoMatch 验证 version 过滤无匹配时返回空结果
func TestSearchFromPackagesWithVersionFilterNoMatch(t *testing.T) {
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
		RepositoryID: repo.ID, Format: "npm", Name: "left-pad", VersionCount: 1, LatestVersion: "1.0.0", UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	if err := db.Create(&model.Artifact{
		RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "left-pad", Version: "1.0.0", UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "npm",
		Version:  "9.9.9",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if got.Total != 0 || len(got.List) != 0 {
		t.Fatalf("expected empty result for non-matching version, got total=%d list=%#v", got.Total, got.List)
	}
}

// TestSearchVersionCharClassFallsBackToArtifactsPath 验证含 [ 的字符类 pattern
// 回退到 artifacts 路径并用 filepath.Match 精确过滤（SQL LIKE 不支持字符类）
func TestSearchVersionCharClassFallsBackToArtifactsPath(t *testing.T) {
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
	// 两个包：pkg-a 有 1.0.0 和 2.0.0，pkg-b 只有 3.0.0
	packages := []model.Package{
		{RepositoryID: repo.ID, Format: "npm", Name: "pkg-a", VersionCount: 2, LatestVersion: "2.0.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Name: "pkg-b", VersionCount: 1, LatestVersion: "3.0.0", UpdatedAt: now},
	}
	if err := db.Create(&packages).Error; err != nil {
		t.Fatalf("create packages: %v", err)
	}
	artifacts := []model.Artifact{
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "pkg-a", Version: "1.0.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "pkg-a", Version: "2.0.0", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "pkg-b", Version: "3.0.0", UpdatedAt: now},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	svc := NewPackageSearchService(db)
	// [12].0.0 应匹配 pkg-a（有 1.0.0 和 2.0.0），不匹配 pkg-b（只有 3.0.0）
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "npm",
		Version:  "[12].0.0",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search with char class version failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 || got.List[0].Name != "pkg-a" {
		t.Fatalf("char class [12].0.0: expected only pkg-a, got total=%d list=%#v", got.Total, got.List)
	}
}

// TestSearchVersionWithUnderscoreInPattern 验证含 _ 的通配符 pattern 正确转义
// _ 在 SQL LIKE 中是单字符通配符，但 versionToSQLCondition 会转义它
func TestSearchVersionWithUnderscoreInPattern(t *testing.T) {
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
	// pkg_special 有 1.0.0_beta 和 1.0.0_release
	packages := []model.Package{
		{RepositoryID: repo.ID, Format: "npm", Name: "pkg_special", VersionCount: 2, LatestVersion: "1.0.0_release", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Name: "pkg_normal", VersionCount: 1, LatestVersion: "1.0.0", UpdatedAt: now},
	}
	if err := db.Create(&packages).Error; err != nil {
		t.Fatalf("create packages: %v", err)
	}
	artifacts := []model.Artifact{
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "pkg_special", Version: "1.0.0_beta", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "pkg_special", Version: "1.0.0_release", UpdatedAt: now},
		{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "pkg_normal", Version: "1.0.0", UpdatedAt: now},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	svc := NewPackageSearchService(db)
	// 1.0.0_* 应匹配 pkg_special（有 _beta 和 _release），不匹配 pkg_normal（无 _ 前缀版本）
	// _ 必须被转义为字面量，而不是 LIKE 的单字符通配符
	got, err := svc.Search(context.Background(), &SearchRequest{
		Type:     "npm",
		Version:  "1.0.0_*",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Search with underscore pattern failed: %v", err)
	}
	if got.Total != 1 || len(got.List) != 1 || got.List[0].Name != "pkg_special" {
		t.Fatalf("underscore pattern 1.0.0_*: expected only pkg_special, got total=%d list=%#v", got.Total, got.List)
	}
}

// ============ 性能基准测试 ============

// setupSearchBenchmarkDB 构造测试数据库：
// numRepos 个仓库，每个仓库 numPkgs 个包，每个包 numVersions 个版本
func setupSearchBenchmarkDB(b *testing.B, numRepos, numPkgs, numVersions int) *gorm.DB {
	b.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		b.Fatalf("migrate db: %v", err)
	}

	now := time.Now()
	for r := 1; r <= numRepos; r++ {
		repo := model.Repository{Name: fmt.Sprintf("repo-%d", r), Type: model.RepoTypeProxy, PackageType: "npm"}
		if err := db.Create(&repo).Error; err != nil {
			b.Fatalf("create repo: %v", err)
		}

		// 批量创建 packages 和 artifacts
		pkgs := make([]model.Package, 0, numPkgs)
		arts := make([]model.Artifact, 0, numPkgs*numVersions)
		for p := 1; p <= numPkgs; p++ {
			pkgName := fmt.Sprintf("pkg-%d-%d", r, p)
			pkgs = append(pkgs, model.Package{
				RepositoryID:  repo.ID,
				Format:        "npm",
				Name:          pkgName,
				VersionCount:  numVersions,
				LatestVersion: fmt.Sprintf("1.%d.0", numVersions),
				UpdatedAt:     now,
			})
			for v := 1; v <= numVersions; v++ {
				arts = append(arts, model.Artifact{
					RepositoryID: repo.ID,
					Format:       "npm",
					Kind:         "version",
					Name:         pkgName,
					Version:      fmt.Sprintf("1.%d.0", v),
					UpdatedAt:    now,
				})
			}
		}
		if err := db.CreateInBatches(pkgs, 100).Error; err != nil {
			b.Fatalf("create packages: %v", err)
		}
		if err := db.CreateInBatches(arts, 100).Error; err != nil {
			b.Fatalf("create artifacts: %v", err)
		}
	}

	// 建索引加速（AutoMigrate 已建单列索引，这里确保 version 索引存在）
	return db
}

// BenchmarkSearchNoVersion 测试不带 version 参数的快速路径
func BenchmarkSearchNoVersion(b *testing.B) {
	sizes := []struct {
		name                           string
		numRepos, numPkgs, numVersions int
	}{
		{"small_1k_artifacts", 1, 100, 10},   // 1 repo × 100 pkgs × 10 versions = 1000 artifacts
		{"medium_10k_artifacts", 2, 500, 10}, // 2 repos × 500 pkgs × 10 versions = 10000 artifacts
		{"large_50k_artifacts", 5, 1000, 10}, // 5 repos × 1000 pkgs × 10 versions = 50000 artifacts
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			db := setupSearchBenchmarkDB(b, sz.numRepos, sz.numPkgs, sz.numVersions)
			svc := NewPackageSearchService(db)
			// 关闭缓存，每次都走真实查询
			svc.cache.Clear()
			req := &SearchRequest{
				Type:     "npm",
				Page:     1,
				PageSize: 20,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				svc.cache.Clear()
				if _, err := svc.Search(context.Background(), req); err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkSearchExactVersion 测试精确版本过滤（走 idx_artifact_version 索引）
func BenchmarkSearchExactVersion(b *testing.B) {
	sizes := []struct {
		name                           string
		numRepos, numPkgs, numVersions int
	}{
		{"small_1k_artifacts", 1, 100, 10},
		{"medium_10k_artifacts", 2, 500, 10},
		{"large_50k_artifacts", 5, 1000, 10},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			db := setupSearchBenchmarkDB(b, sz.numRepos, sz.numPkgs, sz.numVersions)
			svc := NewPackageSearchService(db)
			req := &SearchRequest{
				Type:     "npm",
				Version:  "1.5.0",
				Page:     1,
				PageSize: 20,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				svc.cache.Clear()
				if _, err := svc.Search(context.Background(), req); err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkSearchGlobVersion 测试通配符版本过滤（LIKE 前缀，走索引前缀）
func BenchmarkSearchGlobVersion(b *testing.B) {
	sizes := []struct {
		name                           string
		numRepos, numPkgs, numVersions int
	}{
		{"small_1k_artifacts", 1, 100, 10},
		{"medium_10k_artifacts", 2, 500, 10},
		{"large_50k_artifacts", 5, 1000, 10},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			db := setupSearchBenchmarkDB(b, sz.numRepos, sz.numPkgs, sz.numVersions)
			svc := NewPackageSearchService(db)
			req := &SearchRequest{
				Type:     "npm",
				Version:  "1.*",
				Page:     1,
				PageSize: 20,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				svc.cache.Clear()
				if _, err := svc.Search(context.Background(), req); err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}
		})
	}
}

func TestSearchFromArtifactsDoesNotTruncateBeforeGrouping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "fallback-search", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repository: %v", err)
	}

	now := time.Now()
	artifacts := make([]model.Artifact, 0, 50001)
	for i := 0; i < 50000; i++ {
		artifacts = append(artifacts, model.Artifact{
			RepositoryID: repo.ID,
			Format:       "npm",
			Kind:         "version",
			Name:         "hot",
			Version:      fmt.Sprintf("1.0.%d", i),
			UpdatedAt:    now,
		})
	}
	artifacts = append(artifacts, model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "version",
		Name:         "old-but-valid",
		Version:      "1.0.0",
		UpdatedAt:    now.Add(-time.Hour),
	})
	if err := db.CreateInBatches(artifacts, 1000).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{Type: "npm", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got.Total != 2 || got.RawCount != 50001 {
		t.Fatalf("total=%d raw=%d, want total=2 raw=50001", got.Total, got.RawCount)
	}
	names := map[string]bool{}
	for _, entry := range got.List {
		names[entry.Name] = true
	}
	if !names["hot"] || !names["old-but-valid"] {
		t.Fatalf("names=%#v, want hot and old-but-valid", names)
	}
}

func TestSearchFromArtifactsPagesGroupedPackagesAfterLargeArtifactSet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "fallback-pagination", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repository: %v", err)
	}

	now := time.Now()
	artifacts := make([]model.Artifact, 0, 50002)
	for i := 0; i < 50000; i++ {
		artifacts = append(artifacts, model.Artifact{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "hot", Version: fmt.Sprintf("1.0.%d", i), UpdatedAt: now})
	}
	artifacts = append(artifacts,
		model.Artifact{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "older-one", Version: "1.0.0", UpdatedAt: now.Add(-time.Hour)},
		model.Artifact{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "older-two", Version: "1.0.0", UpdatedAt: now.Add(-2 * time.Hour)},
	)
	if err := db.CreateInBatches(artifacts, 1000).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{Type: "npm", Sort: "updated_at", Page: 2, PageSize: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got.Total != 3 || len(got.List) != 1 || got.List[0].Name != "older-one" {
		t.Fatalf("unexpected page: total=%d list=%#v", got.Total, got.List)
	}
}

func TestSearchFromArtifactsCharClassDoesNotTruncateBeforeMatching(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	repo := model.Repository{Name: "fallback-char-class", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repository: %v", err)
	}

	now := time.Now()
	artifacts := make([]model.Artifact, 0, 50001)
	for i := 0; i < 50000; i++ {
		artifacts = append(artifacts, model.Artifact{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "hot", Version: fmt.Sprintf("1.0.%d", i), UpdatedAt: now})
	}
	artifacts = append(artifacts, model.Artifact{RepositoryID: repo.ID, Format: "npm", Kind: "version", Name: "old-but-valid", Version: "2.0.0", UpdatedAt: now.Add(-time.Hour)})
	if err := db.CreateInBatches(artifacts, 1000).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	svc := NewPackageSearchService(db)
	got, err := svc.Search(context.Background(), &SearchRequest{Type: "npm", Version: "[12].*", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got.Total != 2 || got.RawCount != 50001 {
		t.Fatalf("total=%d raw=%d, want total=2 raw=50001", got.Total, got.RawCount)
	}
}
