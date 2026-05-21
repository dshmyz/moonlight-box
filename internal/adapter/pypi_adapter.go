package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

// PyPI 路径遍历防护：检查路径是否包含危险字符或路径遍历
func validatePyPIPath(path string) error {
	// 检查空路径
	if path == "" {
		return fmt.Errorf("invalid pypi path: empty path")
	}

	// 检查路径遍历攻击
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid pypi path: path traversal not allowed")
	}

	// 检查危险字符
	dangerousPatterns := []string{
		"~", "$", "`", "|", ";", "&", "(", ")", "<", ">", "\n", "\r", "\x00",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(path, pattern) {
			return fmt.Errorf("invalid pypi path: dangerous character not allowed")
		}
	}

	return nil
}

// isValidWheelFilename 验证 wheel 文件名格式 (PEP 427)
func isValidWheelFilename(filename string) bool {
	// Wheel filename: {distribution}-{version}(-{build tag})?-{python tag}-{abi tag}-{platform tag}.whl
	// 示例: my_package-1.0-py3-none-any.whl
	wheelPattern := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*-[A-Za-z0-9_.-]+-py[0-9]+-[a-z]+-[a-z0-9_]+(-[A-Za-z0-9_]+)?\.whl$`)
	return wheelPattern.MatchString(filename)
}

// isValidSdistFilename 验证 source distribution 文件名格式 (PEP 625)
func isValidSdistFilename(filename string) bool {
	// Source distribution: {distribution}-{version}.tar.gz / .tar.bz2 / .zip
	sdistPattern := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*-[A-Za-z0-9_.-]+\.(tar\.gz|tar\.bz2|zip)$`)
	return sdistPattern.MatchString(filename)
}

// isValidPyPIFilename 验证 PyPI 包文件名格式
func isValidPyPIFilename(filename string) bool {
	return isValidWheelFilename(filename) || isValidSdistFilename(filename)
}

var pypiNameExtractRe = regexp.MustCompile(`-\d+.*$`)

func parsePyPIDepName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, ";"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "("); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func parsePyPIDepConstraint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, ";"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "("); idx >= 0 {
		end := strings.Index(raw[idx:], ")")
		if end > 0 {
			return strings.TrimSpace(raw[idx+1 : idx+end])
		}
	}
	fields := strings.Fields(raw)
	if len(fields) >= 2 {
		return strings.Join(fields[1:], " ")
	}
	return "*"
}

func buildPyPIDependencies(requireLines []string) []model.PackageDependency {
	deps := make([]model.PackageDependency, 0, len(requireLines))
	seen := make(map[string]struct{}, len(requireLines))
	for _, line := range requireLines {
		name := parsePyPIDepName(line)
		if name == "" {
			continue
		}
		constraint := parsePyPIDepConstraint(line)
		key := name + "|" + constraint
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deps = append(deps, model.PackageDependency{
			DepName:              name,
			DepVersionConstraint: constraint,
			DepType:              "direct",
			PackageType:          string(model.PackageTypePyPI),
		})
	}
	return deps
}

func looksLikeJSON(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

type PyPIAdapter struct {
	*BaseAdapter
	repoRepo *repository.RepositoryRepository
}

func NewPyPIAdapter(args ...interface{}) *PyPIAdapter {
	var repoRepo *repository.RepositoryRepository
	var storageSvc *service.StorageService
	var pkgCache *cache.PackageCache

	// New signature: (repoRepo, storageSvc, pkgCache)
	if len(args) >= 1 {
		if r, ok := args[0].(*repository.RepositoryRepository); ok {
			repoRepo = r
		}
	}
	if len(args) >= 2 {
		if s, ok := args[1].(*service.StorageService); ok {
			storageSvc = s
		}
	}
	if len(args) >= 3 {
		if c, ok := args[2].(*cache.PackageCache); ok {
			pkgCache = c
		}
	}

	// Legacy signature: (pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache)
	if repoRepo == nil && len(args) >= 2 {
		if r, ok := args[1].(*repository.RepositoryRepository); ok {
			repoRepo = r
		}
	}
	if storageSvc == nil && len(args) >= 3 {
		if s, ok := args[2].(*service.StorageService); ok {
			storageSvc = s
		}
	}
	if pkgCache == nil && len(args) >= 1 {
		if pkgRepo, ok := args[0].(*repository.PackageRepository); ok {
			pkgCache = cache.NewPackageCache(pkgRepo, 5*time.Minute)
		}
	}

	return &PyPIAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
		repoRepo:    repoRepo,
	}
}

func (a *PyPIAdapter) Type() PackageType { return PyPIType }

func (a *PyPIAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	path = strings.Trim(path, "/")

	// 路径遍历防护
	if err := validatePyPIPath(path); err != nil {
		return nil, err
	}

	if path == "" {
		return nil, fmt.Errorf("invalid pypi path: empty path")
	}

	parts := strings.Split(path, "/")

	if len(parts) >= 2 && parts[0] == "pypi" {
		// Handle "pypi/simple/xxx/" paths (URL rewrite output)
		if parts[1] == "simple" {
			if len(parts) >= 3 && parts[2] != "" {
				version := ""
				if len(parts) >= 4 && parts[3] != "" {
					version = parts[3]
				}
				return &types.PackagePathInfo{
					Name:           parts[2],
					Version:        version,
					Filename:       "",
					StorageName:    parts[2],
					StorageVersion: version,
					RemotePath:     fmt.Sprintf("simple/%s/", parts[2]),
				}, nil
			}
			return nil, fmt.Errorf("invalid pypi pypi/simple path: %s", path)
		}

		// Handle "pypi/{name}/json" and "pypi/{name}/{version}/json" paths
		if len(parts) >= 3 && parts[len(parts)-1] == "json" {
			name := parts[1]
			version := ""
			if len(parts) == 4 {
				// pypi/{name}/{version}/json
				version = parts[2]
			}
			return &types.PackagePathInfo{
				Name:           name,
				Version:        version,
				Filename:       "",
				StorageName:    name,
				StorageVersion: version,
				RemotePath:     fmt.Sprintf("pypi/%s/json", name),
			}, nil
		}
	}

	if parts[0] == "simple" {
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid pypi simple path: %s", path)
		}

		version := ""
		if len(parts) >= 3 {
			version = parts[2]
		}

		return &types.PackagePathInfo{
			Name:           parts[1],
			Version:        version,
			Filename:       "",
			StorageName:    parts[1],
			StorageVersion: version,
			RemotePath:     fmt.Sprintf("simple/%s/", parts[1]),
		}, nil
	}

	if parts[0] == "packages" {
		filename := parts[len(parts)-1]

		// 验证包文件名格式
		if !isValidPyPIFilename(filename) {
			return nil, fmt.Errorf("invalid pypi package filename: %s", filename)
		}

		name := strings.TrimSuffix(filename, ".whl")
		name = strings.TrimSuffix(name, ".tar.gz")
		name = pypiNameExtractRe.ReplaceAllString(name, "")

		return &types.PackagePathInfo{
			Name:           name,
			Version:        "",
			Filename:       filename,
			StorageName:    name,
			StorageVersion: filename,
			RemotePath:     path,
		}, nil
	}

	if len(parts) == 1 {
		return &types.PackagePathInfo{
			Name:           parts[0],
			Version:        "",
			Filename:       "",
			StorageName:    parts[0],
			StorageVersion: "",
			RemotePath:     fmt.Sprintf("simple/%s/", parts[0]),
		}, nil
	}

	if len(parts) == 2 {
		return &types.PackagePathInfo{
			Name:           parts[0],
			Version:        parts[1],
			Filename:       "",
			StorageName:    parts[0],
			StorageVersion: parts[1],
			RemotePath:     fmt.Sprintf("simple/%s/", parts[0]),
		}, nil
	}

	return nil, fmt.Errorf("invalid pypi path: %s", path)
}

// ListPackages 列出包列表。根据仓库类型返回不同结果：
// - local: 仅本地缓存的包
// - proxy: 代理上游的完整索引
// - virtual: 调用方应合并成员结果，此处返回本地数据
func (a *PyPIAdapter) ListPackages(ctx context.Context, repo *model.Repository, acceptHeader string) (*types.ContentResult, error) {
	if repo != nil && repo.Type == model.RepoTypeProxy {
		return a.listPackagesFromUpstream(ctx, repo, acceptHeader)
	}
	// local 和 virtual 成员走本地数据库
	if strings.Contains(acceptHeader, "application/vnd.pypi.simple") || strings.Contains(acceptHeader, "application/json") {
		return a.listPackagesJSON(ctx)
	}
	return a.listPackagesHTML(ctx)
}

// listPackagesFromUpstream 从代理上游获取完整的 simple index
func (a *PyPIAdapter) listPackagesFromUpstream(ctx context.Context, repo *model.Repository, acceptHeader string) (*types.ContentResult, error) {
	if a.fetcher == nil {
		// 没有 fetcher 时回退到本地
		if strings.Contains(acceptHeader, "application/vnd.pypi.simple") || strings.Contains(acceptHeader, "application/json") {
			return a.listPackagesJSON(ctx)
		}
		return a.listPackagesHTML(ctx)
	}

	contentType := "text/html"
	if strings.Contains(acceptHeader, "application/json") {
		contentType = "application/vnd.pypi.simple.v1+json"
	} else if strings.Contains(acceptHeader, "application/vnd.pypi.simple") {
		contentType = acceptHeader
	}

	remoteURL := strings.TrimSuffix(repo.RemoteURL, "/") + "/simple/"
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		// 上游获取失败时回退到本地
		if strings.Contains(acceptHeader, "application/vnd.pypi.simple") || strings.Contains(acceptHeader, "application/json") {
			return a.listPackagesJSON(ctx)
		}
		return a.listPackagesHTML(ctx)
	}
	defer result.Content.Close()
	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return nil, readErr
	}

	// 重写上游 HTML 中的下载链接和 simple index 导航链接为相对路径
	if strings.HasPrefix(contentType, "text/html") {
		body = RewritePyPIHTML(body)
	}

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: contentType,
		Content:     io.NopCloser(bytes.NewReader(body)),
		Size:        int64(len(body)),
	}, nil
}

func (a *PyPIAdapter) listPackagesJSON(ctx context.Context) (*types.ContentResult, error) {
	packages, _, err := a.GetPackageRepository().ListContext(ctx, 1, 10000, "pypi", "")
	if err != nil {
		return nil, err
	}

	type project struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	result := make([]project, len(packages))
	for i, pkg := range packages {
		normalized := normalizePackageName(pkg.Name)
		result[i] = project{
			Name: normalized,
			URL:  normalized + "/",
		}
	}

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		ExtraData:   gin.H{"projects": result},
	}, nil
}

func (a *PyPIAdapter) listPackagesHTML(ctx context.Context) (*types.ContentResult, error) {
	packages, _, err := a.GetPackageRepository().ListContext(ctx, 1, 10000, "pypi", "")
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.Grow(100 + len(packages)*80)
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n")
	for _, pkg := range packages {
		normalized := normalizePackageName(pkg.Name)
		sb.WriteString(`<a href="`)
		sb.WriteString(normalized)
		sb.WriteString(`/">`)
		sb.WriteString(normalized)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     io.NopCloser(strings.NewReader(sb.String())),
		Size:        int64(sb.Len()),
	}, nil
}

func (a *PyPIAdapter) PackageFiles(ctx context.Context, acceptHeader string, pkgName string, repo *model.Repository) (*types.ContentResult, error) {
	if a.metaCache != nil && a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		ttl := time.Duration(repo.CacheTTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}

		contentType := "text/html"
		if strings.Contains(acceptHeader, "application/json") {
			contentType = "application/vnd.pypi.simple.v1+json"
		}

		cacheKey := "simple/" + pkgName
		if strings.Contains(contentType, "json") {
			cacheKey = "simple/" + pkgName + "/json"
		}

		content, _, err := a.metaCache.GetOrFetch(ctx, repo.Name, "pypi", cacheKey, ttl, func() (io.ReadCloser, int64, error) {
			pathInfo, pathErr := a.ParsePath("simple/" + pkgName + "/")
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
			// Rewrite upstream URLs to point to this proxy
			raw, readErr := io.ReadAll(content)
			if readErr != nil {
				return nil, readErr
			}
			if strings.HasPrefix(contentType, "text/html") {
				raw = RewritePyPIHTML(raw)
			} else {
				raw = RewritePyPIJSON(raw)
			}
			return &types.ContentResult{
				StatusCode:  200,
				ContentType: contentType,
				Content:     io.NopCloser(bytes.NewReader(raw)),
				Size:        int64(len(raw)),
			}, nil
		}
	}

	if strings.Contains(acceptHeader, "application/json") {
		return a.packageFilesJSON(ctx, pkgName, repo)
	}
	return a.packageFilesHTML(ctx, pkgName, repo)
}

func (a *PyPIAdapter) packageFilesJSON(ctx context.Context, pkgName string, repo *model.Repository) (*types.ContentResult, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), normalizePackageName(pkgName), model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil && repo != nil {
				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						body = RewritePyPIJSON(body)
						return &types.ContentResult{
							StatusCode:  200,
							ContentType: "application/vnd.pypi.simple.v1+json",
							Content:     io.NopCloser(bytes.NewReader(body)),
							Size:        int64(len(body)),
						}, nil
					}
				}
			}
			return nil, fmt.Errorf("package not found")
		}
		return nil, err
	}

	// 按 PEP 691 格式构建文件列表
	type file struct {
		URL         string            `json:"url"`
		Filename    string            `json:"filename"`
		Hashes      map[string]string `json:"hashes,omitempty"`
		Size        int64             `json:"size,omitempty"`
		UploadTime  string            `json:"upload-time,omitempty"`
		Packagetype string            `json:"packagetype,omitempty"`
	}

	files := make([]file, 0)
	for _, ver := range pkg.Versions {
		for _, f := range ver.Files {
			if f.Filename == "" {
				continue
			}
			// 只列出 primary 类型的文件（实际安装包），过滤掉 metadata/pom 等辅助文件
			if f.FileType != "" && f.FileType != model.FileTypePrimary {
				continue
			}
			hashes := make(map[string]string)
			if f.ChecksumSHA256 != "" {
				hashes["sha256"] = f.ChecksumSHA256
			}
			if f.ChecksumMD5 != "" {
				hashes["md5"] = f.ChecksumMD5
			}

			files = append(files, file{
				URL:        "../../packages/" + f.Filename,
				Filename:   f.Filename,
				Hashes:     hashes,
				Size:       f.SizeBytes,
				UploadTime: ver.PublishedAt.Format("2006-01-02T15:04:05"),
			})
		}
	}

	responseJSON, _ := json.Marshal(gin.H{
		"meta": gin.H{
			"api-version": "1.0",
		},
		"files": files,
	})
	hash := sha256.Sum256(responseJSON)
	etag := fmt.Sprintf(`"%x"`, hash)
	lastModified := time.Now().UTC().Format(time.RFC1123)

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/vnd.pypi.simple.v1+json",
		Content:     io.NopCloser(bytes.NewReader(responseJSON)),
		Size:        int64(len(responseJSON)),
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=86400",
		},
	}, nil
}

func (a *PyPIAdapter) packageFilesHTML(ctx context.Context, pkgName string, repo *model.Repository) (*types.ContentResult, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), normalizePackageName(pkgName), model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil && repo != nil {
				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						body = RewritePyPIHTML(body)
						return &types.ContentResult{
							StatusCode:  200,
							ContentType: "text/html",
							Content:     io.NopCloser(bytes.NewReader(body)),
							Size:        int64(len(body)),
						}, nil
					}
				}
			}
			return nil, fmt.Errorf("package not found")
		}
		return nil, err
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>Links for %s</title></head><body>
<h1>Links for %s</h1>
`, pkgName, pkgName)

	var sb strings.Builder
	sb.Grow(len(html) + len(pkg.Versions)*60)
	sb.WriteString(html)

	for _, ver := range pkg.Versions {
		for _, f := range ver.Files {
			filename := f.Filename
			if filename == "" {
				continue
			}
			// 只列出 primary 类型的文件（实际安装包），过滤掉 metadata/pom 等辅助文件
			if f.FileType != "" && f.FileType != model.FileTypePrimary {
				continue
			}
			sb.WriteString(`<a href="`)
			sb.WriteString("../../packages/")
			sb.WriteString(filename)
			sb.WriteString(`">`)
			sb.WriteString(filename)
			sb.WriteString(`</a><br>` + "\n")
		}
	}
	sb.WriteString("</body></html>")

	content := sb.String()

	// 计算 ETag
	hash := sha256.Sum256([]byte(content))
	etag := fmt.Sprintf(`"%x"`, hash)
	lastModified := time.Now().UTC().Format(time.RFC1123)

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     io.NopCloser(strings.NewReader(content)),
		Size:        int64(len(content)),
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=86400",
		},
	}, nil
}

func (a *PyPIAdapter) DownloadPackage(filename string, repo *model.Repository, ctx context.Context) (*types.ContentResult, error) {
	slog.Info("DownloadPackage called", "filename", filename)

	// 路径遍历防护
	if err := validatePyPIPath(filename); err != nil {
		return nil, fmt.Errorf("invalid filename: %v", err)
	}

	if strings.HasSuffix(filename, ".sha256") {
		return a.handleChecksumRequest(filename, repo, ctx)
	}

	actualFilename := filepath.Base(filename)
	name, version := parseWheelFilename(actualFilename)
	slog.Info("Parsed filename", "name", name, "version", version, "actualFilename", actualFilename)
	if name == "" {
		return nil, fmt.Errorf("invalid filename: unable to parse package name from filename")
	}

	content, _, err := a.storageSvc.GetPackageWithBackend(ctx, repo.Name, "pypi", name, actualFilename, repositoryStorageBackendID(repo))
	if err == nil {
		defer content.Close()

		// 读取内容用于计算 ETag
		body, readErr := io.ReadAll(content)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read content: %v", readErr)
		}

		// 生成 ETag（基于内容 SHA256）
		hash := sha256.Sum256(body)
		etag := fmt.Sprintf(`"%x"`, hash)
		lastModified := time.Now().UTC().Format(time.RFC1123)

		contentType := a.storageSvc.GetContentType(actualFilename)
		return &types.ContentResult{
			StatusCode:  200,
			ContentType: contentType,
			Content:     io.NopCloser(bytes.NewReader(body)),
			Size:        int64(len(body)),
			Headers: map[string]string{
				"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, actualFilename),
				"ETag":                etag,
				"Last-Modified":       lastModified,
				"Cache-Control":       "public, max-age=86400",
			},
		}, nil
	}

	if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		slog.Info("PyPI proxy: fetching from remote", "filename", actualFilename, "name", name)
		pathInfo, pathErr := a.ParsePath("packages/" + actualFilename)
		if pathErr != nil {
			fetchErr := pathErr
			slog.Warn("PyPI proxy: failed to resolve path", "filename", actualFilename, "error", fetchErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr == nil && result != nil {
				slog.Info("PyPI proxy: successfully fetched from remote", "filename", actualFilename, "size", result.Size)

				// 读取内容用于计算 ETag
				body, readErr := io.ReadAll(result.Content)
				result.Content.Close()
				if readErr == nil {
					hash := sha256.Sum256(body)
					etag := fmt.Sprintf(`"%x"`, hash)
					lastModified := time.Now().UTC().Format(time.RFC1123)

					contentType := a.storageSvc.GetContentType(actualFilename)
					return &types.ContentResult{
						StatusCode:  200,
						ContentType: contentType,
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
						Headers: map[string]string{
							"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, actualFilename),
							"ETag":                etag,
							"Last-Modified":       lastModified,
							"Cache-Control":       "public, max-age=86400",
						},
					}, nil
				}

				contentType := a.storageSvc.GetContentType(actualFilename)
				return &types.ContentResult{
					StatusCode:  200,
					ContentType: contentType,
					Content:     result.Content,
					Size:        result.Size,
					Headers:     map[string]string{"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, actualFilename)},
				}, nil
			}
			slog.Warn("PyPI proxy: failed to fetch from remote", "filename", actualFilename, "error", fetchErr)
		}
	}

	return nil, fmt.Errorf("package not found")
}

func (a *PyPIAdapter) handleChecksumRequest(filename string, repo *model.Repository, ctx context.Context) (*types.ContentResult, error) {
	// 路径遍历防护
	if err := validatePyPIPath(filename); err != nil {
		return nil, fmt.Errorf("invalid filename: %v", err)
	}

	// 移除 .sha256 后缀获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 从数据库查找文件记录
	files, err := a.GetPackageRepository().FindFilesByFilenameContext(ctx, actualFilename)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("checksum not found")
	}

	// 获取第一个匹配的文件
	file := files[0]
	var checksum string
	if file.ChecksumSHA256 == "" {
		// 如果数据库中没有校验和，尝试从存储中读取文件并计算
		name, _ := parseWheelFilename(actualFilename)
		if name == "" {
			return nil, fmt.Errorf("invalid filename")
		}

		content, _, err := a.storageSvc.GetPackageWithBackend(ctx, repo.Name, "pypi", name, actualFilename, repositoryStorageBackendID(repo))
		if err != nil {
			return nil, fmt.Errorf("file not found")
		}
		defer content.Close()

		// 计算SHA256
		body, err := io.ReadAll(content)
		if err != nil {
			return nil, fmt.Errorf("failed to read file")
		}

		hash := sha256.Sum256(body)
		checksum = hex.EncodeToString(hash[:])
	} else {
		checksum = file.ChecksumSHA256
	}

	// 生成 ETag
	hash := sha256.Sum256([]byte(checksum))
	etag := fmt.Sprintf(`"%x"`, hash)
	lastModified := time.Now().UTC().Format(time.RFC1123)

	// 返回校验和
	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/plain",
		Content:     io.NopCloser(bytes.NewReader([]byte(checksum))),
		Size:        int64(len(checksum)),
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": lastModified,
			"Cache-Control": "public, max-age=86400",
		},
	}, nil
}

func (a *PyPIAdapter) JSONAPI(pkgName string, version string, repo *model.Repository, ctx context.Context) (*types.ContentResult, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), normalizePackageName(pkgName), model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil && repo != nil {
				baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
				jsonPath := fmt.Sprintf("pypi/%s/json", pkgName)
				if version != "" {
					jsonPath = fmt.Sprintf("pypi/%s/%s/json", pkgName, version)
				}
				remoteURL := fmt.Sprintf("%s/%s", baseURL, jsonPath)
				result, resolveErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						if !looksLikeJSON(body) {
							fallback := gin.H{
								"info": gin.H{
									"name":    pkgName,
									"version": version,
									"summary": "",
								},
								"releases": gin.H{},
							}
							body, _ = json.Marshal(fallback)
						}
						// Rewrite absolute CDN URLs in releases to relative paths
						body = RewritePyPIJSON(body)
						return &types.ContentResult{
							StatusCode:  200,
							ContentType: "application/json",
							Content:     io.NopCloser(bytes.NewReader(body)),
							Size:        int64(len(body)),
						}, nil
					}
				}
				// 兜底：某些镜像不支持版本级 JSON，回退到包级 JSON。
				if version != "" {
					remoteURL = fmt.Sprintf("%s/pypi/%s/json", baseURL, pkgName)
					result, resolveErr = a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
					if resolveErr == nil && result != nil {
						defer result.Content.Close()
						body, readErr := io.ReadAll(result.Content)
						if readErr == nil {
							if !looksLikeJSON(body) {
								fallback := gin.H{
									"info": gin.H{
										"name":    pkgName,
										"version": version,
										"summary": "",
									},
									"releases": gin.H{},
								}
								body, _ = json.Marshal(fallback)
							}
							// Rewrite absolute CDN URLs in releases to relative paths
							body = RewritePyPIJSON(body)
							return &types.ContentResult{
								StatusCode:  200,
								ContentType: "application/json",
								Content:     io.NopCloser(bytes.NewReader(body)),
								Size:        int64(len(body)),
							}, nil
						}
					}
				}
			}
			return nil, fmt.Errorf("package not found")
		}
		return nil, err
	}

	// 构建完整的 releases 字段（符合 PEP 658 规范）
	// releases 是一个字典，键是版本号，值是该版本的所有文件列表
	type urlInfo struct {
		URL           string `json:"url"`
		Filename      string `json:"filename"`
		MD5           string `json:"md5_digest,omitempty"`
		SHA256        string `json:"sha256_digest,omitempty"`
		Size          int64  `json:"size"`
		Packagetype   string `json:"packagetype,omitempty"`
		PythonVersion string `json:"python_version,omitempty"`
	}

	// 按版本号分组文件
	releases := make(map[string][]urlInfo)
	var latestVersion string
	var latestInfo *model.PackageVersion

	for _, ver := range pkg.Versions {
		if version != "" && ver.Version != version {
			continue
		}

		files := make([]urlInfo, 0, len(ver.Files))
		for _, f := range ver.Files {
			if f.Filename == "" {
				continue
			}
			files = append(files, urlInfo{
				URL:      "../../packages/" + f.Filename,
				Filename: f.Filename,
				MD5:      f.ChecksumMD5,
				SHA256:   f.ChecksumSHA256,
				Size:     f.SizeBytes,
			})
		}

		// 如果版本没有文件，至少添加一个条目指向包版本
		if len(files) == 0 {
			files = append(files, urlInfo{
				URL:      "../../packages/" + ver.Version,
				Filename: ver.Version,
				Size:     ver.SizeBytes,
				MD5:      ver.ChecksumMD5,
				SHA256:   ver.ChecksumSHA256,
			})
		}

		releases[ver.Version] = files

		// 追踪最新版本
		if latestInfo == nil || pypiCompareVersions(ver.Version, latestInfo.Version) > 0 {
			latestInfo = &model.PackageVersion{}
			*latestInfo = ver
			latestVersion = ver.Version
		}
	}

	if version != "" && latestInfo == nil {
		return nil, fmt.Errorf("version not found")
	}

	// 构建 info 字段
	infoVersion := version
	if infoVersion == "" {
		infoVersion = latestVersion
	}

	return &types.ContentResult{
		StatusCode: 200,
		ExtraData: gin.H{
			"info": gin.H{
				"name":                 pkg.Name,
				"version":              infoVersion,
				"summary":              pkg.Description,
				"description":          pkg.Description,
				"classifiers":          []string{},
				"author":               "",
				"author_email":         "",
				"maintainer":           "",
				"maintainer_email":     "",
				"home_page":            "",
				"license":              "",
				"project_urls":         gin.H{},
				"requires_python":      nil,
				"requires_dist":        []string{},
				"provides_dist":        []string{},
				"obsoletes_dist":       []string{},
				"requires_external":    []string{},
				"upload_time":          latestInfo.PublishedAt.Format("2006-01-02T15:04:05"),
				"upload_time_iso_8601": latestInfo.PublishedAt.Format(time.RFC3339),
				"bugtrack_url":         nil,
				"docs_url":             nil,
			},
			"releases":      releases,
			"urls":          releases[infoVersion],
			"last_serial":   pkg.ID,
			"_cache_sha256": "",
		},
	}, nil
}

// pypiCompareVersions 比较两个语义版本号
// 返回正数 if v1 > v2, 负数 if v1 < v2, 0 if equal
func pypiCompareVersions(v1, v2 string) int {
	// 清理版本号（移除 v 前缀等）
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// 分割主版本号
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int64 = 0, 0

		if i < len(parts1) {
			// 分离数字部分和预发布标识符
			numStr1 := parts1[i]
			if idx := strings.IndexFunc(parts1[i], func(c rune) bool {
				return c < '0' || c > '9'
			}); idx > 0 {
				numStr1 = parts1[i][:idx]
			}
			num1, _ = strconv.ParseInt(numStr1, 10, 64)
		}

		if i < len(parts2) {
			numStr2 := parts2[i]
			if idx := strings.IndexFunc(parts2[i], func(c rune) bool {
				return c < '0' || c > '9'
			}); idx > 0 {
				numStr2 = parts2[i][:idx]
			}
			num2, _ = strconv.ParseInt(numStr2, 10, 64)
		}

		if num1 != num2 {
			if num1 > num2 {
				return 1
			}
			return -1
		}
	}

	return 0
}

func (a *PyPIAdapter) UploadPackage(c *gin.Context) {
	response.InternalError(c, "upload via UploadPackage is deprecated, use the standard publish endpoint")
}

func (a *PyPIAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetRepositoryPackageMetadata(ctx, repositoryFromContext(ctx), name, model.PackageTypePyPI, PyPIType)
}

func (a *PyPIAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypePyPI)
}

func (a *PyPIAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypePyPI)
}

// ParseIntent 解析请求路径为意图
func (a *PyPIAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = trimLeadingSlash(path)
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	if strings.HasPrefix(path, "pypi/simple/") {
		pkgPath := strings.TrimPrefix(path, "pypi/simple/")
		if pkgPath == "" || pkgPath == "/" {
			intent.Type = types.RequestList
			intent.Name = ""
			intent.Filename = ""
		} else {
			pkgName := strings.Trim(pkgPath, "/")
			intent.Type = types.RequestMetadata
			intent.Name = pkgName
			intent.Filename = ""
		}
		return intent
	}

	if strings.HasPrefix(path, "simple/") {
		pkgPath := strings.TrimPrefix(path, "simple/")
		if pkgPath == "" || pkgPath == "/" {
			intent.Type = types.RequestList
			intent.Name = ""
			intent.Filename = ""
		} else {
			pkgName := strings.Trim(pkgPath, "/")
			intent.Type = types.RequestMetadata
			intent.Name = pkgName
			intent.Filename = ""
		}
		return intent
	}

	if strings.HasPrefix(path, "packages/") {
		filename := strings.TrimPrefix(path, "packages/")
		intent.Type = types.RequestDownload
		intent.Filename = filename
		if strings.HasSuffix(filename, ".sha256") {
			intent.Type = types.RequestChecksum
		}
		name, version := parseWheelFilename(filename)
		intent.Name = normalizePackageName(name)
		intent.Version = version
		if pathInfo, err := a.ParsePath(path); err == nil {
			intent.PkgPathInfo = pathInfo
		}
		return intent
	}

	if strings.Contains(intent.Path, "/json") {
		rest := strings.TrimPrefix(intent.Path, "pypi/")
		rest = strings.TrimSuffix(rest, "/json")
		parts := strings.Split(rest, "/")
		if len(parts) >= 1 {
			intent.Type = types.RequestMetadata
			intent.Name = parts[0]
			// pypi/foo/json → Name=foo, no version
			// pypi/foo/1.0.0/json → Name=foo, Version=1.0.0
			if len(parts) >= 2 {
				intent.Version = parts[1]
			}
			return intent
		}
	}

	// Fallback
	pathInfo, _ := a.ParsePath(path)
	intent.Type = types.RequestDownload
	if pathInfo != nil {
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
		intent.PkgPathInfo = pathInfo
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *PyPIAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	accept := ""
	if v, ok := intent.Extra["accept"]; ok {
		accept = v.(string)
	}

	if intent.Type == types.RequestList {
		return a.ListPackages(ctx, repo, accept)
	}

	if strings.HasPrefix(intent.Path, "pypi/simple/") {
		pkgName := strings.Trim(strings.TrimPrefix(intent.Path, "pypi/simple/"), "/")
		return a.PackageFiles(ctx, accept, pkgName, repo)
	}

	if strings.HasPrefix(intent.Path, "simple/") {
		pkgName := strings.Trim(strings.TrimPrefix(intent.Path, "simple/"), "/")
		return a.PackageFiles(ctx, accept, pkgName, repo)
	}

	if strings.HasPrefix(intent.Path, "packages/") {
		filename := strings.TrimPrefix(intent.Path, "packages/")
		return a.DownloadPackage(filename, repo, ctx)
	}

	if strings.Contains(intent.Path, "/json") {
		rest := strings.TrimPrefix(intent.Path, "pypi/")
		rest = strings.TrimSuffix(rest, "/json")
		parts := strings.Split(rest, "/")
		if len(parts) >= 1 {
			version := ""
			// pypi/foo/json → Name=foo, no version
			// pypi/foo/1.0.0/json → Name=foo, Version=1.0.0
			if len(parts) >= 2 {
				version = parts[1]
			}
			return a.JSONAPI(parts[0], version, repo, ctx)
		}
	}

	// 未知路径，尝试从远程获取
	if a.fetcher != nil {
		pathInfo, _ := a.ParsePath(intent.Path)
		if pathInfo != nil {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, resolveErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if resolveErr == nil && result != nil {
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					contentType := a.storageSvc.GetContentType(intent.Path)
					return &types.ContentResult{
						StatusCode:  200,
						ContentType: contentType,
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("path not found")
}

func parseWheelFilename(filename string) (name, version string) {
	basename := filepath.Base(filename)
	basename = strings.Split(basename, "#")[0]

	if strings.HasSuffix(basename, ".whl") {
		return parsePyPIWheel(basename)
	}

	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(basename, ext) {
			base := strings.TrimSuffix(basename, ext)
			return splitNameVersion(base)
		}
	}

	return "", ""
}

func parsePyPIWheel(filename string) (name, version string) {
	base := strings.TrimSuffix(filename, ".whl")
	parts := strings.Split(base, "-")
	if len(parts) < 4 {
		return "", ""
	}

	tagStart := len(parts) - 3

	if tagStart > 1 {
		buildTag := parts[tagStart-1]
		isBuildTag := true
		for _, c := range buildTag {
			if c < '0' || c > '9' {
				isBuildTag = false
				break
			}
		}
		if isBuildTag {
			tagStart--
		}
	}

	nameVersion := strings.Join(parts[:tagStart], "-")
	return splitNameVersion(nameVersion)
}

func splitNameVersion(s string) (name, version string) {
	parts := strings.Split(s, "-")
	for i, part := range parts {
		if len(part) > 0 && part[0] >= '0' && part[0] <= '9' {
			return strings.Join(parts[:i], "-"), strings.Join(parts[i:], "-")
		}
	}
	return s, ""
}

func normalizePackageName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func (a *PyPIAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	file, header, err := c.Request.FormFile("content")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}

	name, version := parseWheelFilename(header.Filename)
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ".whl")
		name = strings.TrimSuffix(name, ".tar.gz")
	}
	if name == "" {
		return nil, fmt.Errorf("failed to parse package name from filename: %s", header.Filename)
	}

	name = normalizePackageName(name)

	requiresDist := c.PostFormArray("requires_dist")
	if len(requiresDist) == 0 {
		requiresDist = c.PostFormArray("requires")
	}
	dependencies := buildPyPIDependencies(requiresDist)

	return &types.PublishResult{
		PackageName:  name,
		Version:      version,
		Filename:     header.Filename,
		Content:      file,
		Size:         header.Size,
		FileType:     model.FileTypePrimary,
		Dependencies: dependencies,
		Response: &types.PypiPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  name,
				Version:  version,
				Filename: header.Filename,
				Size:     header.Size,
			},
		},
	}, nil
}

func (a *PyPIAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := trimLeadingSlash(c.Param("path"))
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid path: expected name/version")
	}

	name := parts[0]
	version := parts[1]

	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    PyPIType,
	}

	if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), identity); err != nil {
		return err
	}

	return nil
}

// MergeMetadata implements the MetadataMerger interface for virtual repository support.
// It merges PyPI metadata from multiple member repositories:
// - RequestList: merges package name lists from HTML/JSON simple index
// - RequestMetadata: merges file lists per package
func (a *PyPIAdapter) MergeMetadata(ctx context.Context, results []*types.ContentResult, intent *types.RequestIntent) (*types.ContentResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to merge")
	}

	switch intent.Type {
	case types.RequestList:
		return a.mergeSimpleIndexList(ctx, results, intent)
	case types.RequestMetadata:
		return a.mergePackageFilesList(ctx, results, intent)
	default:
		return results[0], nil
	}
}

// mergeSimpleIndexList merges simple index page lists from multiple members.
func (a *PyPIAdapter) mergeSimpleIndexList(ctx context.Context, results []*types.ContentResult, intent *types.RequestIntent) (*types.ContentResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to merge")
	}

	contentType := results[0].ContentType
	if strings.Contains(contentType, "json") {
		return a.mergeSimpleIndexJSON(ctx, results)
	}
	return a.mergeSimpleIndexHTML(ctx, results)
}

// mergeSimpleIndexHTML merges HTML simple index pages.
func (a *PyPIAdapter) mergeSimpleIndexHTML(ctx context.Context, results []*types.ContentResult) (*types.ContentResult, error) {
	seen := make(map[string]struct{})
	var links []string

	for _, res := range results {
		if res.Content == nil {
			continue
		}
		body, err := io.ReadAll(res.Content)
		res.Content.Close()
		if err != nil {
			continue
		}
		for _, match := range pypiSimpleLinkRe.FindAllSubmatch(body, -1) {
			if len(match) >= 2 {
				name := string(match[1])
				normalized := normalizePackageName(name)
				if _, exists := seen[normalized]; !exists {
					seen[normalized] = struct{}{}
					links = append(links, normalized)
				}
			}
		}
	}

	if len(links) == 0 {
		return results[0], nil
	}

	var sb strings.Builder
	sb.Grow(100 + len(links)*80)
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n")
	for _, name := range links {
		sb.WriteString(`<a href="`)
		sb.WriteString(name)
		sb.WriteString(`/">`)
		sb.WriteString(name)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	content := sb.String()
	hash := sha256.Sum256([]byte(content))
	etag := fmt.Sprintf(`"%x"`, hash)

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     io.NopCloser(strings.NewReader(content)),
		Size:        int64(len(content)),
		Headers: map[string]string{
			"ETag":          etag,
			"Cache-Control": "public, max-age=60",
		},
	}, nil
}

// mergeSimpleIndexJSON merges JSON simple index pages.
func (a *PyPIAdapter) mergeSimpleIndexJSON(ctx context.Context, results []*types.ContentResult) (*types.ContentResult, error) {
	seen := make(map[string]struct{})
	type project struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	var projects []project

	for _, res := range results {
		if res.Content == nil && res.ExtraData == nil {
			continue
		}
		if res.ExtraData != nil {
			if projList, ok := res.ExtraData["projects"]; ok {
				if arr, ok := projList.([]interface{}); ok {
					for _, item := range arr {
						if m, ok := item.(map[string]interface{}); ok {
							if name, ok := m["name"].(string); ok {
								normalized := normalizePackageName(name)
								if _, exists := seen[normalized]; !exists {
									seen[normalized] = struct{}{}
							projects = append(projects, project{
								Name: normalized,
								URL:  normalized + "/",
							})
								}
							}
						}
					}
				}
			}
			continue
		}
		if res.Content == nil {
			continue
		}
		body, err := io.ReadAll(res.Content)
		res.Content.Close()
		if err != nil {
			continue
		}
		var parsed struct {
			Projects []struct {
				Name string `json:"name"`
			} `json:"projects"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			for _, p := range parsed.Projects {
				normalized := normalizePackageName(p.Name)
				if _, exists := seen[normalized]; !exists {
					seen[normalized] = struct{}{}
					projects = append(projects, project{
						Name: normalized,
						URL:  normalized + "/",
					})
				}
			}
		}
	}

	if len(projects) == 0 {
		return results[0], nil
	}

	resultJSON, _ := json.Marshal(gin.H{"projects": projects})

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(bytes.NewReader(resultJSON)),
		Size:        int64(len(resultJSON)),
		Headers: map[string]string{
			"Cache-Control": "public, max-age=60",
		},
	}, nil
}

// mergePackageFilesList merges file lists for a specific package from multiple members.
func (a *PyPIAdapter) mergePackageFilesList(ctx context.Context, results []*types.ContentResult, intent *types.RequestIntent) (*types.ContentResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to merge")
	}

	contentType := results[0].ContentType
	if strings.Contains(contentType, "json") {
		return a.mergePackageFilesJSON(ctx, results)
	}
	return a.mergePackageFilesHTML(ctx, results)
}

// mergePackageFilesHTML merges HTML package file listings.
func (a *PyPIAdapter) mergePackageFilesHTML(ctx context.Context, results []*types.ContentResult) (*types.ContentResult, error) {
	seen := make(map[string]struct{})
	var links []string

	for _, res := range results {
		if res.Content == nil {
			continue
		}
		body, err := io.ReadAll(res.Content)
		res.Content.Close()
		if err != nil {
			continue
		}
		for _, match := range pypiFileLinkRe.FindAllSubmatch(body, -1) {
			if len(match) >= 2 {
				href := string(match[1])
				if idx := strings.LastIndex(href, "/"); idx != -1 {
					href = href[idx+1:]
				}
				if _, exists := seen[href]; !exists {
					seen[href] = struct{}{}
					links = append(links, href)
				}
			}
		}
	}

	if len(links) == 0 {
		return results[0], nil
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Links</title></head><body>\n")

	for _, filename := range links {
		sb.WriteString(`<a href="`)
		sb.WriteString("../../packages/")
		sb.WriteString(filename)
		sb.WriteString(`">`)
		sb.WriteString(filename)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	content := sb.String()
	hash := sha256.Sum256([]byte(content))
	etag := fmt.Sprintf(`"%x"`, hash)

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     io.NopCloser(strings.NewReader(content)),
		Size:        int64(len(content)),
		Headers: map[string]string{
			"ETag":          etag,
			"Last-Modified": time.Now().UTC().Format(http.TimeFormat),
			"Cache-Control": "public, max-age=60",
		},
	}, nil
}

// mergePackageFilesJSON merges PEP 691 JSON package file listings.
func (a *PyPIAdapter) mergePackageFilesJSON(ctx context.Context, results []*types.ContentResult) (*types.ContentResult, error) {
	seen := make(map[string]struct{})
	type fileEntry struct {
		URL         string            `json:"url"`
		Filename    string            `json:"filename"`
		Hashes      map[string]string `json:"hashes,omitempty"`
		Size        int64             `json:"size,omitempty"`
		UploadTime  string            `json:"upload-time,omitempty"`
		Packagetype string            `json:"packagetype,omitempty"`
	}
	var files []fileEntry

	for _, res := range results {
		if res.Content == nil {
			continue
		}
		body, err := io.ReadAll(res.Content)
		res.Content.Close()
		if err != nil {
			continue
		}
		var parsed struct {
			Files []fileEntry `json:"files"`
		}
		if json.Unmarshal(body, &parsed) != nil {
			var bareFiles []fileEntry
			if json.Unmarshal(body, &bareFiles) == nil {
				parsed.Files = bareFiles
			}
		}
		for _, f := range parsed.Files {
			if _, exists := seen[f.Filename]; !exists {
				seen[f.Filename] = struct{}{}
				// Ensure URL is a relative path, not an upstream CDN absolute URL
				if strings.HasPrefix(f.URL, "http://") || strings.HasPrefix(f.URL, "https://") {
					f.URL = "../../packages/" + f.Filename
				}
				files = append(files, f)
			}
		}
	}

	if len(files) == 0 {
		return results[0], nil
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"meta": map[string]string{
			"api-version": "1.0",
		},
		"files": files,
	})

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/vnd.pypi.simple.v1+json",
		Content:     io.NopCloser(bytes.NewReader(resultJSON)),
		Size:        int64(len(resultJSON)),
		Headers: map[string]string{
			"Cache-Control": "public, max-age=60",
		},
	}, nil
}

var pypiSimpleLinkRe = regexp.MustCompile(`<a\s+href="[^"]*simple/([^/]+)/">`)
var pypiFileLinkRe = regexp.MustCompile(`<a\s+href="([^"]+)">`)
