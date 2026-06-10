package runtime

import (
	"context"

	"github.com/sirupsen/logrus"
)

type GroupRuntime struct {
	Members  []RepositoryNode
	Writable RepositoryNode
}

func (g *GroupRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	for _, node := range g.Members {
		artifact, err := node.GetArtifact(ctx, key)
		if err == nil {
			return artifact, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	logrus.WithFields(logrus.Fields{
		"format":      query.Format,
		"remotePath":  query.RemotePath,
		"memberCount": len(g.Members),
	}).Debug("group: QueryArtifacts called")

	// 方案 C：所有查询按成员优先级短路，第一个有结果的成员返回。
	return g.queryWithPriority(ctx, query)
}

// queryWithPriority 按优先级短路查询
func (g *GroupRuntime) queryWithPriority(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	for i, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"remotePath":  query.RemotePath,
				"error":       err.Error(),
			}).Debug("group: member QueryArtifacts failed, trying next")
			continue
		}
		if len(artifacts) > 0 {
			logrus.WithFields(logrus.Fields{
				"memberIndex":   i,
				"artifactCount": len(artifacts),
			}).Debug("group: found artifacts from member, returning")
			return artifacts, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	for _, node := range g.Members {
		result, err := node.RenderProjection(ctx, query)
		if err == nil {
			return result, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	if g.Writable == nil {
		return nil, ErrReadOnly
	}
	return g.Writable.BeginUpload(ctx, request)
}

func (g *GroupRuntime) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	if g.Writable == nil {
		return ErrReadOnly
	}
	return g.Writable.DeleteArtifact(ctx, key)
}
