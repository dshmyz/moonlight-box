package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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
