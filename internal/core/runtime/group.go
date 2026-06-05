package runtime

import (
	"context"
	"fmt"

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

// artifactDedupeKey 生成去重用的唯一 key，不依赖 Artifact.ID
// 因为回源创建的 Artifact.ID 可能为空。
// 注意：不包含 RepositoryID，因为 GroupRuntime 合并多个成员仓库的结果时，
// 同一个包在不同成员中应视为同一条目。
func artifactDedupeKey(a *Artifact) string {
	if a.ID != "" {
		return a.ID
	}
	if a.IdentityKey != "" {
		return a.Format + "/" + a.IdentityKey
	}
	name := a.Name
	version := a.Version
	group := a.Namespace
	artifact := a.Qualifiers["artifact"]
	filename := a.Filename
	remotePath := firstNonEmpty(a.RemotePath, a.Properties["remote_path"])
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", a.Format, name, version, group, artifact, filename, remotePath)
}

func (g *GroupRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	logrus.WithFields(logrus.Fields{
		"format":      query.Format,
		"remotePath":  query.RemotePath,
		"memberCount": len(g.Members),
	}).Debug("group: QueryArtifacts called")

	// 对于有具体路径且带身份字段的查询（如单个包的回源），使用优先级短路策略。
	// 纯 RemotePath 也可能是仓库级索引（如 PyPI simple/），必须聚合所有成员。
	if query.RemotePath != "" && QueryHasIdentityFields(query) {
		return g.queryWithPriority(ctx, query)
	}

	// 对于没有具体路径的查询（如列出所有包），聚合所有成员的结果
	return g.queryWithAggregation(ctx, query)
}

// queryWithPriority 按优先级短路查询，适用于有具体路径的场景
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

// queryWithAggregation 聚合查询，适用于列表场景
func (g *GroupRuntime) queryWithAggregation(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	allArtifacts := make([]*Artifact, 0)
	seen := make(map[string]bool)

	for i, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"remotePath":  query.RemotePath,
				"error":       err.Error(),
			}).Warn("group: member QueryArtifacts failed, skipping")
			continue
		}
		logrus.WithFields(logrus.Fields{
			"memberIndex":   i,
			"artifactCount": len(artifacts),
		}).Debug("group: member returned artifacts")
		for _, a := range artifacts {
			key := artifactDedupeKey(a)
			if !seen[key] {
				seen[key] = true
				allArtifacts = append(allArtifacts, a)
			}
		}
	}

	logrus.WithFields(logrus.Fields{
		"totalArtifactCount": len(allArtifacts),
	}).Debug("group: QueryArtifacts completed")

	return allArtifacts, nil
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
