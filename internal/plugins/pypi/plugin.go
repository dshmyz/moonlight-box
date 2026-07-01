// Package pypi implements the PyPI Simple API protocol plugin (PEP 503).
//
// # PyPI Simple API 协议要点 (PEP 503)
//
// ## 路径结构
//   - Simple Index: /simple/ -> 列出所有包
//   - Package Files: /simple/{package}/ -> 列出该包所有版本文件
//   - 文件下载: /packages/{hash_prefix}/{filename}
//   - Checksum: /packages/{hash_prefix}/{filename}.sha256
//   - 例如: /packages/cc/15/.../requests-2.19.0-py3-none-any.whl
//
// ## HTML 链接格式
//   - 必须包含 #sha256= hash fragment: <a href="../../packages/.../file.whl#sha256=abc123">
//   - pip 依赖 hash fragment 做完整性校验
//   - --require-hashes 模式下缺少 hash 会直接失败
//
// ## 包名规范化 (PEP 503)
//   - 规则: 将 [-_.]+ 归一化为单个 "-"，然后转小写
//   - 例如: My__Pkg..Name -> my-pkg-name
//
// ## JSON API (PEP 691)
//   - Accept: application/vnd.pypi.simple.v1+json
//   - 返回 JSON 格式的包文件列表
//
// ## 回源策略（重要）
//
// 文件下载必须遵循以下回源路径：
//
//  1. 先通过 GetArtifact 查询本地缓存/MetadataStore
//  2. 未命中时通过 QueryArtifacts(RemotePath=path) 触发 FetchRemote 回源
//  3. 回源成功后再次 GetArtifact 获取带 blob 的完整 artifact
//
// ArtifactKey 必须与 FetchRemote 存储的 Name/Version/Filename/RemotePath 字段一致。
// Checksum 请求使用 QueryArtifacts 按 filename 查找，因为 URL 中不含 package/version 信息。
//
// ## 关键实现点
//   - remote_path: 必须包含 packages/ 前缀，如 packages/cc/15/.../file.whl
//   - path 坐标: 不含 packages/ 前缀，如 cc/15/.../file.whl
//   - HTML 链接: 使用 remote_path，前缀是 ../../ 而非 ../../packages/
//
// ## 参考规范
//   - PEP 503: https://peps.python.org/pep-0503/
//   - PEP 691: https://peps.python.org/pep-0691/
package pypi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	urlpath "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
	nethtml "golang.org/x/net/html"
)

// pypiNormalizeRe 匹配 PEP 503 规范中需要归一化的连续字符：[-_.]+
var pypiNormalizeRe = regexp.MustCompile(`[-_.]+`)

type PyPIPlugin struct {
	httpClient *http.Client // 统一 HTTP 客户端
}

func NewPyPIPlugin(httpClient *http.Client) *PyPIPlugin {
	if httpClient == nil {
		panic("pypi: httpClient is required")
	}
	return &PyPIPlugin{httpClient: httpClient}
}

// SetHTTPClient 允许注入统一的 HTTP 客户端
func (p *PyPIPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

func (p *PyPIPlugin) Name() string {
	return "pypi"
}

// FetchRemote 实现 RemoteFetcher 接口。
// Runtime 在本地缓存为空时回调此方法，Plugin 负责远端协议交互。
func (p *PyPIPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
		"fullURL":   fullURL,
	}).Debug("pypi: FetchRemote called")

	resp, err := p.httpGet(ctx, fullURL)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("pypi: HTTP request failed")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Error("pypi: HTTP request returned non-200 status")
		return nil, fmt.Errorf("remote returned status %d for %s", resp.StatusCode, fullURL)
	}

	var artifacts []*runtime.Artifact
	if p.isSimpleIndexRequest(path) {
		artifacts, err = p.parseSimpleIndex(resp.Body)
	} else if p.isPackageListRequest(path) {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		packageName := normalizePackageName(parts[1])
		if info, fetchErr := p.fetchPyPIPackageInfo(ctx, remoteURL, packageName); fetchErr == nil {
			artifacts = p.buildArtifactsFromJSONAPI(packageName, info)
		} else {
			artifacts, err = p.parsePackageList(packageName, resp.Body)
			if err == nil && len(artifacts) > 0 {
				if info2, fetchErr2 := p.fetchPyPIPackageInfo(ctx, remoteURL, packageName); fetchErr2 == nil {
					p.mergePackageInfo(artifacts, info2)
				}
			}
		}
	} else {
		logrus.WithField("path", path).Error("pypi: unsupported remote path")
		return nil, fmt.Errorf("unsupported remote path: %s", path)
	}

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("pypi: parse response failed")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"fullURL":       fullURL,
		"artifactCount": len(artifacts),
		"duration":      time.Since(start).Seconds(),
	}).Debug("pypi: FetchRemote success")
	return artifacts, nil
}

// FetchArtifactMetadata returns normalized metadata for one PyPI release. It
// reuses the simple-index fetch path, which already enriches artifacts from the
// PyPI JSON API when it is available.
func (p *PyPIPlugin) FetchArtifactMetadata(ctx context.Context, remoteURL string, key runtime.ArtifactKey) (*runtime.ArtifactMetadata, error) {
	if key.Name == "" || key.Version == "" {
		return nil, runtime.ErrMetadataUnavailable
	}
	artifacts, err := p.FetchRemote(ctx, remoteURL, "simple/"+normalizePackageName(key.Name)+"/")
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if artifact.Name == normalizePackageName(key.Name) && artifact.Version == key.Version {
			return &runtime.ArtifactMetadata{Attributes: artifact.Attributes}, nil
		}
	}
	return nil, runtime.ErrMetadataUnavailable
}

func (p *PyPIPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if err := validatePyPIPath(path); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}

	if p.isLegacyUploadRequest(path) {
		return p.handleLegacyUpload(ctx, repoRuntime)
	}

	if p.isSimpleIndexRequest(path) {
		return p.handleSimpleIndex(ctx, repoRuntime, path)
	}

	if p.isPackageListRequest(path) {
		return p.handlePackageList(ctx, repoRuntime, path)
	}

	if p.isPackageFileMetadataRequest(path) {
		return p.handleFileMetadataRequest(ctx, repoRuntime, path)
	}

	if p.isPackagesPath(path) {
		return p.handlePackagesDownload(ctx, repoRuntime, path)
	}

	if p.isJsonAPIRequest(path) {
		return p.handleJsonAPI(ctx, repoRuntime, path)
	}

	// PEP 658: dist-info-metadata 端点
	if p.isMetadataRequest(path) {
		return p.handleMetadataRequest(ctx, repoRuntime, path)
	}

	return errors.New("invalid pypi path")
}

func (p *PyPIPlugin) isSimpleIndexRequest(path string) bool {
	return path == "simple" || path == "simple/"
}

func (p *PyPIPlugin) isPackageListRequest(path string) bool {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	// PEP 503: simple/{pkg}/ 或 simple/{pkg}
	return len(parts) == 2 && parts[0] == "simple"
}

func (p *PyPIPlugin) isPackagesPath(path string) bool {
	return strings.HasPrefix(path, "packages/")
}

func (p *PyPIPlugin) isLegacyUploadRequest(path string) bool {
	return path == "legacy" || path == "legacy/"
}

func (p *PyPIPlugin) isPackageFileMetadataRequest(path string) bool {
	return strings.HasPrefix(path, "packages/") && strings.HasSuffix(path, ".metadata")
}

func (p *PyPIPlugin) isJsonAPIRequest(path string) bool {
	return strings.HasPrefix(path, "pypi/") && strings.HasSuffix(path, "/json")
}

// isMetadataRequest 判断是否为 PEP 658 .metadata 请求。
// 路径格式: simple/{package}/{filename}.metadata
func (p *PyPIPlugin) isMetadataRequest(path string) bool {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 3 || parts[0] != "simple" {
		return false
	}
	return strings.HasSuffix(parts[2], ".metadata")
}

// handleMetadataRequest 处理 PEP 658 .metadata 请求。
// 路径格式: simple/{package}/{filename}.metadata
// 返回包的 dist-info metadata 内容（如 Requires-Python、依赖等）。
func (p *PyPIPlugin) handleMetadataRequest(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
		return errors.New("method not allowed")
	}

	// 解析路径: simple/{package}/{filename}.metadata
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 {
		http.Error(ctx.Writer, "invalid metadata path", http.StatusBadRequest)
		return nil
	}

	packageName := normalizePackageName(parts[1])
	filename := strings.TrimSuffix(parts[2], ".metadata")
	packageListPath := "simple/" + packageName + "/"

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Name:         packageName,
		Filename:     filename,
		RemotePath:   packageListPath,
	})
	if err != nil || len(artifacts) == 0 {
		// Hosted/local fallback: uploaded files may not have a package-list
		// RemotePath, so this query is only for local aggregation.
		artifacts, err = repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "pypi",
			Name:         packageName,
			Filename:     filename,
		})
		if err != nil || len(artifacts) == 0 {
			http.Error(ctx.Writer, "not found", http.StatusNotFound)
			return nil
		}
	}

	var artifact *runtime.Artifact
	for _, candidate := range artifacts {
		if candidate.Filename == filename {
			artifact = candidate
			break
		}
	}
	if artifact == nil {
		http.Error(ctx.Writer, "not found", http.StatusNotFound)
		return nil
	}

	// 检查是否有存储的 metadata
	metadata := artifact.Attributes["metadata"]
	if metadata == "" {
		http.Error(ctx.Writer, "metadata not available", http.StatusNotFound)
		return nil
	}

	// 返回 metadata 内容
	ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	ctx.Writer.Header().Set("Content-Length", strconv.Itoa(len(metadata)))
	ctx.Writer.WriteHeader(http.StatusOK)

	if ctx.Request.Method == http.MethodHead {
		return nil
	}

	ctx.Writer.Write([]byte(metadata))
	return nil
}

func (p *PyPIPlugin) handleSimpleIndex(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
		return errors.New("method not allowed")
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		RemotePath:   path,
	})
	if err != nil && !errors.Is(err, runtime.ErrNotFound) {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// 对 hosted 仓库：RemotePath 查询可能无结果（本地上传的包没有 RemotePath="simple/"），
	// 尝试聚合所有已存储包的 Name。该查询只用于本地索引渲染，不作为 proxy 回源入口。
	if len(artifacts) == 0 {
		allArts, qErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "pypi",
		})
		if qErr == nil && len(allArts) > 0 {
			seen := make(map[string]bool)
			for _, a := range allArts {
				name := a.Name
				if name == "" {
					continue
				}
				if !seen[name] {
					seen[name] = true
					artifacts = append(artifacts, a)
				}
			}
		}
	}

	if len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	accept := ctx.Request.Header.Get("Accept")
	ctx.Writer.Header().Set("Vary", "Accept")
	if wantsPyPISimpleJSON(accept) {
		return p.writeSimpleIndexJSON(ctx, artifacts)
	}
	return p.writeSimpleIndexHTML(ctx, artifacts)
}

func (p *PyPIPlugin) writeSimpleIndexHTML(ctx *runtime.RequestContext, artifacts []*runtime.Artifact) error {
	seen := make(map[string]bool)
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n")
	for _, artifact := range artifacts {
		name := artifact.Name
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		escaped := html.EscapeString(name)
		sb.WriteString(`<a href="`)
		sb.WriteString(escaped)
		sb.WriteString(`/">`)
		sb.WriteString(escaped)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	output := sb.String()
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	ctx.Writer.Write([]byte(output))
	return nil
}

func (p *PyPIPlugin) writeSimpleIndexJSON(ctx *runtime.RequestContext, artifacts []*runtime.Artifact) error {
	seen := make(map[string]bool)
	projects := make([]map[string]string, 0)
	for _, artifact := range artifacts {
		name := artifact.Name
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		projects = append(projects, map[string]string{
			"name": name,
			"url":  name + "/",
		})
	}

	data := map[string]interface{}{
		"meta":     map[string]string{"api-version": "1.0"},
		"projects": projects,
	}

	ctx.Writer.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handlePackageList(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
		return errors.New("method not allowed")
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	packageName := normalizePackageName(parts[1])
	if parts[1] != packageName || !strings.HasSuffix(path, "/") {
		http.Redirect(ctx.Writer, ctx.Request, p.canonicalPackageListPath(ctx, packageName), http.StatusMovedPermanently)
		return nil
	}

	ctx.PackageName = packageName

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Name:         packageName,
		RemotePath:   path,
	})
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	accept := ctx.Request.Header.Get("Accept")
	ctx.Writer.Header().Set("Vary", "Accept")
	if wantsPyPISimpleJSON(accept) {
		return p.writePackageFilesJSON(ctx, packageName, artifacts)
	}
	return p.writePackageFilesHTML(ctx, packageName, artifacts)
}

func (p *PyPIPlugin) writePackageFilesHTML(ctx *runtime.RequestContext, packageName string, artifacts []*runtime.Artifact) error {
	artifacts = sortPyPIArtifactsByVersion(artifacts)
	var sb strings.Builder
	escapedPkg := html.EscapeString(packageName)
	sb.WriteString(fmt.Sprintf("<!DOCTYPE html>\n<html><head><title>Links for %s</title></head><body>\n<h1>Links for %s</h1>\n", escapedPkg, escapedPkg))

	for _, artifact := range artifacts {
		if artifact.Name != packageName {
			continue
		}
		filename := artifact.Filename
		remotePath := firstNonEmptyPyPI(artifact.RemotePath, artifact.Properties["remote_path"])
		if filename == "" || remotePath == "" {
			continue
		}
		// PEP 503: 包文件链接必须包含 #sha256= hash fragment
		hashFragment := ""
		if sha256 := artifactSHA256(artifact); sha256 != "" {
			hashFragment = "#sha256=" + sha256
		}
		// remote_path 已经包含 packages/ 前缀（如 packages/cc/15/.../file.whl）
		sb.WriteString(`<a href="../../`)
		sb.WriteString(html.EscapeString(remotePath))
		sb.WriteString(html.EscapeString(hashFragment))
		if requiresPython := artifact.Attributes["requires_python"]; requiresPython != "" {
			sb.WriteString(`" data-requires-python="`)
			sb.WriteString(html.EscapeString(requiresPython))
		}
		if yanked := pypiYankedValue(artifact); yanked != "" {
			sb.WriteString(`" data-yanked="`)
			sb.WriteString(html.EscapeString(yanked))
		}
		if artifact.Attributes["metadata"] != "" {
			sb.WriteString(`" data-dist-info-metadata="true"`)
		}
		sb.WriteString(`">`)
		sb.WriteString(html.EscapeString(filename))
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	output := sb.String()
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	ctx.Writer.Write([]byte(output))
	return nil
}

func (p *PyPIPlugin) canonicalPackageListPath(ctx *runtime.RequestContext, packageName string) string {
	prefix := strings.TrimSuffix(ctx.Request.URL.Path, ctx.RepositoryPath)
	if prefix == ctx.Request.URL.Path {
		return "/simple/" + packageName + "/"
	}
	return strings.TrimRight(prefix, "/") + "/simple/" + packageName + "/"
}

func (p *PyPIPlugin) writePackageFilesJSON(ctx *runtime.RequestContext, packageName string, artifacts []*runtime.Artifact) error {
	artifacts = sortPyPIArtifactsByVersion(artifacts)
	files := make([]map[string]interface{}, 0)
	for _, artifact := range artifacts {
		if artifact.Name != packageName {
			continue
		}
		filename := artifact.Filename
		remotePath := firstNonEmptyPyPI(artifact.RemotePath, artifact.Properties["remote_path"])
		if filename == "" || remotePath == "" {
			continue
		}

		file := map[string]interface{}{
			"url":      "../../" + remotePath,
			"filename": filename,
		}

		hashes := make(map[string]string)
		if sha256 := artifactSHA256(artifact); sha256 != "" {
			hashes["sha256"] = sha256
		}
		file["hashes"] = hashes
		if requiresPython := artifact.Attributes["requires_python"]; requiresPython != "" {
			file["requires-python"] = requiresPython
		}
		if yanked := pypiYankedValue(artifact); yanked != "" {
			file["yanked"] = yanked
		}
		// PEP 658: dist-info-metadata
		if artifact.Attributes["metadata"] != "" {
			metadataField := map[string]string{}
			if sha256 := artifactSHA256(artifact); sha256 != "" {
				metadataField["sha256"] = sha256
			}
			file["core-metadata"] = metadataField
			file["dist-info-metadata"] = metadataField
		}
		if artifact.SizeBytes > 0 {
			file["size"] = artifact.SizeBytes
		} else if len(artifact.BlobRefs) > 0 && artifact.BlobRefs[0].Size > 0 {
			file["size"] = artifact.BlobRefs[0].Size
		}
		if publishedAt := artifact.Attributes["published_at"]; publishedAt != "" {
			file["upload-time"] = publishedAt
		}

		files = append(files, file)
	}
	versions := uniquePyPIVersions(artifacts, packageName)

	data := map[string]interface{}{
		"meta":     map[string]string{"api-version": "1.1"},
		"name":     packageName,
		"versions": versions,
		"files":    files,
	}

	ctx.Writer.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handleFileMetadataRequest(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
		return errors.New("method not allowed")
	}

	artifactPath := strings.TrimSuffix(path, ".metadata")
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		RemotePath:   artifactPath,
	})
	if err != nil || len(artifacts) == 0 {
		http.Error(ctx.Writer, "not found", http.StatusNotFound)
		return nil
	}
	metadata := artifacts[0].Attributes["metadata"]
	if metadata == "" {
		http.Error(ctx.Writer, "metadata not available", http.StatusNotFound)
		return nil
	}
	ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	ctx.Writer.Header().Set("Content-Length", strconv.Itoa(len(metadata)))
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	_, _ = ctx.Writer.Write([]byte(metadata))
	return nil
}

func (p *PyPIPlugin) handlePackagesDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)
	dir := filepath.Dir(path) // e.g. "packages/62/35/0230421b8c4efad6624518028163329ad0c2df9e58e6b3bee013427bf8f6"

	if strings.HasSuffix(filename, ".sha256") {
		return p.handleChecksumRequest(ctx, repoRuntime, path)
	}

	packageName := p.extractPackageNameFromFilename(filename)
	version := p.extractVersionFromFilename(filename)

	ctx.PackageName = packageName
	ctx.Version = version
	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Kind:         runtime.KindArtifact,
		Name:         packageName,
		Version:      version,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
		Qualifiers: map[string]string{
			"package": packageName,
		},
	}

	switch ctx.Request.Method {
	case http.MethodGet, http.MethodHead:
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			if errors.Is(err, runtime.ErrNotFound) {
				artifacts, queryErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
					RepositoryID: ctx.Repository.ID,
					Format:       "pypi",
					RemotePath:   path,
				})
				if queryErr == nil && len(artifacts) > 0 {
					artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
				}
			}
		}
		if err != nil {
			util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
				"path":  path,
				"key":   key.String(),
				"error": err.Error(),
			}).Warn("pypi: handlePackagesDownload GetArtifact failed")
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if artifact.Content == nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "application/octet-stream", "attachment"); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
			return nil
		}
		return nil
	case http.MethodPut:
		return p.handleUpload(ctx, repoRuntime, key)
	}
	return errors.New("method not allowed")
}

func (p *PyPIPlugin) handleChecksumRequest(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	actualPath := strings.TrimSuffix(path, ".sha256")
	actualFilename := filepath.Base(actualPath)

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		RemotePath:   actualPath,
	})
	if err != nil || len(artifacts) == 0 {
		// Backward-compatible fallback for older metadata that did not store
		// remote_path. Exact path remains preferred to avoid same-filename
		// collisions across different hash directories. This local lookup is not
		// a proxy refetch path.
		artifacts, err = repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "pypi",
			Filename:     actualFilename,
		})
		if err != nil || len(artifacts) == 0 {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
	}

	artifact := artifacts[0]
	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	ctx.SizeBytes = artifact.SizeBytes

	sha256Digest := artifactSHA256(artifact)
	if sha256Digest == "" {
		http.Error(ctx.Writer, "No blob", http.StatusNotFound)
		return nil
	}
	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(sha256Digest + "\n"))
	return nil
}

func (p *PyPIPlugin) handleJsonAPI(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	jsonPath := strings.TrimPrefix(path, "pypi/")
	jsonPath = strings.TrimSuffix(jsonPath, "/json")
	parts := strings.Split(jsonPath, "/")

	packageName := normalizePackageName(parts[0])
	var version string
	if len(parts) > 1 {
		version = parts[1]
	}
	queryRemotePath := "simple/" + packageName + "/"

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		RemotePath:   queryRemotePath, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	releases := make(map[string][]map[string]interface{})
	artifacts = sortPyPIArtifactsByVersion(artifacts)
	for _, artifact := range artifacts {
		if artifact.Name != packageName {
			continue
		}
		v := artifact.Version
		if version != "" && v != version {
			continue
		}

		filename := artifact.Filename
		remotePath := firstNonEmptyPyPI(artifact.RemotePath, artifact.Properties["remote_path"])
		if filename == "" || remotePath == "" {
			continue
		}

		file := map[string]interface{}{
			"filename": filename,
			"url":      "../../" + remotePath,
		}

		if sha256 := artifactSHA256(artifact); sha256 != "" {
			file["digests"] = map[string]string{
				"sha256": sha256,
			}
		}
		if requiresPython := artifact.Attributes["requires_python"]; requiresPython != "" {
			file["requires_python"] = requiresPython
		}
		if yanked := pypiYankedValue(artifact); yanked != "" {
			file["yanked"] = yanked
		}
		if len(artifact.BlobRefs) > 0 {
			file["size"] = artifact.BlobRefs[0].Size
		} else if artifact.SizeBytes > 0 {
			file["size"] = artifact.SizeBytes
		}

		releases[v] = append(releases[v], file)
	}

	data := map[string]interface{}{
		"info": map[string]interface{}{
			"name":    packageName,
			"version": version,
		},
		"releases": releases,
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	if len(artifact.BlobRefs) == 0 {
		http.Error(ctx.Writer, "No blob available", http.StatusNotFound)
		return nil
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "No blob available", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	ctx.SizeBytes = artifact.SizeBytes

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, runtime.SanitizeFilename(key.Filename)))
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}

func (p *PyPIPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	packageName := p.extractPackageNameFromFilename(key.Filename)
	version := p.extractVersionFromFilename(key.Filename)

	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Filename:     key.Filename,
		Size:         ctx.Request.ContentLength,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	blobRef, err := session.PutBlob(ctx.Request.Context(), ctx.Request.Body)
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Kind:         runtime.KindArtifact,
		Name:         packageName,
		Version:      version,
		Filename:     key.Filename,
		RemotePath:   key.RemotePath,
		Qualifiers: map[string]string{
			"package": packageName,
		},
		BlobRefs: []runtime.BlobRef{blobRef},
		Properties: map[string]string{
			"filename": key.Filename,
		},
	})

	if err := session.PutArtifact(ctx.Request.Context(), artifact); err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if err := session.Commit(ctx.Request.Context()); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}

func (p *PyPIPlugin) handleLegacyUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	if ctx.Request.Method != http.MethodPost {
		return errors.New("method not allowed")
	}
	if err := ctx.Request.ParseMultipartForm(64 << 20); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	if action := ctx.Request.FormValue(":action"); action != "file_upload" {
		http.Error(ctx.Writer, "unsupported legacy upload action", http.StatusBadRequest)
		return nil
	}

	file, header, err := ctx.Request.FormFile("content")
	if err != nil {
		http.Error(ctx.Writer, "missing content file", http.StatusBadRequest)
		return nil
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	filename := runtime.SanitizeFilename(header.Filename)
	if filename == "" {
		http.Error(ctx.Writer, "missing filename", http.StatusBadRequest)
		return nil
	}
	packageName := normalizePackageName(firstNonEmptyPyPI(ctx.Request.FormValue("name"), p.extractPackageNameFromFilename(filename)))
	version := firstNonEmptyPyPI(ctx.Request.FormValue("version"), p.extractVersionFromFilename(filename))
	sum := sha256.Sum256(content)
	digest := fmt.Sprintf("%x", sum[:])
	if providedDigest := strings.TrimSpace(ctx.Request.FormValue("sha256_digest")); providedDigest != "" && providedDigest != digest {
		http.Error(ctx.Writer, "sha256_digest does not match uploaded content", http.StatusBadRequest)
		return nil
	}
	remotePath := pypiPackageRemotePath(digest, filename)

	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Filename:     filename,
		Size:         int64(len(content)),
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	blobRef, err := session.PutBlob(ctx.Request.Context(), bytes.NewReader(content))
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}
	blobRef.Size = int64(len(content))

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Kind:         runtime.KindArtifact,
		Name:         packageName,
		Version:      version,
		Path:         filepath.Dir(remotePath),
		Filename:     filename,
		RemotePath:   remotePath,
		SizeBytes:    int64(len(content)),
		BlobRefs:     []runtime.BlobRef{blobRef},
		Checksums:    map[string]string{"sha256": digest},
		Attributes: map[string]string{
			"artifact_type": "package-file",
			"filetype":      ctx.Request.FormValue("filetype"),
			"pyversion":     ctx.Request.FormValue("pyversion"),
		},
		Qualifiers: map[string]string{
			"package": packageName,
		},
		Properties: map[string]string{
			"filename":    filename,
			"remote_path": remotePath,
		},
	})

	if err := session.PutArtifact(ctx.Request.Context(), artifact); err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}
	if err := session.Commit(ctx.Request.Context()); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}

func pypiPackageRemotePath(sha256Digest, filename string) string {
	prefix := sha256Digest
	if len(prefix) < 4 {
		prefix = prefix + strings.Repeat("0", 4-len(prefix))
	}
	return "packages/" + prefix[:2] + "/" + prefix[2:4] + "/" + filename
}

func extractRelativePackagePath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return rawURL
	}
	path := strings.TrimPrefix(u.Path, "/")
	return path
}

func (p *PyPIPlugin) extractPackageNameFromFilename(filename string) string {
	if strings.HasSuffix(filename, ".whl") {
		name, _ := splitPEP427Wheel(strings.TrimSuffix(filename, ".whl"))
		return normalizePackageName(name)
	}

	// sdist: {name}-{version}.tar.gz  or  {name}-{version}.tar.bz2  or  {name}-{version}.zip
	base := filename
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	name, _ := splitSdistNameVersion(base)
	return normalizePackageName(name)
}

func (p *PyPIPlugin) extractVersionFromFilename(filename string) string {
	if strings.HasSuffix(filename, ".whl") {
		_, version := splitPEP427Wheel(strings.TrimSuffix(filename, ".whl"))
		return version
	}

	base := filename
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	_, version := splitSdistNameVersion(base)
	return version
}

// splitPEP427Wheel 从 wheel 文件名中提取 name 和 version。
// PEP 427 格式: {name}-{version}(-{build})?-{python}-{abi}-{platform}
func splitPEP427Wheel(namever string) (name, version string) {
	parts := strings.Split(namever, "-")
	if len(parts) < 5 {
		// 至少 5 段: name version python abi platform
		if len(parts) >= 2 {
			return parts[0], parts[1]
		}
		return namever, ""
	}
	// python=parts[len-3], abi=parts[len-2], platform=parts[len-1]
	// 可选 build=parts[len-4]，build tag 必须以数字开头。
	versionIdx := len(parts) - 4
	if len(parts) >= 6 && looksLikeWheelBuildTag(parts[len(parts)-4]) && strings.Contains(parts[len(parts)-5], ".") {
		versionIdx = len(parts) - 5
	}
	version = parts[versionIdx]
	name = strings.Join(parts[:versionIdx], "-")
	return name, version
}

func looksLikeWheelBuildTag(value string) bool {
	if !startsWithDigit(value) || strings.Contains(value, ".") {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func startsWithDigit(value string) bool {
	return value != "" && value[0] >= '0' && value[0] <= '9'
}

// splitSdistNameVersion 从 sdist 文件名中提取 name 和 version。
// 格式: {name}-{version}  (version 不含连字符)
func splitSdistNameVersion(namever string) (name, version string) {
	idx := strings.LastIndex(namever, "-")
	if idx < 0 {
		return namever, ""
	}
	return namever[:idx], namever[idx+1:]
}

func validatePyPIPath(path string) error {
	if path == "" {
		return nil
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid pypi path: path traversal not allowed")
	}
	dangerousPatterns := []string{"~", "$", "`", "|", ";", "&", "(", ")", "<", ">", "\n", "\r", "\x00"}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(path, pattern) {
			return fmt.Errorf("invalid pypi path: dangerous character not allowed")
		}
	}
	return nil
}

func sortPyPIArtifactsByVersion(artifacts []*runtime.Artifact) []*runtime.Artifact {
	out := append([]*runtime.Artifact(nil), artifacts...)
	sort.SliceStable(out, func(i, j int) bool {
		cmp := comparePEP440Versions(out[i].Version, out[j].Version)
		if cmp == 0 {
			return out[i].Filename < out[j].Filename
		}
		return cmp < 0
	})
	return out
}

func normalizePackageName(name string) string {
	// PEP 503: 将 [-_.]+ 归一化为单个 "-"，然后转小写
	return pypiNormalizeRe.ReplaceAllString(strings.ToLower(name), "-")
}

type pep440Version struct {
	epoch   int
	release []int
	preTag  string
	preNum  int
	devNum  int
	postNum int
	hasPre  bool
	hasDev  bool
	hasPost bool
}

func comparePEP440Versions(a, b string) int {
	va := parsePEP440Version(a)
	vb := parsePEP440Version(b)
	if va.epoch != vb.epoch {
		if va.epoch < vb.epoch {
			return -1
		}
		return 1
	}
	maxLen := len(va.release)
	if len(vb.release) > maxLen {
		maxLen = len(vb.release)
	}
	for i := 0; i < maxLen; i++ {
		ai, bi := 0, 0
		if i < len(va.release) {
			ai = va.release[i]
		}
		if i < len(vb.release) {
			bi = vb.release[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return comparePEP440Suffix(va, vb)
}

func parsePEP440Version(version string) pep440Version {
	v := strings.ToLower(strings.TrimSpace(version))
	v = strings.TrimPrefix(v, "v")
	parsed := pep440Version{}
	if bang := strings.Index(v, "!"); bang >= 0 {
		parsed.epoch = parseLeadingInt(v[:bang])
		v = v[bang+1:]
	}

	for _, marker := range []string{".dev", "dev"} {
		if idx := strings.Index(v, marker); idx >= 0 {
			parsed.hasDev = true
			parsed.devNum = parseLeadingInt(v[idx+len(marker):])
			v = v[:idx]
			break
		}
	}
	for _, marker := range []string{".post", "post"} {
		if idx := strings.Index(v, marker); idx >= 0 {
			parsed.hasPost = true
			parsed.postNum = parseLeadingInt(v[idx+len(marker):])
			v = v[:idx]
			break
		}
	}
	for _, marker := range []string{"a", "b", "rc"} {
		if idx := strings.Index(v, marker); idx > 0 {
			parsed.hasPre = true
			parsed.preTag = marker
			parsed.preNum = parseLeadingInt(v[idx+len(marker):])
			v = v[:idx]
			break
		}
	}
	for _, part := range strings.Split(v, ".") {
		parsed.release = append(parsed.release, parseLeadingInt(part))
	}
	return parsed
}

func comparePEP440Suffix(a, b pep440Version) int {
	if a.hasDev != b.hasDev {
		if a.hasDev {
			return -1
		}
		return 1
	}
	if a.hasDev && b.hasDev && a.devNum != b.devNum {
		if a.devNum < b.devNum {
			return -1
		}
		return 1
	}
	if a.hasPre != b.hasPre {
		if a.hasPre {
			return -1
		}
		return 1
	}
	if a.hasPre && b.hasPre {
		order := map[string]int{"a": 0, "b": 1, "rc": 2}
		if order[a.preTag] != order[b.preTag] {
			if order[a.preTag] < order[b.preTag] {
				return -1
			}
			return 1
		}
		if a.preNum != b.preNum {
			if a.preNum < b.preNum {
				return -1
			}
			return 1
		}
	}
	if a.hasPost != b.hasPost {
		if a.hasPost {
			return 1
		}
		return -1
	}
	if a.hasPost && b.hasPost && a.postNum != b.postNum {
		if a.postNum < b.postNum {
			return -1
		}
		return 1
	}
	return 0
}

func parseLeadingInt(value string) int {
	result := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		result = result*10 + int(r-'0')
	}
	return result
}

func isValidWheelFilename(filename string) bool {
	// PEP 427: {distribution}-{version}(-{build})?-{python}-{abi}-{platform}.whl
	// 标签可含字母数字及 . _ 等，放宽校验避免误杀合法文件名
	if !strings.HasSuffix(filename, ".whl") {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(filename, ".whl"), "-")
	return len(parts) >= 5 // name, version, python-tag, abi-tag, platform-tag
}

func isValidSdistFilename(filename string) bool {
	// PEP 625: {name}-{version}.tar.gz / .tar.bz2 / .zip
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(filename, ext) {
			return strings.Count(filename, "-") >= 1
		}
	}
	return false
}

func isValidPyPIFilename(filename string) bool {
	return isValidWheelFilename(filename) || isValidSdistFilename(filename)
}

func (p *PyPIPlugin) httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Moonlight-Registry/1.0")
	return p.httpClient.Do(req)
}

func (p *PyPIPlugin) fetchPyPIPackageInfo(ctx context.Context, remoteURL, packageName string) (map[string]interface{}, error) {
	jsonURL := strings.TrimRight(remoteURL, "/") + "/pypi/" + packageName + "/json"
	resp, err := p.httpGet(ctx, jsonURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pypi json api returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *PyPIPlugin) buildArtifactsFromJSONAPI(packageName string, info map[string]interface{}) []*runtime.Artifact {
	infoData, _ := info["info"].(map[string]interface{})
	var license, description, homepage string
	if infoData != nil {
		license = selectPyPILicense(infoData)
		if s, ok := infoData["summary"].(string); ok && s != "" {
			description = s
		}
		if h, ok := infoData["home_page"].(string); ok && h != "" {
			homepage = h
		}
	}
	releases, _ := info["releases"].(map[string]interface{})
	if releases == nil {
		return nil
	}
	var artifacts []*runtime.Artifact
	for version, filesRaw := range releases {
		files, ok := filesRaw.([]interface{})
		if !ok || len(files) == 0 {
			continue
		}
		earliestTime := ""
		for _, f := range files {
			file, ok := f.(map[string]interface{})
			if !ok {
				continue
			}
			filename, _ := file["filename"].(string)
			if filename == "" {
				continue
			}
			rawURL, _ := file["url"].(string)
			if rawURL == "" {
				rawURL = "packages/" + string([]rune(filename)[0:1]) + "/" + packageName + "/" + filename
			}
			remotePath := extractRelativePackagePath(rawURL)
			dir := filepath.Dir(remotePath)
			props := map[string]string{
				"remote_path": remotePath,
			}
			checksums := map[string]string{}
			if digests, ok := file["digests"].(map[string]interface{}); ok {
				if sha256, ok := digests["sha256"].(string); ok && sha256 != "" {
					checksums["sha256"] = sha256
				}
			}
			sizeBytes := parsePyPIFileSize(file["size"])
			attrs := map[string]string{"artifact_type": "package-file"}
			// 存储 PyPI 原始下载 URL，供 ensureArtifactBlob 回源时使用。
			// PyPI 文件下载域名(files.pythonhosted.org)与 Simple API 域名(pypi.org/simple/)不同，
			// buildRemoteURL 拼出的 URL 对 PyPI 不可用，必须使用上游返回的真实 URL。
			if parsed, err := url.Parse(rawURL); err == nil && parsed.IsAbs() {
				props["download_url"] = rawURL
			}
			if license != "" {
				attrs["license"] = license
			}
			if description != "" {
				attrs["description"] = description
			}
			if homepage != "" {
				attrs["homepage"] = homepage
			}
			if requiresPython, ok := file["requires_python"].(string); ok && requiresPython != "" {
				attrs["requires_python"] = requiresPython
			}
			if yanked, ok := file["yanked"].(bool); ok && yanked {
				attrs["yanked"] = "true"
			}
			if yankedReason, ok := file["yanked_reason"].(string); ok && yankedReason != "" {
				attrs["yanked"] = "true"
				attrs["yanked_reason"] = yankedReason
			}
			if uploadTime, ok := file["upload_time"].(string); ok && uploadTime != "" {
				if t, err := parsePyPITime(uploadTime); err == nil {
					parsed := t.Format(time.RFC3339)
					if earliestTime == "" || parsed < earliestTime {
						earliestTime = parsed
					}
				}
			}
			artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
				Format:      "pypi",
				Kind:        runtime.KindArtifact,
				Name:        packageName,
				Version:     version,
				Path:        dir,
				Filename:    filename,
				RemotePath:  props["remote_path"],
				DownloadURL: props["download_url"],
				SizeBytes:   sizeBytes,
				Checksums:   checksums,
				Attributes:  attrs,
				Qualifiers: map[string]string{
					"package": packageName,
				},
				Properties: props,
			}))
		}
		if earliestTime != "" {
			for _, a := range artifacts {
				if a.Version == version && a.Attributes["published_at"] == "" {
					a.Attributes["published_at"] = earliestTime
				}
			}
		}
	}
	return artifacts
}

func artifactSHA256(artifact *runtime.Artifact) string {
	if artifact == nil {
		return ""
	}
	if artifact.Checksums != nil && artifact.Checksums["sha256"] != "" {
		return artifact.Checksums["sha256"]
	}
	for _, blobRef := range artifact.BlobRefs {
		if blobRef.Algorithm == "sha256" && blobRef.Digest != "" {
			return blobRef.Digest
		}
	}
	return ""
}

func parsePyPIFileSize(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

func (p *PyPIPlugin) extractStringField(info map[string]interface{}, key string) string {
	if v, ok := info[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func selectPyPILicense(info map[string]interface{}) string {
	if info == nil {
		return ""
	}
	if expr, ok := info["license_expression"].(string); ok {
		if normalized := normalizePyPILicense(expr); normalized != "" {
			return normalized
		}
	}
	if raw, ok := info["license"].(string); ok {
		if normalized := normalizePyPILicense(raw); normalized != "" {
			return normalized
		}
	}
	if classifiers, ok := info["classifiers"].([]interface{}); ok {
		for _, item := range classifiers {
			classifier, ok := item.(string)
			if !ok || !strings.HasPrefix(classifier, "License ::") {
				continue
			}
			parts := strings.Split(classifier, "::")
			if len(parts) == 0 {
				continue
			}
			if normalized := normalizePyPILicense(parts[len(parts)-1]); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func normalizePyPILicense(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "unknown", "none", "n/a", "na", "not specified":
		return ""
	}
	return value
}

func (p *PyPIPlugin) mergePackageInfo(artifacts []*runtime.Artifact, info map[string]interface{}) {
	infoData, _ := info["info"].(map[string]interface{})
	releases, _ := info["releases"].(map[string]interface{})

	var license, description, homepage string
	if infoData != nil {
		license = selectPyPILicense(infoData)
		if s, ok := infoData["summary"].(string); ok && s != "" {
			description = s
		}
		if h, ok := infoData["home_page"].(string); ok && h != "" {
			homepage = h
		}
	}

	versionUploadTimes := make(map[string]string)
	if releases != nil {
		for version, files := range releases {
			filesList, ok := files.([]interface{})
			if !ok || len(filesList) == 0 {
				continue
			}
			var earliest time.Time
			found := false
			for _, f := range filesList {
				fileMap, ok := f.(map[string]interface{})
				if !ok {
					continue
				}
				uploadTime, ok := fileMap["upload_time"].(string)
				if !ok || uploadTime == "" {
					continue
				}
				t, err := parsePyPITime(uploadTime)
				if err != nil {
					continue
				}
				if !found || t.Before(earliest) {
					earliest = t
					found = true
				}
			}
			if found {
				versionUploadTimes[version] = earliest.Format(time.RFC3339)
			}
		}
	}

	for _, artifact := range artifacts {
		if artifact.Attributes == nil {
			artifact.Attributes = make(map[string]string)
		}
		if license != "" {
			artifact.Attributes["license"] = license
		}
		if description != "" {
			artifact.Attributes["description"] = description
		}
		if homepage != "" {
			artifact.Attributes["homepage"] = homepage
		}
		version := artifact.Version
		if version != "" {
			if uploadTime, ok := versionUploadTimes[version]; ok {
				artifact.Attributes["published_at"] = uploadTime
			}
		}
	}
}

func parsePyPITime(s string) (time.Time, error) {
	// 支持多种 ISO 8601 时间格式
	layouts := []string{
		time.RFC3339,                  // 2006-01-02T15:04:05Z07:00
		time.RFC3339Nano,              // 2006-01-02T15:04:05.999999999Z07:00
		"2006-01-02T15:04:05.999999Z", // PyPI 标准：6 位小数
		"2006-01-02T15:04:05.99999Z",  // 5 位小数
		"2006-01-02T15:04:05.9999Z",   // 4 位小数
		"2006-01-02T15:04:05.999Z",    // 3 位小数
		"2006-01-02T15:04:05.99Z",     // 2 位小数（非标准但可能存在）
		"2006-01-02T15:04:05.9Z",      // 1 位小数
		"2006-01-02T15:04:05Z",        // 无小数
		"2006-01-02T15:04:05",         // 无时区信息
		"2006-01-02 15:04:05",         // 空格分隔
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}

// parseSimpleIndex 解析 PyPI Simple Index 页面，提取包名列表
func (p *PyPIPlugin) parseSimpleIndex(body io.Reader) ([]*runtime.Artifact, error) {
	anchors, err := parsePyPIAnchors(body)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var artifacts []*runtime.Artifact
	for _, anchor := range anchors {
		pkgName := normalizePackageName(strings.TrimSpace(anchor.text))
		if pkgName == "" || seen[pkgName] {
			continue
		}
		seen[pkgName] = true
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format: "pypi",
			Kind:   runtime.KindMetadata,
			Name:   pkgName,
			Qualifiers: map[string]string{
				"package": pkgName,
			},
		}))
	}
	return artifacts, nil
}

// parsePackageList 解析 PyPI 包版本列表页面，提取文件列表
func (p *PyPIPlugin) parsePackageList(packageName string, body io.Reader) ([]*runtime.Artifact, error) {
	anchors, err := parsePyPIAnchors(body)
	if err != nil {
		return nil, err
	}
	var artifacts []*runtime.Artifact
	for _, anchor := range anchors {
		remotePath, downloadURL, fragment := pypiRemotePathFromHref(anchor.attrs["href"])
		if remotePath == "" {
			continue
		}
		filename := filepath.Base(remotePath)
		if !isValidPyPIFilename(filename) {
			continue
		}
		version := p.extractVersionFromFilename(filename)
		dir := filepath.Dir(remotePath)
		props := map[string]string{
			"remote_path": remotePath,
		}
		if downloadURL != "" {
			props["download_url"] = downloadURL
		}
		checksums := map[string]string{}
		if sha256 := extractPyPISHA256Fragment(fragment); sha256 != "" {
			checksums["sha256"] = sha256
		}
		attrs := map[string]string{"artifact_type": "package-file"}
		if requiresPython := anchor.attrs["data-requires-python"]; requiresPython != "" {
			attrs["requires_python"] = requiresPython
		}
		if yanked, ok := anchor.attrs["data-yanked"]; ok {
			attrs["yanked"] = "true"
			if yanked != "" {
				attrs["yanked_reason"] = yanked
			}
		}
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:      "pypi",
			Kind:        runtime.KindArtifact,
			Name:        packageName,
			Version:     version,
			Path:        dir,
			Filename:    filename,
			RemotePath:  remotePath,
			DownloadURL: props["download_url"],
			Checksums:   checksums,
			Attributes:  attrs,
			Qualifiers: map[string]string{
				"package": packageName,
			},
			Properties: props,
		}))
	}
	return artifacts, nil
}

func firstNonEmptyPyPI(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func uniquePyPIVersions(artifacts []*runtime.Artifact, packageName string) []string {
	seen := make(map[string]bool)
	versions := make([]string, 0)
	for _, artifact := range artifacts {
		if artifact.Name != packageName || artifact.Version == "" || seen[artifact.Version] {
			continue
		}
		seen[artifact.Version] = true
		versions = append(versions, artifact.Version)
	}
	return versions
}

type pypiAnchor struct {
	attrs map[string]string
	text  string
}

func parsePyPIAnchors(body io.Reader) ([]pypiAnchor, error) {
	doc, err := nethtml.Parse(body)
	if err != nil {
		return nil, err
	}
	var anchors []pypiAnchor
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && strings.EqualFold(n.Data, "a") {
			attrs := make(map[string]string)
			for _, attr := range n.Attr {
				attrs[strings.ToLower(attr.Key)] = attr.Val
			}
			anchors = append(anchors, pypiAnchor{
				attrs: attrs,
				text:  strings.TrimSpace(pypiNodeText(n)),
			})
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return anchors, nil
}

func pypiNodeText(n *nethtml.Node) string {
	var sb strings.Builder
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func pypiRemotePathFromHref(href string) (remotePath, downloadURL, fragment string) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", "", ""
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return "", "", ""
	}
	fragment = parsed.RawFragment
	if fragment == "" {
		fragment = parsed.Fragment
	}
	cleanURL := *parsed
	cleanURL.Fragment = ""
	cleanURL.RawFragment = ""
	if parsed.IsAbs() {
		downloadURL = cleanURL.String()
	}
	remotePath = cleanPyPIHrefPath(parsed.Path)
	return remotePath, downloadURL, fragment
}

func cleanPyPIHrefPath(path string) string {
	cleaned := strings.TrimPrefix(urlpath.Clean("/"+path), "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func wantsPyPISimpleJSON(accept string) bool {
	if accept == "" || accept == "*/*" {
		return false
	}
	bestQ := -1.0
	bestWantsJSON := false
	for _, part := range strings.Split(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		q := 1.0
		if rawQ := params["q"]; rawQ != "" {
			parsedQ, err := strconv.ParseFloat(rawQ, 64)
			if err != nil {
				continue
			}
			q = parsedQ
		}
		if q <= 0 || q <= bestQ {
			continue
		}
		switch mediaType {
		case "application/vnd.pypi.simple.v1+json", "application/vnd.pypi.simple+json", "application/json":
			bestQ = q
			bestWantsJSON = true
		case "application/vnd.pypi.simple.v1+html", "application/vnd.pypi.simple+html", "text/html":
			bestQ = q
			bestWantsJSON = false
		}
	}
	return bestWantsJSON
}

func pypiYankedValue(artifact *runtime.Artifact) string {
	if artifact == nil || artifact.Attributes["yanked"] != "true" {
		return ""
	}
	if reason := artifact.Attributes["yanked_reason"]; reason != "" {
		return reason
	}
	return "true"
}

func extractPyPISHA256Fragment(fragment string) string {
	values, err := url.ParseQuery(fragment)
	if err != nil {
		return ""
	}
	return values.Get("sha256")
}
