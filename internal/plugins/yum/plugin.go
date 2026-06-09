// Package yum implements the YUM/DNF repository protocol plugin.
//
// # YUM/DNF 仓库协议要点
//
// ## 目录结构
//   - repodata/repomd.xml: 仓库元数据索引
//   - repodata/*-primary.xml.gz: 包列表（名称、版本、依赖）
//   - repodata/*-filelists.xml.gz: 文件列表
//   - repodata/*-other.xml.gz: changelog 等
//   - repodata/*-updateinfo.xml.gz: 安全更新信息
//   - repodata/*-comps.xml.gz: 包组定义
//   - packages/*.rpm: RPM 包文件
//
// ## repomd.xml 结构
//   - 必须包含: type, location, checksum, timestamp
//   - type 类型: primary, filelists, other, updateinfo 等
//   - revision: 仓库修订号（时间戳）
//
// ## primary.xml 结构
//   - 包含: name, version, arch, summary, description
//   - location href: RPM 文件相对路径
//   - checksum: 包文件校验和
//
// ## 关键实现点
//   - 动态生成 repomd.xml 时从 artifact.Qualifiers["type"] 读取类型
//   - 添加 revision 字段（时间戳）
//   - 压缩格式 (.gz) 应透传上游原始文件，不动态生成
//
// ## 回源策略（重要）
//
// 所有文件下载（RPM 包、repodata 元数据文件）必须遵循以下回源路径：
//
//  1. 先通过 GetArtifact 查询本地缓存/MetadataStore
//  2. 未命中时通过 QueryArtifacts(RemotePath=path) 触发 FetchRemote 回源
//  3. 回源成功后再次 GetArtifact 获取带 blob 的完整 artifact
//
// 禁止直接构建远程 URL，必须通过 QueryArtifacts + RemotePath 让 Runtime 层
// 统一管理回源，确保:
//   - RemotePath 包含完整路径（如 "Packages/nginx-1.0.rpm"、"repodata/filelists.xml.gz"）
//   - ArtifactKey 使用 RemotePath/Path/Filename 与 FetchRemote 存储的字段一致
//   - 负缓存由 Runtime 层统一管理
//
// ## 路由分发
//
// Handle 方法按以下优先级匹配路径：
//  1. repomd.xml → handleRepomd（动态生成索引）
//  2. *primary.xml* → handlePrimary（动态生成包列表）
//  3. *.rpm → handleRpmPackage（代理 RPM 包）
//  4. repodata/* → handleRepodataGeneric（透传 filelists/other/updateinfo 等）
//  5. 其他 → 404
//
// ## 参考规范
//   - http://linux.duke.edu/metadata/repo/
//   - https://dnf.readthedocs.io/
package yum

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
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
)

type repomdData struct {
	Type     string `xml:"type,attr"`
	Location struct {
		Href string `xml:"href,attr"`
	} `xml:"location"`
}
type repomdXML struct {
	XMLName xml.Name     `xml:"repomd"`
	Data    []repomdData `xml:"data"`
}

type primaryPackage struct {
	Name    string `xml:"name"`
	Arch    string `xml:"arch"`
	Version struct {
		Epoch string `xml:"epoch,attr"`
		Ver   string `xml:"ver,attr"`
		Rel   string `xml:"rel,attr"`
	} `xml:"version"`
	Location struct {
		Href string `xml:"href,attr"`
	} `xml:"location"`
	Summary     string `xml:"summary"`
	Description string `xml:"description"`
	URL         string `xml:"url"`
	Packager    string `xml:"packager"`
	License     string `xml:"license"`
}
type primaryXML struct {
	XMLName  xml.Name         `xml:"metadata"`
	Packages []primaryPackage `xml:"package"`
}

type YumPlugin struct {
	cache      *cache.MemoryCache
	httpClient *http.Client
}

func NewYumPlugin() *YumPlugin {
	return &YumPlugin{
		cache:      cache.NewMemoryCache(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *YumPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote YUM protocol interaction.
// It fetches repomd.xml or other repository metadata from the remote repository.
func (p *YumPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("yum: empty path")
	}

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
	}).Debug("yum: FetchRemote called")

	// For repomd.xml requests, fetch and parse the XML from remote.
	if strings.HasSuffix(path, "repodata/repomd.xml") {
		return p.fetchRepomd(ctx, remoteURL, path)
	}

	// For other paths (RPM packages, primary.xml, etc.), return a basic artifact indicating the remote resource exists.
	filename := filepath.Base(path)
	dir := yumArtifactDir(path)
	logrus.WithFields(logrus.Fields{
		"path":     path,
		"filename": filename,
	}).Debug("yum: FetchRemote returning file reference")
	return []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "yum",
			Kind:       runtime.KindFile,
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

// fetchRepomd fetches repomd.xml from the remote repository and parses data references.
func (p *YumPlugin) fetchRepomd(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
		"fullURL":   fullURL,
	}).Debug("yum: fetchRepomd called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("fullURL", fullURL).Error("yum: create request for repomd failed")
		return nil, fmt.Errorf("yum: create request for repomd: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: fetch repomd HTTP request failed")
		return nil, fmt.Errorf("yum: fetch repomd from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Error("yum: fetch repomd returned non-200 status")
		return nil, fmt.Errorf("yum: fetch repomd from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: read repomd body failed")
		return nil, fmt.Errorf("yum: read repomd body: %w", err)
	}

	var repomd repomdXML
	if err := xml.Unmarshal(body, &repomd); err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: unmarshal repomd XML failed")
		return nil, fmt.Errorf("yum: unmarshal repomd XML: %w", err)
	}

	var artifacts []*runtime.Artifact
	// Add the repomd.xml itself as an artifact.
	artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
		Format:     "yum",
		Kind:       runtime.KindMetadata,
		Name:       "repomd.xml",
		Path:       yumArtifactDir(path),
		Filename:   "repomd.xml",
		RemotePath: path,
		Properties: map[string]string{
			"filename":    "repomd.xml",
			"remote_path": path,
		},
	}))
	// Add each data reference as an artifact.
	for _, d := range repomd.Data {
		href := d.Location.Href
		if href == "" {
			continue
		}
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "yum",
			Kind:       runtime.KindMetadata,
			Name:       filepath.Base(href),
			Path:       yumArtifactDir(href),
			Filename:   filepath.Base(href),
			RemotePath: href,
			Qualifiers: map[string]string{
				"type": d.Type,
				"href": href,
			},
			Properties: map[string]string{
				"filename":    filepath.Base(href),
				"remote_path": href,
			},
		}))
	}

	primaryArtifacts, err := p.fetchPrimaryIndexPackages(ctx, remoteURL, repomd.Data)
	if err != nil {
		logrus.WithError(err).Warn("yum: fetch primary.xml.gz packages failed, fallback to repomd refs only")
	} else {
		artifacts = append(artifacts, primaryArtifacts...)
	}

	logrus.WithFields(logrus.Fields{
		"fullURL":       fullURL,
		"artifactCount": len(artifacts),
		"duration":      time.Since(start).Seconds(),
	}).Debug("yum: fetchRepomd success")
	return artifacts, nil
}

// fetchPrimaryIndexPackages 解析 primary.xml.gz，把 RPM 包提取为独立 artifact。
// 这样 packages 聚合表可以使用真正的包名（如 nginx），而不是文件名（nginx-1.20.1-1.el8.x86_64.rpm）。
func (p *YumPlugin) fetchPrimaryIndexPackages(ctx context.Context, remoteURL string, data []repomdData) ([]*runtime.Artifact, error) {
	var primaryHref string
	for _, d := range data {
		if d.Type == "primary" {
			primaryHref = d.Location.Href
			break
		}
	}
	if primaryHref == "" {
		return nil, nil
	}

	fullURL := strings.TrimRight(remoteURL, "/") + "/" + primaryHref
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("yum: create primary request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yum: fetch primary from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yum: fetch primary from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("yum: read primary body: %w", err)
	}

	// primary.xml.gz is gzip-compressed
	if strings.HasSuffix(primaryHref, ".gz") {
		zr, zErr := gzip.NewReader(bytes.NewReader(body))
		if zErr != nil {
			return nil, fmt.Errorf("yum: open primary gzip: %w", zErr)
		}
		defer zr.Close()
		body, err = io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("yum: read primary gzip: %w", err)
		}
	}

	var primary primaryXML
	if err := xml.Unmarshal(body, &primary); err != nil {
		return nil, fmt.Errorf("yum: unmarshal primary xml: %w", err)
	}

	var artifacts []*runtime.Artifact
	for _, pkg := range primary.Packages {
		if pkg.Name == "" || pkg.Location.Href == "" {
			continue
		}

		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "yum",
			Kind:       runtime.KindFile,
			Name:       pkg.Name,
			Namespace:  pkg.Arch,
			Version:    pkg.Version.Ver,
			Path:       yumArtifactDir(pkg.Location.Href),
			Filename:   filepath.Base(pkg.Location.Href),
			RemotePath: pkg.Location.Href,
			Properties: map[string]string{
				"filename":    filepath.Base(pkg.Location.Href),
				"remote_path": pkg.Location.Href,
			},
			Attributes: map[string]string{
				"arch":        pkg.Arch,
				"release":     pkg.Version.Rel,
				"epoch":       pkg.Version.Epoch,
				"summary":     pkg.Summary,
				"description": pkg.Description,
				"url":         pkg.URL,
				"packager":    pkg.Packager,
				"license":     pkg.License,
			},
		}))
	}

	logrus.WithFields(logrus.Fields{
		"fullURL":      fullURL,
		"packageCount": len(artifacts),
	}).Debug("yum: parsed primary.xml.gz packages")
	return artifacts, nil
}

func (p *YumPlugin) Name() string {
	return "yum"
}

func (p *YumPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	logrus.WithFields(logrus.Fields{
		"originalPath":   ctx.RepositoryPath,
		"trimmedPath":    path,
		"isRepomd":       p.isRepomdRequest(path),
		"isPrimary":      p.isPrimaryRequest(path),
		"isRpmPackage":   p.isRpmPackageRequest(path),
		"isRepodata":     p.isRepodataRequest(path),
		"repositoryName": ctx.Repository.Name,
	}).Debug("yum: Handle called")

	if p.isRepomdRequest(path) {
		return p.handleRepomd(ctx, repoRuntime, path)
	}

	if p.isPrimaryRequest(path) {
		return p.handlePrimary(ctx, repoRuntime, path)
	}

	if p.isRpmPackageRequest(path) {
		return p.handleRpmPackage(ctx, repoRuntime, path)
	}

	// 其他 repodata 元数据文件（filelists、other、updateinfo、comps 等）
	if p.isRepodataRequest(path) {
		return p.handleRepodataGeneric(ctx, repoRuntime, path)
	}

	logrus.WithFields(logrus.Fields{
		"path":           path,
		"repositoryName": ctx.Repository.Name,
	}).Warn("yum: path does not match any known pattern, returning 404")
	http.Error(ctx.Writer, "Not found", http.StatusNotFound)
	return nil
}

func (p *YumPlugin) isRepomdRequest(path string) bool {
	return strings.HasSuffix(path, "repodata/repomd.xml")
}

func (p *YumPlugin) isPrimaryRequest(path string) bool {
	return strings.Contains(path, "repodata/") && strings.Contains(path, "primary.xml")
}

func (p *YumPlugin) isRpmPackageRequest(path string) bool {
	return strings.HasSuffix(path, ".rpm")
}

func yumArtifactDir(remotePath string) string {
	dir := filepath.Dir(strings.Trim(remotePath, "/"))
	if dir == "." {
		return ""
	}
	return dir
}

// isRepodataRequest matches any file under repodata/ that isn't handled by
// isRepomdRequest or isPrimaryRequest (e.g. filelists, other, updateinfo, comps).
func (p *YumPlugin) isRepodataRequest(path string) bool {
	return strings.HasPrefix(path, "repodata/") || strings.Contains(path, "/repodata/")
}

func (p *YumPlugin) handleRepomd(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Kind:         runtime.KindMetadata,
		Name:         "repomd.xml",
		Filename:     "repomd.xml",
		RemotePath:   path,
	}

	if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", "application/xml")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
			return nil
		}
		return nil
	}

	cacheKey := "yum:repomd:" + ctx.Repository.ID + ":" + path
	if p.cache != nil {
		if v, ok := p.cache.Get(cacheKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				ctx.Writer.Header().Set("Content-Type", "application/xml")
				ctx.Writer.WriteHeader(http.StatusOK)
				_, _ = ctx.Writer.Write(b)
				return nil
			}
		}
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil || len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	type data struct {
		Type     string `xml:"type,attr"`
		Location struct {
			Href string `xml:"href,attr"`
		} `xml:"location"`
	}
	type repomd struct {
		XMLName  xml.Name `xml:"repomd"`
		Xmlns    string   `xml:"xmlns,attr"`
		Revision string   `xml:"revision"`
		Data     []data   `xml:"data"`
	}
	out := repomd{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		Revision: fmt.Sprintf("%d", time.Now().Unix()),
	}
	for _, a := range artifacts {
		f := a.Filename
		if f == "" {
			continue
		}
		dataType := a.Qualifiers["type"]
		if dataType == "" {
			dataType = "primary"
		}
		d := data{Type: dataType}
		d.Location.Href = f
		out.Data = append(out.Data, d)
	}
	if len(out.Data) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	body, _ := xml.MarshalIndent(out, "", "  ")
	finalBody := append([]byte(xml.Header), body...)
	if p.cache != nil {
		p.cache.Set(cacheKey, finalBody, 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write(finalBody)
	return nil
}

func (p *YumPlugin) handlePrimary(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	dir := yumArtifactDir(path)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", contentTypeForFile(filename))
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
			return nil
		}
		return nil
	}

	cacheKey := "yum:primary:" + ctx.Repository.ID + ":" + path
	if p.cache != nil {
		if v, ok := p.cache.Get(cacheKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				ctx.Writer.Header().Set("Content-Type", "application/xml")
				ctx.Writer.WriteHeader(http.StatusOK)
				_, _ = ctx.Writer.Write(b)
				return nil
			}
		}
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil || len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<metadata xmlns="http://linux.duke.edu/metadata/common">` + "\n")
	for _, a := range artifacts {
		name := a.Name
		ver := a.Version
		if name == "" || ver == "" {
			continue
		}
		fmt.Fprintf(&b, "  <package type=\"rpm\"><name>%s</name><version ver=\"%s\"/></package>\n", name, ver)
	}
	b.WriteString(`</metadata>`)
	body := []byte(b.String())
	if p.cache != nil {
		p.cache.Set(cacheKey, body, 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write(body)
	return nil
}

func firstNonEmptyYum(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (p *YumPlugin) handleRpmPackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)
	dir := yumArtifactDir(path)

	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err == nil && artifact.Content != nil {
			defer artifact.Content.Close()
			ctx.FromCache = artifact.FromCache
			ctx.RemoteURL = artifact.RemoteURL
			ctx.SizeBytes = artifact.SizeBytes
			ctx.Writer.Header().Set("Content-Type", "application/x-rpm")
			ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
			ctx.Writer.WriteHeader(http.StatusOK)
			if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
				logrus.WithError(err).Warn("failed to write artifact content to client")
			}
			return nil
		}

		// GetArtifact 未命中，通过 QueryArtifacts + RemotePath 回源
		artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "yum",
			RemotePath:   path,
		})
		if err != nil || len(artifacts) == 0 {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}

		// 回源成功后再次通过 GetArtifact 获取带 blob 的完整 artifact
		artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil || artifact.Content == nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", "application/x-rpm")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
		}
		return nil
	}
	return errors.New("method not allowed")
}

// handleRepodataGeneric handles repodata files not covered by handleRepomd/handlePrimary,
// such as filelists.xml.gz, other.xml.gz, updateinfo.xml.gz, comps.xml.gz, etc.
// These files are proxied transparently from the upstream repository.
func (p *YumPlugin) handleRepodataGeneric(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	dir := yumArtifactDir(path)

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Name:         filename,
		Path:         dir,
		Filename:     filename,
		RemotePath:   path,
	}

	// 尝试从本地缓存或 MetadataStore 获取
	if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", contentTypeForFile(filename))
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write repodata content to client")
		}
		return nil
	}

	// 通过 QueryArtifacts + RemotePath 回源
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		RemotePath:   path,
	})
	if err != nil || len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	// 回源成功后再次获取带 blob 的完整 artifact
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil || artifact.Content == nil {
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
		logrus.WithError(err).Warn("failed to write repodata content to client")
	}
	return nil
}

// contentTypeForFile returns an appropriate Content-Type for repodata files.
func contentTypeForFile(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".xml.gz"):
		return "application/gzip"
	case strings.HasSuffix(filename, ".xml"):
		return "application/xml"
	case strings.HasSuffix(filename, ".gz"):
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}
