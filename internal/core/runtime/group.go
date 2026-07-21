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
	var firstErr error
	for _, node := range g.Members {
		response, err := node.OpenRemote(ctx, request)
		if err == nil {
			return response, nil
		}
		// ErrRemoteUnsupported：该成员不支持 OpenRemote（如 hosted 仓库），跳过
		// ErrCircuitOpen：该成员上游熔断打开，跳过试下一个成员（语义同"暂时不可用"）
		// 其他错误（如 503）：立即返回，不雪崩到后续成员
		if errors.Is(err, ErrRemoteUnsupported) || errors.Is(err, ErrCircuitOpen) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return nil, err
	}
	if firstErr != nil {
		return nil, firstErr
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
	var firstErr error
	for i, node := range g.Members {
		result, err := node.RenderProjection(ctx, query)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if errors.Is(err, ErrBlocked) {
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"remotePath":  query.RemotePath,
			}).Warn("group: member blocked projection request")
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"memberIndex": i,
			"remotePath":  query.RemotePath,
			"error":       err.Error(),
		}).Warn("group: member RenderProjection failed")
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
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
