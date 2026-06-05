package runtime

import (
	"context"
	"testing"
)

func TestGroupQueryArtifactsAggregatesRemotePathWithStructuredFields(t *testing.T) {
	group := &GroupRuntime{
		Members: []RepositoryNode{
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "pypi",
				Kind:       "package-index",
				Name:       "requests",
				Qualifiers: map[string]string{"package": "requests"},
			})}},
			&groupQueryNode{artifacts: []*Artifact{NewArtifact(ArtifactSpec{
				Format:     "pypi",
				Kind:       "package-index",
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
	if len(artifacts) != 2 {
		var got []string
		for _, a := range artifacts {
			got = append(got, a.Name+"|"+a.IdentityKey+"|"+artifactDedupeKey(a))
		}
		t.Fatalf("expected artifacts from both members, got %d: %#v", len(artifacts), got)
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
