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
		if resp.StatusCode == http.StatusNotFound {
			return nil, runtime.ErrNotFound
		}
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

func (p *NpmPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/-all") || path == "-/all" {
		return p.handleAllPackages(ctx, repoRuntime)
	}

	if strings.HasPrefix(path, "-/npm/") {
		return p.handleNpmInternal(ctx, repoRuntime, path)
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
		Kind:         runtime.KindArtifact,
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
		if !errors.Is(err, runtime.ErrNotFound) {
			// 其他错误（含 ErrBlocked）交给 router 处理
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
			if v != "" {
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

func firstNonEmptyNpm(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
		http.Error(ctx.Writer, "invalid npm metadata: "+err.Error(), http.StatusBadRequest)
		return nil
	}
	version := ""
	if v, ok := npmMeta["version"].(string); ok {
		version = v
	}

	// 提取 _attachments 中的 tarball 数据
	attachmentsRaw, hasAttachments := npmMeta["_attachments"]
	logrus.WithFields(logrus.Fields{
		"packageName":     packageName,
		"version":         version,
		"hasAttachments":  hasAttachments,
		"attachmentsType": fmt.Sprintf("%T", attachmentsRaw),
	}).Debug("npm: handlePackagePut called")

	attachments, ok := attachmentsRaw.(map[string]interface{})
	if !ok {
		logrus.WithFields(logrus.Fields{
			"packageName": packageName,
			"reason":      "attachments type assertion failed",
		}).Debug("npm: handlePackagePut skipping attachments")
		attachments = nil
	}
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
	logrus.WithFields(logrus.Fields{
		"packageName":    packageName,
		"attachmentsLen": len(attachments),
	}).Debug("npm: starting tarball loop")

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
			http.Error(ctx.Writer, "invalid tarball base64: "+err.Error(), http.StatusBadRequest)
			return nil
		}
		logrus.WithFields(logrus.Fields{
			"packageName": packageName,
			"tarballName": tarballName,
			"tarballBlob": fmt.Sprintf("%v", tarballBlob),
		}).Debug("npm: PutBlob success")

		tarballVersion := extractNpmVersionFromTarball(packageName, tarballName)
		tarballArtifact := runtime.NewArtifact(runtime.ArtifactSpec{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Kind:         runtime.KindArtifact,
			Name:         packageName,
			Version:      tarballVersion,
			Path:         packageName + "/-",
			Filename:     tarballName,
			RemotePath:   packageName + "/-/" + tarballName,
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
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
			return nil
		}
		logrus.WithFields(logrus.Fields{
			"packageName": packageName,
			"tarballName": tarballName,
		}).Debug("npm: PutArtifact success")
	}

	// 存储 metadata blob
	blob, err := session.PutBlob(ctx.Request.Context(), strings.NewReader(string(metadataJSON)))
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// 从上传的 npm 元数据中提取关键字段到 version artifact 的 Attributes
	// 这样 handlePackageGet 可以统一从 Attributes 还原完整版本元数据
	versionAttrs := extractNpmVersionAttributes(npmMeta)

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Kind:         runtime.KindMetadata,
		Name:         packageName,
		Version:      version,
		Path:         packageName + "/" + version,
		RemotePath:   packageName,
		BlobRefs:     []runtime.BlobRef{blob},
		Attributes:   versionAttrs,
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
		logrus.WithFields(logrus.Fields{
			"packageName": packageName,
			"error":       err.Error(),
		}).Error("npm: Commit failed")
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}
	logrus.WithFields(logrus.Fields{
		"packageName": packageName,
	}).Debug("npm: Commit success")

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}
