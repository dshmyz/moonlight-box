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
	"github.com/ulikunitz/xz"
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

const (
	maxYumPrimaryCompressedBytes   = 64 * 1024 * 1024
	maxYumPrimaryDecompressedBytes = 256 * 1024 * 1024
)

func NewYumPlugin(httpClient *http.Client) *YumPlugin {
	if httpClient == nil {
		panic("yum: httpClient is required")
	}
	return &YumPlugin{
		cache:      cache.NewMemoryCache(),
		httpClient: httpClient,
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
		"remote_url": remoteURL,
		"path":      path,
	}).Debug("yum: FetchRemote called")

	// For repomd.xml requests, fetch and parse the XML from remote.
	if strings.HasSuffix(path, "repodata/repomd.xml") {
		return p.fetchRepomd(ctx, remoteURL, path)
	}

	// For other paths (RPM packages, primary.xml, etc.), return a basic artifact indicating the remote resource exists.
	filename := filepath.Base(path)
	dir := yumArtifactDir(path)
	kind := runtime.KindFile
	if p.isRepodataRequest(path) {
		kind = runtime.KindMetadata
	}
	logrus.WithFields(logrus.Fields{
		"path":     path,
		"filename": filename,
	}).Debug("yum: FetchRemote returning file reference")
	return []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "yum",
			Kind:       kind,
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
		"remote_url": remoteURL,
		"path":      path,
		"full_url":   fullURL,
	}).Debug("yum: fetchRepomd called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("full_url", fullURL).Error("yum: create request for repomd failed")
		return nil, fmt.Errorf("yum: create request for repomd: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"full_url":  fullURL,
			"duration_ms": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: fetch repomd HTTP request failed")
		return nil, fmt.Errorf("yum: fetch repomd from %s: %w", fullURL, err)
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
		}).Error("yum: fetch repomd returned non-200 status")
		return nil, fmt.Errorf("yum: fetch repomd from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"full_url":  fullURL,
			"duration_ms": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: read repomd body failed")
		return nil, fmt.Errorf("yum: read repomd body: %w", err)
	}

	var repomd repomdXML
	if err := xml.Unmarshal(body, &repomd); err != nil {
		logrus.WithFields(logrus.Fields{
			"full_url":  fullURL,
			"duration_ms": time.Since(start).Seconds(),
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
		"full_url":       fullURL,
		"artifactCount": len(artifacts),
		"duration_ms":      time.Since(start).Seconds(),
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
		if resp.StatusCode == http.StatusNotFound {
			return nil, runtime.ErrNotFound
		}
		return nil, fmt.Errorf("yum: fetch primary from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := readLimitedYumPrimaryBody(resp)
	if err != nil {
		return nil, fmt.Errorf("yum: read primary body: %w", err)
	}

	body, err = decompressYumPrimary(primaryHref, body)
	if err != nil {
		return nil, err
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
		"full_url":      fullURL,
		"packageCount": len(artifacts),
	}).Debug("yum: parsed primary.xml.gz packages")
	return artifacts, nil
}

func decompressYumPrimary(primaryHref string, body []byte) ([]byte, error) {
	switch {
	case strings.HasSuffix(primaryHref, ".gz"):
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("yum: open primary gzip: %w", err)
		}
		defer zr.Close()
		out, err := readLimitedYumPrimaryDecompressed(zr)
		if err != nil {
			return nil, fmt.Errorf("yum: read primary gzip: %w", err)
		}
		return out, nil
	case strings.HasSuffix(primaryHref, ".xz"):
		xr, err := xz.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("yum: open primary xz: %w", err)
		}
		out, err := readLimitedYumPrimaryDecompressed(xr)
		if err != nil {
			return nil, fmt.Errorf("yum: read primary xz: %w", err)
		}
		return out, nil
	default:
		if len(body) > maxYumPrimaryDecompressedBytes {
			return nil, fmt.Errorf("yum: primary body too large: %d > %d", len(body), maxYumPrimaryDecompressedBytes)
		}
		return body, nil
	}
}

func readLimitedYumPrimaryBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxYumPrimaryCompressedBytes {
		return nil, fmt.Errorf("primary body too large: %d > %d", resp.ContentLength, maxYumPrimaryCompressedBytes)
	}
	return readLimitedYumPrimary(resp.Body, maxYumPrimaryCompressedBytes, "primary body")
}

func readLimitedYumPrimaryDecompressed(reader io.Reader) ([]byte, error) {
	return readLimitedYumPrimary(reader, maxYumPrimaryDecompressedBytes, "primary decompressed body")
}

func readLimitedYumPrimary(reader io.Reader, maxBytes int64, label string) ([]byte, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%s too large: exceeds %d bytes", label, maxBytes)
	}
	return body, nil
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
		"repo_name": ctx.Repository.Name,
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
		"repo_name": ctx.Repository.Name,
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
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead {
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
		if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "application/xml", "inline"); err != nil {
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
	type checksumXML struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	}
	type data struct {
		Type     string `xml:"type,attr"`
		Location struct {
			Href string `xml:"href,attr"`
		} `xml:"location"`
		Checksum     checksumXML `xml:"checksum"`
		OpenChecksum checksumXML `xml:"open-checksum"`
		Size         string      `xml:"size"`
		OpenSize     string      `xml:"open-size"`
		Timestamp    string      `xml:"timestamp"`
	}
	type repomd struct {
		XMLName  xml.Name `xml:"repomd"`
		Xmlns    string   `xml:"xmlns,attr"`
		Revision string   `xml:"revision"`
		Data     []data   `xml:"data"`
	}
	now := time.Now().Unix()
	out := repomd{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		Revision: fmt.Sprintf("%d", now),
	}
	for _, a := range artifacts {
		// repomd.xml 只应引用元数据文件（primary/filelists/other 等），跳过 RPM 包
		if a.Kind != runtime.KindMetadata {
			continue
		}
		// repomd.xml 本身不应作为 data 条目自引用
		if a.Filename == "repomd.xml" {
			continue
		}
		f := a.Filename
		if f == "" {
			continue
		}
		dataType := a.Qualifiers["type"]
		if dataType == "" {
			// 根据文件名推断 metadata 类型
			dataType = inferYumDataType(f)
		}
		if dataType == "" {
			continue
		}
		d := data{Type: dataType}
		// location.href 必须是相对仓库根的完整路径（如 repodata/primary.xml.gz）
		// 优先从 Qualifiers["href"] 获取（FetchRemote 设置的正确路径），
		// 其次用 RemotePath，最后回退到 Filename
		href := a.Qualifiers["href"]
		if href == "" {
			href = a.RemotePath
		}
		if href == "" {
			href = f
		}
		d.Location.Href = href
		d.Timestamp = fmt.Sprintf("%d", now)
		if len(a.BlobRefs) > 0 {
			d.Checksum = checksumXML{Type: a.BlobRefs[0].Algorithm, Value: a.BlobRefs[0].Digest}
			d.OpenChecksum = checksumXML{Type: a.BlobRefs[0].Algorithm, Value: a.BlobRefs[0].Digest}
			d.Size = fmt.Sprintf("%d", a.BlobRefs[0].Size)
			d.OpenSize = fmt.Sprintf("%d", a.BlobRefs[0].Size)
		}
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
	if isCompressedYumMetadata(filename) {
		artifact, getErr := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if getErr != nil {
			if !errors.Is(getErr, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return getErr
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
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			logrus.WithError(err).Warn("failed to write artifact content to client")
		}
		return nil
	}
	var b strings.Builder
	packageCount := 0
	for _, a := range artifacts {
		name := a.Name
		ver := a.Version
		if name == "" || ver == "" {
			continue
		}
		arch := firstNonEmptyYum(a.Attributes["arch"], a.Namespace, "noarch")
		release := a.Attributes["release"]
		epoch := a.Attributes["epoch"]
		href := firstNonEmptyYum(a.RemotePath, a.Filename)
		fmt.Fprintf(&b, "  <package type=\"rpm\">\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", xmlEscapeYum(name))
		fmt.Fprintf(&b, "    <arch>%s</arch>\n", xmlEscapeYum(arch))
		b.WriteString("    <version")
		if epoch != "" {
			fmt.Fprintf(&b, ` epoch="%s"`, xmlEscapeYum(epoch))
		}
		fmt.Fprintf(&b, ` ver="%s"`, xmlEscapeYum(ver))
		if release != "" {
			fmt.Fprintf(&b, ` rel="%s"`, xmlEscapeYum(release))
		}
		b.WriteString("/>\n")
		// 校验和信息（从 BlobRefs 获取）
		if len(a.BlobRefs) > 0 && a.BlobRefs[0].Digest != "" {
			checksumType := a.BlobRefs[0].Algorithm
			if checksumType == "" {
				checksumType = "sha256"
			}
			fmt.Fprintf(&b, `    <checksum type="%s" pkgid="YES">%s</checksum>`+"\n",
				xmlEscapeYum(checksumType), a.BlobRefs[0].Digest)
		}
		// 文件大小信息
		pkgSize := a.SizeBytes
		if pkgSize == 0 && len(a.BlobRefs) > 0 {
			pkgSize = a.BlobRefs[0].Size
		}
		if pkgSize > 0 {
			fmt.Fprintf(&b, `    <size package="%d" installed="%d" archive="%d"/>`+"\n", pkgSize, pkgSize, pkgSize)
		}
		fmt.Fprintf(&b, `    <location href="%s"/>`+"\n", xmlEscapeYum(href))
		b.WriteString("  </package>\n")
		packageCount++
	}
	if packageCount == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	// 构建完整 XML（packages 数量在循环后确定）
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="%d">`+"\n",
		packageCount) + b.String() + `</metadata>`
	if p.cache != nil {
		p.cache.Set(cacheKey, []byte(body), 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write([]byte(body))
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

// inferYumDataType 根据元数据文件名推断 repomd data type。
// 返回空字符串表示无法识别（调用方应跳过该 artifact）。
func inferYumDataType(filename string) string {
	base := strings.ToLower(filename)
	switch {
	case strings.Contains(base, "primary"):
		return "primary"
	case strings.Contains(base, "filelists"):
		return "filelists"
	case strings.Contains(base, "other"):
		return "other"
	case strings.Contains(base, "updateinfo"):
		return "updateinfo"
	case strings.Contains(base, "comps"):
		return "group"
	case strings.Contains(base, "prestodelta"), strings.Contains(base, "deltainfo"):
		return "prestodelta"
	default:
		return ""
	}
}

// xmlEscapeYum 转义 XML 特殊字符，防止注入。
func xmlEscapeYum(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
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
			if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "application/x-rpm", "inline"); err != nil {
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

		// 回源成功后再次通过 GetArtifact 获取带 blob 的完整 artifact
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
	artifact, getErr := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if getErr != nil {
		if !errors.Is(getErr, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return getErr
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
	case strings.HasSuffix(filename, ".xz"):
		return "application/x-xz"
	case strings.HasSuffix(filename, ".bz2"):
		return "application/x-bzip2"
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

func isCompressedYumMetadata(filename string) bool {
	return strings.HasSuffix(filename, ".gz") ||
		strings.HasSuffix(filename, ".xz") ||
		strings.HasSuffix(filename, ".bz2")
}
