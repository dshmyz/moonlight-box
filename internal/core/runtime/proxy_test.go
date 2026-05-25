package runtime

import (
	"context"
	"errors"
	"io"
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
