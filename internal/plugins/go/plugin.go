// Package gomod implements the Go module proxy protocol plugin.
//
// # Go Module Proxy 协议要点
//
// ## 端点格式
//   - 版本列表: $GOPROXY/{module}/@v/list -> 每行一个版本号
//   - 最新版本: $GOPROXY/{module}/@latest -> JSON: {"Version": "v1.2.3", "Time": "..."}
//   - 版本信息: $GOPROXY/{module}/@v/{version}.info -> JSON: {"Version": "...", "Time": "..."}
//   - go.mod: $GOPROXY/{module}/@v/{version}.mod -> go.mod 文件内容
//   - 源码: $GOPROXY/{module}/@v/{version}.zip -> zip 归档
//
// ## 大写字母编码
//   - 客户端请求: 大写字母编码为 !a-!z（如 Azure -> !azure）
//   - 服务端处理: 必须解码 !a->A, !b->B 等回大写字母
//   - 向上游请求: 必须编码大写字母为 !x 格式
//   - 例如: github.com/Azure/... -> github.com/!azure/...
//
// ## .info 文件
//   - Time 字段必须是该版本的实际发布时间（commit time）
//   - 不能使用 time.Now()，否则会破坏客户端缓存
//
// ## +incompatible 版本
//   - 表示未遵循语义导入版本控制的模块
//   - 在 @v/list 中正常列出
//   - @latest 可以返回 +incompatible 版本
//
// ## 关键实现点
//   - decodeGoPath: 在 Handle 入口解码 !x -> 大写字母
//   - encodeGoPath: 向上游请求时编码大写字母 -> !x
//   - .info/.mod/.zip 统一走 GetArtifact，不再动态生成
//   - 并发获取 info 使用 semaphore 限制并发数
//
// ## 参考规范
//   - https://go.dev/ref/mod#module-proxy
//   - https://proxy.golang.org/
package gomod

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"
)

type GoPlugin struct {
	httpClient *http.Client
}

func NewGoPlugin() *GoPlugin {
	return &GoPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *GoPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote Go protocol interaction.
func (p *GoPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("go: empty module path")
	}

	// Determine the remote fetch strategy based on the path suffix.
	if strings.HasSuffix(path, "/@v/list") {
		return p.fetchVersionList(ctx, remoteURL, path)
	}
	if strings.HasSuffix(path, "/@latest") {
		return p.fetchLatest(ctx, remoteURL, path)
	}
	// For other paths (e.g. /@v/*.info, *.mod, *.zip), return a basic artifact indicating the resource exists.
	modulePath, filename := p.splitModulePath(path)
	return []*runtime.Artifact{
		{
			Format: "go",
			Kind:   "module-file",
			Coordinates: map[string]string{
				"module":   modulePath,
				"name":     modulePath,
				"version":  strings.TrimSuffix(filename, filepath.Ext(filename)),
				"path":     modulePath + "/@v",
				"filename": filename,
			},
		},
	}, nil
}

// fetchVersionList fetches the @v/list endpoint from a Go module proxy
// and parses the line-separated version list.
func (p *GoPlugin) fetchVersionList(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	modulePath := strings.TrimSuffix(path, "/@v/list")
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodeGoPath(path)

	logrus.WithFields(logrus.Fields{
		"remoteURL":  remoteURL,
		"path":       path,
		"modulePath": modulePath,
		"fullURL":    fullURL,
	}).Debug("go: fetchVersionList called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).Error("go: create request for version list failed")
		return nil, fmt.Errorf("go: create request for version list: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL": fullURL,
			"error":   err.Error(),
		}).Error("go: fetch version list HTTP request failed")
		return nil, fmt.Errorf("go: fetch version list from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
		}).Error("go: fetch version list returned non-200 status")
		return nil, fmt.Errorf("go: fetch version list from %s: status %d", fullURL, resp.StatusCode)
	}

	var artifacts []*runtime.Artifact
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		version := strings.TrimSpace(scanner.Text())
		if version == "" {
			continue
		}
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "go",
			Kind:   "version",
			Coordinates: map[string]string{
				"module":  modulePath,
				"version": version,
			},
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("go: scan version list: %w", err)
	}
	logrus.WithFields(logrus.Fields{
		"modulePath":   modulePath,
		"versionCount": len(artifacts),
	}).Debug("go: fetchVersionList success")

	const maxInfoFetches = 10
	infoStart := len(artifacts) - maxInfoFetches
	if infoStart < 0 {
		infoStart = 0
	}

	const maxConcurrentInfo = 3
	sem := make(chan struct{}, maxConcurrentInfo)
	var wg sync.WaitGroup
	for _, a := range artifacts[infoStart:] {
		wg.Add(1)
		go func(art *runtime.Artifact) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info, err := p.fetchVersionInfo(ctx, remoteURL, art.Coordinates["module"], art.Coordinates["version"])
			if err == nil && info.Time != "" {
				art.Properties = map[string]string{"published_at": info.Time}
			}
		}(a)
	}
	wg.Wait()

	return artifacts, nil
}

func (p *GoPlugin) fetchVersionInfo(ctx context.Context, remoteURL, modulePath, version string) (struct {
	Version string
	Time    string
}, error) {
	infoPath := modulePath + "/@v/" + version + ".info"
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodeGoPath(infoPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return struct {
			Version string
			Time    string
		}{}, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return struct {
			Version string
			Time    string
		}{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return struct {
			Version string
			Time    string
		}{}, fmt.Errorf("go: fetch version info from %s: status %d", fullURL, resp.StatusCode)
	}

	var result struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return struct {
			Version string
			Time    string
		}{}, err
	}
	return struct {
		Version string
		Time    string
	}{Version: result.Version, Time: result.Time}, nil
}

// fetchLatest fetches the @latest endpoint from a Go module proxy
// and parses the JSON response.
func (p *GoPlugin) fetchLatest(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	modulePath := strings.TrimSuffix(path, "/@latest")
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodeGoPath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("go: create request for @latest: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("go: fetch @latest from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go: fetch @latest from %s: status %d", fullURL, resp.StatusCode)
	}

	var result struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("go: decode @latest response: %w", err)
	}
	if result.Version == "" {
		return nil, nil
	}
	return []*runtime.Artifact{
		{
			Format: "go",
			Kind:   "version",
			Coordinates: map[string]string{
				"module":  modulePath,
				"version": result.Version,
			},
			Properties: map[string]string{
				"time": result.Time,
			},
		},
	}, nil
}

// splitModulePath splits a Go module path like "github.com/foo/bar/@v/v1.0.0.zip"
// into the module path and the filename portion.
func (p *GoPlugin) splitModulePath(path string) (modulePath, filename string) {
	if idx := strings.Index(path, "/@v/"); idx >= 0 {
		return path[:idx], path[idx+4:]
	}
	if idx := strings.Index(path, "/@latest"); idx >= 0 {
		return path[:idx], "@latest"
	}
	return path, ""
}

// encodeGoPath encodes uppercase letters per Go module proxy spec:
// A → !a, B → !b, ..., Z → !z. Required for fetching from upstream proxies
// like proxy.golang.org when module paths contain uppercase letters.
func encodeGoPath(path string) string {
	var b strings.Builder
	b.Grow(len(path) + 8)
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteByte(byte(r) + 32) // to lowercase
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// decodeGoPath decodes the Go module proxy path encoding:
// !a → A, !b → B, ..., !z → Z. Required for parsing client requests
// where the Go CLI encodes uppercase letters in module paths.
func decodeGoPath(path string) string {
	var b strings.Builder
	b.Grow(len(path))
	i := 0
	for i < len(path) {
		if path[i] == '!' && i+1 < len(path) && path[i+1] >= 'a' && path[i+1] <= 'z' {
			b.WriteByte(path[i+1] - 32) // lowercase → uppercase
			i += 2
		} else {
			b.WriteByte(path[i])
			i++
		}
	}
	return b.String()
}

func (p *GoPlugin) Name() string {
	return "go"
}

func (p *GoPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")
	// 解码 Go module proxy 路径编码：!a → A, !b → B, ..., !z → Z
	// Go CLI 发送请求时会将大写字母编码为 !x 格式
	path = decodeGoPath(path)

	if strings.HasSuffix(path, "/@latest") {
		return p.handleLatest(ctx, repoRuntime, path)
	}

	if strings.HasSuffix(path, "/@v/list") {
		return p.handleVersionList(ctx, repoRuntime, path)
	}

	if strings.Contains(path, "/@v/") {
		return p.handleModuleDownload(ctx, repoRuntime, path)
	}

	return errors.New("invalid go module path")
}

func (p *GoPlugin) handleLatest(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@latest")

	ctx.PackageName = modulePath
	ctx.Filename = "@latest"

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
		Coordinates: map[string]string{
			"module": modulePath,
		},
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	latest := p.selectLatestStableVersion(artifacts)
	if latest == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, `{"Version":"%s"}`, latest.Coordinates["version"])
	return nil
}

// selectLatestStableVersion selects the highest stable semver version from the list,
// filtering out pre-release versions (e.g. v2.0.0-rc1). Requires versions with
// valid semver format (with 'v' prefix) to participate in comparison.
// Returns nil if no stable version exists.
func (p *GoPlugin) selectLatestStableVersion(artifacts []*runtime.Artifact) *runtime.Artifact {
	var best *runtime.Artifact
	for _, a := range artifacts {
		v := a.Coordinates["version"]
		if v == "" {
			continue
		}
		if !semver.IsValid(v) {
			continue
		}
		if semver.Prerelease(v) != "" {
			continue
		}
		if best == nil || semver.Compare(v, best.Coordinates["version"]) > 0 {
			best = a
		}
	}
	return best
}

func (p *GoPlugin) handleVersionList(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@v/list")

	ctx.PackageName = modulePath
	ctx.Filename = "list"

	logrus.WithFields(logrus.Fields{
		"repository":   ctx.Repository.Name,
		"repositoryID": ctx.Repository.ID,
		"path":         path,
		"modulePath":   modulePath,
		"repoType":     ctx.Repository.Type,
	}).Debug("go: handleVersionList called")

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		RemotePath:   path,
		Coordinates: map[string]string{
			"module": modulePath,
		},
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"modulePath": modulePath,
			"error":      err.Error(),
		}).Error("go: QueryArtifacts failed in handleVersionList")
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"modulePath":    modulePath,
		"artifactCount": len(artifacts),
	}).Debug("go: handleVersionList got artifacts")

	var sb strings.Builder
	seen := make(map[string]bool)
	for _, artifact := range artifacts {
		version := artifact.Coordinates["version"]
		if version != "" && !seen[version] {
			seen[version] = true
			sb.WriteString(version + "\n")
		}
	}

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(sb.String()))
	return nil
}

func (p *GoPlugin) handleModuleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/@v/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}

	modulePath := parts[0]
	filename := parts[1]

	// Determine file type and clean version.
	var fileType string
	cleanVersion := filename
	switch {
	case strings.HasSuffix(filename, ".info"):
		fileType = "info"
		cleanVersion = strings.TrimSuffix(filename, ".info")
	case strings.HasSuffix(filename, ".mod"):
		fileType = "mod"
		cleanVersion = strings.TrimSuffix(filename, ".mod")
	case strings.HasSuffix(filename, ".zip"):
		fileType = "zip"
		cleanVersion = strings.TrimSuffix(filename, ".zip")
	}

	ctx.PackageName = modulePath
	ctx.Version = cleanVersion
	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		Coordinates: map[string]string{
			"name":     modulePath,
			"module":   modulePath,
			"version":  cleanVersion,
			"path":     modulePath + "/@v",
			"ext":      fileType,
			"filename": filename,
		},
		Filename: filename,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
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

	switch fileType {
	case "info":
		ctx.Writer.Header().Set("Content-Type", "application/json")
	case "mod":
		ctx.Writer.Header().Set("Content-Type", "text/plain")
	case "zip":
		ctx.Writer.Header().Set("Content-Type", "application/zip")
	}
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}
