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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"
)

type NpmPlugin struct {
	httpClient *http.Client // 统一 HTTP 客户端，可注入
}

func NewNpmPlugin(httpClient *http.Client) *NpmPlugin {
	if httpClient == nil {
		panic("npm: httpClient is required")
	}
	return &NpmPlugin{httpClient: httpClient}
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
		"remote_url":   remoteURL,
		"path":        path,
		"packageName": packageName,
		"full_url":     fullURL,
	}).Debug("npm: FetchRemote called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("full_url", fullURL).Error("npm: create request failed")
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"full_url":  fullURL,
			"duration_ms": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("npm: HTTP request failed")
		return nil, err
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
		}).Error("npm: HTTP request returned non-200 status")
		return nil, fmt.Errorf("npm: fetch from %s: status %d", fullURL, resp.StatusCode)
	}

	artifacts, err := p.parseNpmMetadata(packageName, resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"full_url":  fullURL,
			"duration_ms": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("npm: parse metadata failed")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"full_url":      fullURL,
		"versionCount": len(artifacts),
		"duration_ms":     time.Since(start).Seconds(),
	}).Debug("npm: FetchRemote success")
	return artifacts, nil
}

// FetchArtifactMetadata returns normalized attributes for one npm package
// version. It reuses FetchRemote so package metadata parsing stays identical
// between projection and conditional-rule evaluation.
func (p *NpmPlugin) FetchArtifactMetadata(ctx context.Context, remoteURL string, key runtime.ArtifactKey) (*runtime.ArtifactMetadata, error) {
	if key.Name == "" || key.Version == "" {
		return nil, runtime.ErrMetadataUnavailable
	}
	artifacts, err := p.FetchRemote(ctx, remoteURL, key.Name)
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if artifact.Kind == runtime.KindVersion && artifact.Name == key.Name && artifact.Version == key.Version {
			return &runtime.ArtifactMetadata{Attributes: artifact.Attributes}, nil
		}
	}
	return nil, runtime.ErrMetadataUnavailable
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
	// npm 包可能存在多个版本共享同一个 tarball 的情况
	// （如 lodash 4.17.15-4.17.18 共享 lodash-4.17.15.tgz），
	// 需要去重以避免唯一约束冲突。
	tarballSeen := make(map[string]bool)
	for ver, verRaw := range versions {
		verObj, _ := verRaw.(map[string]interface{})
		props := map[string]string{}
		if verObj != nil {
			props = extractNpmVersionAttributes(verObj)
			// 顶层字段 fallback：版本级字段缺失时使用顶层值
			if props["license"] == "" && topLicense != "" {
				props["license"] = topLicense
			}
			if props["description"] == "" && topDesc != "" {
				props["description"] = topDesc
			}
			if props["homepage"] == "" && topHomepage != "" {
				props["homepage"] = topHomepage
			}
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
			RemotePath: packageName,
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
		// 跳过已存在的 tarball，避免唯一约束冲突
		tarballKey := packageName + "/-/" + tarballName
		if tarballSeen[tarballKey] {
			continue
		}
		tarballSeen[tarballKey] = true
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:      "npm",
			Kind:        runtime.KindArtifact,
			Name:        packageName,
			Version:     ver,
			Path:        packageName + "/-",
			Filename:    tarballName,
			RemotePath:  tarballKey,
			DownloadURL: tarballProps["download_url"],
			Attributes:  map[string]string{"artifact_type": "tarball"},
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
	var raw string
	switch v := lic.(type) {
	case string:
		raw = v
	case map[string]interface{}:
		t, _ := v["type"].(string)
		raw = t
	default:
		return ""
	}
	if !isValidSPDXLicense(raw) {
		return ""
	}
	return raw
}

// isValidSPDXLicense 检查 license 字符串是否为合法的 SPDX 标识符或 SPDX 表达式。
// SPDX 标识符由字母、数字、-、.、+ 组成，不允许空格。
// SPDX 表达式可包含 AND/OR/WITH 关键字和括号。
// 过滤掉 "SEE LICENSE IN README.md"、文件路径、URL 等非 SPDX 值。
func isValidSPDXLicense(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// 过滤常见的非 SPDX 模式
	lower := strings.ToLower(s)
	switch lower {
	case "unlicensed", "none", "n/a", "na", "unknown", "not specified":
		return false
	}
	// 过滤包含 "SEE LICENSE" 的提示性文字
	if strings.Contains(lower, "see license") {
		return false
	}
	// 过滤文件路径和 URL
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return false
	}
	// SPDX 标识符或表达式：允许字母、数字、-、.、+、空格（AND/OR/WITH）、括号
	for _, ch := range s {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '-' && ch != '.' && ch != '+' && ch != ' ' && ch != '(' && ch != ')' {
			return false
		}
	}
	return true
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

// hasVersions 判断 packument 是否已带标准 publish 的 versions 或顶层 version。
func hasVersions(npmMeta map[string]interface{}) bool {
	if versions, ok := npmMeta["versions"].(map[string]interface{}); ok && len(versions) > 0 {
		return true
	}
	if topVersion, ok := npmMeta["version"].(string); ok && topVersion != "" {
		return true
	}
	return false
}

// synthesizePackumentFromTarball 从只有 _attachments 的 UI 上传输入中，解出
// 标准 packument：读取 tarball 的 package/package.json 得到 name/version，
// 再把 attachments 的 key 规整为 <shortName>-<version>.tgz（否则下游按文件名
// 提取版本会失败）。
func (p *NpmPlugin) synthesizePackumentFromTarball(attachments map[string]interface{}) (map[string]interface{}, string, error) {
	var data string
	for _, att := range attachments {
		attMap, ok := att.(map[string]interface{})
		if !ok {
			continue
		}
		if d, _ := attMap["data"].(string); d != "" {
			data = d
			break
		}
	}
	if data == "" {
		return nil, "", errors.New("no tarball attachment data")
	}

	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", fmt.Errorf("decode attachment base64: %w", err)
	}
	name, version, pkgJSON, err := readNpmTarballPackageJSON(raw)
	if err != nil {
		return nil, "", err
	}
	if name == "" || version == "" {
		return nil, "", errors.New("tarball package.json missing name or version")
	}

	versions := map[string]interface{}{version: pkgJSON}
	distTags := map[string]interface{}{"latest": version}

	// 把 attachment 文件名规整为标准 <shortName>-<version>.tgz。
	// scoped 包取 "/" 后的短名称。UI 上传只带单个 tarball，规整第一个即可
	// （版本由 package.json 决定，而非原始文件名的任意版本写法）。
	shortName := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		shortName = name[idx+1:]
	}
	stdName := shortName + "-" + version + ".tgz"
	if _, exists := attachments[stdName]; !exists {
		for origName, att := range attachments {
			delete(attachments, origName)
			attachments[stdName] = att
			break
		}
	}

	result := map[string]interface{}{
		"name":      name,
		"versions":  versions,
		"dist-tags": distTags,
	}
	return result, name, nil
}

// readNpmTarballPackageJSON 解压 npm tarball，读取 package/package.json。
// 返回包名、版本和完整的 package.json 对象（不含由 registry 补充的字段）。
func readNpmTarballPackageJSON(raw []byte) (string, string, map[string]interface{}, error) {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return "", "", nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// tarball 通常以 package/ 为前缀目录
		name := strings.TrimPrefix(path.Clean(hdr.Name), "./")
		if name != "package/package.json" && name != "package.json" {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(tr, 20<<20))
		if err != nil {
			return "", "", nil, err
		}
		var pkgJSON map[string]interface{}
		if err := json.Unmarshal(content, &pkgJSON); err != nil {
			return "", "", nil, err
		}
		data, _ := pkgJSON["name"].(string)
		ver, _ := pkgJSON["version"].(string)
		return data, ver, pkgJSON, nil
	}
	return "", "", nil, errors.New("package.json not found in tarball")
}

func (p *NpmPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/-all") || path == "-/all" {
		return p.handleAllPackages(ctx, repoRuntime)
	}

	// npm CLI 心跳端点：GET /-/ping（不是 /-/npm/ping）
	if path == "-/ping" {
		return p.handlePing(ctx)
	}

	// npm whoami: GET /-/whoami
	if path == "-/whoami" {
		return p.handleWhoami(ctx)
	}

	if strings.HasPrefix(path, "-/npm/") {
		return p.handleNpmInternal(ctx, repoRuntime, path)
	}

	// npm search: GET /-/v1/search?text=xxx&size=N
	if path == "-/v1/search" || strings.HasPrefix(path, "-/v1/search?") {
		return p.handleSearch(ctx, repoRuntime)
	}

	// npm dist-tag: /-/package/{pkg}/dist-tags[/{tag}]
	if strings.HasPrefix(path, "-/package/") && strings.Contains(path, "/dist-tags") {
		return p.handleDistTag(ctx, repoRuntime)
	}

	// npm adduser/login 端点已在路由层直接处理（cmd/registry/router.go），
	// 不经过 Plugin 架构，因为认证操作需要 AuthService 访问权限。

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
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
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

// handlePing 处理 npm CLI 心跳端点 GET /-/ping。
// npm CLI 在每次操作前会请求此端点验证 registry 可用性。
func (p *NpmPlugin) handlePing(ctx *runtime.RequestContext) error {
	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(ctx.Writer).Encode(map[string]interface{}{
		"ok":   true,
		"pong": time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func (p *NpmPlugin) handleNpmInternal(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodPost {
		return errors.New("method not allowed")
	}

	normalized := strings.TrimPrefix(path, "-/npm/")

	// npm registry heartbeat endpoint used by npm CLI (legacy path -/-/npm/ping)
	if normalized == "ping" {
		return p.handlePing(ctx)
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
		Kind:         runtime.KindArtifact,
		Name:         packageName,
		Version:      version,
		Path:         packageName + "/-",
		Filename:     filename,
		RemotePath:   path,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
		// ErrNotFound: 尝试 QueryArtifacts 回源后再 GetArtifact
		artifacts, queryErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Name:         packageName,
			RemotePath:   packageName,
		})
		if queryErr != nil {
			if !errors.Is(queryErr, runtime.ErrNotFound) {
				// 其他错误（含 ErrBlocked）交给 router 处理
				return queryErr
			}
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if len(artifacts) == 0 {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		artifact, err = repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			if errors.Is(err, runtime.ErrNotFound) {
				http.Error(ctx.Writer, "Not found", http.StatusNotFound)
				return nil
			}
			return err
		}
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	ctx.SizeBytes = artifact.SizeBytes

	if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "application/octet-stream", "inline"); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}

func (p *NpmPlugin) handlePackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	packageName := strings.TrimSuffix(path, "/")

	ctx.PackageName = packageName

	switch ctx.Request.Method {
	case http.MethodGet, http.MethodHead:
		return p.handlePackageGet(ctx, repoRuntime, packageName)
	case http.MethodPut:
		return p.handlePackagePut(ctx, repoRuntime, packageName)
	}
	return errors.New("method not allowed")
}

func (p *NpmPlugin) handlePackageGet(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Name:         packageName,
		RemotePath:   packageName,
	})
	if err != nil {
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
		// 主查询 NotFound 不直接 404，继续走 fallback 兜底
		artifacts = nil
	}

	hasVersions := false
	for _, a := range artifacts {
		if a.Version != "" {
			hasVersions = true
			break
		}
	}
	// 兜底查询：存量数据可能只有 Version="" 的 metadata（旧版 bug 写入）
	// + 正常 tarball（RemotePath 不同，主查询未命中）。
	// 放宽 RemotePath 限制，只按 Name 查，让 tarball artifact 纳入结果。
	if !hasVersions {
		fallbackArts, fbErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Name:         packageName,
		})
		if fbErr == nil {
			for _, a := range fallbackArts {
				if a.Version != "" {
					hasVersions = true
					artifacts = fallbackArts
					break
				}
			}
		}
	}
	if !hasVersions {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	repoBase := repoBaseURL(ctx.Request, ctx.Repository.Name)

	versions := make(map[string]interface{})
	var versionList []string
	// 优先处理带有 Attributes 的 artifact（KindVersion 或 KindMetadata），
	// 确保版本元数据完整；tarball 类型的 artifact 没有 Attributes，应后处理。
	sort.SliceStable(artifacts, func(i, j int) bool {
		iHasAttrs := len(artifacts[i].Attributes) > 0
		jHasAttrs := len(artifacts[j].Attributes) > 0
		return iHasAttrs && !jHasAttrs
	})
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

		// 从 artifact Attributes 还原完整版本元数据
		verObj := map[string]interface{}{
			"name":    packageName,
			"version": version,
		}
		dist := map[string]interface{}{
			"tarball": repoBase + "/" + packageName + "/-/" + tarballName,
		}

		// 从 Attributes 还原各字段
		npmStringFields := []string{"description", "main", "module", "type", "license", "homepage"}
		for _, f := range npmStringFields {
			restoreStringField(artifact.Attributes, f, verObj)
		}
		npmJSONFields := []string{
			"bin", "scripts", "dependencies", "devDependencies",
			"peerDependencies", "optionalDependencies", "engines",
			"os", "cpu", "directories", "man", "repository",
			"keywords", "author", "contributors", "bundledDependencies",
			"peerDependenciesMeta", "config",
		}
		for _, f := range npmJSONFields {
			restoreJSONField(artifact.Attributes, f, verObj)
		}

		// dist 子字段
		restoreStringField(artifact.Attributes, "shasum", dist)
		restoreStringField(artifact.Attributes, "integrity", dist)
		restoreJSONField(artifact.Attributes, "unpackedSize", dist)
		restoreJSONField(artifact.Attributes, "dist_signatures", dist, "signatures")

		verObj["dist"] = dist
		versions[version] = verObj
		versionList = append(versionList, version)
	}

	// Compute dist-tags: 优先从 artifact Properties 中提取自定义 tag，fallback 自动计算 latest
	distTags := map[string]string{}
	for _, artifact := range artifacts {
		if tag := artifact.Properties["dist-tag"]; tag != "" {
			v := artifact.Version
			if v == "" {
				continue
			}
			if tag == "latest" {
				// 多次 publish/上传时，旧版本 artifact 不会自动摘除 dist-tag=latest，
				// 多个版本都可能声明 latest。固定选 semver 最新，避免 map 遍历顺序
				// 导致解析到旧版本（否则 npm install pkg 可能装到旧版）。
				if cur, ok := distTags["latest"]; !ok || compareNpmVersions(v, cur) > 0 {
					distTags["latest"] = v
				}
			} else {
				distTags[tag] = v
			}
		}
	}
	if _, hasLatest := distTags["latest"]; !hasLatest && len(versionList) > 0 {
		distTags["latest"] = selectNpmLatestVersion(versionList)
	}

	// 构建 time 字段：从 artifact Attributes 中的 published_at 还原
	timeMap := map[string]string{}
	for _, artifact := range artifacts {
		if artifact.Version != "" {
			if ts, ok := artifact.Attributes["published_at"]; ok && ts != "" {
				timeMap[artifact.Version] = ts
			}
		}
	}

	data := map[string]interface{}{
		"name":      packageName,
		"dist-tags": distTags,
		"versions":  versions,
	}
	if len(timeMap) > 0 {
		data["time"] = timeMap
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	if ctx.Request.Method == http.MethodHead {
		return nil
	}
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func selectNpmLatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	sorted := append([]string(nil), versions...)
	sort.Slice(sorted, func(i, j int) bool {
		return compareNpmVersions(sorted[i], sorted[j]) > 0
	})
	return sorted[0]
}

func compareNpmVersions(a, b string) int {
	normA := normalizeNpmSemver(a)
	normB := normalizeNpmSemver(b)
	validA := semver.IsValid(normA)
	validB := semver.IsValid(normB)
	switch {
	case validA && validB:
		preA := semver.Prerelease(normA) != ""
		preB := semver.Prerelease(normB) != ""
		if preA != preB {
			if preA {
				return -1
			}
			return 1
		}
		return semver.Compare(normA, normB)
	case validA:
		return 1
	case validB:
		return -1
	default:
		return strings.Compare(a, b)
	}
}

func normalizeNpmSemver(version string) string {
	if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") {
		return "v" + strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")
	}
	return "v" + version
}

// restoreStringField 从 Attributes 中读取字符串字段，写入目标 map。
func restoreStringField(attrs map[string]string, key string, target map[string]interface{}) {
	if v, ok := attrs[key]; ok && v != "" {
		target[key] = v
	}
}

// restoreJSONField 从 Attributes 中读取 JSON 序列化的字段，反序列化后写入目标 map。
// 可选 targetKey 参数指定写入目标 map 时的键名（与存储键名不同时使用）。
func restoreJSONField(attrs map[string]string, key string, target map[string]interface{}, targetKey ...string) {
	v, ok := attrs[key]
	if !ok || v == "" {
		return
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(v), &decoded); err != nil {
		// 如果不是合法 JSON，作为原始字符串写入
		dstKey := key
		if len(targetKey) > 0 {
			dstKey = targetKey[0]
		}
		target[dstKey] = v
		return
	}
	dstKey := key
	if len(targetKey) > 0 {
		dstKey = targetKey[0]
	}
	target[dstKey] = decoded
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

// extractNpmVersionAttributes 从 npm 上传元数据中提取关键字段到 Attributes map。
// 复用与 parseNpmMetadata 相同的字段提取逻辑，确保 Hosted 和 Proxy 场景一致。
func extractNpmVersionAttributes(npmMeta map[string]interface{}) map[string]string {
	props := map[string]string{}
	if lic := extractLicense(npmMeta); lic != "" {
		props["license"] = lic
	}
	if desc, ok := npmMeta["description"].(string); ok && desc != "" {
		props["description"] = desc
	}
	if hp, ok := npmMeta["homepage"].(string); ok && hp != "" {
		props["homepage"] = hp
	}
	if main, ok := npmMeta["main"].(string); ok && main != "" {
		props["main"] = main
	}
	if module, ok := npmMeta["module"].(string); ok && module != "" {
		props["module"] = module
	}
	if typ, ok := npmMeta["type"].(string); ok && typ != "" {
		props["type"] = typ
	}
	// 复合字段 JSON 序列化
	npmComplexFields := []string{
		"bin", "scripts", "dependencies", "devDependencies",
		"peerDependencies", "optionalDependencies", "engines",
		"os", "cpu", "directories", "man", "repository",
		"keywords", "author", "contributors", "bundledDependencies",
		"peerDependenciesMeta", "config",
	}
	for _, field := range npmComplexFields {
		if v, ok := npmMeta[field]; ok && v != nil {
			if b, err := json.Marshal(v); err == nil {
				props[field] = string(b)
			}
		}
	}
	// dist 子字段
	if dist, ok := npmMeta["dist"].(map[string]interface{}); ok && dist != nil {
		if shasum, ok := dist["shasum"].(string); ok && shasum != "" {
			props["shasum"] = shasum
		}
		if integrity, ok := dist["integrity"].(string); ok && integrity != "" {
			props["integrity"] = integrity
		}
		if unpackedSize, ok := dist["unpackedSize"]; ok && unpackedSize != nil {
			if b, err := json.Marshal(unpackedSize); err == nil {
				props["unpackedSize"] = string(b)
			}
		}
		if signatures, ok := dist["signatures"].([]interface{}); ok && len(signatures) > 0 {
			if b, err := json.Marshal(signatures); err == nil {
				props["dist_signatures"] = string(b)
			}
		}
	}
	return props
}

func (p *NpmPlugin) handlePackagePut(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	var npmMeta map[string]interface{}
	if err := json.NewDecoder(ctx.Request.Body).Decode(&npmMeta); err != nil {
		{ logrus.WithError(err).Warn("invalid npm metadata"); http.Error(ctx.Writer, "invalid npm metadata", http.StatusBadRequest) }
		return nil
	}

	// 提取 _attachments 中的 tarball 数据
	attachmentsRaw, hasAttachments := npmMeta["_attachments"]
	attachments, _ := attachmentsRaw.(map[string]interface{})
	delete(npmMeta, "_attachments") // 存储 metadata 时移除 attachments 数据

	// UI 上传适配（非 npm CLI publish 标准输入）：
	// 标准 npm publish 会带上完整 versions/dist-tags；而网页上传只提供一个 tarball，
	// 因此 packs 里只有 _attachments、缺少 versions（也没有顶层 version）。
	// 此时从 tarball 的 package/package.json 解出 name/version，补全成标准 packument，
	// 让下面的版本收集与落库逻辑（与 npm publish 完全同一主干）可复用。
	if !hasVersions(npmMeta) && len(attachments) > 0 {
		synthesized, nameFromTarball, err := p.synthesizePackumentFromTarball(attachments)
		if err != nil {
			logrus.WithError(err).WithField("packageName", packageName).Warn("npm: UI upload adapter failed")
			http.Error(ctx.Writer, "unable to read package.json from tarball", http.StatusBadRequest)
			return nil
		}
		npmMeta["versions"] = synthesized["versions"]
		npmMeta["dist-tags"] = synthesized["dist-tags"]
		// 用 tarball 里的包名校验/纠正 URL 中的包名
		if nameFromTarball != "" && nameFromTarball != packageName {
			logrus.WithField("from_url", packageName).WithField("from_tarball", nameFromTarball).Warn("npm: upload package name mismatch, using tarball name")
			packageName = nameFromTarball
		}
	}

	// 解析 dist-tags（用于 metadata artifact 的 Properties）
	distTags := map[string]string{}
	if dt, ok := npmMeta["dist-tags"].(map[string]interface{}); ok {
		for tag, v := range dt {
			if vs, ok := v.(string); ok {
				distTags[tag] = vs
			}
		}
	}

	// 解析 time 字段（版本发布时间）
	timeMap := map[string]string{}
	if tm, ok := npmMeta["time"].(map[string]interface{}); ok {
		for ver, v := range tm {
			if vs, ok := v.(string); ok {
				timeMap[ver] = vs
			}
		}
	}

	// 收集要写入的版本：优先 versions 字典（标准 npm publish 格式），
	// fallback 顶层 version（非标准 body 兼容）。
	type versionMeta struct {
		version string
		obj     map[string]interface{}
	}
	var versionsToWrite []versionMeta
	if versionsRaw, ok := npmMeta["versions"].(map[string]interface{}); ok && len(versionsRaw) > 0 {
		for ver, verRaw := range versionsRaw {
			verObj, ok := verRaw.(map[string]interface{})
			if !ok {
				continue
			}
			versionsToWrite = append(versionsToWrite, versionMeta{version: ver, obj: verObj})
		}
	} else if topVersion, ok := npmMeta["version"].(string); ok && topVersion != "" {
		versionsToWrite = append(versionsToWrite, versionMeta{version: topVersion, obj: npmMeta})
	} else {
		http.Error(ctx.Writer, "invalid npm metadata: missing versions", http.StatusBadRequest)
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"packageName":    packageName,
		"hasAttachments": hasAttachments,
		"versionCount":   len(versionsToWrite),
	}).Debug("npm: handlePackagePut called")

	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Filename:     packageName,
	})
	if err != nil {
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
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
			logrus.WithFields(logrus.Fields{
				"packageName": packageName,
				"tarballName": tarballName,
				"error":       err.Error(),
			}).Error("npm: PutBlob failed")
			session.Abort(ctx.Request.Context())
			{ logrus.WithError(err).Warn("invalid tarball base64"); http.Error(ctx.Writer, "invalid tarball payload", http.StatusBadRequest) }
			return nil
		}

		// 提取纯文件名（_attachments key 可能包含路径前缀，如 @scope/pkg/-/file.tgz）
		tarballFilename := path.Base(tarballName)
		tarballVersion := extractNpmVersionFromTarball(packageName, tarballFilename)
		tarballArtifact := runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Kind:         runtime.KindArtifact,
			Name:         packageName,
			Version:      tarballVersion,
			Path:         packageName + "/-",
			Filename:     tarballFilename,
			RemotePath:   packageName + "/-/" + tarballFilename,
			BlobRefs:     []runtime.BlobRef{tarballBlob},
			Attributes:   map[string]string{"artifact_type": "tarball"},
			Properties: map[string]string{
				"package": packageName,
				"version": tarballVersion,
			},
		})
		if err := session.PutArtifact(ctx.Request.Context(), tarballArtifact); err != nil {
			logrus.WithFields(logrus.Fields{
				"packageName":     packageName,
				"tarballArtifact": fmt.Sprintf("%+v", tarballArtifact),
				"error":           err.Error(),
			}).Error("npm: PutArtifact failed")
			session.Abort(ctx.Request.Context())
			{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
			return nil
		}
	}

	// 为每个版本写入独立的 metadata artifact。
	// IdentityKey 包含 version，避免同包多版本 metadata 互相覆盖。
	for _, vm := range versionsToWrite {
		ver := vm.version
		verObj := vm.obj

		// metadata blob：存对应版本对象序列化
		metadataJSON, _ := json.Marshal(verObj)
		blob, err := session.PutBlob(ctx.Request.Context(), strings.NewReader(string(metadataJSON)))
		if err != nil {
			session.Abort(ctx.Request.Context())
			{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
			return nil
		}

		versionAttrs := extractNpmVersionAttributes(verObj)
		if ts, ok := timeMap[ver]; ok && ts != "" {
			versionAttrs["published_at"] = ts
		}

		props := map[string]string{
			"package": packageName,
			"version": ver,
		}
		// dist-tag：如果某个 tag 指向这个版本，记录到 Properties
		for tag, v := range distTags {
			if v == ver {
				props["dist-tag"] = tag
				break
			}
		}

		artifact := runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Kind:         runtime.KindMetadata,
			Name:         packageName,
			Version:      ver,
			Path:         packageName + "/" + ver,
			RemotePath:   packageName,
			BlobRefs:     []runtime.BlobRef{blob},
			Attributes:   versionAttrs,
			Properties:   props,
			IdentityKey:  "metadata/" + packageName + "/" + ver,
		})

		if err := session.PutArtifact(ctx.Request.Context(), artifact); err != nil {
			session.Abort(ctx.Request.Context())
			{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
			return nil
		}
	}

	if err := session.Commit(ctx.Request.Context()); err != nil {
		logrus.WithFields(logrus.Fields{
			"packageName": packageName,
			"error":       err.Error(),
		}).Error("npm: Commit failed")
		{ logrus.WithError(err).Error("internal error"); http.Error(ctx.Writer, "internal server error", http.StatusInternalServerError) }
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}
