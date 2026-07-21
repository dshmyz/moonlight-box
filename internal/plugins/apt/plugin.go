// Package apt implements the APT/deb repository protocol plugin.
//
// # APT/deb 仓库协议要点
//
// ## 目录结构
//   - dists/{dist}/InRelease: 仓库元数据索引（内联签名）
//   - dists/{dist}/Release: 仓库元数据索引
//   - dists/{dist}/Release.gpg: 签名文件
//   - dists/{dist}/main/binary-{arch}/Packages: 包索引
//   - dists/{dist}/main/binary-{arch}/Packages.gz: 压缩格式
//   - pool/main/{prefix}/{package}/*.deb: DEB 包文件
//
// ## Packages 文件格式
//   - 必需字段: Package, Version, Architecture, Filename, Size, SHA256
//   - Filename: 相对于仓库根目录的路径（如 pool/main/p/pkg/pkg_1.0_amd64.deb）
//   - Depends: 依赖关系
//   - Description: 包描述
//
// ## 压缩格式
//   - 支持: .gz (gzip), .xz (xz), .bz2 (bzip2)
//   - 压缩文件应透传上游原始文件，不动态生成
//   - 客户端根据 Accept-Encoding 或 URL 后缀选择格式
//
// ## InRelease 文件
//   - 包含: 仓库元数据 + 内联 GPG 签名
//   - 替代旧的 Release + Release.gpg 组合
//
// ## 回源策略（重要）
//
// 所有文件下载（.deb 包、InRelease、压缩 Packages）必须遵循以下回源路径：
//
//  1. 先通过 GetArtifact 查询本地缓存/MetadataStore
//  2. 未命中时通过 QueryArtifacts(RemotePath=path) 触发 FetchRemote 回源
//  3. 回源成功后再次 GetArtifact 获取带 blob 的完整 artifact
//
// 禁止直接构建远程 URL，必须通过 QueryArtifacts + RemotePath 让 Runtime 层
// 统一管理回源，确保:
//   - RemotePath 包含完整路径（如 "dists/focal/InRelease"、"pool/main/p/pkg/pkg_1.0_amd64.deb"）
//   - ArtifactKey 的强字段与 FetchRemote 存储字段一致（含 filename/path/remote_path）
//   - 负缓存由 Runtime 层统一管理
//
// ## 路由分发
//
// Handle 方法按以下优先级匹配路径：
//  1. InRelease/Release/Release.gpg → handleInRelease（透传原始文件）
//  2. Packages/Packages.gz/.xz/.bz2 → handlePackages（未压缩动态生成，压缩格式透传）
//  3. *.deb → handleDebPackage（代理 .deb 包）
//  4. 其他 → 404
//
// ## 参考规范
//   - https://wiki.debian.org/DebianRepository/Format
//   - https://apt.debian.org/
package apt

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
	"github.com/ulikunitz/xz"
)

type AptPlugin struct {
	cache      *cache.MemoryCache
	httpClient *http.Client
}

func NewAptPlugin(httpClient *http.Client) *AptPlugin {
	if httpClient == nil {
		panic("apt: httpClient is required")
	}
	return &AptPlugin{
		cache:      cache.NewMemoryCache(),
		httpClient: httpClient,
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *AptPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote APT protocol interaction.
// It fetches Packages index or InRelease/Release files from the remote repository.
func (p *AptPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("apt: empty path")
	}

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
	}).Debug("apt: FetchRemote called")

	// For Packages index requests, fetch and parse the Packages file.
	// Compressed formats still produce a metadata artifact for transparent
	// proxying, plus parsed package artifacts for UI/search aggregation.
	if p.isPackagesRequest(path) {
		if strings.HasSuffix(path, ".gz") || strings.HasSuffix(path, ".xz") || strings.HasSuffix(path, ".bz2") {
			filename := filepath.Base(path)
			dir := aptArtifactDir(path)
			metadata := runtime.NewArtifact(runtime.ArtifactSpec{
				Format:     "apt",
				Kind:       runtime.KindMetadata,
				Name:       filename,
				Path:       dir,
				Filename:   filename,
				RemotePath: path,
				Properties: map[string]string{
					"filename":    filename,
					"remote_path": path,
				},
			})
			packages, err := p.fetchPackagesIndex(ctx, remoteURL, path)
			if err != nil {
				return []*runtime.Artifact{metadata}, nil
			}
			return append([]*runtime.Artifact{metadata}, packages...), nil
		}
		return p.fetchPackagesIndex(ctx, remoteURL, path)
	}

	// For InRelease/Release requests, return a basic artifact indicating the remote resource exists.
	if p.isInReleaseRequest(path) {
		filename := filepath.Base(path)
		dir := aptArtifactDir(path)
		logrus.WithFields(logrus.Fields{
			"path":     path,
			"filename": filename,
			"kind":     runtime.KindMetadata,
		}).Debug("apt: FetchRemote returning release file reference")
		return []*runtime.Artifact{
			runtime.NewArtifact(runtime.ArtifactSpec{
				Format:     "apt",
				Kind:       runtime.KindMetadata,
				Name:       filename,
				Path:       dir,
				Filename:   filename,
				RemotePath: path,
				Attributes: map[string]string{"metadata_type": "release"},
				Properties: map[string]string{
					"filename":    filename,
					"remote_path": path,
				},
			}),
		}, nil
	}

	// For .deb package requests, return a basic artifact indicating the remote resource exists.
	filename := filepath.Base(path)
	dir := aptArtifactDir(path)
	logrus.WithFields(logrus.Fields{
		"path":     path,
		"filename": filename,
		"kind":     "package",
	}).Debug("apt: FetchRemote returning package file reference")
	return []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "apt",
			Kind:       runtime.KindPackage,
			Name:       filename,
			Path:       dir,
			Filename:   filename,
			RemotePath: path,
			Properties: map[string]string{
				"filename":    filename,
				"remote_path": path,
			},
		}),
	}, nil
}

// fetchPackagesIndex fetches a Packages index file from the remote repository
// and parses it to extract package entries.
func (p *AptPlugin) fetchPackagesIndex(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
		"fullURL":   fullURL,
	}).Debug("apt: fetchPackagesIndex called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("fullURL", fullURL).Error("apt: create request for packages index failed")
		return nil, fmt.Errorf("apt: create request for packages index: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("apt: fetch packages index HTTP request failed")
		return nil, fmt.Errorf("apt: fetch packages index from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, runtime.ErrNotFound
		}
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Error("apt: fetch packages index returned non-200 status")
		return nil, fmt.Errorf("apt: fetch packages index from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("apt: read packages index body failed")
		return nil, fmt.Errorf("apt: read packages index body: %w", err)
	}
	body, err = decompressAptPackages(path, body)
	if err != nil {
		return nil, err
	}

	artifacts := p.parsePackagesIndex(string(body))

	logrus.WithFields(logrus.Fields{
		"fullURL":      fullURL,
		"packageCount": len(artifacts),
		"duration":     time.Since(start).Seconds(),
	}).Debug("apt: fetchPackagesIndex success")
	return artifacts, nil
}

func decompressAptPackages(path string, body []byte) ([]byte, error) {
	switch {
	case strings.HasSuffix(path, ".gz"):
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("apt: open Packages gzip: %w", err)
		}
		defer zr.Close()
		out, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("apt: read Packages gzip: %w", err)
		}
		return out, nil
	case strings.HasSuffix(path, ".xz"):
		xr, err := xz.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("apt: open Packages xz: %w", err)
		}
		out, err := io.ReadAll(xr)
		if err != nil {
			return nil, fmt.Errorf("apt: read Packages xz: %w", err)
		}
		return out, nil
	case strings.HasSuffix(path, ".bz2"):
		out, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(body)))
		if err != nil {
			return nil, fmt.Errorf("apt: read Packages bzip2: %w", err)
		}
		return out, nil
	default:
		return body, nil
	}
}

// parsePackagesIndex parses a Debian Packages index file and extracts package entries.
func (p *AptPlugin) parsePackagesIndex(content string) []*runtime.Artifact {
	var artifacts []*runtime.Artifact
	var currentPkg map[string]string

	finishPkg := func() {
		if currentPkg == nil {
			return
		}
		pkgName := currentPkg["Package"]
		version := currentPkg["Version"]
		filename := currentPkg["Filename"]
		if pkgName != "" && version != "" {
			artifact := runtime.NewArtifact(runtime.ArtifactSpec{
				Format:     "apt",
				Kind:       runtime.KindPackage,
				Name:       pkgName,
				Version:    version,
				Path:       aptArtifactDir(filename),
				Filename:   filepath.Base(filename),
				RemotePath: filename,
				Qualifiers: map[string]string{
					"package": pkgName,
				},
				Properties: map[string]string{
					"filename":    filepath.Base(filename),
					"remote_path": filename,
				},
			})
			artifacts = append(artifacts, artifact)
		}
		currentPkg = nil
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			finishPkg()
			continue
		}
		if strings.Contains(line, ": ") {
			parts := strings.SplitN(line, ": ", 2)
			key := parts[0]
			value := parts[1]
			if currentPkg == nil {
				currentPkg = make(map[string]string)
			}
			currentPkg[key] = value
		}
	}
	finishPkg()
	return artifacts
}

func (p *AptPlugin) Name() string {
	return "apt"
}

func (p *AptPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if p.isInReleaseRequest(path) {
		return p.handleInRelease(ctx, repoRuntime, path)
	}

	if p.isPackagesRequest(path) {
		return p.handlePackages(ctx, repoRuntime, path)
	}

	if p.isByHashRequest(path) {
		return p.handleByHash(ctx, repoRuntime, path)
	}

	if p.isDebPackageRequest(path) {
		return p.handleDebPackage(ctx, repoRuntime, path)
	}

	http.Error(ctx.Writer, "Not found", http.StatusNotFound)
	return nil
}

func (p *AptPlugin) isInReleaseRequest(path string) bool {
	return strings.HasSuffix(path, "InRelease") || strings.HasSuffix(path, "Release") || strings.HasSuffix(path, "Release.gpg")
}

func (p *AptPlugin) isPackagesRequest(path string) bool {
	return strings.HasSuffix(path, "Packages") ||
		strings.HasSuffix(path, "Packages.gz") ||
		strings.HasSuffix(path, "Packages.xz") ||
		strings.HasSuffix(path, "Packages.bz2")
}

func (p *AptPlugin) isDebPackageRequest(path string) bool {
	return strings.HasSuffix(path, ".deb")
}

// isByHashRequest 检测 apt by-hash 路径。
// 格式: {dir}/by-hash/{algorithm}/{hash}
// 例如: dists/stable/main/binary-amd64/by-hash/SHA256/abc123...
func (p *AptPlugin) isByHashRequest(path string) bool {
	return strings.Contains(path, "/by-hash/")
}

// handleByHash 处理 apt by-hash 请求。
// by-hash 是 Debian 的 Acquire-By-Hash 机制，客户端通过哈希值而非文件名查找索引文件。
// 将 by-hash 路径直接作为 RemotePath 传给 Runtime，触发上游回源和缓存。
func (p *AptPlugin) handleByHash(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	dir := aptArtifactDir(path)

	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Kind:         runtime.KindMetadata,
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	// 先尝试从缓存获取
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			return err
		}
		// ErrNotFound: 继续走回源 fallback
	} else if artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", contentTypeForFile(filename))
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write by-hash content to client")
		}
		return nil
	}

	// GetArtifact 未命中，通过 QueryArtifacts + RemotePath 回源
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		RemotePath:   path,
	})
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			return err
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	if len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	// 回源成功后再次获取带 blob 的完整 artifact
	artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			return err
		}
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
	ctx.Writer.Header().Set("Content-Type", contentTypeForFile(filename))
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		logrus.WithError(err).Warn("failed to write by-hash content to client")
	}
	return nil
}

func aptArtifactDir(remotePath string) string {
	dir := filepath.Dir(strings.Trim(remotePath, "/"))
	if dir == "." {
		return ""
	}
	return dir
}

func contentTypeForFile(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".gz"):
		return "application/gzip"
	case strings.HasSuffix(filename, ".xz"):
		return "application/x-xz"
	case strings.HasSuffix(filename, ".bz2"):
		return "application/x-bzip2"
	case filename == "Packages" || strings.HasSuffix(filename, "/Packages"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func (p *AptPlugin) handleInRelease(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	dir := aptArtifactDir(path)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
		// ErrNotFound: 继续走回源 fallback
	} else if artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "text/plain", "inline"); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
		}
		return nil
	}

	// GetArtifact 未命中，通过 QueryArtifacts + RemotePath 回源
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		RemotePath:   path,
	})
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	if len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	// 回源成功后再次获取带 blob 的完整 artifact
	artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
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
	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
	}
	return nil
}

func (p *AptPlugin) handlePackages(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	// Try serve stored Packages file first.
	filename := filepath.Base(path)
	dir := aptArtifactDir(path)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	artifact, getErr := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if getErr != nil {
		if !errors.Is(getErr, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return getErr
		}
		// ErrNotFound: 继续走缓存/回源 fallback
	} else if artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", contentTypeForFile(filename))
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
		}
		return nil
	}

	// 压缩格式的 Packages 索引无法动态生成，通过 QueryArtifacts 回源获取原始文件
	if strings.HasSuffix(path, ".gz") || strings.HasSuffix(path, ".xz") || strings.HasSuffix(path, ".bz2") {
		artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "apt",
			RemotePath:   path,
		})
		if err != nil {
			if !errors.Is(err, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return err
			}
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if len(artifacts) == 0 {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		// 回源成功后再次获取带 blob 的完整 artifact
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			if !errors.Is(err, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return err
			}
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
		ctx.Writer.Header().Set("Content-Type", contentTypeForFile(filename))
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write packages content to client")
		}
		return nil
	}

	// Fallback: render lightweight dynamic Packages index from artifact graph.
	cacheKey := "apt:packages:" + ctx.Repository.ID + ":" + path
	if p.cache != nil {
		if v, ok := p.cache.Get(cacheKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
				ctx.Writer.WriteHeader(http.StatusOK)
				_, _ = ctx.Writer.Write(b)
				return nil
			}
		}
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	var b strings.Builder
	for _, a := range artifacts {
		name := firstNonEmptyApt(a.Name, a.Qualifiers["package"])
		version := a.Version
		file := a.Filename
		if file == "" {
			file = a.Properties["filename"]
		}
		if name == "" || version == "" || file == "" {
			continue
		}
		fmt.Fprintf(&b, "Package: %s\n", name)
		fmt.Fprintf(&b, "Version: %s\n", version)
		fmt.Fprintf(&b, "Filename: %s\n", file)
		if len(a.BlobRefs) > 0 {
			fmt.Fprintf(&b, "Size: %d\n", a.BlobRefs[0].Size)
			if a.BlobRefs[0].Algorithm == "sha256" && a.BlobRefs[0].Digest != "" {
				fmt.Fprintf(&b, "SHA256: %s\n", a.BlobRefs[0].Digest)
			}
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	body := []byte(b.String())
	if p.cache != nil {
		p.cache.Set(cacheKey, body, 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write(body)
	return nil
}

func firstNonEmptyApt(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (p *AptPlugin) handleDebPackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)
	dir := aptArtifactDir(path)

	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	switch ctx.Request.Method {
	case http.MethodGet, http.MethodHead:
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			if !errors.Is(err, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return err
			}
			// ErrNotFound: 继续走回源 fallback
		} else if artifact.Content != nil {
			defer artifact.Content.Close()
			ctx.FromCache = artifact.FromCache
			ctx.RemoteURL = artifact.RemoteURL
			ctx.SizeBytes = artifact.SizeBytes
			if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "application/vnd.debian.binary-package", "inline"); err != nil {
				logrus.WithError(err).Warn("failed to write artifact content to client")
			}
			return nil
		}

		// GetArtifact 未命中，通过 QueryArtifacts + RemotePath 回源
		artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "apt",
			RemotePath:   path,
		})
		if err != nil {
			if !errors.Is(err, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return err
			}
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if len(artifacts) == 0 {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}

		// 回源成功后再次获取带 blob 的完整 artifact
		artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			if !errors.Is(err, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return err
			}
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
		ctx.Writer.Header().Set("Content-Type", "application/vnd.debian.binary-package")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
		}
		return nil
	}
	return errors.New("method not allowed")
}
