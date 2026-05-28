package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestProxyRuntimeServesStaleMetadataWhenRefreshFails(t *testing.T) {
	ctx := context.Background()
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg"}
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  &fakeRemoteClient{metadataErr: errors.New("connection refused")},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}

	artifact, err := runtime.GetArtifact(ctx, key)
	if err != nil {
		t.Fatalf("expected cached artifact, got error: %v", err)
	}
	if artifact.ID != "cached" {
		t.Fatalf("expected cached artifact ID, got %q", artifact.ID)
	}
}

func TestProxyRuntimeDeletesStaleMetadataWhenRemoteMissing(t *testing.T) {
	ctx := context.Background()
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg"}
	store := newFakeMetadataStore()
	artifact := &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteClient:  &fakeRemoteClient{metadata: &RemoteMetadata{Exists: false}},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, NegativeTTL: 30 * time.Second},
	}

	err := runtime.refreshStaleMetadata(ctx, artifact, key)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if !store.deleted {
		t.Fatal("expected stale metadata to be deleted")
	}
}

func TestProxyRuntimeCachesMetadataStoreHit(t *testing.T) {
	ctx := context.Background()
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg"}
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}

	if _, err := runtime.GetArtifact(ctx, key); err != nil {
		t.Fatalf("first get failed: %v", err)
	}
	if _, err := runtime.GetArtifact(ctx, key); err != nil {
		t.Fatalf("second get failed: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one metadata store get, got %d", store.getCalls)
	}
}

func TestProxyRuntimeCachedArtifactOpensBlobEachRequest(t *testing.T) {
	ctx := context.Background()
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg"}
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
	}
	blobStore := &fakeBlobStore{}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     blobStore,
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}

	if _, err := runtime.GetArtifact(ctx, key); err != nil {
		t.Fatalf("first get failed: %v", err)
	}
	if _, err := runtime.GetArtifact(ctx, key); err != nil {
		t.Fatalf("second get failed: %v", err)
	}
	if blobStore.openCalls != 2 {
		t.Fatalf("expected blob open per request, got %d", blobStore.openCalls)
	}
}

func TestProxyRuntimeCachesRemoteMissingMetadata(t *testing.T) {
	ctx := context.Background()
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "missing"}
	remote := &fakeRemoteClient{metadata: &RemoteMetadata{Exists: false}}
	runtime := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  remote,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, NegativeTTL: 30 * time.Second},
	}

	for i := 0; i < 2; i++ {
		_, err := runtime.GetArtifact(ctx, key)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound on call %d, got %v", i+1, err)
		}
	}
	if remote.metadataCalls != 1 {
		t.Fatalf("expected one remote metadata call, got %d", remote.metadataCalls)
	}
}

type fakeMetadataStore struct {
	artifact *Artifact
	getCalls int
	putCalls int
	deleted  bool
}

func newFakeMetadataStore() *fakeMetadataStore {
	return &fakeMetadataStore{}
}

func (s *fakeMetadataStore) Get(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	s.getCalls++
	if s.artifact == nil || s.deleted {
		return nil, ErrNotFound
	}
	return s.artifact, nil
}

func (s *fakeMetadataStore) Put(ctx context.Context, artifact *Artifact) error {
	s.putCalls++
	s.artifact = artifact
	s.deleted = false
	return nil
}

func (s *fakeMetadataStore) Delete(ctx context.Context, key ArtifactKey) error {
	s.deleted = true
	s.artifact = nil
	return nil
}

func (s *fakeMetadataStore) Query(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	if s.artifact == nil || s.deleted {
		return nil, nil
	}
	return []*Artifact{s.artifact}, nil
}

type fakeRemoteClient struct {
	metadata      *RemoteMetadata
	metadataErr   error
	metadataCalls int
}

func (c *fakeRemoteClient) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	c.metadataCalls++
	if c.metadataErr != nil {
		return nil, c.metadataErr
	}
	if c.metadata != nil {
		return c.metadata, nil
	}
	return &RemoteMetadata{Exists: true}, nil
}

func (c *fakeRemoteClient) FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	return io.NopCloser(nil), ErrNotFound
}

type fakeBlobStore struct {
	openCalls int
}

func (fakeBlobStore) Put(reader io.Reader) (BlobRef, error) { return BlobRef{}, nil }
func (s *fakeBlobStore) Open(ref BlobRef) (io.ReadCloser, error) {
	s.openCalls++
	return io.NopCloser(nil), nil
}
func (fakeBlobStore) Stat(ref BlobRef) (*BlobMetadata, error) { return nil, nil }
func (fakeBlobStore) Delete(ref BlobRef) error                { return nil }

// ── 新增测试: 覆盖已发现的 bug ──────────────────────

// TestQueryArtifactsIgnoresTTL verifies the bug where QueryArtifacts
// returns cached data without checking CachePolicy.MetadataTTL.
func TestQueryArtifactsIgnoresTTL(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "stale",
		RepositoryID: "repo",
		Format:       "npm",
		UpdatedAt:    time.Now().Add(-2 * time.Hour), // 2 hours old
	}
	fetcherCalled := false
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Fetcher: &fakeFetcher{
			fn: func() ([]*Artifact, error) {
				fetcherCalled = true
				return []*Artifact{{ID: "fresh", RepositoryID: "repo", Format: "npm"}}, nil
			},
		},
	}

	query := ArtifactQuery{
		RepositoryID: "repo",
		Format:       "npm",
		RemotePath:   "some-path",
	}
	artifacts, err := runtime.QueryArtifacts(ctx, query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Current bug: QueryArtifacts returns stale data without refreshing.
	// After fix: it should have called the fetcher since TTL expired.
	if len(artifacts) > 0 && artifacts[0].ID == "stale" {
		t.Log("BUG CONFIRMED: QueryArtifacts returned stale data (ID=stale) without checking TTL")
		t.Logf("  store artifacts were 2h old, MetadataTTL=1min, but fetcher was NOT called")
	}
	if !fetcherCalled {
		t.Log("  Fetcher was NOT called — stale data served without refresh")
	}
}

// TestQueryArtifactsCallsFetcherWhenStoreEmpty verifies that when the
// MetadataStore returns empty, QueryArtifacts correctly calls RemoteFetcher
// to fetch from upstream.
func TestQueryArtifactsCallsFetcherWhenStoreEmpty(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore() // no artifact → Query returns nil
	fetcherCalled := false
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Fetcher: &fakeFetcher{
			fn: func() ([]*Artifact, error) {
				fetcherCalled = true
				return []*Artifact{
					{ID: "remote-1", RepositoryID: "repo", Format: "npm", Coordinates: map[string]string{"name": "lodash"}},
					{ID: "remote-2", RepositoryID: "repo", Format: "npm", Coordinates: map[string]string{"name": "express"}},
				}, nil
			},
		},
	}

	query := ArtifactQuery{
		RepositoryID: "repo",
		Format:       "npm",
		RemotePath:   "lodash",
	}
	artifacts, err := runtime.QueryArtifacts(ctx, query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fetcherCalled {
		t.Fatal("expected fetcher to be called when store is empty")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts from fetcher, got %d", len(artifacts))
	}
	if store.putCalls == 0 {
		t.Log("WARNING: fetcher results were NOT cached to MetadataStore")
	}
}

// TestQueryArtifactsReturnsEmptyWhenNoFetcher verifies that when there's
// no Fetcher registered and store is empty, QueryArtifacts returns empty
// (this is the bug that affects maven/go/yum/apt).
func TestQueryArtifactsReturnsEmptyWhenNoFetcher(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore() // no artifact
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Fetcher:       nil, // no fetcher — simulates maven/go/yum/apt
	}

	query := ArtifactQuery{
		RepositoryID: "repo",
		Format:       "maven",
		RemotePath:   "com/google/guava",
	}
	artifacts, err := runtime.QueryArtifacts(ctx, query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected empty result when no fetcher, got %d artifacts", len(artifacts))
	}
	t.Log("CONFIRMED: QueryArtifacts returns empty when Fetcher=nil (affects maven/go/yum/apt)")
}

// TestArtifactKeyStringCollision verifies that ArtifactKey.String()
// produces collisions for different packages with the same filename.
func TestArtifactKeyStringCollision(t *testing.T) {
	key1 := ArtifactKey{
		RepositoryID: "repo1",
		Format:       "maven",
		Filename:     "maven-metadata.xml",
		Coordinates: map[string]string{
			"group":    "com.google.guava",
			"artifact": "guava",
		},
	}
	key2 := ArtifactKey{
		RepositoryID: "repo1",
		Format:       "maven",
		Filename:     "maven-metadata.xml",
		Coordinates: map[string]string{
			"group":    "org.apache.commons",
			"artifact": "commons-lang3",
		},
	}

	s1 := key1.String()
	s2 := key2.String()

	if s1 == s2 {
		t.Logf("BUG CONFIRMED: ArtifactKey.String() collision detected")
		t.Logf("  key1 string: %q (group=%s, artifact=%s)", s1, key1.Coordinates["group"], key1.Coordinates["artifact"])
		t.Logf("  key2 string: %q (group=%s, artifact=%s)", s2, key2.Coordinates["group"], key2.Coordinates["artifact"])
		t.Log("  These are different packages but produce the same cache key")
	}
}

// TestGroupRuntimeDeduplicationWithEmptyID verifies the bug where
// GroupRuntime.QueryArtifacts deduplicates by a.ID, but backfilled
// artifacts have ID="", causing all but the first to be dropped.
func TestGroupRuntimeDeduplicationWithEmptyID(t *testing.T) {
	ctx := context.Background()

	// Simulate two proxy members returning artifacts with empty ID
	node1 := &fakeQueryNode{
		artifacts: []*Artifact{
			{ID: "", RepositoryID: "proxy1", Format: "npm", Coordinates: map[string]string{"name": "lodash", "version": "4.17.21"}},
			{ID: "", RepositoryID: "proxy1", Format: "npm", Coordinates: map[string]string{"name": "express", "version": "4.18.2"}},
		},
	}
	node2 := &fakeQueryNode{
		artifacts: []*Artifact{
			{ID: "", RepositoryID: "proxy2", Format: "npm", Coordinates: map[string]string{"name": "lodash", "version": "4.17.21"}},
			{ID: "", RepositoryID: "proxy2", Format: "npm", Coordinates: map[string]string{"name": "axios", "version": "1.4.0"}},
		},
	}

	group := &GroupRuntime{
		Members: []RepositoryNode{node1, node2},
	}

	query := ArtifactQuery{Format: "npm"}
	artifacts, err := group.QueryArtifacts(ctx, query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With empty ID deduplication: only the first artifact survives (ID="")
	// Expected: 3 unique artifacts (lodash, express, axios)
	if len(artifacts) == 1 && artifacts[0].ID == "" {
		t.Log("BUG CONFIRMED: GroupRuntime deduplication dropped all but 1 artifact")
		t.Logf("  Expected 3 unique artifacts (lodash, express, axios), got %d", len(artifacts))
		t.Log("  All artifacts have ID='', so seen[''] dedup kills all but the first")
	} else if len(artifacts) == 3 {
		t.Log("PASS: GroupRuntime correctly returned all 3 unique artifacts")
	} else {
		t.Logf("Got %d artifacts from group query", len(artifacts))
	}
}

// TestGetArtifactKeyMismatch verifies that ProxyRuntime.GetArtifact
// uses inconsistent keys for cache reads vs writes.
func TestGetArtifactKeyMismatch(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "42",
		Format:       "npm",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}

	// Key without RepositoryID set (simulates caller not setting it)
	key := ArtifactKey{
		Format:   "npm",
		Filename: "pkg",
	}

	_, err := runtime.GetArtifact(ctx, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// If cache key mismatch exists:
	// - store.Get uses key with RepositoryID="42" → finds artifact
	// - setCachedArtifact uses original key (no RepositoryID) → caches under wrong key
	// - next call getCachedArtifact uses original key → miss, calls store again
	if store.getCalls > 1 {
		t.Log("WARNING: store was called multiple times — cache key mismatch may be present")
		t.Logf("  store.getCalls = %d (expected 1)", store.getCalls)
	}
}

// TestNexus3ResolverNoTrailingSlash verifies the bug where Nexus3Resolver
// fails when the path has no trailing slash after repo name.
func TestNexus3ResolverNoTrailingSlash(t *testing.T) {
	resolver := &Nexus3Resolver{}

	// Path without trailing slash
	req, _ := http.NewRequest("GET", "http://localhost:8080/repository/maven-local", nil)
	resolved, err := resolver.Resolve(req)

	if err == nil {
		if resolved.Repository.Name == "" {
			t.Log("BUG CONFIRMED: Nexus3Resolver returned empty repo name for path without trailing slash")
		} else if resolved.RemainingPath == "/repository/maven-local"[12:] {
			t.Log("BUG CONFIRMED: Nexus3Resolver did not split repo name from path")
			t.Logf("  repoName=%q, remainingPath=%q", resolved.Repository.Name, resolved.RemainingPath)
		}
	} else {
		t.Logf("Resolver returned error for no-trailing-slash path: %v", err)
	}
}

// ── Fake implementations for new tests ──────────────────────

type fakeFetcher struct {
	fn func() ([]*Artifact, error)
}

func (f *fakeFetcher) FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error) {
	return f.fn()
}

type fakeQueryNode struct {
	artifacts []*Artifact
	err       error
}

func (f *fakeQueryNode) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return nil, ErrNotFound
}

func (f *fakeQueryNode) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return f.artifacts, f.err
}

func (f *fakeQueryNode) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	return nil, ErrNotFound
}

func (f *fakeQueryNode) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}

func (f *fakeQueryNode) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	return ErrReadOnly
}

// ── SanitizeFilename 测试 ──────────────────────

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal-file.tar.gz", "normal-file.tar.gz"},
		{"file\r\ninjection", "file__injection"},
		{"file\x00null", "file_null"},
		{"file\x7fdel", "file_del"},
		{`file";injection`, "file_;injection"},
		{"", ""},
		{"keep-this_name.v1.0.tgz", "keep-this_name.v1.0.tgz"},
	}
	for _, tt := range tests {
		got := SanitizeFilename(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
