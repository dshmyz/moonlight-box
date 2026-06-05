// Package npm implements the npm registry protocol plugin.
//
// # npm Registry 协议要点
//
// ## 路径结构
//   - 包元数据: /{package} 或 /@scope/{package}
//   - Tarball 下载: /{package}/-/{package}-{version}.tgz
//   - Scoped 包: /@scope/pkg/-/pkg-{version}.tgz (tarball 文件名不含 scope 前缀)
//
// ## 关键实现点
//   - Scoped 包版本提取: tarball 文件名是 pkg-1.0.0.tgz，不是 @scope/pkg-1.0.0.tgz
//   - Tarball URL 构造: scoped 包使用短名称（去掉 @scope/ 前缀）
//   - dist-tags: 优先从 artifact.Properties["dist-tag"] 提取，fallback 自动计算 latest
//   - 流式 base64: 使用 base64.NewDecoder 避免内存翻倍
//
// ## 包元数据响应
//   - 必须包含 name、versions、dist-tags、time 字段
//   - each version 必须包含 dist.tarball URL
//
// ## 参考规范
//   - https://github.com/npm/registry/blob/main/docs/REGISTRY-API.md
package npm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"
)

type NpmPlugin struct {
	httpClient *http.Client // 统一 HTTP 客户端，可注入
}

func NewNpmPlugin() *NpmPlugin {
	return &NpmPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient 允许注入统一的 HTTP 客户端（含 DNS 映射、TLS 配置等）
func (p *NpmPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

func (p *NpmPlugin) Name() string {
	return "npm"
}

// FetchRemote 实现 RemoteFetcher 接口——Runtime 回调, 负责远端 npm registry 协议交互
func (p *NpmPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	packageName := strings.TrimPrefix(path, "/")
	if packageName == "" {
		return nil, errors.New("npm: empty package path")
	}
	// scoped package 需要 URL 编码: @scope/pkg → %40scope%2Fpkg
	encodedName := strings.ReplaceAll(url.PathEscape(packageName), "@", "%40")
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodedName

	logrus.WithFields(logrus.Fields{
		"remoteURL":   remoteURL,
		"path":        path,
		"packageName": packageName,
		"fullURL":     fullURL,
	}).Debug("npm: FetchRemote called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("fullURL", fullURL).Error("npm: create request failed")
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("npm: HTTP request failed")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Error("npm: HTTP request returned non-200 status")
		return nil, fmt.Errorf("npm: fetch from %s: status %d", fullURL, resp.StatusCode)
	}

	artifacts, err := p.parseNpmMetadata(packageName, resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("npm: parse metadata failed")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"fullURL":      fullURL,
		"versionCount": len(artifacts),
		"duration":     time.Since(start).Seconds(),
	}).Debug("npm: FetchRemote success")
	return artifacts, nil
}

// parseNpmMetadata 解析 npm registry JSON, 提取版本列表为 artifact
func (p *NpmPlugin) parseNpmMetadata(packageName string, body io.Reader) ([]*runtime.Artifact, error) {
	var raw map[string]interface{}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	versions, _ := raw["versions"].(map[string]interface{})
	if versions == nil {
		return nil, nil
	}
	timeMap, _ := raw["time"].(map[string]interface{})
	topLicense := extractLicense(raw)
	topDesc, _ := raw["description"].(string)
	topHomepage, _ := raw["homepage"].(string)

	var artifacts []*runtime.Artifact
	for ver, verRaw := range versions {
		props := map[string]string{}
		verObj, _ := verRaw.(map[string]interface{})
		if verObj != nil {
			if lic := extractLicense(verObj); lic != "" {
				props["license"] = lic
			}
			if desc, _ := verObj["description"].(string); desc != "" {
				props["description"] = desc
			}
			if hp, _ := verObj["homepage"].(string); hp != "" {
				props["homepage"] = hp
			}
		}
		if props["license"] == "" && topLicense != "" {
			props["license"] = topLicense
		}
		if props["description"] == "" && topDesc != "" {
			props["description"] = topDesc
		}
		if props["homepage"] == "" && topHomepage != "" {
			props["homepage"] = topHomepage
		}
		if timeMap != nil {
			if ts, ok := timeMap[ver].(string); ok {
				props["published_at"] = ts
			}
		}
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       packageName,
			Version:    ver,
			Attributes: props,
		}))
		tarballName := npmTarballName(packageName, ver)
		tarballProps := map[string]string{}
		if verObj != nil {
			if dist, _ := verObj["dist"].(map[string]interface{}); dist != nil {
				if tarballURL, _ := dist["tarball"].(string); tarballURL != "" {
					tarballName = pathBase(tarballURL)
					if parsed, err := url.Parse(tarballURL); err == nil && parsed.IsAbs() {
						tarballProps["download_url"] = tarballURL
					}
				}
			}
		}
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:      "npm",
			Kind:        "tarball",
			Name:        packageName,
			Version:     ver,
			Path:        packageName + "/-",
			Filename:    tarballName,
			RemotePath:  packageName + "/-/" + tarballName,
			DownloadURL: tarballProps["download_url"],
			Properties:  tarballProps,
		}))
	}
	return artifacts, nil
}

func npmTarballName(packageName, version string) string {
	shortName := packageName
	if idx := strings.LastIndex(packageName, "/"); idx >= 0 {
		shortName = packageName[idx+1:]
	}
	return shortName + "-" + version + ".tgz"
}

func pathBase(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
		return parts[len(parts)-1]
	}
	parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
	return parts[len(parts)-1]
}

func extractLicense(obj map[string]interface{}) string {
	lic, ok := obj["license"]
	if !ok || lic == nil {
		return ""
	}
	switch v := lic.(type) {
	case string:
		return v
	case map[string]interface{}:
		t, _ := v["type"].(string)
		return t
	default:
		return ""
	}
}

// extractNpmVersionFromTarball 从 tarball 文件名中提取版本号。
// npm 规范：scoped 包的 tarball 文件名不含 scope 前缀，
// 例如 @babel/core 的 tarball 是 core-7.22.0.tgz 而非 @babel/core-7.22.0.tgz。
func extractNpmVersionFromTarball(packageName, filename string) string {
	nameForTrim := packageName
	// scoped package: 取 "/" 后的短名称
	if idx := strings.LastIndex(packageName, "/"); idx >= 0 {
		nameForTrim = packageName[idx+1:]
	}
	return strings.TrimSuffix(strings.TrimPrefix(filename, nameForTrim+"-"), ".tgz")
}

func (p *NpmPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/-all") || path == "-/all" {
		return p.handleAllPackages(ctx, repoRuntime)
	}

	if strings.HasPrefix(path, "-/npm/") {
		return p.handleNpmInternal(ctx, repoRuntime, path)
	}

	if strings.Contains(path, "/-/") {
		return p.handleTarballDownload(ctx, repoRuntime, path)
	}

	return p.handlePackage(ctx, repoRuntime, path)
}

func (p *NpmPlugin) handleAllPackages(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	packages := make(map[string]interface{})
	for _, artifact := range artifacts {
		name := artifact.Name
		if name == "" {
			continue
		}
		if _, ok := packages[name]; !ok {
			packages[name] = map[string]interface{}{
				"name": name,
			}
		}
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(packages)
	return nil
}

func (p *NpmPlugin) handleNpmInternal(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodPost {
		return errors.New("method not allowed")
	}

	normalized := strings.TrimPrefix(path, "-/npm/")

	// npm registry heartbeat endpoint used by npm CLI.
	if normalized == "ping" {
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(ctx.Writer).Encode(map[string]interface{}{
			"ok":   true,
			"pong": time.Now().UTC().Format(time.RFC3339),
		})
		return nil
	}

	// npm audit endpoints: return empty advisory set to keep client flow working.
	if normalized == "v1/security/advisories/bulk" || normalized == "v1/security/audits/quick" {
		_, _ = io.Copy(io.Discard, ctx.Request.Body)
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(http.StatusOK)
		_, _ = ctx.Writer.Write([]byte(`{}`))
		return nil
	}

	http.Error(ctx.Writer, "Not found", http.StatusNotFound)
	return nil
}

func (p *NpmPlugin) handleTarballDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/-/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}

	packageName := parts[0]
	filename := parts[1]
	version := extractNpmVersionFromTarball(packageName, filename)

	ctx.PackageName = packageName
	ctx.Version = version
	ctx.Filename = filename

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Kind:         "tarball",
		Name:         packageName,
		Version:      version,
		Path:         packageName + "/-",
		Filename:     filename,
		RemotePath:   path,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			artifacts, queryErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
				RepositoryID: ctx.Repository.ID,
				Format:       "npm",
				Name:         packageName,
				RemotePath:   packageName,
			})
			if queryErr == nil && len(artifacts) > 0 {
				artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
			}
		}
	}
	if err != nil {
		if errors.Is(err, runtime.ErrBlocked) {
			return err
		}
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

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}

func (p *NpmPlugin) handlePackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	packageName := strings.TrimSuffix(path, "/")

	ctx.PackageName = packageName

	switch ctx.Request.Method {
	case http.MethodGet:
		return p.handlePackageGet(ctx, repoRuntime, packageName)
	case http.MethodPut:
		return p.handlePackagePut(ctx, repoRuntime, packageName)
	}
	return errors.New("method not allowed")
}

func (p *NpmPlugin) handleTarballDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/-/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}
	packageName := parts[0]
	filename := parts[1]
	version := extractNpmVersionFromTarball(packageName, filename)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Kind:         "tarball",
		Name:         packageName,
		Version:      version,
		Path:         packageName + "/-",
		Filename:     filename,
		RemotePath:   path,
	}
	return deleteArtifact(ctx, repoRuntime, key)
}

func (p *NpmPlugin) handlePackageGet(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Name:         packageName,
		RemotePath:   packageName,
	})
	if err != nil {
		if errors.Is(err, runtime.ErrBlocked) {
			return err
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	hasVersions := false
	for _, a := range artifacts {
		if a.Version != "" {
			hasVersions = true
			break
		}
	}
	if !hasVersions {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	repoBase := repoBaseURL(ctx.Request, ctx.Repository.Name)

	versions := make(map[string]interface{})
	var versionList []string
	for _, artifact := range artifacts {
		version := artifact.Version
		if version == "" {
			continue
		}
		if _, exists := versions[version]; exists {
			continue
		}
		// npm 规范：scoped 包的 tarball 文件名不含 scope 前缀
		// 例如 @babel/core 的 tarball 是 core-7.22.0.tgz
		shortName := packageName
		if idx := strings.LastIndex(packageName, "/"); idx >= 0 {
			shortName = packageName[idx+1:]
		}
		tarballName := shortName + "-" + version + ".tgz"
		versions[version] = map[string]interface{}{
			"name":    packageName,
			"version": version,
			"dist": map[string]interface{}{
				"tarball": repoBase + "/" + packageName + "/-/" + tarballName,
			},
		}
		versionList = append(versionList, version)
	}

	// Compute dist-tags: 优先从 artifact Properties 中提取自定义 tag，fallback 自动计算 latest
	distTags := map[string]string{}
	for _, artifact := range artifacts {
		if tag := artifact.Properties["dist-tag"]; tag != "" {
			v := artifact.Version
			if v != "" {
				distTags[tag] = v
			}
		}
	}
	if _, hasLatest := distTags["latest"]; !hasLatest && len(versionList) > 0 {
		sort.Slice(versionList, func(i, j int) bool {
			return semver.Compare(versionList[i], versionList[j]) > 0
		})
		distTags["latest"] = versionList[0]
	}

	data := map[string]interface{}{
		"name":      packageName,
		"dist-tags": distTags,
		"versions":  versions,
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func firstNonEmptyNpm(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// repoBaseURL 构造仓库的基础 URL，支持反向代理 (X-Forwarded-* 头)
func repoBaseURL(r *http.Request, repoName string) string {
	scheme := "http"
	if proto := firstForwardedHeader(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := firstForwardedHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	repoPath := "/repository/" + strings.Trim(repoName, "/")
	prefix, hasPrefix := forwardedPrefix(r, "X-Forwarded-Prefix")
	if !hasPrefix {
		prefix, hasPrefix = forwardedPrefix(r, "X-Script-Name")
	}
	switch {
	case hasPrefix && prefix == "":
		return fmt.Sprintf("%s://%s", scheme, host)
	case prefix == "":
		return fmt.Sprintf("%s://%s%s", scheme, host, repoPath)
	case prefix == repoPath || strings.HasSuffix(prefix, repoPath):
		return fmt.Sprintf("%s://%s%s", scheme, host, prefix)
	case strings.HasSuffix(prefix, "/repository"):
		return fmt.Sprintf("%s://%s%s/%s", scheme, host, prefix, strings.Trim(repoName, "/"))
	default:
		return fmt.Sprintf("%s://%s%s%s", scheme, host, prefix, repoPath)
	}
}

func firstForwardedHeader(value string) string {
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func forwardedPrefix(r *http.Request, header string) (string, bool) {
	values, ok := r.Header[http.CanonicalHeaderKey(header)]
	if !ok || len(values) == 0 {
		return "", false
	}
	value := firstForwardedHeader(values[0])
	if value == "" || value == "/" {
		return "", true
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return strings.TrimRight(value, "/"), true
}

func (p *NpmPlugin) handlePackageDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Name:         packageName,
	}
	return deleteArtifact(ctx, repoRuntime, key)
}

func deleteArtifact(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
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

func (p *NpmPlugin) handlePackagePut(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	var npmMeta map[string]interface{}
	if err := json.NewDecoder(ctx.Request.Body).Decode(&npmMeta); err != nil {
		http.Error(ctx.Writer, "invalid npm metadata: "+err.Error(), http.StatusBadRequest)
		return nil
	}
	version := ""
	if v, ok := npmMeta["version"].(string); ok {
		version = v
	}

	// 提取 _attachments 中的 tarball 数据
	attachments, _ := npmMeta["_attachments"].(map[string]interface{})
	delete(npmMeta, "_attachments") // 存储 metadata 时移除 attachments 数据

	metadataJSON, _ := json.Marshal(npmMeta)
	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Filename:     packageName,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// 存储 tarball blob（限制单个附件最大 100MB，防止内存耗尽）
	const maxTarballSize = 100 * 1024 * 1024
	for tarballName, att := range attachments {
		attMap, ok := att.(map[string]interface{})
		if !ok {
			continue
		}
		data, _ := attMap["data"].(string)
		if data == "" {
			continue
		}
		// base64 编码后体积约为原始的 4/3 倍，提前估算防止 OOM
		if int64(len(data)) > maxTarballSize*4/3+4 {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, "tarball too large (max 100MB)", http.StatusRequestEntityTooLarge)
			return nil
		}
		tarballBlob, err := session.PutBlob(ctx.Request.Context(), base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)))
		if err != nil {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, "invalid tarball base64: "+err.Error(), http.StatusBadRequest)
			return nil
		}

		tarballVersion := extractNpmVersionFromTarball(packageName, tarballName)
		tarballArtifact := runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Kind:         "tarball",
			Name:         packageName,
			Version:      tarballVersion,
			Path:         packageName + "/-",
			Filename:     tarballName,
			RemotePath:   packageName + "/-/" + tarballName,
			BlobRefs:     []runtime.BlobRef{tarballBlob},
			Properties: map[string]string{
				"package": packageName,
				"version": tarballVersion,
			},
		})
		if err := session.PutArtifact(ctx.Request.Context(), tarballArtifact); err != nil {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
			return nil
		}
	}

	// 存储 metadata blob
	blob, err := session.PutBlob(ctx.Request.Context(), strings.NewReader(string(metadataJSON)))
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Kind:         runtime.KindMetadata,
		Name:         packageName,
		Version:      version,
		BlobRefs:     []runtime.BlobRef{blob},
		Properties: map[string]string{
			"package": packageName,
			"version": version,
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
