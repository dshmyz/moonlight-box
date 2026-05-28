package testhelper

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

// MockRuntime implements runtime.RepositoryRuntime for plugin unit tests.
type MockRuntime struct {
	mu        sync.Mutex
	Artifacts []*runtime.Artifact
	QueryErr  error
	GetErr    error

	// Track calls
	QueryCalls []runtime.ArtifactQuery
	GetCalls   []runtime.ArtifactKey

	// Upload tracking
	UploadCalls  []runtime.UploadRequest
	UploadErr    error
	UploadedArts []*runtime.Artifact

	// Delete tracking
	DeleteCalls []runtime.ArtifactKey
	DeleteErr   error
}

func (m *MockRuntime) GetArtifact(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetCalls = append(m.GetCalls, key)
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	for _, a := range m.Artifacts {
		if matchArtifact(a, key) {
			return a, nil
		}
	}
	return nil, runtime.ErrNotFound
}

func (m *MockRuntime) QueryArtifacts(ctx context.Context, query runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QueryCalls = append(m.QueryCalls, query)
	if m.QueryErr != nil {
		return nil, m.QueryErr
	}
	if len(query.Coordinates) == 0 {
		return m.Artifacts, nil
	}
	var matched []*runtime.Artifact
	for _, a := range m.Artifacts {
		if matchCoordinates(a, query.Coordinates) {
			matched = append(matched, a)
		}
	}
	return matched, nil
}

func (m *MockRuntime) RenderProjection(ctx context.Context, query runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, runtime.ErrNotFound
}

func (m *MockRuntime) BeginUpload(ctx context.Context, req runtime.UploadRequest) (runtime.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UploadCalls = append(m.UploadCalls, req)
	if m.UploadErr != nil {
		return nil, m.UploadErr
	}
	return &mockUploadSession{mock: m}, nil
}

func (m *MockRuntime) DeleteArtifact(ctx context.Context, key runtime.ArtifactKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteCalls = append(m.DeleteCalls, key)
	return m.DeleteErr
}

// mockUploadSession implements runtime.UploadSession.
type mockUploadSession struct {
	mock    *MockRuntime
	blobRef runtime.BlobRef
	arts    []*runtime.Artifact
}

func (s *mockUploadSession) PutBlob(ctx context.Context, blob io.Reader) (runtime.BlobRef, error) {
	data, _ := io.ReadAll(blob)
	s.blobRef = runtime.BlobRef{Algorithm: "sha256", Digest: "test-digest", Size: int64(len(data))}
	return s.blobRef, nil
}

func (s *mockUploadSession) PutArtifact(ctx context.Context, artifact *runtime.Artifact) error {
	s.arts = append(s.arts, artifact)
	return nil
}

func (s *mockUploadSession) Commit(ctx context.Context) error {
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for _, a := range s.arts {
		a.BlobRefs = []runtime.BlobRef{s.blobRef}
		s.mock.UploadedArts = append(s.mock.UploadedArts, a)
	}
	return nil
}

func (s *mockUploadSession) Abort(ctx context.Context) error { return nil }

func matchArtifact(a *runtime.Artifact, key runtime.ArtifactKey) bool {
	for k, v := range key.Coordinates {
		if a.Coordinates[k] != v {
			return false
		}
	}
	return true
}

func matchCoordinates(a *runtime.Artifact, coords map[string]string) bool {
	for k, v := range coords {
		av, ok := a.Coordinates[k]
		if !ok || av != v {
			return false
		}
	}
	return true
}

// NewArtifact is a helper to create test artifacts.
func NewArtifact(format, kind string, coords map[string]string, content string) *runtime.Artifact {
	a := &runtime.Artifact{
		Format:      format,
		Kind:        kind,
		Coordinates: coords,
		Properties:  map[string]string{},
	}
	if content != "" {
		a.Content = io.NopCloser(strings.NewReader(content))
	}
	return a
}
