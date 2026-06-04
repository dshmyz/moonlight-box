package runtime

import (
	"context"
	"testing"
)

func TestGroupQueryArtifactsAggregatesRemotePathWithoutCoordinates(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{{
				Format:      "pypi",
				Kind:        "package-index",
				Coordinates: map[string]string{"name": "requests", "package": "requests"},
			}}},
			&groupQueryNode{artifacts: []*Artifact{{
				Format:      "pypi",
				Kind:        "package-index",
				Coordinates: map[string]string{"name": "flask", "package": "flask"},
			}}},
		},
	}

	artifacts, err := group.QueryArtifacts(context.Background(), ArtifactQuery{
		Format:     "pypi",
		RemotePath: "simple/",
	})
	if err != nil {
		t.Fatalf("QueryArtifacts failed: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected artifacts from both members, got %d", len(artifacts))
	}
}

type groupQueryNode struct {
	artifacts []*Artifact
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

func (n *groupQueryNode) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}

func (n *groupQueryNode) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	return ErrReadOnly
}
