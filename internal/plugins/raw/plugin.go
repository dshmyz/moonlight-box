// Package raw implements a generic file storage plugin (no protocol-specific handling).
//
// # Generic/Raw 文件存储要点
//
// ## 特点
//   - 无协议特定的元数据或索引文件
//   - 直接存储和检索任意文件
//   - 支持目录结构（通过路径）
//
// ## 路径处理
//   - 上传: PUT /repository/{repo}/{path/to/file.ext}
//   - 下载: GET /repository/{repo}/{path/to/file.ext}
//   - 删除: DELETE /repository/{repo}/{path/to/file.ext}
//
// ## 目录列表
//   - 当请求路径以 / 结尾时，尝试返回目录列表
//   - 解析上游 HTML 响应提取文件链接
//   - 支持简单的浏览功能
//
// ## 关键实现点
//   - 无需解析包名、版本等元数据
//   - Content-Type: application/octet-stream
//   - 支持大文件流式传输
//   - 目录列表解析需处理多种 HTML 格式
//
// ## 适用场景
//   - 静态资源托管
//   - 二进制文件分发
//   - 任意文件存储
package raw

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

type GenericPlugin struct {
	httpClient *http.Client
}

func NewGenericPlugin(httpClient *http.Client) *GenericPlugin {
	if httpClient == nil {
		panic("generic: httpClient is required")
	}
	return &GenericPlugin{httpClient: httpClient}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *GenericPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote generic/raw protocol interaction.
// It performs a simple directory listing or file fetch from the remote repository.
func (p *GenericPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("generic: empty path")
	}

	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path

	logrus.WithFields(logrus.Fields{
		"remote_url": remoteURL,
		"path":      path,
		"full_url":   fullURL,
	}).Debug("generic: FetchRemote called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("full_url", fullURL).Error("generic: create request failed")
		return nil, fmt.Errorf("generic: create request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"full_url":  fullURL,
			"duration_ms": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("generic: HTTP request failed")
		return nil, fmt.Errorf("generic: fetch from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, runtime.ErrNotFound
		}
		logrus.WithFields(logrus.Fields{
			"full_url":    fullURL,
			"status_code": resp.StatusCode,
			"duration_ms":   time.Since(start).Seconds(),
		}).Error("generic: HTTP request returned non-200 status")
		return nil, fmt.Errorf("generic: fetch from %s: status %d", fullURL, resp.StatusCode)
	}

	// If the response is HTML (directory listing), attempt to parse links.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		artifacts, err := p.parseDirectoryListing(path, resp.Body)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"full_url":  fullURL,
				"duration_ms": time.Since(start).Seconds(),
				"error":    err.Error(),
			}).Error("generic: parse directory listing failed")
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"full_url":     fullURL,
			"itemCount":   len(artifacts),
			"contentType": contentType,
			"duration_ms":    time.Since(start).Seconds(),
		}).Debug("generic: FetchRemote directory listing success")
		return artifacts, nil
	}

	// For non-HTML responses (direct file download), return a single artifact.
	filename := filepath.Base(path)
	dir := filepath.Dir(path)
	if dir == "." {
		dir = ""
	}

	logrus.WithFields(logrus.Fields{
		"full_url":     fullURL,
		"filename":    filename,
		"contentType": contentType,
		"duration_ms":    time.Since(start).Seconds(),
	}).Debug("generic: FetchRemote file success")
	return []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "generic",
			Kind:       "file",
			Name:       filename,
			Path:       dir,
			Filename:   filename,
			RemotePath: strings.TrimPrefix(filepath.ToSlash(filepath.Join(dir, filename)), "/"),
		}),
	}, nil
}

// parseDirectoryListing attempts to extract file links from an HTML directory listing.
func (p *GenericPlugin) parseDirectoryListing(basePath string, body io.Reader) ([]*runtime.Artifact, error) {
	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("generic: read directory listing: %w", err)
	}
	html := string(htmlBytes)

	// Match href attributes in anchor tags, a common pattern for directory listings.
	// This handles both auto-index pages (nginx, apache) and simple HTML listings.
	var artifacts []*runtime.Artifact
	seen := make(map[string]bool)
	// Simple regex-free approach: find href="..." patterns in <a> tags.
	for _, segment := range strings.Split(html, `href="`) {
		if len(segment) == 0 {
			continue
		}
		endIdx := strings.Index(segment, `"`)
		if endIdx <= 0 {
			continue
		}
		href := segment[:endIdx]
		// Skip parent directory links, query strings, and absolute URLs.
		if href == "../" || href == "/" || strings.HasPrefix(href, "?") || strings.HasPrefix(href, "http") {
			continue
		}
		name := strings.TrimRight(href, "/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		isDir := strings.HasSuffix(href, "/")
		kind := "file"
		if isDir {
			kind = "directory"
		}
		dir := filepath.Dir(basePath)
		if dir == "." {
			dir = ""
		}
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "generic",
			Kind:       kind,
			Name:       name,
			Path:       dir,
			Filename:   name,
			RemotePath: strings.TrimPrefix(filepath.ToSlash(filepath.Join(dir, name)), "/"),
		}))
	}
	return artifacts, nil
}

func (p *GenericPlugin) Name() string {
	return "generic"
}

func (p *GenericPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if path == "" || path == "/" {
		return p.handleRootListing(ctx, repoRuntime)
	}

	// 规范化路径，拒绝任何尝试逃逸仓库根的输入
	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) || strings.Contains(path, "\\") {
		http.Error(ctx.Writer, "invalid path: path traversal not allowed", http.StatusBadRequest)
		return nil
	}
	// 将内部 ".." 段清理掉（filepath.Clean 已解析），但保留原始的结尾斜杠判断
	path = cleanPath
	// 重新补上原始结尾斜杠（已被 Clean 去掉），用于下方目录列表判断
	if strings.HasSuffix(ctx.RepositoryPath, "/") && path != "" {
		path = path + "/"
	}

	if strings.HasSuffix(path, "/") {
		return p.handleDirectoryListing(ctx, repoRuntime, path)
	}

	filename := filepath.Base(path)
	dir := filepath.Dir(path)
	if dir == "." {
		dir = ""
	}

	ctx.PackageName = filename
	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
		Extension:    filepath.Ext(filename),
	}

	switch ctx.Request.Method {
	case http.MethodGet, http.MethodHead:
		return p.handleDownload(ctx, repoRuntime, key)
	case http.MethodPut:
		return p.handleUpload(ctx, repoRuntime, key)
	case http.MethodDelete:
		return p.handleDelete(ctx, repoRuntime, key)
	}
	return errors.New("method not allowed")
}

func (p *GenericPlugin) handleRootListing(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	return p.handleDirectoryListing(ctx, repoRuntime, "/")
}

func (p *GenericPlugin) handleDirectoryListing(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	queryPath := path
	if queryPath == "/" {
		queryPath = ""
	}
	var artifacts []*runtime.Artifact
	if queryPath == "" {
		all, qErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "generic",
		})
		if qErr == nil {
			artifacts = all
		}
	} else {
		all, qErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID:     ctx.Repository.ID,
			Format:           "generic",
			RemotePathPrefix: queryPath,
		})
		if qErr == nil {
			artifacts = all
		}
	}
	if len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Index of ")
	sb.WriteString(html.EscapeString(path))
	sb.WriteString("</title></head><body>\n<h1>Index of ")
	sb.WriteString(html.EscapeString(path))
	sb.WriteString("</h1>\n")
	for _, artifact := range artifacts {
		name := artifact.Name
		if name == "" {
			name = artifact.Filename
		}
		if name == "" {
			continue
		}
		href := name
		if artifact.Kind == "directory" && !strings.HasSuffix(href, "/") {
			href += "/"
		}
		sb.WriteString(`<a href="`)
		sb.WriteString(html.EscapeString(href))
		sb.WriteString(`">`)
		sb.WriteString(html.EscapeString(name))
		sb.WriteString("</a><br>\n")
	}
	sb.WriteString("</body></html>")
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	_, _ = ctx.Writer.Write([]byte(sb.String()))
	return nil
}

func (p *GenericPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		// 其他错误（含 ErrBlocked）交给 router 处理
		return err
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	ctx.SizeBytes = artifact.SizeBytes

	contentType := "application/octet-stream"
	ext := strings.ToLower(key.Extension)
	switch ext {
	case ".txt":
		contentType = "text/plain"
	case ".json":
		contentType = "application/json"
	case ".xml":
		contentType = "application/xml"
	case ".zip":
		contentType = "application/zip"
	}

	if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, contentType, "inline"); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}

func (p *GenericPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	// 上传大小限制由全局中间件 middleware.BodySizeLimit 统一控制
	// （配置项 server.max_upload_size，默认 200MB，可通过配置调整）

	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
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
		// 全局 BodySizeLimit 触发的是 *http.MaxBytesError，返回 413
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(ctx.Writer, "file too large", http.StatusRequestEntityTooLarge)
			return nil
		}
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Kind:         "file",
		Name:         key.Name,
		Path:         key.Path,
		Filename:     key.Filename,
		RemotePath:   key.RemotePath,
		BlobRefs:     []runtime.BlobRef{blobRef},
		Properties: map[string]string{
			"filename": key.Filename,
			"path":     key.Path,
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

func (p *GenericPlugin) handleDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	err := repoRuntime.DeleteArtifact(ctx.Request.Context(), key)
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrNotFound):
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		case errors.Is(err, runtime.ErrReadOnly):
			http.Error(ctx.Writer, "Repository is read only", http.StatusMethodNotAllowed)
		default:
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	ctx.Writer.WriteHeader(http.StatusNoContent)
	return nil
}
