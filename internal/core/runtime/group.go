package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

type GroupRuntime struct {
	Members  []RepositoryNode
	Writable RepositoryNode
	// 组合仓库自身的部署策略：与可写成员策略取交集（两者都允许才放行）。
	AllowOverwrite bool
	AllowDelete    bool
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
		"remote_path": query.RemotePath,
		"memberCount": len(g.Members),
	}).Debug("group: QueryArtifacts called")

	// metadata/index 投影查询聚合所有成员：否则 group 只能看到首个成员的版本列表
	// （docs/new3.md「GroupRuntime 支持 merge/shadowing/priority」，包级语义路径走 merge）。
	if isMetadataProjectionQuery(query) {
		return g.queryWithAggregation(ctx, query)
	}

	// 有具体路径且带身份字段的查询（如单个包文件）保持优先级短路。
	if query.RemotePath != "" && QueryHasIdentityFields(query) {
		return g.queryWithPriority(ctx, query)
	}

	// 纯 RemotePath 无身份字段的查询可能是仓库级索引（如 PyPI simple/），聚合所有成员。
	return g.queryWithAggregation(ctx, query)
}

// isMetadataProjectionQuery 判断查询是否为 metadata/index 投影（需要跨成员 merge）。
// checksum（.sha1/.md5）不在此列：checksum 对应的是成员各自渲染的内容，
// 合并无意义，插件会按合并后的投影重新计算。
func isMetadataProjectionQuery(query ArtifactQuery) bool {
	if query.Kind == KindMetadata {
		return true
	}
	if query.RemotePath == "" {
		return false
	}
	return strings.HasSuffix(query.RemotePath, "maven-metadata.xml") ||
		strings.HasSuffix(query.RemotePath, "repodata/repomd.xml") ||
		strings.HasSuffix(query.RemotePath, "/Packages") ||
		strings.HasSuffix(query.RemotePath, "/Packages.gz") ||
		strings.HasSuffix(query.RemotePath, "/Release") ||
		strings.HasSuffix(query.RemotePath, "/InRelease")
}

// artifactDedupeKey 生成跨成员去重的身份键。
// 优先用协议身份 IdentityKey（由 BuildArtifactIdentityKey 计算，不含仓库信息，
// 同一包版本在各成员中相同）；缺失时退回字段组合。绝不用 DB ID / RepositoryID——
// 同一个包在不同成员中应视为同一条目。
func artifactDedupeKey(a *Artifact) string {
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
					"remote_path":  query.RemotePath,
				}).Warn("group: member blocked artifact query")
				return nil, err
			}
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"remote_path":  query.RemotePath,
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

// queryWithAggregation 聚合所有成员的结果并按协议身份去重。
// 成员顺序即优先级：先出现的成员（hosted 优先）在去重时胜出。
// 单个成员失败不阻塞聚合（记录 firstErr 继续跳过）；全部成员失败且无结果时
// 返回首个非 NotFound 错误，避免把 503/熔断伪装成 404。
func (g *GroupRuntime) queryWithAggregation(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	all := make([]*Artifact, 0)
	seen := make(map[string]struct{})
	var firstErr error

	for i, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			if errors.Is(err, ErrBlocked) {
				logrus.WithFields(logrus.Fields{
					"memberIndex": i,
					"remote_path": query.RemotePath,
				}).Warn("group: member blocked artifact query")
				return nil, err
			}
			if errors.Is(err, ErrNotFound) {
				continue
			}
			logrus.WithFields(logrus.Fields{
				"memberIndex": i,
				"remote_path": query.RemotePath,
				"error":       err.Error(),
			}).Warn("group: member QueryArtifacts failed, skipping")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, a := range artifacts {
			if a == nil {
				continue
			}
			key := artifactDedupeKey(a)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			all = append(all, a)
		}
	}

	if len(all) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, ErrNotFound
	}

	logrus.WithFields(logrus.Fields{
		"totalArtifactCount": len(all),
		"remote_path":        query.RemotePath,
	}).Debug("group: aggregated artifacts across members")
	return all, nil
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
				"remote_path":  query.RemotePath,
			}).Warn("group: member blocked projection request")
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"memberIndex": i,
			"remote_path":  query.RemotePath,
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
	session, err := g.Writable.BeginUpload(ctx, request)
	if err != nil {
		return nil, err
	}
	// 组合仓库策略与可写成员策略取交集：组合库不允许覆盖时，即便成员允许也拒绝。
	if hosted, ok := session.(*HostedUploadSession); ok {
		hosted.allowOverwrite = hosted.allowOverwrite && g.AllowOverwrite
	}
	return session, nil
}

func (g *GroupRuntime) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	if g.Writable == nil {
		return ErrReadOnly
	}
	// 与 hosted 成员的删除守卫一致：先确认存在性（404 优先），再判组合库策略。
	if _, err := g.GetArtifact(ctx, key); err != nil {
		return err
	}
	if !g.AllowDelete {
		return ErrDeleteNotAllowed
	}
	return g.Writable.DeleteArtifact(ctx, key)
}
