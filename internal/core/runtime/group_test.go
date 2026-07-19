package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGroupRuntimeOpenRemoteSkipsHostedMember(t *testing.T) {
	proxy := &ProxyRuntime{
		RemoteBaseURL: "https://upstream.example",
		RemoteClient: &fakeRemoteClient{openResponse: &RemoteResponse{
			StatusCode: http.StatusOK,
			Header:     http.Header{"ETag": {"upstream-etag"}},
			Body:       io.NopCloser(strings.NewReader("body")),
		}},
	}
	group := &GroupRuntime{Members: []RepositoryNode{&HostedRuntime{}, proxy}}

	response, err := group.OpenRemote(context.Background(), RemoteOpenRequest{Path: "package.tgz", Method: http.MethodGet})
	if err != nil {
		t.Fatalf("OpenRemote failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestGroupRuntimeOpenRemoteReturnsFirstSuccessfulMemberWithoutCallingLaterSuccess(t *testing.T) {
	firstBody := io.NopCloser(strings.NewReader("first"))
	first := &groupOpenNode{response: &RemoteResponse{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Member": {"first"}},
		Body:       firstBody,
	}}
	later := &groupOpenNode{response: &RemoteResponse{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Member": {"later"}},
		Body:       io.NopCloser(strings.NewReader("later")),
	}}
	group := &GroupRuntime{Members: []RepositoryNode{&HostedRuntime{}, first, later}}

	response, err := group.OpenRemote(context.Background(), RemoteOpenRequest{Path: "simple/", Method: http.MethodGet})
	if err != nil {
		t.Fatalf("OpenRemote failed: %v", err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("X-Member"); got != "first" {
		t.Fatalf("X-Member = %q, want first", got)
	}
	if first.openCalls != 1 {
		t.Fatalf("first successful member calls = %d, want 1", first.openCalls)
	}
	if later.openCalls != 0 {
		t.Fatalf("later successful member calls = %d, want 0", later.openCalls)
	}
}

func TestGroupRuntimeOpenRemoteReturnsUnsupportedWhenAllMembersDecline(t *testing.T) {
	group := &GroupRuntime{Members: []RepositoryNode{&HostedRuntime{}, &HostedRuntime{}}}

	_, err := group.OpenRemote(context.Background(), RemoteOpenRequest{Path: "package.tgz", Method: http.MethodGet})
	if !errors.Is(err, ErrRemoteUnsupported) {
		t.Fatalf("error = %v, want ErrRemoteUnsupported", err)
	}
}

func TestGroupRuntimeOpenRemoteReturnsFirstNonUnsupportedErrorWithoutCallingLaterMembers(t *testing.T) {
	errUpstream := errors.New("upstream unavailable")
	first := &groupErrorNode{err: errUpstream}
	later := &groupErrorNode{err: ErrRemoteUnsupported}
	group := &GroupRuntime{Members: []RepositoryNode{&HostedRuntime{}, first, later}}

	_, err := group.OpenRemote(context.Background(), RemoteOpenRequest{Path: "package.tgz", Method: http.MethodGet})
	if !errors.Is(err, errUpstream) {
		t.Fatalf("error = %v, want %v", err, errUpstream)
	}
	if first.openCalls != 1 {
		t.Fatalf("first member OpenRemote calls = %d, want 1", first.openCalls)
	}
	if later.openCalls != 0 {
		t.Fatalf("later member OpenRemote calls = %d, want 0", later.openCalls)
	}
}

func TestGroupQueryArtifactsPropagatesBlocked(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupErrorNode{err: ErrBlocked},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{Format: "npm", Name: "lodash"})}},
		},
	}

	_, err := group.QueryArtifacts(context.Background(), ArtifactQuery{Format: "npm"})
	if err != ErrBlocked {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestGroupQueryArtifactsReturnsFirstNonNotFoundError(t *testing.T) {
	errUpstream := errors.New("upstream 502")
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupErrorNode{err: errUpstream},
			&groupErrorNode{err: ErrNotFound},
		},
	}

	_, err := group.QueryArtifacts(context.Background(), ArtifactQuery{Format: "npm"})
	if err != errUpstream {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

func TestGroupGetArtifactPropagatesBlocked(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupErrorNode{err: ErrBlocked},
			&groupArtifactNode{artifact: NewArtifact(ArtifactSpec{Format: "npm", Name: "lodash"})},
		},
	}

	_, err := group.GetArtifact(context.Background(), ArtifactKey{Format: "npm", Name: "lodash"})
	if err != ErrBlocked {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestGroupGetArtifactReturnsFirstNonNotFoundError(t *testing.T) {
	errUpstream := errors.New("upstream 502")
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupErrorNode{err: errUpstream},
			&groupErrorNode{err: ErrNotFound},
		},
	}

	_, err := group.GetArtifact(context.Background(), ArtifactKey{Format: "npm", Name: "lodash"})
	if err != errUpstream {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

func TestGroupQueryArtifactsPriorityShortCircuit(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format: "npm",
				Name:   "lodash",
			})}},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format: "npm",
				Name:   "express",
			})}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format: "npm",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	// 方案 C：只返回第一个有结果的成员
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from priority member, got %d", len(artifacts))
	}
	if artifacts[0].Name != "lodash" {
		t.Fatalf("expected first member's artifact 'lodash', got %q", artifacts[0].Name)
	}
}

func TestGroupQueryArtifactsSkipsEmptyMembers(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{}}, // 空成员
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format: "npm",
				Name:   "express",
			})}},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format: "npm",
				Name:   "axios",
			})}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format: "npm",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Name != "express" {
		t.Fatalf("expected 'express' from second member, got %q", artifacts[0].Name)
	}
}

func TestGroupQueryArtifactsSkipsNotFoundMembers(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupErrorNode{err: ErrNotFound},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format: "npm",
				Name:   "lodash",
			})}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format: "npm",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from second member, got %d", len(artifacts))
	}
	if artifacts[0].Name != "lodash" {
		t.Fatalf("expected 'lodash' from second member, got %q", artifacts[0].Name)
	}
}

func TestGroupQueryArtifactsReturnsNotFoundWhenAllEmpty(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{}},
			&groupQueryNode{artifacts: []*Artifact{}},
		},
	}

	_, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format: "npm",
	})
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGroupQueryArtifactsRemotePathPriorityShortCircuit(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "pypi",
				Name:       "requests",
				RemotePath: "simple/requests/",
			})}},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "pypi",
				Name:       "flask",
				RemotePath: "simple/flask/",
			})}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format:     "pypi",
		RemotePath: "simple/requests/",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from priority member, got %d", len(artifacts))
	}
	if artifacts[0].Name != "requests" {
		t.Fatalf("expected 'requests', got %q", artifacts[0].Name)
	}
}

// 更新现有测试：优先级短路只返回第一个成员的结果

func TestGroupQueryArtifactsAggregatesRemotePathWithStructuredFields(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "pypi",
				Kind:       KindMetadata,
				Name:       "requests",
				Qualifiers: map[string]string{"package": "requests"},
			})}},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "pypi",
				Kind:       KindMetadata,
				Name:       "flask",
				Qualifiers: map[string]string{"package": "flask"},
			})}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format:     "pypi",
		RemotePath: "simple/",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	// 方案 C：优先级短路，只返回第一个成员的 results
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from priority member, got %d", len(artifacts))
	}
	if artifacts[0].Name != "requests" {
		t.Fatalf("expected 'requests', got %q", artifacts[0].Name)
	}
}

func TestGroupQueryArtifactsAggregatesMavenMetadataAcrossMembers(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "maven",
				Kind:       KindVersion,
				Namespace:  "com.google.guava",
				Name:       "com.google.guava:guava",
				Version:    "31.1-jre",
				RemotePath: "com/google/guava/guava/maven-metadata.xml",
				Qualifiers: map[string]string{"group": "com.google.guava", "artifact": "guava"},
			})}},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "maven",
				Kind:       KindVersion,
				Namespace:  "com.google.guava",
				Name:       "com.google.guava:guava",
				Version:    "32.1.2-jre",
				RemotePath: "com/google/guava/guava/maven-metadata.xml",
				Qualifiers: map[string]string{"group": "com.google.guava", "artifact": "guava"},
			})}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format:     "maven",
		Namespace:  "com.google.guava",
		Name:       "com.google.guava:guava",
		RemotePath: "com/google/guava/guava/maven-metadata.xml",
		Qualifiers: map[string]string{"group": "com.google.guava", "artifact": "guava"},
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	// 方案 C：优先级短路，只返回第一个成员的版本
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 version from priority member, got %d", len(artifacts))
	}
	if artifacts[0].Version != "31.1-jre" {
		t.Fatalf("expected '31.1-jre', got %q", artifacts[0].Version)
	}
}

type groupQueryNode struct {
	artifacts []*Artifact
}

type groupOpenNode struct {
	HostedRuntime
	response  *RemoteResponse
	err       error
	openCalls int
}

func (n *groupOpenNode) OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error) {
	n.openCalls++
	return n.response, n.err
}

func (n *groupQueryNode) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return nil, ErrNotFound
}

func (n *groupQueryNode) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return n.artifacts, nil
}

func (n *groupQueryNode) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	return nil, ErrNotFound
}

func (n *groupQueryNode) OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error) {
	return nil, ErrRemoteUnsupported
}

func (n *groupQueryNode) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}

func (n *groupQueryNode) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	return ErrReadOnly
}

// groupErrorNode 模拟查询失败的成员
type groupErrorNode struct {
	err       error
	openCalls int
}

func (n *groupErrorNode) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return nil, n.err
}

func (n *groupErrorNode) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return nil, n.err
}

func (n *groupErrorNode) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	return nil, n.err
}

func (n *groupErrorNode) OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error) {
	n.openCalls++
	return nil, n.err
}

func (n *groupErrorNode) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, n.err
}

func (n *groupErrorNode) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	return n.err
}

type groupArtifactNode struct {
	artifact *Artifact
}

func (n *groupArtifactNode) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return n.artifact, nil
}

func (n *groupArtifactNode) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return []*Artifact{n.artifact}, nil
}

func (n *groupArtifactNode) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	return nil, ErrNotFound
}

func (n *groupArtifactNode) OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error) {
	return nil, ErrRemoteUnsupported
}

func (n *groupArtifactNode) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}

func (n *groupArtifactNode) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	return ErrReadOnly
}
