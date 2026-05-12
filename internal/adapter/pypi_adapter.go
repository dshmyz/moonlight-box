package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type PyPIAdapter struct {
	*BaseAdapter
	repoRepo *repository.RepositoryRepository
}

func NewPyPIAdapter(
	repoRepo *repository.RepositoryRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *PyPIAdapter {
	return &PyPIAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, auditSvc, pkgCache),
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
			Name:        name,
			Version:     "",
			Filename:    filename,
			StorageName: name,
			RemotePath:  path,
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

func (a *PyPIAdapter) ListPackages(c *gin.Context) {
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/vnd.pypi.simple") || strings.Contains(accept, "application/json") {
		a.listPackagesJSON(c)
	} else {
		a.listPackagesHTML(c)
	}
}

func (a *PyPIAdapter) listPackagesJSON(c *gin.Context) {
	packages, _, err := a.GetPackageRepository().List(1, 10000, "pypi", "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
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

	c.JSON(200, gin.H{"projects": result})
}

func (a *PyPIAdapter) listPackagesHTML(c *gin.Context) {
	packages, _, err := a.GetPackageRepository().List(1, 10000, "pypi", "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
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

	c.Data(200, "text/html", []byte(sb.String()))
}

func (a *PyPIAdapter) PackageFiles(c *gin.Context) {
	pkgName := c.Param("package")

	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/json") {
		a.packageFilesJSON(c, pkgName)
	} else {
		a.packageFilesHTML(c, pkgName)
	}
}

func (a *PyPIAdapter) packageFilesJSON(c *gin.Context, pkgName string) {
	pkg, err := a.GetPackageRepository().FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "application/vnd.pypi.simple.v1+json", body)
						return
					}
				}
			}
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	type file struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}

	files := make([]file, 0)
	for _, ver := range pkg.Versions {
		files = append(files, file{
			URL:      fmt.Sprintf("/pypi/packages/%s", filepath.Base(ver.StoragePath)),
			Filename: filepath.Base(ver.StoragePath),
		})
	}

	c.JSON(200, gin.H{
		"files": files,
		"meta": gin.H{
			"api-version": "1.0",
		},
	})
}

func (a *PyPIAdapter) packageFilesHTML(c *gin.Context, pkgName string) {
	pkg, err := a.GetPackageRepository().FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "text/html", body)
						return
					}
				}
			}
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>Links for %s</title></head><body>
<h1>Links for %s</h1>
`, pkgName, pkgName)

	var sb strings.Builder
	sb.Grow(len(html) + len(pkg.Versions)*60)
	sb.WriteString(html)

	for _, ver := range pkg.Versions {
		filename := filepath.Base(ver.StoragePath)
		sb.WriteString(`<a href="/pypi/packages/`)
		sb.WriteString(filename)
		sb.WriteString(`">`)
		sb.WriteString(filename)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	c.Data(200, "text/html", []byte(sb.String()))
}

func (a *PyPIAdapter) DownloadPackage(c *gin.Context) {
	filename := c.Param("filename")
	slog.Info("DownloadPackage called", "filename", filename)

	if strings.HasSuffix(filename, ".sha256") {
		a.handleChecksumRequest(c, filename)
		return
	}

	actualFilename := filepath.Base(filename)
	name, version := parseWheelFilename(actualFilename)
	slog.Info("Parsed filename", "name", name, "version", version, "actualFilename", actualFilename)
	if name == "" {
		response.BadRequest(c, "invalid filename", "unable to parse package name from filename")
		return
	}

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypePyPI, name, version, actualFilename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", name, actualFilename)
	if err == nil {
		defer content.Close()

		contentType := a.storageSvc.GetContentType(actualFilename)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, actualFilename))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.fetcher != nil && repo != nil && repo.Type == "proxy" {
		slog.Info("PyPI proxy: fetching from remote", "filename", actualFilename, "name", name)
		pathInfo, pathErr := a.ParsePath("packages/" + actualFilename)
		if pathErr != nil {
			fetchErr := pathErr
			slog.Warn("PyPI proxy: failed to resolve path", "filename", actualFilename, "error", fetchErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				slog.Info("PyPI proxy: successfully fetched from remote", "filename", actualFilename, "size", result.Size)
				contentType := a.storageSvc.GetContentType(actualFilename)
				c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, actualFilename))
				c.DataFromReader(200, result.Size, contentType, result.Content, nil)
				return
			}
			slog.Warn("PyPI proxy: failed to fetch from remote", "filename", actualFilename, "error", fetchErr)
		}
	}

	response.NotFound(c, "package not found")
}

func (a *PyPIAdapter) handleChecksumRequest(c *gin.Context, filename string) {
	// 移除 .sha256 后缀获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 从数据库查找文件记录
	files, err := a.GetPackageRepository().FindFilesByFilename(actualFilename)
	if err != nil || len(files) == 0 {
		response.NotFound(c, "checksum not found")
		return
	}

	// 获取第一个匹配的文件
	file := files[0]
	if file.ChecksumSHA256 == "" {
		// 如果数据库中没有校验和，尝试从存储中读取文件并计算
		name, _ := parseWheelFilename(actualFilename)
		if name == "" {
			response.NotFound(c, "invalid filename")
			return
		}

		content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", name, actualFilename)
		if err != nil {
			response.NotFound(c, "file not found")
			return
		}
		defer content.Close()

		// 计算SHA256
		body, err := io.ReadAll(content)
		if err != nil {
			response.InternalError(c, "failed to read file")
			return
		}

		hash := sha256.Sum256(body)
		checksum := hex.EncodeToString(hash[:])

		// 返回校验和
		c.Data(200, "text/plain", []byte(checksum))
		return
	}

	// 返回数据库中的校验和
	c.Data(200, "text/plain", []byte(file.ChecksumSHA256))
}

func (a *PyPIAdapter) JSONAPI(c *gin.Context) {
	pkgName := c.Param("package")
	version := c.Param("version")

	pkg, err := a.GetPackageRepository().FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				pathInfo, _ := a.ParsePath("simple/" + pkgName + "/")
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "application/json", body)
						return
					}
				}
			}
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
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
		response.NotFound(c, "version not found")
		return
	}

	type urlInfo struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		MD5      string `json:"md5_digest,omitempty"`
		SHA256   string `json:"sha256_digest,omitempty"`
		Size     int64  `json:"size"`
	}

	c.JSON(200, gin.H{
		"info": gin.H{
			"name":    pkg.Name,
			"version": version,
			"summary": pkg.Description,
		},
		"releases": gin.H{
			version: []urlInfo{{
				URL:    fmt.Sprintf("/pypi/packages/%s", filepath.Base(versionInfo.StoragePath)),
				Size:   versionInfo.SizeBytes,
				MD5:    versionInfo.ChecksumMD5,
				SHA256: versionInfo.ChecksumSHA256,
			}},
		},
	})
}

func (a *PyPIAdapter) UploadPackage(c *gin.Context) {
	response.InternalError(c, "upload via UploadPackage is deprecated, use the standard publish endpoint")
}

func (a *PyPIAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypePyPI, PyPIType)
}

func (a *PyPIAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypePyPI)
}

func (a *PyPIAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersions(name, model.PackageTypePyPI)
}

func (a *PyPIAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	if strings.HasPrefix(ctx.Path, "simple/") {
		pkgPath := strings.TrimPrefix(ctx.Path, "simple/")
		if pkgPath == "" || pkgPath == "/" {
			a.ListPackages(c)
		} else {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: strings.Trim(pkgPath, "/")})
			a.PackageFiles(c)
		}
	} else if strings.HasPrefix(ctx.Path, "packages/") {
		filename := strings.TrimPrefix(ctx.Path, "packages/")
		c.Params = append(c.Params, gin.Param{Key: "filename", Value: filename})
		a.DownloadPackage(c)
	} else if strings.Contains(ctx.Path, "/json") {
		parts := strings.Split(ctx.Path, "/")
		if len(parts) >= 2 {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: parts[0]})
			c.Params = append(c.Params, gin.Param{Key: "version", Value: parts[1]})
			a.JSONAPI(c)
		}
	} else {
		if a.fetcher != nil {
			pathInfo, _ := a.ParsePath(ctx.Path)
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(ctx.Repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), ctx.Repo, remoteURL)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					contentType := a.storageSvc.GetContentType(ctx.Path)
					c.Data(200, contentType, body)
					return
				}
			}
		}

		response.NotFound(c, "path not found")
	}
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

func (a *PyPIAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := a.storageSvc.GetContentType(result.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *PyPIAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
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
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
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

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	pkg, _ := a.GetPackageRepository().FindByNameAndType(name, model.PackageTypePyPI)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, ctx.Repo.Name, name, version, pkgID)

	return nil
}
