package adapter

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"
)

type NpmAdapter struct {
	*BaseAdapter
}

type NpmPackageMetadata struct {
	ID          string                     `json:"_id"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Versions    map[string]*NpmVersionInfo `json:"versions"`
	DistTags    map[string]string          `json:"dist-tags"`
	Time        map[string]string          `json:"time"`
	Readme      string                     `json:"readme"`
}

type NpmVersionInfo struct {
	ID              string                 `json:"_id"`
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	Description     string                 `json:"description"`
	Main            string                 `json:"main"`
	Homepage        string                 `json:"homepage"`
	License         string                 `json:"license"`
	Author          NpmAuthor              `json:"author"`
	Repository      *NpmRepository         `json:"repository"`
	Dependencies    map[string]interface{} `json:"dependencies"`
	DevDependencies map[string]interface{} `json:"devDependencies"`
	Dist            NpmDist                `json:"dist"`
}

type NpmAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

func (a *NpmAuthor) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	if data[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		a.Name = name
		return nil
	}

	type Alias NpmAuthor
	aux := &struct{ *Alias }{Alias: (*Alias)(a)}
	return json.Unmarshal(data, aux)
}

type NpmDist struct {
	Integrity string `json:"integrity"`
	Tarball   string `json:"tarball"`
	Shasum    string `json:"shasum"`
}

type NpmPerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

type NpmRepository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func NewNpmAdapter(storageSvc *service.StorageService, pkgCache *cache.PackageCache) *NpmAdapter {
	return &NpmAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
	}
}

// validateNpmPath 验证 npm 路径，防止路径遍历攻击
func validateNpmPath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}

	// 检查路径遍历
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal detected")
	}

	// 检查危险字符
	dangerousChars := []string{"\\", "\x00", "\n", "\r"}
	for _, char := range dangerousChars {
		if strings.Contains(path, char) {
			return fmt.Errorf("invalid character in path")
		}
	}

	// 检查绝对路径
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("absolute path not allowed")
	}

	return nil
}

// isValidTarballFilename 验证 npm tarball 文件名是否符合规范
// 格式: package-version.tgz 或 @scope/package-version.tgz
func isValidTarballFilename(filename string) bool {
	if filename == "" || !strings.HasSuffix(filename, ".tgz") {
		return false
	}

	// npm tarball 文件名正则: 可包含字母、数字、@、/、.、-、+、_
	// 简单验证：只允许安全的字符
	validPattern := regexp.MustCompile(`^[@a-zA-Z0-9._\-+]+$`)
	if !validPattern.MatchString(filename) {
		return false
	}

	base := strings.TrimSuffix(filename, ".tgz")
	lastDash := strings.LastIndex(base, "-")
	if lastDash <= 0 {
		return false
	}
	versionPart := base[lastDash+1:]
	return isLikelyVersion(versionPart)
}

// isValidNpmPackageName 验证 npm 包名是否符合规范
// 支持: package-name, @scope/package, @scope/sub/package
func isValidNpmPackageName(name string) bool {
	if name == "" {
		return false
	}

	// scoped 包
	if strings.HasPrefix(name, "@") {
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			return false
		}
		// scope 必须以 @ 开头
		if !strings.HasPrefix(parts[0], "@") {
			return false
		}
		// scope 名称验证（除了开头的 @）
		scopeName := strings.TrimPrefix(parts[0], "@")
		if !isValidScopedName(scopeName) {
			return false
		}
		// 包名验证
		if !isValidScopedName(parts[1]) {
			return false
		}
		return true
	}

	// 普通包名
	return isValidScopedName(name)
}

// isValidScopedName 验证 scope 或包名的有效性
func isValidScopedName(name string) bool {
	if name == "" {
		return false
	}

	// 长度限制
	if len(name) > 214 {
		return false
	}

	// 只能包含小写字母、数字、连字符、下划线
	validPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	return validPattern.MatchString(name)
}

// findLatestSemverVersion 从版本列表中找出语义版本最高的版本号
func findLatestSemverVersion(versions []types.VersionInfo) string {
	var best string
	for _, v := range versions {
		sv := "v" + v.Version // semver 需要 "v" 前缀
		if best == "" {
			best = v.Version
			continue
		}
		if semver.Compare(sv, "v"+best) > 0 {
			best = v.Version
		}
	}
	return best
}

// NewNpmAdapterCompat keeps backward compatibility with old call sites.
// Supported signatures:
//   - NewNpmAdapter(storageSvc, pkgCache)
//   - NewNpmAdapterCompat(pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache)
func NewNpmAdapterCompat(args ...interface{}) *NpmAdapter {
	if len(args) >= 2 {
		if storageSvc, ok := args[0].(*service.StorageService); ok {
			if pkgCache, ok := args[1].(*cache.PackageCache); ok {
				return NewNpmAdapter(storageSvc, pkgCache)
			}
		}
	}
	if len(args) >= 3 {
		if storageSvc, ok := args[2].(*service.StorageService); ok {
			var pkgCache *cache.PackageCache
			if pkgRepo, ok := args[0].(*repository.PackageRepository); ok {
				pkgCache = cache.NewPackageCache(pkgRepo, 5*time.Minute)
			}
			if len(args) >= 5 {
				if c, ok := args[4].(*cache.PackageCache); ok {
					pkgCache = c
				}
			}
			return NewNpmAdapter(storageSvc, pkgCache)
		}
	}
	// 不匹配任何已知签名，应返回错误而非 nil 依赖的适配器
	panic(fmt.Sprintf("NewNpmAdapterCompat: unsupported argument signature (got %d args), expected (storageSvc, pkgCache) or (pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache)", len(args)))
}

func (a *NpmAdapter) Type() PackageType { return NpmType }

func (a *NpmAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	if path == "" {
		return nil, fmt.Errorf("invalid npm package path: %s", path)
	}

	// 路径遍历防护
	if err := validateNpmPath(path); err != nil {
		return nil, fmt.Errorf("invalid npm path: %w", err)
	}

	// 清理路径
	path = strings.Trim(path, "/")

	if strings.Contains(path, ".tgz") {
		return a.resolveTarballPath(path)
	}

	// 解析 npm 包名（包括 scoped 包）
	name, version := parseNpmPackageName(path)

	// 验证包名
	if !isValidNpmPackageName(name) {
		return nil, fmt.Errorf("invalid npm package name: %s", name)
	}

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       "",
		StorageName:    name,
		StorageVersion: version,
		RemotePath:     path,
	}, nil
}

// parseNpmPackageName 解析 npm 包名，支持标准和非标准格式
// 支持的格式:
//   - package → package, ""
//   - @scope/package → @scope/package, ""
//   - @scope/package/version → @scope/package, version
//   - package/version → package, version
//   - @scope/sub-scope/package → @scope/sub-scope/package, "" (嵌套 scoped)
//   - @scope/sub-scope/package/version → @scope/sub-scope/package, version
func parseNpmPackageName(path string) (name, version string) {
	if path == "" {
		return "", ""
	}

	parts := strings.Split(path, "/")

	if !strings.HasPrefix(path, "@") {
		// 非 scoped 包: package 或 package/version
		if len(parts) >= 1 {
			name = parts[0]
			if len(parts) >= 2 {
				version = parts[1]
			}
		}
		return name, version
	}

	// Scoped 包处理
	// npm scoped 包格式: @scope/name 或 @scope/name/version
	// 嵌套 scoped: @scope/sub/name 或 @scope/sub/name/version

	// 计算 scoped 包名的部分数
	// @scope → 1 part
	// @scope/name → 2 parts
	// @scope/sub/name → 3 parts

	// npm registry API 总是返回 @scope/name 格式的两部分
	// 但我们也支持更长的嵌套 scoped 包

	// 找出版本号（最后一个部分是版本号的情况）
	// 版本号通常是 semver 格式，包含数字和点号

	// 策略：从后往前找，如果剩余部分是版本号格式，则分离
	// 否则整个路径都是包名

	if len(parts) == 1 {
		// 只有 @scope，没有包名
		return path, ""
	}

	if len(parts) == 2 {
		// 标准格式: @scope/package
		return parts[0] + "/" + parts[1], ""
	}

	// 可能是嵌套 scoped 或包含版本号
	// 检查最后一部分是否是版本号
	lastPart := parts[len(parts)-1]
	if isLikelyVersion(lastPart) {
		// 最后一部分是版本号
		version = lastPart
		// 包名是前面的所有部分
		name = strings.Join(parts[:len(parts)-1], "/")
		return name, version
	}

	// 没有版本号，整个是包名
	// 可能是嵌套 scoped 包
	name = strings.Join(parts, "/")
	return name, ""
}

// isLikelyVersion 判断字符串是否可能是版本号
func isLikelyVersion(s string) bool {
	if s == "" {
		return false
	}

	// 版本号通常包含数字和点号，可能有预发布标识符
	// 常见格式: 1.0.0, 1.0.0-beta.1, v1.0.0, 1.0.0-rc.1, 2.0.0-alpha+sha

	hasDigit := false
	hasDot := false
	hasPrerelease := false

	validPrerelease := []string{"alpha", "beta", "rc", "pre", "snapshot", "dev", "next", "canary", "preview"}

	for i := 0; i < len(s); {
		c := s[i]
		if c >= '0' && c <= '9' {
			hasDigit = true
			i++
		} else if c == '.' {
			hasDot = true
			i++
		} else if c == '-' {
			i++
		} else if c == '+' {
			// 构建元数据（build metadata），可以包含任何字符，直接跳过剩余部分
			break
		} else if c == '_' {
			i++
		} else if i == 0 && (c == 'v' || c == 'V') {
			i++
		} else if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			// 检查是否为预发布标识符
			rest := strings.ToLower(s[i:])
			isPrerelease := false
			for _, prefix := range validPrerelease {
				if strings.HasPrefix(rest, prefix) {
					isPrerelease = true
					hasPrerelease = true
					i += len(prefix)
					break
				}
			}
			if !isPrerelease {
				return false
			}
		} else {
			return false
		}
	}

	// 标准版本号需要数字和点号（如 1.0.0）
	// 但纯预发布标识符也是允许的（如 alpha, beta.1）
	return (hasDigit && hasDot) || hasPrerelease
}

func (a *NpmAdapter) resolveTarballPath(path string) (*types.PackagePathInfo, error) {
	// 路径遍历防护
	if err := validateNpmPath(path); err != nil {
		return nil, fmt.Errorf("invalid npm tarball path: %w", err)
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid npm tarball path: %s", path)
	}

	filename := parts[len(parts)-1]

	// 验证 tarball 文件名
	if !isValidTarballFilename(filename) {
		return nil, fmt.Errorf("invalid tarball filename: %s", filename)
	}

	var name string
	if idx := strings.Index(path, "/-/"); idx != -1 {
		name = path[:idx]
	} else {
		name = strings.Join(parts[:len(parts)-1], "/")
	}

	version := extractVersionFromTarball(filename, name)

	storageName := name + "/" + filename
	remotePath := path

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       filename,
		StorageName:    storageName,
		StorageVersion: version,
		RemotePath:     remotePath,
	}, nil
}

// ParseIntent 解析请求路径为意图
func (a *NpmAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = trimLeadingSlash(path)
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	// 检查特殊端点
	if strings.HasPrefix(path, "-/package/") {
		// dist-tags 端点: -/package/:package/dist-tags
		return a.parseDistTagsIntent(path)
	}

	pathInfo, err := a.ParsePath(path)
	if err != nil {
		// 无法解析，设为未知意图
		intent.Type = types.RequestUnknown
		return intent
	}
	intent.PkgPathInfo = pathInfo

	// 根据路径特征区分意图
	if strings.Contains(path, ".tgz") {
		// tarball 路径是下载请求
		intent.Type = types.RequestDownload
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
	} else if strings.Contains(path, "/-/") {
		// 附件路径也是下载请求
		intent.Type = types.RequestDownload
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
	} else if pathInfo.Version != "" {
		// 特定版本请求: /:package/:version
		intent.Type = types.RequestMetadata
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
	} else {
		// 其余都是元数据请求
		intent.Type = types.RequestMetadata
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
	}

	return intent
}

// parseDistTagsIntent 解析 dist-tags 端点
func (a *NpmAdapter) parseDistTagsIntent(path string) *types.RequestIntent {
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
		Type:  types.RequestDistTags,
	}

	// 路径格式: -/package/:package/dist-tags 或 -/package/:package/dist-tags/:tag
	rest := strings.TrimPrefix(path, "-/package/")
	marker := "/dist-tags"
	idx := strings.Index(rest, marker)
	if idx < 0 {
		return intent
	}

	intent.Name = rest[:idx]
	tagPath := strings.TrimPrefix(rest[idx+len(marker):], "/")
	if tagPath != "" {
		intent.Extra["tag"] = tagPath
		intent.Type = types.RequestDistTagUpdate
	}

	return intent
}

// FetchContent 根据意图获取内容
//
// 注意：下载请求 (RequestDownload) 由 DownloadService 统一处理，
// 此处不再处理下载，以免绕过日志记录、本地缓存和下载计数。
func (a *NpmAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	switch intent.Type {
	case types.RequestDownload:
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "download requests are handled by DownloadService"},
		}, nil
	case types.RequestMetadata:
		return a.handleMetadataFetch(ctx, repo, intent)
	case types.RequestDistTags:
		return a.handleDistTagsFetch(ctx, repo, intent)
	case types.RequestDistTagUpdate:
		return &types.ContentResult{
			StatusCode: 405,
			ExtraData:  map[string]interface{}{"message": "dist-tag updates require PUT method"},
		}, nil
	default:
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "unknown request type"},
		}, nil
	}
}

// handleMetadataFetch 处理元数据获取
func (a *NpmAdapter) handleMetadataFetch(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	name := intent.Name

	switch repo.Type {
	case model.RepoTypeLocal:
		meta, err := a.GetMetadata(ctx, name)
		if err != nil {
			if util.IsErr(err, util.ErrPackageNotFound) {
				return &types.ContentResult{
					StatusCode: 404,
					ExtraData:  map[string]interface{}{"message": "package not found"},
				}, nil
			}
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:  map[string]interface{}{"message": err.Error()},
			}, nil
		}
		metaJSON, marshalErr := json.Marshal(meta)
		if marshalErr != nil {
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:  map[string]interface{}{"message": "failed to marshal metadata"},
			}, nil
		}

		// 计算 ETag
		hash := sha512.Sum512(metaJSON)
		etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
		lastModified := time.Now().UTC().Format(http.TimeFormat)

		return &types.ContentResult{
			ContentType: "application/json",
			StatusCode:  200,
			Content:     io.NopCloser(bytes.NewReader(metaJSON)),
			Size:        int64(len(metaJSON)),
			Headers: map[string]string{
				"ETag":          etag,
				"Last-Modified": lastModified,
				"Cache-Control": "public, max-age=86400",
			},
		}, nil

	case model.RepoTypeProxy:
		return a.handleProxyMetadataFetch(ctx, repo, name)

	case model.RepoTypeVirtual:
		return a.handleVirtualMetadataFetch(ctx, repo, name)

	default:
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "unknown repository type"},
		}, nil
	}
}

// handleDistTagsFetch 处理 dist-tags 查询
// GET /-/package/:package/dist-tags
func (a *NpmAdapter) handleDistTagsFetch(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	name := intent.Name

	meta, err := a.GetMetadata(ctx, name)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			return &types.ContentResult{
				StatusCode: 404,
				ExtraData:  map[string]interface{}{"error": "not found"},
			}, nil
		}
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"error": err.Error()},
		}, nil
	}

	// 构建 dist-tags 响应
	// npm registry API 的 dist-tags 只包含 tag → version 映射
	distTags := make(map[string]string)

	// 从版本信息中收集已有的 dist-tags
	for _, ver := range meta.Versions {
		for _, tag := range ver.DistTags {
			if tag != "" && tag != ver.Version {
				distTags[tag] = ver.Version
			}
		}
	}

	// 如果没有任何 dist-tags，使用语义版本排序确定最高版本作为 latest
	if _, ok := distTags["latest"]; !ok && len(meta.Versions) > 0 {
		latestVer := findLatestSemverVersion(meta.Versions)
		if latestVer != "" {
			distTags["latest"] = latestVer
		}
	}

	distTagsJSON, marshalErr := json.Marshal(distTags)
	if marshalErr != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"error": "failed to marshal dist-tags"},
		}, nil
	}

	// 计算 ETag
	hash := sha512.Sum512(distTagsJSON)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	return &types.ContentResult{
		ContentType: "application/json",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader(distTagsJSON)),
		Size:        int64(len(distTagsJSON)),
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=86400",
		},
	}, nil
}

// handleProxyMetadataFetch 处理代理仓库元数据请求
func (a *NpmAdapter) handleProxyMetadataFetch(ctx context.Context, repo *model.Repository, name string) (*types.ContentResult, error) {
	if a.metaCache != nil && a.fetcher != nil {
		ttl := time.Duration(repo.CacheTTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}

		content, _, err := a.metaCache.GetOrFetch(ctx, repo.Name, "npm", name, ttl, func() (io.ReadCloser, int64, error) {
			pathInfo, pathErr := a.ParsePath(name)
			if pathErr != nil {
				return nil, 0, pathErr
			}
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr != nil {
				return nil, 0, fetchErr
			}
			return result.Content, result.Size, nil
		})
		if err == nil {
			// Rewrite upstream tarball URLs to point to this proxy
			raw, readErr := io.ReadAll(content)
			if readErr != nil {
				return nil, readErr
			}
			baseURL := BaseURLFromContext(ctx, repo)
			raw = RewriteNpmTarballURLs(raw, baseURL)

			// 计算 ETag
			hash := sha512.Sum512(raw)
			etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
			lastModified := time.Now().UTC().Format(http.TimeFormat)

			return &types.ContentResult{
				Content:     io.NopCloser(bytes.NewReader(raw)),
				Size:        int64(len(raw)),
				ContentType: "application/json",
				StatusCode:  200,
				Headers: map[string]string{
					"ETag":          etag,
					"Last-Modified": lastModified,
					"Cache-Control": "public, max-age=86400",
				},
			}, nil
		}
	}

	// Fallback: 无缓存或无 fetcher 时直接远程获取
	if a.fetcher == nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}
	pathInfo, err := a.ParsePath(name)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}
	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}

	// Rewrite upstream tarball URLs to point to this proxy
	defer result.Content.Close()
	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": "failed to read response"},
		}, nil
	}
	baseURL := BaseURLFromContext(ctx, repo)
	body = RewriteNpmTarballURLs(body, baseURL)

	// 计算 ETag
	hash := sha512.Sum512(body)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	return &types.ContentResult{
		Content:     io.NopCloser(bytes.NewReader(body)),
		Size:        int64(len(body)),
		ContentType: "application/json",
		StatusCode:  200,
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=86400",
		},
	}, nil
}

// handleVirtualMetadataFetch 处理虚拟仓库元数据请求（不依赖 gin.Context）
func (a *NpmAdapter) handleVirtualMetadataFetch(ctx context.Context, repo *model.Repository, name string) (*types.ContentResult, error) {
	if a.fetcher == nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}

	// 尝试使用元数据缓存（与 proxy 仓库行为一致）
	if a.metaCache != nil {
		ttl := time.Duration(repo.CacheTTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}

		content, _, err := a.metaCache.GetOrFetch(ctx, repo.Name, "npm", name, ttl, func() (io.ReadCloser, int64, error) {
			pathInfo, pathErr := a.ParsePath(name)
			if pathErr != nil {
				return nil, 0, pathErr
			}
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr != nil {
				return nil, 0, fetchErr
			}
			return result.Content, result.Size, nil
		})
		if err == nil {
			raw, readErr := io.ReadAll(content)
			if readErr != nil {
				return nil, readErr
			}
			// 重写 tarball URL，指向当前仓库地址
			baseURL := BaseURLFromContext(ctx, repo)
			raw = RewriteNpmTarballURLs(raw, baseURL)

			hash := sha512.Sum512(raw)
			etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
			lastModified := time.Now().UTC().Format(http.TimeFormat)

			return &types.ContentResult{
				Content:     io.NopCloser(bytes.NewReader(raw)),
				Size:        int64(len(raw)),
				ContentType: "application/json",
				StatusCode:  200,
				Headers: map[string]string{
					"ETag":          etag,
					"Last-Modified": lastModified,
					"Cache-Control": "public, max-age=86400",
				},
			}, nil
		}
	}

	// Fallback: 无缓存时直接远程获取
	pathInfo, err := a.ParsePath(name)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}
	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}

	// 读取内容并添加缓存头
	defer result.Content.Close()
	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": "failed to read response"},
		}, nil
	}

	// 重写 tarball URL，指向当前仓库地址
	baseURL := BaseURLFromContext(ctx, repo)
	body = RewriteNpmTarballURLs(body, baseURL)

	// 计算 ETag
	hash := sha512.Sum512(body)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	return &types.ContentResult{
		Content:     io.NopCloser(bytes.NewReader(body)),
		Size:        int64(len(body)),
		ContentType: "application/json",
		StatusCode:  200,
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=86400",
		},
	}, nil
}

func extractVersionFromTarball(filename, pkgName string) string {
	// package-1.0.0.tgz → 1.0.0
	// @scope/package-1.0.0.tgz → 1.0.0
	base := strings.TrimSuffix(filename, ".tgz")

	// 移除包名部分
	pkgNamePart := filepath.Base(pkgName)
	if strings.HasPrefix(base, pkgNamePart+"-") {
		return strings.TrimPrefix(base, pkgNamePart+"-")
	}

	return base
}

func (a *NpmAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetRepositoryPackageMetadata(ctx, repositoryFromContext(ctx), name, model.PackageTypeNPM, NpmType)
}

func (a *NpmAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypeNPM)
}

func (a *NpmAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeNPM)
}

func (a *NpmAdapter) syncFromProxy(ctx context.Context, name string, repo *model.Repository) error {
	pathInfo, err := a.ParsePath(name)
	if err != nil {
		return err
	}
	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return err
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return readErr
	}

	var metadata NpmPackageMetadata
	if jsonErr := json.Unmarshal(body, &metadata); jsonErr != nil {
		return jsonErr
	}

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(ctx, &model.Package{
		Name:           metadata.Name,
		Type:           model.PackageTypeNPM,
		RepositoryType: model.RepoTypeProxy,
		Description:    metadata.Description,
	}, nil)
	if err != nil {
		return err
	}

	// 提取许可证信息（从 latest 版本获取）
	packageLicense := extractNpmLicense(&metadata)

	// 更新包的许可证信息
	if packageLicense != "" {
		a.GetPackageRepository().DB().Model(pkg).Update("license", packageLicense)
	}

	for version, verInfo := range metadata.Versions {
		if verInfo == nil {
			continue
		}
		publishedAt := parseTime(metadata.Time[version])
		versionMeta := marshalMetadata(map[string]interface{}{
			"version":     version,
			"publishedAt": publishedAt.Format(time.RFC3339),
			"tarball":     verInfo.Dist.Tarball,
			"license":     verInfo.License,
		})

		_, savedVer, err := a.GetPackageRepository().CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
			Version:     version,
			Status:      model.StatusPublished,
			PublishedAt: publishedAt,
			Metadata:    versionMeta,
			License:     verInfo.License, // 版本级许可证
		})
		if err != nil {
			continue
		}
		a.enqueueDependencyUpsert(savedVer.ID, buildNpmDependencies(*verInfo))
	}

	return nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// extractNpmLicense 从 npm 元数据中提取许可证信息
// npm 包的许可证通常在各版本一致，取 latest 版本或任意版本
func extractNpmLicense(meta *NpmPackageMetadata) string {
	// 优先从 latest 版本获取
	if latest, ok := meta.DistTags["latest"]; ok {
		if v, ok := meta.Versions[latest]; ok && v.License != "" {
			return normalizeLicense(v.License)
		}
	}
	// 或者取任意一个版本
	for _, v := range meta.Versions {
		if v.License != "" {
			return normalizeLicense(v.License)
		}
	}
	return ""
}

// normalizeLicense 标准化许可证名称为 SPDX 格式
func normalizeLicense(license string) string {
	if license == "" {
		return ""
	}
	license = strings.TrimSpace(license)
	license = strings.TrimPrefix(license, "(")
	license = strings.TrimSuffix(license, ")")

	// 常见许可证标准化映射
	licenseAliases := map[string]string{
		"MIT":                "MIT",
		"Apache 2.0":         "Apache-2.0",
		"Apache-2":           "Apache-2.0",
		"Apache License 2.0": "Apache-2.0",
		"BSD-3-Clause":       "BSD-3-Clause",
		"BSD-2-Clause":       "BSD-2-Clause",
		"BSD":                "BSD-3-Clause",
		"ISC":                "ISC",
		"GPL-3.0":            "GPL-3.0",
		"GPLv3":              "GPL-3.0",
		"LGPL-2.1":           "LGPL-2.1",
		"LGPL-3.0":           "LGPL-3.0",
		"MPL-2.0":            "MPL-2.0",
		"EPL-2.0":            "EPL-2.0",
		"CC0-1.0":            "CC0-1.0",
	}

	upperLicense := strings.ToUpper(license)
	for alias, standard := range licenseAliases {
		if strings.Contains(upperLicense, strings.ToUpper(alias)) {
			return standard
		}
	}
	return license
}

// MergeMetadata implements the MetadataMerger interface for virtual repository support.
// Merges npm metadata from multiple members by deeply merging the "versions" map
// and picking the best "dist-tags".
func (a *NpmAdapter) MergeMetadata(ctx context.Context, results []*types.ContentResult, intent *types.RequestIntent) (*types.ContentResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to merge")
	}

	// All results should be npm metadata JSON
	merged := make(map[string]interface{})
	mergedVersions := make(map[string]interface{})
	var distTags map[string]interface{}
	var name string
	var firstResultData map[string]interface{} // 保存首个成功解析的结果，用于复制其他顶层字段

	for _, res := range results {
		if res.Content == nil && res.ExtraData == nil {
			continue
		}

		var data map[string]interface{}
		if res.ExtraData != nil {
			// Clone to avoid mutating the original
			data = shallowCopyMap(res.ExtraData)
		} else {
			body, err := io.ReadAll(res.Content)
			res.Content.Close()
			if err != nil {
				continue
			}
			if json.Unmarshal(body, &data) != nil {
				continue
			}
		}

		// 保存首个成功解析的结果
		if firstResultData == nil {
			firstResultData = data
		}

		// Extract name
		if n, ok := data["name"].(string); ok && name == "" {
			name = n
		}

		// Merge versions
		if versions, ok := data["versions"].(map[string]interface{}); ok {
			for ver, verInfo := range versions {
				if _, exists := mergedVersions[ver]; !exists {
					mergedVersions[ver] = verInfo
				}
			}
		}

		// Merge dist-tags (last one wins for each tag)
		if dt, ok := data["dist-tags"].(map[string]interface{}); ok {
			if distTags == nil {
				distTags = make(map[string]interface{})
			}
			for tag, ver := range dt {
				distTags[tag] = ver
			}
		}
	}

	if len(mergedVersions) == 0 {
		return results[0], nil
	}

	merged["name"] = name
	merged["versions"] = mergedVersions
	if distTags != nil {
		merged["dist-tags"] = distTags
	}

	// 从首个解析结果复制其他顶层字段（description、readme、time 等）
	if firstResultData != nil {
		for k, v := range firstResultData {
			switch k {
			case "versions", "dist-tags", "name":
				continue
			}
			if _, exists := merged[k]; !exists {
				merged[k] = v
			}
		}
	}

	// Rewrite tarball URLs to use the virtual repo base URL
	baseURL := BaseURLFromContext(ctx, nil)
	if baseURL != "" {
		RewriteNpmTarballURLsInMap(merged, baseURL)
	}

	resultJSON, err := json.Marshal(merged)
	if err != nil {
		return results[0], nil
	}

	hash := sha512.Sum512(resultJSON)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:16]))
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	return &types.ContentResult{
		Content:     io.NopCloser(bytes.NewReader(resultJSON)),
		Size:        int64(len(resultJSON)),
		ContentType: "application/json",
		StatusCode:  200,
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=60",
		},
	}, nil
}

// shallowCopyMap creates a shallow copy of a map (sufficient for our merge use case
// where top-level values are either primitives or maps we don't modify in-place).
func shallowCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func generateRevision() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func buildNpmDependencies(meta NpmVersionInfo) []model.PackageDependency {
	deps := make([]model.PackageDependency, 0, len(meta.Dependencies)+len(meta.DevDependencies))
	seen := make(map[string]struct{}, len(meta.Dependencies)+len(meta.DevDependencies))

	appendDep := func(name string, constraint interface{}, optional bool) {
		if name == "" {
			return
		}
		version := fmt.Sprint(constraint)
		if version == "" {
			return
		}
		key := name + "|" + version + "|" + fmt.Sprint(optional)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		deps = append(deps, model.PackageDependency{
			DepName:              name,
			DepVersionConstraint: version,
			DepType:              "direct",
			PackageType:          string(model.PackageTypeNPM),
			IsOptional:           optional,
		})
	}

	for name, constraint := range meta.Dependencies {
		appendDep(name, constraint, false)
	}
	for name, constraint := range meta.DevDependencies {
		appendDep(name, constraint, true)
	}

	return deps
}

func (a *NpmAdapter) enqueueDependencyUpsert(versionID uint, deps []model.PackageDependency) {
	if versionID == 0 || len(deps) == 0 {
		return
	}
	pkgRepo := a.GetPackageRepository()
	if pkgRepo == nil {
		return
	}

	cloned := make([]model.PackageDependency, len(deps))
	copy(cloned, deps)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := pkgRepo.UpsertVersionDependencies(ctx, versionID, cloned); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"version_id": versionID,
				"dep_count":  len(cloned),
			}).Warn("failed to async upsert npm dependencies from metadata sync")
		}
	}()
}

func marshalMetadata(meta map[string]interface{}) string {
	data, _ := json.Marshal(meta)
	return string(data)
}

func (a *NpmAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	fullPath := cutBeforeMarker(trimLeadingSlash(c.Param("path")), "/-rev/")
	parts := splitPathN(fullPath, 2)
	scope := ""
	pkgName := fullPath

	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
		scope = parts[0]
		pkgName = parts[1]
	}

	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	file, header, err := c.Request.FormFile("_attachments")
	if err != nil {
		return nil, fmt.Errorf("missing attachment: %v", err)
	}
	defer file.Close()

	// npm publish 的请求体中，metadata 通过包名作为 key 的 JSON 字段传递
	// 格式：multipart form 中，包名对应的字段包含该版本的 metadata
	var metadata NpmVersionInfo
	if metadataRaw := c.PostForm(name); metadataRaw != "" {
		if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
			return nil, fmt.Errorf("invalid metadata for package %s: %v", name, err)
		}
	} else {
		// 兼容某些客户端使用包名不含 scope 的形式
		shortName := pkgName
		if metadataRaw := c.PostForm(shortName); metadataRaw != "" {
			if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
				return nil, fmt.Errorf("invalid metadata for package %s: %v", shortName, err)
			}
		} else {
			return nil, fmt.Errorf("missing metadata for package %s in request body", name)
		}
	}

	allowOverwrite := c.GetBool("allowOverwrite")
	if !allowOverwrite {
		pkgRepo := a.GetPackageRepository()
		if pkgRepo == nil {
			return nil, fmt.Errorf("package repository not initialized")
		}
		existingPkg, err := pkgRepo.FindByRepoNameAndTypeContext(c.Request.Context(), repositoryID(repositoryFromContext(c.Request.Context())), name, model.PackageTypeNPM)
		if err == nil {
			for _, ver := range existingPkg.Versions {
				if ver.Version == metadata.Version {
					return nil, fmt.Errorf("版本 %s 已存在，不允许覆盖", metadata.Version)
				}
			}
		}
	}

	return &types.PublishResult{
		PackageName:  name,
		Version:      metadata.Version,
		Filename:     header.Filename,
		Content:      file,
		Size:         header.Size,
		FileType:     model.FileTypePrimary,
		Dependencies: buildNpmDependencies(metadata),
		Response: &types.NpmPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  name,
				Version:  metadata.Version,
				Filename: header.Filename,
				Size:     header.Size,
			},
			Description: metadata.Description,
		},
	}, nil
}

func (a *NpmAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := cutBeforeMarker(trimLeadingSlash(c.Param("path")), "/-rev/")
	parts := splitPathN(fullPath, 2)
	scope := ""
	pkgName := fullPath

	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
		scope = parts[0]
		pkgName = parts[1]
	}

	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	identity := &PackageIdentity{
		Name: name,
		Type: NpmType,
	}

	if identity.Version == "" {
		versions, err := a.ListVersions(context.WithValue(c.Request.Context(), "repo", ctx.Repo), name)
		if err != nil {
			return err
		}
		for _, version := range versions {
			if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), &PackageIdentity{
				Name:    name,
				Version: version,
				Type:    NpmType,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), identity); err != nil {
		return err
	}

	return nil
}
