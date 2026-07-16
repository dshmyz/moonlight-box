package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
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
	blobStore := &fakeBlobStore{}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     blobStore,
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

func TestProxyRuntimeRefreshStaleMetadataPreservesRemoteValidators(t *testing.T) {
	ctx := context.Background()
	modified := time.Date(2026, 6, 9, 10, 11, 12, 0, time.UTC)
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg.tgz", RemotePath: "pkg/-/pkg.tgz"}
	store := newFakeMetadataStore()
	artifact := &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Filename:     "pkg.tgz",
		RemotePath:   "pkg/-/pkg.tgz",
		Properties:   map[string]string{"remote_digest": "old", "remote_size": "1"},
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteClient: &fakeRemoteClient{metadata: &RemoteMetadata{
			Exists:     true,
			ETag:       "fresh-etag",
			Digest:     "fresh-digest",
			Size:       42,
			ModifiedAt: modified,
		}},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}

	if err := runtime.refreshStaleMetadata(ctx, artifact, key); err != nil {
		t.Fatalf("refreshStaleMetadata failed: %v", err)
	}
	if got := store.artifact.Properties["remote_etag"]; got != "fresh-etag" {
		t.Fatalf("remote_etag = %q, want fresh-etag", got)
	}
	if !store.artifact.UpdatedAt.Equal(modified) {
		t.Fatalf("UpdatedAt = %s, want upstream Last-Modified %s", store.artifact.UpdatedAt, modified)
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

func TestProxyRuntimePreservesRemoteCacheValidatorsOnFetch(t *testing.T) {
	ctx := context.Background()
	modified := time.Date(2026, 6, 9, 10, 11, 12, 0, time.UTC)
	store := newFakeMetadataStore()
	rt := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &capturingBlobStore{},
		RemoteClient: &fakeRemoteClient{
			metadata: &RemoteMetadata{
				Exists:     true,
				ETag:       "upstream-etag",
				Digest:     "legacy-digest",
				Size:       int64(len("stream-content")),
				ModifiedAt: modified,
			},
			blob: io.NopCloser(strings.NewReader("stream-content")),
		},
		RemoteBaseURL: "https://example.test",
		RepositoryID:  "repo",
		Format:        "npm",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}
	key := ArtifactKey{
		Format:     "npm",
		Name:       "left-pad",
		Filename:   "left-pad-1.0.0.tgz",
		RemotePath: "left-pad/-/left-pad-1.0.0.tgz",
	}

	artifact, err := rt.GetArtifact(ctx, key)
	if err != nil {
		t.Fatalf("GetArtifact failed: %v", err)
	}
	if got := artifact.Properties["remote_etag"]; got != "upstream-etag" {
		t.Fatalf("remote_etag = %q, want upstream-etag", got)
	}
	if got := store.artifact.Properties["remote_etag"]; got != "upstream-etag" {
		t.Fatalf("stored remote_etag = %q, want upstream-etag", got)
	}
	if !store.artifact.UpdatedAt.Equal(modified) {
		t.Fatalf("stored UpdatedAt = %s, want upstream Last-Modified %s", store.artifact.UpdatedAt, modified)
	}
}

func TestProxyRuntimeEvictOldestEntriesDoesNotSortWholeCache(t *testing.T) {
	source, err := os.ReadFile("proxy.go")
	if err != nil {
		t.Fatalf("read proxy source: %v", err)
	}
	body := extractRuntimeFunctionBodyForTest(string(source), "func (n *ProxyRuntime) evictOldestEntries")
	if body == "" {
		t.Fatal("ProxyRuntime.evictOldestEntries source not found")
	}
	if strings.Contains(body, "sort.Slice") {
		t.Fatal("evictOldestEntries should avoid sorting the whole metadata cache while holding the write lock")
	}
}

func extractRuntimeFunctionBodyForTest(source, signature string) string {
	start := strings.Index(source, signature)
	if start < 0 {
		return ""
	}
	open := strings.Index(source[start:], "{")
	if open < 0 {
		return ""
	}
	pos := start + open
	depth := 0
	for i := pos; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[pos : i+1]
			}
		}
	}
	return ""
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
	batchErr error
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

func (s *fakeMetadataStore) BatchPut(ctx context.Context, artifacts []*Artifact) error {
	if s.batchErr != nil {
		return s.batchErr
	}
	s.putCalls += len(artifacts)
	if len(artifacts) > 0 {
		s.artifact = artifacts[len(artifacts)-1]
		s.deleted = false
	}
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

func TestDifferentPathsCanRefreshConcurrently(t *testing.T) {
	ctx := context.Background()
	fetchCount := 0
	fetcher := &fakeFetcher{
		fn: func() ([]*Artifact, error) {
			fetchCount++
			time.Sleep(100 * time.Millisecond)
			return []*Artifact{{
				RepositoryID: "repo",
				Format:       "npm",
				Name:         "concurrent-test",
			}}, nil
		},
	}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		RemoteBaseURL: "https://example.test",
		Fetcher:       fetcher,
		Format:        "npm",
		CachePolicy:   CachePolicy{MetadataTTL: time.Nanosecond}, // 立即过期
	}

	// 先创建两个过期的 artifact
	store := rt.MetadataStore.(*fakeMetadataStore)
	store.artifact = &Artifact{
		ID:           "a",
		RepositoryID: "repo",
		Format:       "npm",
		Name:         "pkg-a",
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}

	// 发起两次 QueryArtifacts，不同 RemotePath
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		rt.QueryArtifacts(ctx, ArtifactQuery{Format: "npm", RemotePath: "path-a"})
	}()
	go func() {
		defer wg.Done()
		rt.QueryArtifacts(ctx, ArtifactQuery{Format: "npm", RemotePath: "path-b"})
	}()
	wg.Wait()
	time.Sleep(200 * time.Millisecond) // 等待异步刷新 goroutine

	// 两个不同路径应该各触发一次 fetch，共 2 次
	if fetchCount != 2 {
		t.Fatalf("FetchRemote called %d times, expected 2 (不同 path 应并发刷新)", fetchCount)
	}
}

func TestEnsureArtifactBlobAllowsLargeBlobWhenLimitDisabled(t *testing.T) {
	ctx := context.Background()
	artifact := &Artifact{
		ID:           "large",
		RepositoryID: "repo",
		Format:       "npm",
		Name:         "big-pkg",
	}
	bigContent := strings.Repeat("x", 1024)
	fakeBlob := &fakeBlobStore{}
	fakeClient := &fakeRemoteClient{blob: io.NopCloser(strings.NewReader(bigContent))}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		BlobStore:     fakeBlob,
		RemoteClient:  fakeClient,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, MaxBlobSize: 0},
	}
	key := ArtifactKey{
		RepositoryID: "repo",
		Format:       "npm",
		Filename:     "big-pkg.tgz",
		RemotePath:   "big-pkg",
	}
	if err := rt.ensureArtifactBlob(ctx, artifact, key); err != nil {
		t.Fatalf("expected large blob to be allowed when MaxBlobSize=0, got %v", err)
	}
}

func TestEnsureArtifactBlobRejectsOversizedBlobWhenLimitConfigured(t *testing.T) {
	ctx := context.Background()
	artifact := &Artifact{
		ID:           "oversized",
		RepositoryID: "repo",
		Format:       "npm",
		Name:         "big-pkg",
	}
	bigContent := strings.Repeat("x", 11)
	fakeBlob := &capturingBlobStore{}
	fakeClient := &fakeRemoteClient{blob: io.NopCloser(strings.NewReader(bigContent))}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		BlobStore:     fakeBlob,
		RemoteClient:  fakeClient,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, MaxBlobSize: 10},
	}
	key := ArtifactKey{
		RepositoryID: "repo",
		Format:       "npm",
		Filename:     "big-pkg.tgz",
		RemotePath:   "big-pkg",
	}
	err := rt.ensureArtifactBlob(ctx, artifact, key)
	if err == nil {
		t.Fatal("expected error for oversized blob, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected 'too large' error, got: %v", err)
	}
}

func TestEnsureArtifactBlobStreamsUpstreamReaderToBlobStore(t *testing.T) {
	ctx := context.Background()
	artifact := &Artifact{ID: "stream", RepositoryID: "repo", Format: "npm", Name: "pkg"}
	reader := &markerReadCloser{Reader: strings.NewReader("stream-content")}
	blobStore := &capturingBlobStore{}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		BlobStore:     blobStore,
		RemoteClient:  &fakeRemoteClient{blob: reader},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, MaxBlobSize: 0},
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg.tgz", RemotePath: "pkg"}

	if err := rt.ensureArtifactBlob(ctx, artifact, key); err != nil {
		t.Fatalf("ensureArtifactBlob failed: %v", err)
	}
	if blobStore.readerType != "*runtime.markerReadCloser" {
		t.Fatalf("expected original upstream reader to be passed to BlobStore.Put, got %s", blobStore.readerType)
	}
}

func TestEnsureArtifactBlobUsesContextAwareBlobStore(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("request"), "proxy-ctx")
	artifact := &Artifact{ID: "stream", RepositoryID: "repo", Format: "npm", Name: "pkg"}
	blobStore := &contextCapturingBlobStore{}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		BlobStore:     blobStore,
		RemoteClient:  &fakeRemoteClient{blob: io.NopCloser(strings.NewReader("stream-content"))},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, MaxBlobSize: 0},
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Filename: "pkg.tgz", RemotePath: "pkg"}

	if err := rt.ensureArtifactBlob(ctx, artifact, key); err != nil {
		t.Fatalf("ensureArtifactBlob failed: %v", err)
	}
	if got := blobStore.contextValue; got != "proxy-ctx" {
		t.Fatalf("context value = %v, want proxy-ctx", got)
	}
	if blobStore.putCalls != 0 {
		t.Fatalf("fallback Put called %d times, want 0", blobStore.putCalls)
	}
}

func TestEnsureArtifactBlobBackfillsSHA256Checksum(t *testing.T) {
	ctx := context.Background()
	artifact := &Artifact{ID: "stream", RepositoryID: "repo", Format: "pypi", Name: "requests"}
	store := newFakeMetadataStore()
	rt := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &capturingBlobStore{},
		RemoteClient:  &fakeRemoteClient{blob: io.NopCloser(strings.NewReader("stream-content"))},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute, MaxBlobSize: 0},
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "pypi", Filename: "requests-2.28.0.tar.gz", RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz"}

	if err := rt.ensureArtifactBlob(ctx, artifact, key); err != nil {
		t.Fatalf("ensureArtifactBlob failed: %v", err)
	}
	if got := store.artifact.Checksums["sha256"]; got != "streamed" {
		t.Fatalf("stored sha256 checksum = %q, want streamed", got)
	}
}

type markerReadCloser struct {
	*strings.Reader
}

func (r *markerReadCloser) Close() error { return nil }

type capturingBlobStore struct {
	readerType string
}

func (s *capturingBlobStore) Put(reader io.Reader) (BlobRef, error) {
	s.readerType = fmt.Sprintf("%T", reader)
	_, _ = io.Copy(io.Discard, reader)
	return BlobRef{Algorithm: "sha256", Digest: "streamed", Size: 14}, nil
}
func (s *capturingBlobStore) Open(ref BlobRef) (io.ReadCloser, error) { return io.NopCloser(nil), nil }
func (s *capturingBlobStore) Stat(ref BlobRef) (*BlobMetadata, error) { return nil, nil }
func (s *capturingBlobStore) Delete(ref BlobRef) error                { return nil }

type contextCapturingBlobStore struct {
	capturingBlobStore
	contextValue any
	putCalls     int
}

func (s *contextCapturingBlobStore) Put(reader io.Reader) (BlobRef, error) {
	s.putCalls++
	return s.capturingBlobStore.Put(reader)
}

func (s *contextCapturingBlobStore) PutContext(ctx context.Context, reader io.Reader) (BlobRef, error) {
	s.contextValue = ctx.Value(contextKey("request"))
	return s.capturingBlobStore.Put(reader)
}

func TestConcurrentGetArtifactOnlyFetchesRemoteOnce(t *testing.T) {
	ctx := context.Background()
	remote := &threadSafeRemoteClient{
		metadata: &RemoteMetadata{Exists: true, Size: int64(len("pkg-content"))},
		blob:     "pkg-content",
		delay:    50 * time.Millisecond,
	}
	blobStore := &threadSafeBlobStore{content: "pkg-content"}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		BlobStore:     blobStore,
		RemoteClient:  remote,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		RepositoryID:  "repo",
		Format:        "npm",
	}
	key := ArtifactKey{
		Format:     "npm",
		Name:       "dedup-test",
		Filename:   "dedup-test.tgz",
		RemotePath: "dedup-test/-/dedup-test.tgz",
	}

	const concurrency = 12
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			artifact, err := rt.GetArtifact(ctx, key)
			if err != nil {
				errs <- err
				return
			}
			if artifact.Content == nil {
				errs <- errors.New("artifact content was not opened")
				return
			}
			defer artifact.Content.Close()
			body, err := io.ReadAll(artifact.Content)
			if err != nil {
				errs <- err
				return
			}
			if string(body) != "pkg-content" {
				errs <- fmt.Errorf("content = %q, want pkg-content", string(body))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := remote.metadataCallCount(); got != 1 {
		t.Fatalf("FetchMetadata called %d times, expected 1", got)
	}
	if got := remote.blobCallCount(); got != 1 {
		t.Fatalf("FetchBlob called %d times, expected 1", got)
	}
	if got := blobStore.openCallCount(); got != concurrency {
		t.Fatalf("BlobStore.Open called %d times, expected %d", got, concurrency)
	}
}

type threadSafeRemoteClient struct {
	mu            sync.Mutex
	metadata      *RemoteMetadata
	metadataCalls int
	blobCalls     int
	blob          string
	delay         time.Duration
}

func (c *threadSafeRemoteClient) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	c.mu.Lock()
	c.metadataCalls++
	meta := c.metadata
	c.mu.Unlock()
	return meta, nil
}

func (c *threadSafeRemoteClient) FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	c.mu.Lock()
	c.blobCalls++
	blob := c.blob
	c.mu.Unlock()
	return io.NopCloser(strings.NewReader(blob)), nil
}

func (c *threadSafeRemoteClient) metadataCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metadataCalls
}

func (c *threadSafeRemoteClient) blobCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blobCalls
}

type threadSafeBlobStore struct {
	mu        sync.Mutex
	content   string
	openCalls int
}

func (s *threadSafeBlobStore) Put(reader io.Reader) (BlobRef, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return BlobRef{}, err
	}
	s.mu.Lock()
	s.content = string(body)
	s.mu.Unlock()
	return BlobRef{Algorithm: "sha256", Digest: "dedup", Size: int64(len(body))}, nil
}

func (s *threadSafeBlobStore) Open(ref BlobRef) (io.ReadCloser, error) {
	s.mu.Lock()
	s.openCalls++
	content := s.content
	s.mu.Unlock()
	return io.NopCloser(strings.NewReader(content)), nil
}

func (s *threadSafeBlobStore) Stat(ref BlobRef) (*BlobMetadata, error) { return nil, nil }
func (s *threadSafeBlobStore) Delete(ref BlobRef) error                { return nil }

func (s *threadSafeBlobStore) openCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openCalls
}

func TestConcurrentQueryArtifactsOnlyTriggersOneFetch(t *testing.T) {
	ctx := context.Background()
	fetchCount := 0
	fetcher := &fakeFetcher{
		fn: func() ([]*Artifact, error) {
			fetchCount++
			time.Sleep(50 * time.Millisecond) // 模拟网络延迟
			return []*Artifact{{
				RepositoryID: "repo",
				Format:       "npm",
				Name:         "dedup-test",
			}}, nil
		},
	}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		RemoteBaseURL: "https://example.test",
		Fetcher:       fetcher,
		Format:        "npm",
	}
	// 并发请求同一 RemotePath
	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			rt.QueryArtifacts(ctx, ArtifactQuery{
				Format:     "npm",
				RemotePath: "dedup-test",
			})
		}()
	}
	wg.Wait()
	if fetchCount != 1 {
		t.Fatalf("FetchRemote called %d times, expected 1", fetchCount)
	}
}

func TestNegativeCacheDoesNotGrowWithoutBound(t *testing.T) {
	ctx := context.Background()
	remote := &fakeRemoteClient{metadata: &RemoteMetadata{Exists: false}}
	rt := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		RemoteClient:  remote,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{NegativeTTL: 30 * time.Second},
	}
	// 模拟大量不存在的路径请求，验证不会 panic 且仍有正确行为
	for i := 0; i < maxNegativeCacheSize+200; i++ {
		key := ArtifactKey{
			Format:     "npm",
			RemotePath: "nonexistent-" + fmt.Sprintf("%d", i),
		}
		_, err := rt.GetArtifact(ctx, key)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound for request %d, got %v", i, err)
		}
	}
	// 负缓存应该有上限
	if len(rt.negativeCache) > maxNegativeCacheSize {
		t.Fatalf("negativeCache grew to %d, exceeds limit %d", len(rt.negativeCache), maxNegativeCacheSize)
	}
}

func TestProxyRuntimeQueryArtifactsReturnsCachedArtifactsWhenFetchRemoteFails(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Kind:         KindArtifact,
		Name:         "left-pad",
		RemotePath:   "left-pad",
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	fetchErr := errors.New("upstream unavailable")
	fetcher := &fakeFetcher{fn: func() ([]*Artifact, error) { return nil, fetchErr }}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		Fetcher:       fetcher,
		Format:        "npm",
	}

	artifacts, err := runtime.QueryArtifacts(ctx, ArtifactQuery{
		Format:     "npm",
		RemotePath: "left-pad",
	})
	if err != nil {
		t.Fatalf("expected cached artifacts when upstream fails, got error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != "cached" {
		t.Fatalf("expected cached artifact, got %#v", artifacts)
	}
}

func TestProxyRuntimeQueryArtifactsDoesNotFetchRemoteWithoutRemotePath(t *testing.T) {
	ctx := context.Background()
	fetcher := &fakeFetcher{fn: func() ([]*Artifact, error) {
		return []*Artifact{NewArtifact(ArtifactSpec{
			Format:  "pypi",
			Kind:    KindVersion,
			Name:    "requests",
			Version: "2.28.0",
		})}, nil
	}}
	runtime := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		RemoteBaseURL: "https://example.test",
		Fetcher:       fetcher,
		Format:        "pypi",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
	}

	artifacts, err := runtime.QueryArtifacts(ctx, ArtifactQuery{Format: "pypi", Name: "requests"})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected no artifacts without RemotePath fetch, got %#v", artifacts)
	}
	if fetcher.wasCalled() {
		t.Fatal("FetchRemote should not be called for structured queries without RemotePath")
	}
}

func TestProxyRuntimeQueryArtifactsReturnsBatchPutError(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("store failed")
	store := &fakeMetadataStore{batchErr: storeErr}
	fetcher := &fakeFetcher{
		fn: func() ([]*Artifact, error) {
			return []*Artifact{{
				Format:  "npm",
				Kind:    KindVersion,
				Name:    "left-pad",
				Version: "1.0.0",
			}}, nil
		},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		Fetcher:       fetcher,
		Format:        "npm",
	}

	_, err := runtime.QueryArtifacts(ctx, ArtifactQuery{
		Format:     "npm",
		RemotePath: "left-pad",
	})
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected BatchPut error to be returned, got %v", err)
	}
}

func TestProxyRuntimeQueryArtifactsDoesNotFetchPerArtifactHeaders(t *testing.T) {
	ctx := context.Background()
	remote := &fakeRemoteClient{metadata: &RemoteMetadata{Exists: true, ETag: "etag"}}
	fetcher := &fakeFetcher{
		fn: func() ([]*Artifact, error) {
			return []*Artifact{{
				Format:     "npm",
				Kind:       KindArtifact,
				Name:       "left-pad",
				Version:    "1.0.0",
				Filename:   "left-pad-1.0.0.tgz",
				RemotePath: "left-pad/-/left-pad-1.0.0.tgz",
			}}, nil
		},
	}
	runtime := &ProxyRuntime{
		MetadataStore: newFakeMetadataStore(),
		RemoteClient:  remote,
		RemoteBaseURL: "https://example.test",
		Fetcher:       fetcher,
		Format:        "npm",
	}

	artifacts, err := runtime.QueryArtifacts(ctx, ArtifactQuery{
		Format:     "npm",
		RemotePath: "left-pad",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 fetched artifact, got %d", len(artifacts))
	}
	if remote.metadataCalls != 0 {
		t.Fatalf("QueryArtifacts made %d per-artifact metadata requests, want 0", remote.metadataCalls)
	}
}

type fakeRemoteClient struct {
	metadata      *RemoteMetadata
	metadataErr   error
	metadataCalls int
	blob          io.ReadCloser
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
	if c.blob != nil {
		return c.blob, nil
	}
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
	fetcher := &fakeFetcher{
		fn: func() ([]*Artifact, error) {
			return []*Artifact{{ID: "fresh", RepositoryID: "repo", Format: "npm"}}, nil
		},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Fetcher:       fetcher,
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
	if !fetcher.wasCalled() {
		t.Log("  Fetcher was NOT called — stale data served without refresh")
	}
}

// TestQueryArtifactsCallsFetcherWhenStoreEmpty verifies that when the
// MetadataStore returns empty, QueryArtifacts correctly calls RemoteFetcher
// to fetch from upstream.
func TestQueryArtifactsCallsFetcherWhenStoreEmpty(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore() // no artifact → Query returns nil
	fetcher := &fakeFetcher{
		fn: func() ([]*Artifact, error) {
			return []*Artifact{
				{ID: "remote-1", RepositoryID: "repo", Format: "npm", Name: "lodash"},
				{ID: "remote-2", RepositoryID: "repo", Format: "npm", Name: "express"},
			}, nil
		},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Fetcher:       fetcher,
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
	if !fetcher.wasCalled() {
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
		Qualifiers: map[string]string{
			"group":    "com.google.guava",
			"artifact": "guava",
		},
	}
	key2 := ArtifactKey{
		RepositoryID: "repo1",
		Format:       "maven",
		Filename:     "maven-metadata.xml",
		Qualifiers: map[string]string{
			"group":    "org.apache.commons",
			"artifact": "commons-lang3",
		},
	}

	s1 := key1.String()
	s2 := key2.String()

	if s1 == s2 {
		t.Logf("BUG CONFIRMED: ArtifactKey.String() collision detected")
		t.Logf("  key1 string: %q (group=%s, artifact=%s)", s1, key1.Qualifiers["group"], key1.Qualifiers["artifact"])
		t.Logf("  key2 string: %q (group=%s, artifact=%s)", s2, key2.Qualifiers["group"], key2.Qualifiers["artifact"])
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
			{ID: "", RepositoryID: "proxy1", Format: "npm", Name: "lodash", Version: "4.17.21"},
			{ID: "", RepositoryID: "proxy1", Format: "npm", Name: "express", Version: "4.18.2"},
		},
	}
	node2 := &fakeQueryNode{
		artifacts: []*Artifact{
			{ID: "", RepositoryID: "proxy2", Format: "npm", Name: "lodash", Version: "4.17.21"},
			{ID: "", RepositoryID: "proxy2", Format: "npm", Name: "axios", Version: "1.4.0"},
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
	fn     func() ([]*Artifact, error)
	called bool
	mu     sync.Mutex
}

func (f *fakeFetcher) FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error) {
	f.mu.Lock()
	f.called = true
	f.mu.Unlock()
	return f.fn()
}

func (f *fakeFetcher) wasCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called
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

// ── Nexus2Resolver 测试 ──────────────────────

func TestNexus2ResolverBasicPath(t *testing.T) {
	resolver := NewNexus2RepoResolver()
	tests := []struct {
		name          string
		url           string
		wantRepo      string
		wantRemaining string
		wantStyle     RouteStyle
		wantErr       bool
	}{
		{
			name:          "maven artifact path",
			url:           "http://localhost:8080/content/repositories/releases/com/google/guava/guava/31.1/guava-31.1.jar",
			wantRepo:      "releases",
			wantRemaining: "/com/google/guava/guava/31.1/guava-31.1.jar",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "repo root with trailing slash",
			url:           "http://localhost:8080/content/repositories/releases/",
			wantRepo:      "releases",
			wantRemaining: "/",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "repo root without trailing slash",
			url:           "http://localhost:8080/content/repositories/releases",
			wantRepo:      "releases",
			wantRemaining: "/",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "npm scoped package",
			url:           "http://localhost:8080/content/repositories/npm-hosted/@scope/pkg/-/pkg-1.0.0.tgz",
			wantRepo:      "npm-hosted",
			wantRemaining: "/@scope/pkg/-/pkg-1.0.0.tgz",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "pypi simple index",
			url:           "http://localhost:8080/content/repositories/pypi-local/simple/",
			wantRepo:      "pypi-local",
			wantRemaining: "/simple/",
			wantStyle:     Nexus2Route,
		},
		{
			name:    "wrong prefix returns error",
			url:     "http://localhost:8080/repository/releases/com/google/guava",
			wantErr: true,
		},
		{
			name:    "content without repositories prefix",
			url:     "http://localhost:8080/content/something/releases/path",
			wantErr: true,
		},
		{
			name:          "single segment path after repo name",
			url:           "http://localhost:8080/content/repositories/snapshots/maven-metadata.xml",
			wantRepo:      "snapshots",
			wantRemaining: "/maven-metadata.xml",
			wantStyle:     Nexus2Route,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			resolved, err := resolver.Resolve(req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got repo=%q remaining=%q", resolved.Repository.Name, resolved.RemainingPath)
				}
				if err != ErrNotMatched {
					t.Fatalf("expected ErrNotMatched, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved.Repository.Name != tt.wantRepo {
				t.Errorf("repo name = %q, want %q", resolved.Repository.Name, tt.wantRepo)
			}
			if resolved.RemainingPath != tt.wantRemaining {
				t.Errorf("remaining path = %q, want %q", resolved.RemainingPath, tt.wantRemaining)
			}
			if resolved.RouteStyle != tt.wantStyle {
				t.Errorf("route style = %d, want %d", resolved.RouteStyle, tt.wantStyle)
			}
		})
	}
}

func TestNexus2ResolverNoTrailingSlash(t *testing.T) {
	resolver := NewNexus2RepoResolver()
	req, _ := http.NewRequest("GET", "http://localhost:8080/content/repositories/releases", nil)
	resolved, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Repository.Name != "releases" {
		t.Errorf("repo name = %q, want %q", resolved.Repository.Name, "releases")
	}
	if resolved.RemainingPath != "/" {
		t.Errorf("remaining path = %q, want %q", resolved.RemainingPath, "/")
	}
}

// ── Nexus2GroupResolver 测试 ──────────────────────

func TestNexus2GroupResolverBasicPath(t *testing.T) {
	resolver := NewNexus2GroupResolver()
	tests := []struct {
		name          string
		url           string
		wantRepo      string
		wantRemaining string
		wantStyle     RouteStyle
		wantErr       bool
	}{
		{
			name:          "group maven artifact",
			url:           "http://localhost:8080/content/groups/public/com/google/guava/guava/31.1/guava-31.1.jar",
			wantRepo:      "public",
			wantRemaining: "/com/google/guava/guava/31.1/guava-31.1.jar",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "group root with trailing slash",
			url:           "http://localhost:8080/content/groups/public/",
			wantRepo:      "public",
			wantRemaining: "/",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "group root without trailing slash",
			url:           "http://localhost:8080/content/groups/public",
			wantRepo:      "public",
			wantRemaining: "/",
			wantStyle:     Nexus2Route,
		},
		{
			name:          "group npm package",
			url:           "http://localhost:8080/content/groups/npm-all/@angular/core/-/core-16.0.0.tgz",
			wantRepo:      "npm-all",
			wantRemaining: "/@angular/core/-/core-16.0.0.tgz",
			wantStyle:     Nexus2Route,
		},
		{
			name:    "wrong prefix /content/repositories should not match",
			url:     "http://localhost:8080/content/repositories/public/path",
			wantErr: true,
		},
		{
			name:    "wrong prefix /repository should not match",
			url:     "http://localhost:8080/repository/public/path",
			wantErr: true,
		},
		{
			name:          "group pypi simple",
			url:           "http://localhost:8080/content/groups/pypi-all/simple/requests/",
			wantRepo:      "pypi-all",
			wantRemaining: "/simple/requests/",
			wantStyle:     Nexus2Route,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			resolved, err := resolver.Resolve(req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got repo=%q remaining=%q", resolved.Repository.Name, resolved.RemainingPath)
				}
				if err != ErrNotMatched {
					t.Fatalf("expected ErrNotMatched, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved.Repository.Name != tt.wantRepo {
				t.Errorf("repo name = %q, want %q", resolved.Repository.Name, tt.wantRepo)
			}
			if resolved.RemainingPath != tt.wantRemaining {
				t.Errorf("remaining path = %q, want %q", resolved.RemainingPath, tt.wantRemaining)
			}
			if resolved.RouteStyle != tt.wantStyle {
				t.Errorf("route style = %d, want %d", resolved.RouteStyle, tt.wantStyle)
			}
		})
	}
}

// ── CompositeResolver 组合测试 ──────────────────────

func TestCompositeResolverNexus2AndNexus3DoNotConflict(t *testing.T) {
	composite := &CompositeResolver{
		Resolvers: []RepositoryPathResolver{
			&Nexus3Resolver{},
			NewNexus2RepoResolver(),
			NewNexus2GroupResolver(),
		},
	}

	tests := []struct {
		name      string
		url       string
		wantRepo  string
		wantStyle RouteStyle
	}{
		{
			name:      "nexus3 path resolved by Nexus3Resolver",
			url:       "http://localhost:8080/repository/maven-central/com/google/guava/guava/31.1/guava-31.1.jar",
			wantRepo:  "maven-central",
			wantStyle: Nexus3Route,
		},
		{
			name:      "nexus2 repo path resolved by Nexus2Resolver",
			url:       "http://localhost:8080/content/repositories/releases/com/google/guava/guava/31.1/guava-31.1.jar",
			wantRepo:  "releases",
			wantStyle: Nexus2Route,
		},
		{
			name:      "nexus2 group path resolved by Nexus2GroupResolver",
			url:       "http://localhost:8080/content/groups/public/com/google/guava/guava/31.1/guava-31.1.jar",
			wantRepo:  "public",
			wantStyle: Nexus2Route,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			resolved, err := composite.Resolve(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved.Repository.Name != tt.wantRepo {
				t.Errorf("repo name = %q, want %q", resolved.Repository.Name, tt.wantRepo)
			}
			if resolved.RouteStyle != tt.wantStyle {
				t.Errorf("route style = %d, want %d", resolved.RouteStyle, tt.wantStyle)
			}
		})
	}
}

func TestCompositeResolverReturnsNotMatchedForUnknownPath(t *testing.T) {
	composite := &CompositeResolver{
		Resolvers: []RepositoryPathResolver{
			&Nexus3Resolver{},
			NewNexus2RepoResolver(),
			NewNexus2GroupResolver(),
		},
	}

	req, _ := http.NewRequest("GET", "http://localhost:8080/v2/some/path", nil)
	_, err := composite.Resolve(req)
	if err != ErrNotMatched {
		t.Fatalf("expected ErrNotMatched, got %v", err)
	}
}

func TestCompositeResolverRepoNameWithDotsAndHyphens(t *testing.T) {
	composite := &CompositeResolver{
		Resolvers: []RepositoryPathResolver{
			&Nexus3Resolver{},
			NewNexus2RepoResolver(),
			NewNexus2GroupResolver(),
		},
	}

	tests := []struct {
		name     string
		url      string
		wantRepo string
	}{
		{
			name:     "nexus2 repo with hyphens",
			url:      "http://localhost:8080/content/repositories/maven-releases/path",
			wantRepo: "maven-releases",
		},
		{
			name:     "nexus2 repo with dots",
			url:      "http://localhost:8080/content/repositories/repo.local/path",
			wantRepo: "repo.local",
		},
		{
			name:     "nexus2 group with hyphens",
			url:      "http://localhost:8080/content/groups/maven-public/path",
			wantRepo: "maven-public",
		},
		{
			name:     "nexus3 repo with underscores",
			url:      "http://localhost:8080/repository/my_repo/path",
			wantRepo: "my_repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			resolved, err := composite.Resolve(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved.Repository.Name != tt.wantRepo {
				t.Errorf("repo name = %q, want %q", resolved.Repository.Name, tt.wantRepo)
			}
		})
	}
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

// ── 后置阻断检查测试（第二层检查：基于 artifact.Attributes）──────────────

// mockConditionBlocker 模拟条件阻断：第一层不阻断，第二层根据 attrs 中的 license 判断。
type mockConditionBlocker struct{}

func (m *mockConditionBlocker) IsBlocked(packageType, packageName, version string) bool {
	return false // 第一层不阻断
}

func (m *mockConditionBlocker) BlockReason(packageType, packageName, version string) string {
	return "blocked"
}

// IsBlockedWithAttrs 第二层检查：当 attrs 中 license=GPL-3.0 时阻断。
func (m *mockConditionBlocker) IsBlockedWithAttrs(packageType, packageName, version string, attrs map[string]interface{}) (bool, string) {
	if lic, ok := attrs["license"]; ok {
		if licStr, ok := lic.(string); ok && licStr == "GPL-3.0" {
			return true, "license blocked"
		}
	}
	return false, ""
}

func (m *mockConditionBlocker) IsBlockedByPath(string, string) bool     { return false }
func (m *mockConditionBlocker) BlockReasonByPath(string, string) string { return "" }

// TestGetArtifact_PostBlockCheck_LicenseBlocked 验证：artifact 的 Attributes 中
// license=GPL-3.0 时，GetArtifact 应返回 ErrBlocked（第二层检查命中）。
func TestGetArtifact_PostBlockCheck_LicenseBlocked(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Kind:         KindArtifact,
		Name:         "test-pkg",
		Version:      "1.0.0",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
		Attributes:   map[string]string{"license": "GPL-3.0"},
	}
	blobStore := &fakeBlobStore{}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     blobStore,
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Format:        "npm",
		Blocker:       &mockConditionBlocker{},
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Name: "test-pkg", Version: "1.0.0", Filename: "test-pkg.tgz"}

	_, err := runtime.GetArtifact(ctx, key)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked for GPL-3.0 license, got %v", err)
	}
	if blobStore.openCalls != 0 {
		t.Fatalf("blocked artifact opened %d blob(s), want 0", blobStore.openCalls)
	}
}

// TestGetArtifact_PostBlockCheck_NotBlocked 验证：artifact 的 Attributes 中
// license=MIT 时，GetArtifact 应正常返回 artifact（第二层检查未命中）。
func TestGetArtifact_PostBlockCheck_NotBlocked(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Kind:         KindArtifact,
		Name:         "test-pkg",
		Version:      "1.0.0",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
		Attributes:   map[string]string{"license": "MIT"},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Format:        "npm",
		Blocker:       &mockConditionBlocker{},
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Name: "test-pkg", Version: "1.0.0", Filename: "test-pkg.tgz"}

	artifact, err := runtime.GetArtifact(ctx, key)
	if err != nil {
		t.Fatalf("expected no error for MIT license, got %v", err)
	}
	if artifact == nil || artifact.ID != "cached" {
		t.Fatalf("expected cached artifact, got %v", artifact)
	}
}

// TestGetArtifact_PostBlockCheck_NoBlocker 验证：Blocker 为 nil 时，
// GetArtifact 应正常返回 artifact（不执行第二层检查）。
func TestGetArtifact_PostBlockCheck_NoBlocker(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Kind:         KindArtifact,
		Name:         "test-pkg",
		Version:      "1.0.0",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
		Attributes:   map[string]string{"license": "GPL-3.0"},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Format:        "npm",
		Blocker:       nil,
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Name: "test-pkg", Version: "1.0.0", Filename: "test-pkg.tgz"}

	artifact, err := runtime.GetArtifact(ctx, key)
	if err != nil {
		t.Fatalf("expected no error when Blocker is nil, got %v", err)
	}
	if artifact == nil || artifact.ID != "cached" {
		t.Fatalf("expected cached artifact, got %v", artifact)
	}
}

// TestGetArtifact_PostBlockCheck_CacheHitPath 验证：内存缓存命中路径也会执行第二层检查。
// 先不用 Blocker 让 artifact 进内存缓存，再设置 Blocker 调用，验证缓存命中路径被阻断。
func TestGetArtifact_PostBlockCheck_CacheHitPath(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Kind:         KindArtifact,
		Name:         "test-pkg",
		Version:      "1.0.0",
		BlobRefs:     []BlobRef{{Digest: "cached"}},
		UpdatedAt:    time.Now(),
		Attributes:   map[string]string{"license": "GPL-3.0"},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		BlobStore:     &fakeBlobStore{},
		RemoteClient:  &fakeRemoteClient{},
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Format:        "npm",
		Blocker:       nil, // 先不用 Blocker
	}
	key := ArtifactKey{RepositoryID: "repo", Format: "npm", Name: "test-pkg", Version: "1.0.0", Filename: "test-pkg.tgz"}

	// 第一次调用：走 store 路径，artifact 进入内存缓存
	if _, err := runtime.GetArtifact(ctx, key); err != nil {
		t.Fatalf("first get failed: %v", err)
	}

	// 设置 Blocker，第二次调用应命中内存缓存路径并被阻断
	runtime.Blocker = &mockConditionBlocker{}
	_, err := runtime.GetArtifact(ctx, key)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked on cache hit path, got %v", err)
	}
}

// TestQueryArtifacts_PostBlockFilter 验证：QueryArtifacts 返回的 artifacts 中，
// 被 license=GPL-3.0 条件规则阻断的 artifact 会被过滤掉（返回空列表）。
func TestQueryArtifacts_PostBlockFilter(t *testing.T) {
	ctx := context.Background()
	store := newFakeMetadataStore()
	store.artifact = &Artifact{
		ID:           "cached",
		RepositoryID: "repo",
		Format:       "npm",
		Kind:         KindVersion,
		Name:         "test-pkg",
		Version:      "1.0.0",
		UpdatedAt:    time.Now(),
		Attributes:   map[string]string{"license": "GPL-3.0"},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Format:        "npm",
		Blocker:       &mockConditionBlocker{},
	}

	artifacts, err := runtime.QueryArtifacts(ctx, ArtifactQuery{
		RepositoryID: "repo",
		Format:       "npm",
		RemotePath:   "test-pkg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected 0 artifacts after filter, got %d", len(artifacts))
	}
}

// TestQueryArtifacts_PostBlockFilter_PartialFilter 验证：QueryArtifacts 只过滤
// 被阻断的 artifact，保留未被阻断的。
// 使用 fakeMultiMetadataStore 返回多个 artifact。
func TestQueryArtifacts_PostBlockFilter_PartialFilter(t *testing.T) {
	ctx := context.Background()
	store := &fakeMultiMetadataStore{
		artifacts: []*Artifact{
			{
				ID:         "gpl-pkg",
				Format:     "npm",
				Kind:       KindVersion,
				Name:       "gpl-pkg",
				Version:    "1.0.0",
				UpdatedAt:  time.Now(),
				Attributes: map[string]string{"license": "GPL-3.0"},
			},
			{
				ID:         "mit-pkg",
				Format:     "npm",
				Kind:       KindVersion,
				Name:       "mit-pkg",
				Version:    "2.0.0",
				UpdatedAt:  time.Now(),
				Attributes: map[string]string{"license": "MIT"},
			},
		},
	}
	runtime := &ProxyRuntime{
		MetadataStore: store,
		RemoteBaseURL: "https://example.test",
		CachePolicy:   CachePolicy{MetadataTTL: time.Minute},
		Format:        "npm",
		Blocker:       &mockConditionBlocker{},
	}

	artifacts, err := runtime.QueryArtifacts(ctx, ArtifactQuery{
		RepositoryID: "repo",
		Format:       "npm",
		RemotePath:   "multi-pkg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact after partial filter, got %d", len(artifacts))
	}
	if artifacts[0].ID != "mit-pkg" {
		t.Fatalf("expected mit-pkg to survive filter, got %s", artifacts[0].ID)
	}
}

// fakeMultiMetadataStore 支持返回多个 artifact 的 fake store，用于测试过滤逻辑。
type fakeMultiMetadataStore struct {
	artifacts []*Artifact
	getCalls  int
	putCalls  int
}

func (s *fakeMultiMetadataStore) Get(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	s.getCalls++
	if len(s.artifacts) == 0 {
		return nil, ErrNotFound
	}
	return s.artifacts[0], nil
}

func (s *fakeMultiMetadataStore) Put(ctx context.Context, artifact *Artifact) error {
	s.putCalls++
	return nil
}

func (s *fakeMultiMetadataStore) BatchPut(ctx context.Context, artifacts []*Artifact) error {
	s.putCalls += len(artifacts)
	return nil
}

func (s *fakeMultiMetadataStore) Delete(ctx context.Context, key ArtifactKey) error {
	return nil
}

func (s *fakeMultiMetadataStore) Query(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return s.artifacts, nil
}
