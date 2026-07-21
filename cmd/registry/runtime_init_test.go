package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMainDoesNotRebuildPackagesOnStartup(t *testing.T) {
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if strings.Contains(string(content), ".RebuildPackages(") {
		t.Fatalf("startup must not rebuild packages; keep RebuildPackages behind an explicit repair or migration path")
	}
}

func TestCreateRuntimeForProxyRepoAllowsNilConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo := &model.Repository{
		ID:          1,
		Name:        "npm-proxy",
		Type:        model.RepoTypeProxy,
		PackageType: "npm",
	}

	nilFetchers := map[string]runtime.RemoteFetcher(nil)
	var nilBlocker runtime.PackageBlocker
	repoRuntime, err := createRuntimeForRepo(repo, nil, nil, db, fakeStorageBackend{}, runtime.NewDefaultRepositoryManager(), nilFetchers, nilBlocker, nil, nil, nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	proxyRuntime, ok := repoRuntime.(*runtime.ProxyRuntime)
	if !ok {
		t.Fatalf("expected ProxyRuntime, got %T", repoRuntime)
	}
	if proxyRuntime.RemoteBaseURL != "" {
		t.Fatalf("expected empty remote base URL, got %q", proxyRuntime.RemoteBaseURL)
	}
	if proxyRuntime.CachePolicy.MetadataTTL != 24*time.Hour {
		t.Fatalf("MetadataTTL = %v, want 24h default", proxyRuntime.CachePolicy.MetadataTTL)
	}
	if proxyRuntime.CachePolicy.NegativeTTL != 5*time.Minute {
		t.Fatalf("NegativeTTL = %v, want 5m default", proxyRuntime.CachePolicy.NegativeTTL)
	}
}

func TestCreateRuntimeForLocalRepoInjectsBlocker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo := &model.Repository{
		ID:          1,
		Name:        "npm-hosted",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	blocker := &testRuntimeBlocker{}

	repoRuntime, err := createRuntimeForRepo(repo, nil, nil, db, fakeStorageBackend{}, runtime.NewDefaultRepositoryManager(), nil, blocker, nil, nil, nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	hostedRuntime, ok := repoRuntime.(*runtime.HostedRuntime)
	if !ok {
		t.Fatalf("expected HostedRuntime, got %T", repoRuntime)
	}
	if hostedRuntime.Blocker != blocker {
		t.Fatalf("blocker = %#v, want supplied blocker", hostedRuntime.Blocker)
	}
	if hostedRuntime.Format != repo.PackageType {
		t.Fatalf("format = %q, want %q", hostedRuntime.Format, repo.PackageType)
	}
	if hostedRuntime.ConditionAudit != blocker {
		t.Fatalf("condition audit = %#v, want supplied blocker", hostedRuntime.ConditionAudit)
	}
}

func TestCreateGroupRuntimeAllowsProxyMemberWithNilConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryMember{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	group := model.Repository{ID: 1, Name: "public", Type: model.RepoTypeVirtual, PackageType: "npm"}
	member := model.Repository{ID: 2, Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := db.Create(&model.RepositoryMember{RepositoryID: group.ID, MemberID: member.ID}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	nilFetchers := map[string]runtime.RemoteFetcher(nil)
	var nilBlocker runtime.PackageBlocker
	repoRuntime, err := createGroupRuntime(&group, repository.NewRepositoryRepository(db), nil, db, fakeStorageBackend{}, runtime.NewDefaultRepositoryManager(), nilFetchers, nilBlocker, nil, nil, nil)
	if err != nil {
		t.Fatalf("create group runtime: %v", err)
	}
	groupRuntime, ok := repoRuntime.(*runtime.GroupRuntime)
	if !ok {
		t.Fatalf("expected GroupRuntime, got %T", repoRuntime)
	}
	if len(groupRuntime.Members) != 1 {
		t.Fatalf("expected one member, got %d", len(groupRuntime.Members))
	}
	proxyMember, ok := groupRuntime.Members[0].(*runtime.ProxyRuntime)
	if !ok {
		t.Fatalf("expected proxy member runtime, got %T", groupRuntime.Members[0])
	}
	if proxyMember.CachePolicy.MetadataTTL != 24*time.Hour {
		t.Fatalf("group proxy MetadataTTL = %v, want 24h default", proxyMember.CachePolicy.MetadataTTL)
	}
	if proxyMember.CachePolicy.NegativeTTL != 5*time.Minute {
		t.Fatalf("group proxy NegativeTTL = %v, want 5m default", proxyMember.CachePolicy.NegativeTTL)
	}
}

func TestCreateGroupRuntimeInjectsBlockerIntoLocalMember(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryMember{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	group := model.Repository{ID: 1, Name: "public", Type: model.RepoTypeVirtual, PackageType: "npm"}
	member := model.Repository{ID: 2, Name: "npm-hosted", Type: model.RepoTypeLocal, PackageType: "npm"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := db.Create(&model.RepositoryMember{RepositoryID: group.ID, MemberID: member.ID}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	blocker := &testRuntimeBlocker{}

	repoRuntime, err := createGroupRuntime(&group, repository.NewRepositoryRepository(db), nil, db, fakeStorageBackend{}, runtime.NewDefaultRepositoryManager(), nil, blocker, nil, nil, nil)
	if err != nil {
		t.Fatalf("create group runtime: %v", err)
	}
	groupRuntime, ok := repoRuntime.(*runtime.GroupRuntime)
	if !ok || len(groupRuntime.Members) != 1 {
		t.Fatalf("group runtime = %#v, want one member", repoRuntime)
	}
	hostedMember, ok := groupRuntime.Members[0].(*runtime.HostedRuntime)
	if !ok {
		t.Fatalf("expected hosted member, got %T", groupRuntime.Members[0])
	}
	if hostedMember.Blocker != blocker {
		t.Fatalf("blocker = %#v, want supplied blocker", hostedMember.Blocker)
	}
	if hostedMember.Format != member.PackageType {
		t.Fatalf("format = %q, want %q", hostedMember.Format, member.PackageType)
	}
	if hostedMember.ConditionAudit != blocker {
		t.Fatalf("condition audit = %#v, want supplied blocker", hostedMember.ConditionAudit)
	}
}

type fakeStorageBackend struct{}

type testRuntimeBlocker struct{}

func (testRuntimeBlocker) IsBlocked(string, string, string) bool     { return false }
func (testRuntimeBlocker) BlockReason(string, string, string) string { return "" }
func (testRuntimeBlocker) IsBlockedWithAttrs(string, string, string, map[string]interface{}) (bool, string) {
	return false, ""
}
func (testRuntimeBlocker) IsBlockedByPath(string, string) bool     { return false }
func (testRuntimeBlocker) BlockReasonByPath(string, string) string { return "" }
func (*testRuntimeBlocker) LogConditionUnverified(context.Context, runtime.ConditionUnverifiedEntry) {
}

// TestBlockRuleBlocker_SplitsURLPathAndRuntimeChecks 验证 #14 的架构拆分：
// blockRuleBlocker 在 URL 层（IsBlockedByPath）只评估按路径匹配的通配符规则，
// exact 版本规则不在此层命中；exact 规则经 runtime 路径（IsBlocked，用真包名+版本）命中。
func TestBlockRuleBlocker_SplitsURLPathAndRuntimeChecks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pathRule := model.BlockRule{
		PackageName: "lodash/*",
		Version:     "*",
		MatchType:   model.BlockMatchWildcard,
		PackageType: "npm",
		Enabled:     true,
		Reason:      "path-wildcard",
	}
	if err := db.Create(&pathRule).Error; err != nil {
		t.Fatalf("create wildcard rule: %v", err)
	}
	exactRule := model.BlockRule{
		PackageName: "lodash",
		Version:     "4.17.21",
		MatchType:   model.BlockMatchExact,
		PackageType: "npm",
		Enabled:     true,
		Reason:      "exact-version",
	}
	if err := db.Create(&exactRule).Error; err != nil {
		t.Fatalf("create exact rule: %v", err)
	}

	blocker := &blockRuleBlocker{svc: newBlockRuleServiceForTest(db)}

	// URL 层：剩余路径命中 wildcard 路径规则 → 阻断
	if !blocker.IsBlockedByPath("npm", "lodash/-/lodash-4.17.21.tgz") {
		t.Fatalf("IsBlockedByPath: 期望 wildcard 路径规则命中")
	}
	if got := blocker.BlockReasonByPath("npm", "lodash/-/lodash-4.17.21.tgz"); got != "path-wildcard" {
		t.Fatalf("BlockReasonByPath = %q, want path-wildcard", got)
	}
	// URL 层：不匹配的路径放行
	if blocker.IsBlockedByPath("npm", "express/-/express-4.18.0.tgz") {
		t.Fatalf("IsBlockedByPath: 期望不匹配路径放行")
	}

	// runtime 层：exact 版本规则用真包名+版本命中
	if !blocker.IsBlocked("npm", "lodash", "4.17.21") {
		t.Fatalf("IsBlocked: 期望 exact 版本规则经 runtime 路径命中")
	}
	if got := blocker.BlockReason("npm", "lodash", "4.17.21"); got != "exact-version" {
		t.Fatalf("BlockReason = %q, want exact-version", got)
	}
}

func newBlockRuleServiceForTest(db *gorm.DB) *service.BlockRuleService {
	return service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
}

func (fakeStorageBackend) Name() string               { return "fake" }
func (fakeStorageBackend) Init(basePath string) error { return nil }
func (fakeStorageBackend) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	return nil
}
func (fakeStorageBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}
func (fakeStorageBackend) Delete(ctx context.Context, key string) error         { return nil }
func (fakeStorageBackend) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (fakeStorageBackend) Size(ctx context.Context, key string) (int64, error)  { return 0, nil }
func (fakeStorageBackend) List(ctx context.Context, prefix string) ([]storage.Entry, error) {
	return nil, nil
}
func (fakeStorageBackend) Browse(ctx context.Context, path string) ([]storage.BrowseEntry, error) {
	return nil, nil
}
func (fakeStorageBackend) Close() error     { return nil }
func (fakeStorageBackend) BasePath() string { return "" }
