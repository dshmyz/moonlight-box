package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type PyPIAdapter struct {
	*BaseAdapter
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	auditSvc    *service.AuditService
	proxyRouter *proxy.ProxyRouter
}

func NewPyPIAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	proxyRouter *proxy.ProxyRouter,
) *PyPIAdapter {
	return &PyPIAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
		proxyRouter: proxyRouter,
	}
}

func (a *PyPIAdapter) Type() PackageType   { return PyPIType }
func (a *PyPIAdapter) RoutePrefix() string { return "/pypi" }

func (a *PyPIAdapter) SetProxyRouter(pr *proxy.ProxyRouter) {
	a.proxyRouter = pr
}

func (a *PyPIAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/simple/", a.ListPackages)
		r.GET("/simple/:package/", a.PackageFiles)
		r.GET("/packages/:filename", a.DownloadPackage)
		r.GET("/:package/:version/json", a.JSONAPI)
		r.POST("/upload", authMw, permMw("pypi", "write"), a.UploadPackage)
	}
}

func (a *PyPIAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid pypi path")
	}
	return &PackageIdentity{
		Name:    parts[0],
		Version: parts[1],
		Type:    PyPIType,
	}, nil
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
	packages, _, err := a.pkgRepo.List(1, 10000, "pypi", "")
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
	packages, _, err := a.pkgRepo.List(1, 10000, "pypi", "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	html := "<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n"
	for _, pkg := range packages {
		html += fmt.Sprintf(`<a href="/pypi/simple/%s/">%s</a><br>`+"\n", normalizePackageName(pkg.Name), normalizePackageName(pkg.Name))
	}
	html += "</body></html>"

	c.Data(200, "text/html", []byte(html))
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
	pkg, err := a.pkgRepo.FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.proxyRouter != nil {
				urlBuilder := func(repo *model.Repository, name, _ string) string {
					return fmt.Sprintf("%s/%s/", strings.TrimSuffix(repo.RemoteURL, "/"), normalizePackageName(name))
				}

				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "pypi", pkgName, "", urlBuilder)
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
	pkg, err := a.pkgRepo.FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.proxyRouter != nil {
				urlBuilder := func(repo *model.Repository, name, _ string) string {
					return fmt.Sprintf("%s/simple/%s/", strings.TrimSuffix(repo.RemoteURL, "/"), normalizePackageName(name))
				}

				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "pypi", pkgName, "", urlBuilder)
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

	for _, ver := range pkg.Versions {
		filename := filepath.Base(ver.StoragePath)
		html += fmt.Sprintf(`<a href="/pypi/packages/%s">%s</a><br>`+"\n", filename, filename)
	}
	html += "</body></html>"

	c.Data(200, "text/html", []byte(html))
}

func (a *PyPIAdapter) DownloadPackage(c *gin.Context) {
	filename := c.Param("filename")
	slog.Info("DownloadPackage called", "filename", filename)

	actualFilename := filepath.Base(filename)
	name, version := parseWheelFilename(actualFilename)
	slog.Info("Parsed filename", "name", name, "version", version, "actualFilename", actualFilename)
	if name == "" {
		response.BadRequest(c, "invalid filename", "unable to parse package name from filename")
		return
	}

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", name, actualFilename)
	if err == nil {
		defer content.Close()

		a.IncrementDownloadCountForPackage(name, model.PackageTypePyPI, version, actualFilename)

		contentType := a.storageSvc.GetContentType(actualFilename)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, actualFilename))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.proxyRouter == nil {
		slog.Warn("proxyRouter is nil")
		response.NotFound(c, "package not found")
		return
	}

	urlBuilder := func(repo *model.Repository, pkgName, _ string) string {
		url := fmt.Sprintf("%s/packages/%s", strings.TrimSuffix(repo.RemoteURL, "/"), filename)
		slog.Info("Built proxy URL", "url", url)
		return url
	}

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	slog.Info("Calling ResolveSmart", "repo", repo != nil, "name", name, "version", version)
	result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "pypi", name, version, urlBuilder)
	if resolveErr != nil {
		slog.Error("ResolveSmart failed", "error", resolveErr)
		response.NotFound(c, "package not found")
		return
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		response.NotFound(c, "package not found")
		return
	}

	storageKey, storeErr := a.storageSvc.StorePackage(c.Request.Context(), "pypi", name, actualFilename, bytes.NewReader(body), result.Size)
	if storeErr == nil {
		a.pkgRepo.StorePackageFileAndIncrementDownload(c.Request.Context(), &model.Package{
			Name:           name,
			Type:           model.PackageTypePyPI,
			RepositoryID:   result.RepoID,
			RepositoryType: model.RepoTypeProxy,
		}, &model.PackageVersion{
			Version:     version,
			Status:      model.StatusPublished,
			StoragePath: filepath.Dir(storageKey),
			SizeBytes:   result.Size,
		}, &model.PackageFile{
			Filename:    actualFilename,
			FileType:    model.FileTypePrimary,
			StoragePath: storageKey,
			SizeBytes:   result.Size,
		})
	}

	if storeErr != nil {
		response.InternalError(c, "failed to store package")
		return
	}

	contentType := a.storageSvc.GetContentType(actualFilename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, actualFilename))
	c.Data(200, contentType, body)
}

func (a *PyPIAdapter) handleChecksumRequest(c *gin.Context, filename string) {
	// 移除 .sha256 后缀获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 从数据库查找文件记录
	files, err := a.pkgRepo.FindFilesByFilename(actualFilename)
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

	pkg, err := a.pkgRepo.FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.proxyRouter != nil {
				urlBuilder := func(repo *model.Repository, name, ver string) string {
					return fmt.Sprintf("%s/%s/%s/json", strings.TrimSuffix(repo.RemoteURL, "/"), normalizePackageName(name), ver)
				}

				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "pypi", pkgName, version, urlBuilder)
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
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("content")
	if err != nil {
		response.BadRequest(c, "missing file", "no file uploaded")
		return
	}
	defer file.Close()

	pkgName := c.PostForm("name")
	version := c.PostForm("version")
	if pkgName == "" || version == "" {
		response.BadRequest(c, "missing metadata", "name and version are required")
		return
	}

	req := &UploadRequest{
		Package:  file,
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"name":        pkgName,
			"version":     version,
			"description": c.PostForm("summary"),
			"filename":    header.Filename,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{"success": true, "result": result})
}

func (a *PyPIAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["name"].(string)
	version, _ := req.Metadata["version"].(string)
	filename, _ := req.Metadata["filename"].(string)
	if filename == "" {
		filename = req.Filename
	}
	if name == "" || version == "" {
		return nil, fmt.Errorf("missing name or version")
	}

	// 读取文件内容
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 计算SHA256
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	// 存储文件
	storageKey, err := a.storageSvc.StorePackage(ctx, "pypi", name, filename, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, err
	}

	pkg, ver, _, err := a.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypePyPI,
		Description:    getDescription(req.Metadata),
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
		PublishedBy: req.UploadedBy,
		Metadata:    marshalMetadata(req.Metadata),
	}, &model.PackageFile{
		Filename:       filename,
		FileType:       model.FileTypePrimary,
		StoragePath:    storageKey,
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "pypi", name, filename)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		VersionID:  ver.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       int64(len(content)),
	}, nil
}

func (a *PyPIAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "pypi", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/octet-stream",
		Size:        size,
	}, nil
}

func (a *PyPIAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypePyPI)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        PyPIType,
		Description: pkg.Description,
	}

	for _, ver := range pkg.Versions {
		meta.Versions = append(meta.Versions, VersionInfo{
			Version:       ver.Version,
			PublishedAt:   ver.PublishedAt.Format(time.RFC3339),
			Size:          ver.SizeBytes,
			DownloadCount: int64(ver.DownloadCount),
		})
	}

	return meta, nil
}

func (a *PyPIAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *PyPIAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypePyPI)
}

func (a *PyPIAdapter) HandleRepoRequest(c *gin.Context, repo *model.Repository, path string) {
	c.Set("repo", repo)
	if strings.HasPrefix(path, "simple/") {
		pkgPath := strings.TrimPrefix(path, "simple/")
		if pkgPath == "" || pkgPath == "/" {
			a.ListPackages(c)
		} else {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: strings.Trim(pkgPath, "/")})
			a.PackageFiles(c)
		}
	} else if strings.HasPrefix(path, "packages/") {
		filename := strings.TrimPrefix(path, "packages/")
		c.Params = append(c.Params, gin.Param{Key: "filename", Value: filename})
		a.DownloadPackage(c)
	} else if strings.Contains(path, "/json") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: parts[0]})
			c.Params = append(c.Params, gin.Param{Key: "version", Value: parts[1]})
			a.JSONAPI(c)
		}
	} else {
		if a.proxyRouter != nil {
			urlBuilder := func(r *model.Repository, pkgName, pkgVersion string) string {
				baseURL := strings.TrimSuffix(r.RemoteURL, "/")
				return fmt.Sprintf("%s/%s", baseURL, path)
			}

			result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "pypi", path, "", urlBuilder)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					contentType := a.storageSvc.GetContentType(path)
					c.Data(200, contentType, body)
					return
				}
			}
		}

		response.NotFound(c, "path not found")
	}
}

func (a *PyPIAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("content")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return
	}
	defer file.Close()

	pkgData, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file", err.Error())
		return
	}

	name, version := parseWheelFilename(header.Filename)
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ".whl")
		name = strings.TrimSuffix(name, ".tar.gz")
	}

	req := &UploadRequest{
		Package:  bytes.NewReader(pkgData),
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"name":      name,
			"version":   version,
			"repo_name": repo.Name,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"result":  result,
	})
}

func (a *PyPIAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 {
		response.BadRequest(c, "invalid path", "expected name/version")
		return
	}

	name := parts[0]
	version := parts[1]

	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    PyPIType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{"ok": true})
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
