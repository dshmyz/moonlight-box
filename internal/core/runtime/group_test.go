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
	if len(artifacts) != 2 {
		var got []string
		for _, a := range artifacts {
			got = append(got, a.Name+"|"+a.IdentityKey+"|"+artifactDedupeKey(a))
		}
		t.Fatalf("expected artifacts from both members, got %d: %#v", len(artifacts), got)
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
	if len(artifacts) != 2 {
		t.Fatalf("expected metadata versions from both members, got %d", len(artifacts))
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
