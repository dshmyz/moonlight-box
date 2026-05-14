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
	"path/filepath"
	"regexp"
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
	if path == "" {
		return nil, fmt.Errorf("invalid pypi path: empty path")
	}

	parts := strings.Split(path, "/")

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
		name := strings.TrimSuffix(filename, ".whl")
		name = strings.TrimSuffix(name, ".tar.gz")
		name = regexp.MustCompile(`-\d+.*$`).ReplaceAllString(name, "")

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

func (a *PyPIAdapter) ListPackages(ctx context.Context, acceptHeader string) (*types.ContentResult, error) {
	if strings.Contains(acceptHeader, "application/vnd.pypi.simple") || strings.Contains(acceptHeader, "application/json") {
		return a.listPackagesJSON(ctx)
	}
	return a.listPackagesHTML(ctx)
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
		result[i] = project{
			Name: normalizePackageName(pkg.Name),
			URL:  fmt.Sprintf("/pypi/simple/%s/", normalizePackageName(pkg.Name)),
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
		sb.WriteString(`<a href="/pypi/simple/`)
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

		content, size, err := a.metaCache.GetOrFetch(context.Background(), repo.Name, "pypi", cacheKey, ttl, func() (io.ReadCloser, int64, error) {
			pathInfo, pathErr := a.ParsePath("simple/" + pkgName + "/")
			if pathErr != nil {
				return nil, 0, pathErr
			}
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
			if fetchErr != nil {
				return nil, 0, fetchErr
			}
			return result.Content, result.Size, nil
		})
		if err == nil {
			return &types.ContentResult{
				StatusCode:  200,
				ContentType: contentType,
				Content:     content,
				Size:        size,
			}, nil
		}
	}

	if strings.Contains(acceptHeader, "application/json") {
		return a.packageFilesJSON(ctx, pkgName, repo)
	}
	return a.packageFilesHTML(ctx, pkgName, repo)
}

func (a *PyPIAdapter) packageFilesJSON(ctx context.Context, pkgName string, repo *model.Repository) (*types.ContentResult, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil && repo != nil {
				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
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

	type file struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}

	files := make([]file, 0)
	for _, ver := range pkg.Versions {
		files = append(files, file{
			URL:      fmt.Sprintf("/pypi/packages/%s", filepath.Base(ver.Version)),
			Filename: filepath.Base(ver.Version),
		})
	}

	return &types.ContentResult{
		StatusCode: 200,
		ExtraData: gin.H{
			"files": files,
			"meta": gin.H{
				"api-version": "1.0",
			},
		},
	}, nil
}

func (a *PyPIAdapter) packageFilesHTML(ctx context.Context, pkgName string, repo *model.Repository) (*types.ContentResult, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil && repo != nil {
				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
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
			sb.WriteString(`<a href="/pypi/packages/`)
			sb.WriteString(filename)
			sb.WriteString(`">`)
			sb.WriteString(filename)
			sb.WriteString(`</a><br>` + "\n")
		}
	}
	sb.WriteString("</body></html>")

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     io.NopCloser(strings.NewReader(sb.String())),
		Size:        int64(sb.Len()),
	}, nil
}

func (a *PyPIAdapter) DownloadPackage(filename string, repo *model.Repository, ctx context.Context) (*types.ContentResult, error) {
	slog.Info("DownloadPackage called", "filename", filename)

	if strings.HasSuffix(filename, ".sha256") {
		return a.handleChecksumRequest(filename, repo, ctx)
	}

	actualFilename := filepath.Base(filename)
	name, version := parseWheelFilename(actualFilename)
	slog.Info("Parsed filename", "name", name, "version", version, "actualFilename", actualFilename)
	if name == "" {
		return nil, fmt.Errorf("invalid filename: unable to parse package name from filename")
	}

	content, size, err := a.storageSvc.GetPackageWithBackend(ctx, repo.Name, "pypi", name, actualFilename, repositoryStorageBackendID(repo))
	if err == nil {
		contentType := a.storageSvc.GetContentType(actualFilename)
		return &types.ContentResult{
			StatusCode:  200,
			ContentType: contentType,
			Content:     content,
			Size:        size,
			Headers:     map[string]string{"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, actualFilename)},
		}, nil
	}

	if a.fetcher != nil && repo != nil && repo.Type == "proxy" {
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
	// 移除 .sha256 后缀获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 从数据库查找文件记录
	files, err := a.GetPackageRepository().FindFilesByFilenameContext(ctx, actualFilename)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("checksum not found")
	}

	// 获取第一个匹配的文件
	file := files[0]
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
		checksum := hex.EncodeToString(hash[:])

		// 返回校验和
		return &types.ContentResult{
			StatusCode:  200,
			ContentType: "text/plain",
			Content:     io.NopCloser(bytes.NewReader([]byte(checksum))),
			Size:        int64(len(checksum)),
		}, nil
	}

	// 返回数据库中的校验和
	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/plain",
		Content:     io.NopCloser(bytes.NewReader([]byte(file.ChecksumSHA256))),
		Size:        int64(len(file.ChecksumSHA256)),
	}, nil
}

func (a *PyPIAdapter) JSONAPI(pkgName string, version string, repo *model.Repository, ctx context.Context) (*types.ContentResult, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), pkgName, model.PackageTypePyPI)
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

	var versionInfo *model.PackageVersion
	for _, ver := range pkg.Versions {
		if ver.Version == version {
			v := ver
			versionInfo = &v
			break
		}
	}

	if versionInfo == nil {
		return nil, fmt.Errorf("version not found")
	}

	type urlInfo struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		MD5      string `json:"md5_digest,omitempty"`
		SHA256   string `json:"sha256_digest,omitempty"`
		Size     int64  `json:"size"`
	}

	return &types.ContentResult{
		StatusCode: 200,
		ExtraData: gin.H{
			"info": gin.H{
				"name":    pkg.Name,
				"version": version,
				"summary": pkg.Description,
			},
			"releases": gin.H{
				version: []urlInfo{{
					URL:    fmt.Sprintf("/pypi/packages/%s", versionInfo.Version),
					Size:   versionInfo.SizeBytes,
					MD5:    versionInfo.ChecksumMD5,
					SHA256: versionInfo.ChecksumSHA256,
				}},
			},
		},
	}, nil
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
		intent.Name = name
		intent.Version = version
		if pathInfo, err := a.ParsePath(path); err == nil {
			intent.PkgPathInfo = pathInfo
		}
		return intent
	}

	if strings.Contains(intent.Path, "/json") {
		rest := strings.TrimPrefix(intent.Path, "pypi/")
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			intent.Type = types.RequestMetadata
			intent.Name = parts[0]
			if len(parts) >= 3 {
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
		return a.ListPackages(ctx, accept)
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
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			version := ""
			if len(parts) >= 3 {
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

var wheelRegex = regexp.MustCompile(`^([A-Za-z0-9_]+)-([^-]+)-.+\.whl$`)
var sdistRegex = regexp.MustCompile(`^([A-Za-z0-9_]+)-([^-]+)\.(tar\.gz|tar\.bz2|zip)$`)

func parseWheelFilename(filename string) (name, version string) {
	basename := filepath.Base(filename)
	basename = strings.Split(basename, "#")[0]

	if matches := wheelRegex.FindStringSubmatch(basename); len(matches) >= 3 {
		return matches[1], matches[2]
	}

	if matches := sdistRegex.FindStringSubmatch(basename); len(matches) >= 3 {
		return matches[1], matches[2]
	}

	parts := strings.SplitN(basename, "-", 2)
	if len(parts) == 2 {
		version = strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
		return parts[0], version
	}
	return "", ""
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

	return &types.PublishResult{
		PackageName: name,
		Version:     version,
		Filename:    header.Filename,
		Content:     file,
		Size:        header.Size,
		FileType:    model.FileTypePrimary,
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
