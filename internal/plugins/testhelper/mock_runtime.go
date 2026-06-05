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
	if isEmptyQuery(query) {
		return m.Artifacts, nil
	}
	var matched []*runtime.Artifact
	for _, a := range m.Artifacts {
		if matchQuery(a, query) {
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
	if key.IdentityKey != "" && a.IdentityKey != "" && a.IdentityKey != key.IdentityKey {
		return false
	}
	if key.Name != "" && a.Name != "" && a.Name != key.Name {
		return false
	}
	if key.Version != "" && a.Version != "" && a.Version != key.Version {
		return false
	}
	if key.RemotePath != "" && a.RemotePath != "" && a.RemotePath != key.RemotePath {
		return false
	}
	if key.Path != "" && a.Path != "" && a.Path != key.Path {
		return false
	}
	if key.Filename != "" && a.Filename != "" && a.Filename != key.Filename {
		return false
	}
	for k, v := range key.Qualifiers {
		if a.Qualifiers[k] != v {
			return false
		}
	}
	return true
}

func isEmptyQuery(query runtime.ArtifactQuery) bool {
	return query.IdentityKey == "" && query.Name == "" && query.Version == "" && query.RemotePath == "" &&
		query.Path == "" && query.Filename == "" && len(query.Qualifiers) == 0
}

func matchQuery(a *runtime.Artifact, query runtime.ArtifactQuery) bool {
	if query.IdentityKey != "" && a.IdentityKey != query.IdentityKey {
		return false
	}
	if query.Name != "" && a.Name != query.Name {
		return false
	}
	if query.Version != "" && a.Version != query.Version {
		return false
	}
	if query.RemotePath != "" && a.RemotePath != "" && a.RemotePath != query.RemotePath {
		return false
	}
	if query.Path != "" && a.Path != "" && a.Path != query.Path {
		return false
	}
	if query.Filename != "" && a.Filename != "" && a.Filename != query.Filename {
		return false
	}
	for k, v := range query.Qualifiers {
		if a.Qualifiers[k] != v {
			return false
		}
	}
	return true
}

// NewArtifact is a helper to create test artifacts.
func NewArtifact(format, kind string, coords map[string]string, content string) *runtime.Artifact {
	name := firstNonEmptyTest(coords["name"], coords["package"], coords["module"])
	if name == "" && coords["group"] != "" && coords["artifact"] != "" {
		name = coords["group"] + ":" + coords["artifact"]
	}
	qualifiers := map[string]string{}
	for k, v := range coords {
		switch k {
		case "name", "version", "path", "filename", "file":
			continue
		default:
			qualifiers[k] = v
		}
	}
	a := runtime.NewArtifact(runtime.ArtifactSpec{
		Format:     format,
		Kind:       kind,
		Name:       name,
		Version:    coords["version"],
		Path:       coords["path"],
		Filename:   firstNonEmptyTest(coords["filename"], coords["file"]),
		Qualifiers: qualifiers,
		Properties: map[string]string{},
	})
	if content != "" {
		a.Content = io.NopCloser(strings.NewReader(content))
	}
	return a
}

func firstNonEmptyTest(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
