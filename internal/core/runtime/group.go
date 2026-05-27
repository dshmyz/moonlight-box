package runtime

import (
	"context"
	"fmt"
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
	name := a.Coordinates["name"]
	pkg := a.Coordinates["package"]
	version := a.Coordinates["version"]
	group := a.Coordinates["group"]
	artifact := a.Coordinates["artifact"]
	filename := a.Coordinates["filename"]
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", a.Format, name, pkg, version, group, artifact, filename)
}

func (g *GroupRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	allArtifacts := make([]*Artifact, 0)
	seen := make(map[string]bool)

	for _, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			continue
		}
		for _, a := range artifacts {
			key := artifactDedupeKey(a)
			if !seen[key] {
				seen[key] = true
				allArtifacts = append(allArtifacts, a)
			}
		}
	}
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
