package runtime

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
)

type GroupRuntime struct {
	Members  []RepositoryNode
	Writable RepositoryNode
}

func (g *GroupRuntime) OpenRemote(ctx context.Context, request RemoteOpenRequest) (*RemoteResponse, error) {
	for _, node := range g.Members {
		response, err := node.OpenRemote(ctx, request)
		if err == nil {
			return response, nil
		}
		if !errors.Is(err, ErrRemoteUnsupported) {
			return nil, err
		}
	}
	return nil, ErrRemoteUnsupported
}

func (g *GroupRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	var firstErr error
	for i, node := range g.Members {
		artifact, err := node.GetArtifact(ctx, key)
		if err == nil {
			return artifact, nil
		}
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if errors.Is(err, ErrBlocked) {
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"key":         key.String(),
			}).Warn("group: member blocked artifact request")
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"memberIndex": i,
			"key":         key.String(),
			"error":       err.Error(),
		}).Warn("group: member GetArtifact failed")
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
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
	var firstErr error
	for i, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if errors.Is(err, ErrBlocked) {
				logrus.WithFields(logrus.Fields{
					"memberIndex": i,
					"remotePath":  query.RemotePath,
				}).Warn("group: member blocked artifact query")
				return nil, err
			}
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"remotePath":  query.RemotePath,
				"error":       err.Error(),
			}).Warn("group: member QueryArtifacts failed")
			if firstErr == nil {
				firstErr = err
			}
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
	if firstErr != nil {
		return nil, firstErr
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
