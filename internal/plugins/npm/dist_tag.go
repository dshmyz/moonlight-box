package npm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

// handleDistTag 处理 npm dist-tag API 请求。
//
// 路径格式：
//   - GET/PUT/DELETE /-/package/{pkg}/dist-tags          → 列出所有标签
//   - GET/PUT/DELETE /-/package/{pkg}/dist-tags/{tag}    → 操作单个标签
//
// API 参考: https://github.com/npm/registry/blob/main/docs/REGISTRY-API.md
func (p *NpmPlugin) handleDistTag(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := strings.TrimPrefix(ctx.RepositoryPath, "/")
	// path: -/package/{pkg}/dist-tags or -/package/{pkg}/dist-tags/{tag}

	// 解析路径：-/package/{pkg}/dist-tags[/{tag}]
	remaining := strings.TrimPrefix(path, "-/package/")
	parts := strings.SplitN(remaining, "/dist-tags", 2)
	if len(parts) != 2 {
		http.Error(ctx.Writer, "invalid dist-tags path", http.StatusBadRequest)
		return nil
	}

	packageName := strings.TrimSpace(parts[0])
	if packageName == "" {
		http.Error(ctx.Writer, "package name required", http.StatusBadRequest)
		return nil
	}

	tag := strings.TrimPrefix(parts[1], "/")
	tag = strings.TrimSpace(tag)

	switch ctx.Request.Method {
	case http.MethodGet:
		if tag != "" {
			return p.handleDistTagGetOne(ctx, repoRuntime, packageName, tag)
		}
		return p.handleDistTagList(ctx, repoRuntime, packageName)
	case http.MethodPut:
		return p.handleDistTagAdd(ctx, repoRuntime, packageName, tag)
	case http.MethodDelete:
		return p.handleDistTagRemove(ctx, repoRuntime, packageName, tag)
	default:
		return errors.New("method not allowed")
	}
}

// handleDistTagList 处理 GET /-/package/{pkg}/dist-tags
// 返回所有标签的 tag→version 映射。
func (p *NpmPlugin) handleDistTagList(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	distTags, err := p.computeDistTags(ctx, repoRuntime, packageName)
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(distTags)
	return nil
}

// handleDistTagGetOne 处理 GET /-/package/{pkg}/dist-tags/{tag}
// 返回指定标签对应的版本号。
func (p *NpmPlugin) handleDistTagGetOne(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName, tag string) error {
	distTags, err := p.computeDistTags(ctx, repoRuntime, packageName)
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	version, ok := distTags[tag]
	if !ok {
		http.Error(ctx.Writer, "tag not found: "+tag, http.StatusNotFound)
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(version)
	return nil
}

// handleDistTagAdd 处理 PUT /-/package/{pkg}/dist-tags/{tag}
// 请求体为版本字符串（如 "1.0.0"）。
func (p *NpmPlugin) handleDistTagAdd(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName, tag string) error {
	if tag == "" {
		http.Error(ctx.Writer, "tag name required", http.StatusBadRequest)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 256))
	if err != nil {
		http.Error(ctx.Writer, "failed to read request body", http.StatusBadRequest)
		return nil
	}
	version := strings.TrimSpace(string(body))
	version = strings.Trim(version, "\"'") // 去除可能的引号
	if version == "" {
		http.Error(ctx.Writer, "version required in request body", http.StatusBadRequest)
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"package": packageName,
		"tag":     tag,
		"version": version,
	}).Debug("npm: dist-tag add")

	// 查询所有 metadata artifacts
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Name:         packageName,
		RemotePath:   packageName,
	})
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	// 找到目标版本的 artifact 和旧 tag 持有者的 artifact
	var targetArtifact *runtime.Artifact
	var oldTagArtifact *runtime.Artifact
	for _, a := range artifacts {
		if a.Version == version && a.Kind == runtime.KindMetadata {
			targetArtifact = a
		}
		if a.Properties["dist-tag"] == tag && a.Version != version && a.Kind == runtime.KindMetadata {
			oldTagArtifact = a
		}
	}

	if targetArtifact == nil {
		http.Error(ctx.Writer, "version not found: "+version, http.StatusNotFound)
		return nil
	}

	// 通过 upload session 更新 Properties
	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Filename:     packageName,
	})
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	// 移除旧 tag 持有者的标签（如果存在）
	if oldTagArtifact != nil {
		oldTagArtifact.Properties["dist-tag"] = ""
		if err := session.PutArtifact(ctx.Request.Context(), oldTagArtifact); err != nil {
			session.Abort(ctx.Request.Context())
			{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
			return nil
		}
	}

	// 设置新标签
	targetArtifact.Properties["dist-tag"] = tag
	if err := session.PutArtifact(ctx.Request.Context(), targetArtifact); err != nil {
		session.Abort(ctx.Request.Context())
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	if err := session.Commit(ctx.Request.Context()); err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}

// handleDistTagRemove 处理 DELETE /-/package/{pkg}/dist-tags/{tag}
func (p *NpmPlugin) handleDistTagRemove(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName, tag string) error {
	if tag == "" {
		http.Error(ctx.Writer, "tag name required", http.StatusBadRequest)
		return nil
	}

	// latest 标签不应通过 API 删除（自动计算）
	if tag == "latest" {
		http.Error(ctx.Writer, "cannot remove latest tag", http.StatusBadRequest)
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"package": packageName,
		"tag":     tag,
	}).Debug("npm: dist-tag remove")

	// 查询所有 metadata artifacts
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Name:         packageName,
		RemotePath:   packageName,
	})
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	// 找到持有该 tag 的 artifact
	var tagArtifact *runtime.Artifact
	for _, a := range artifacts {
		if a.Properties["dist-tag"] == tag && a.Kind == runtime.KindMetadata {
			tagArtifact = a
			break
		}
	}

	if tagArtifact == nil {
		http.Error(ctx.Writer, "tag not found: "+tag, http.StatusNotFound)
		return nil
	}

	// 通过 upload session 清除标签
	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Filename:     packageName,
	})
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	tagArtifact.Properties["dist-tag"] = ""
	if err := session.PutArtifact(ctx.Request.Context(), tagArtifact); err != nil {
		session.Abort(ctx.Request.Context())
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	if err := session.Commit(ctx.Request.Context()); err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	return nil
}

// computeDistTags 从 artifacts 中计算完整的 dist-tags 映射。
func (p *NpmPlugin) computeDistTags(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) (map[string]string, error) {
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Name:         packageName,
		RemotePath:   packageName,
	})
	if err != nil {
		return nil, err
	}

	distTags := map[string]string{}
	var versionList []string

	for _, a := range artifacts {
		if a.Version == "" {
			continue
		}
		versionList = append(versionList, a.Version)

		if tag := a.Properties["dist-tag"]; tag != "" {
			distTags[tag] = a.Version
		}
	}

	// 自动计算 latest（如果没有自定义 latest tag）
	if _, hasLatest := distTags["latest"]; !hasLatest && len(versionList) > 0 {
		distTags["latest"] = selectNpmLatestVersion(versionList)
	}

	return distTags, nil
}
