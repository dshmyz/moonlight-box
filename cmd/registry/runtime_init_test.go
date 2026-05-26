package main

import (
	"context"
	"io"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
	repoRuntime, err := createRuntimeForRepo(repo, nil, nil, db, fakeStorageBackend{}, runtime.NewDefaultRepositoryManager(), nilFetchers, nilBlocker)
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
	repoRuntime, err := createGroupRuntime(&group, repository.NewRepositoryRepository(db), nil, db, fakeStorageBackend{}, runtime.NewDefaultRepositoryManager(), nilFetchers, nilBlocker)
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
}

type fakeStorageBackend struct{}

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
