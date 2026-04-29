package adapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
		proxyRouter: proxyRouter,
	}
}

func (a *PyPIAdapter) Type() PackageType { return PyPIType }
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
				result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "pypi", pkgName, "", urlBuilder)
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
					return fmt.Sprintf("%s/%s/", strings.TrimSuffix(repo.RemoteURL, "/"), normalizePackageName(name))
				}
				result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "pypi", pkgName, "", urlBuilder)
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

	name, version := parseWheelFilename(filename)
	if name == "" {
		response.BadRequest(c, "invalid filename", "unable to parse package name from filename")
		return
	}

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", name, version)
	if err == nil {
		defer content.Close()
		contentType := a.storageSvc.GetContentType(filename)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.proxyRouter == nil {
		response.NotFound(c, "package not found")
		return
	}

	urlBuilder := func(repo *model.Repository, pkgName, _ string) string {
		return fmt.Sprintf("%s/packages/%s", strings.TrimSuffix(repo.RemoteURL, "/"), filename)
	}

	result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "pypi", name, version, urlBuilder)
	if resolveErr != nil {
		response.NotFound(c, "package not found")
		return
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		response.NotFound(c, "package not found")
		return
	}

	a.storageSvc.StorePackage(c.Request.Context(), "pypi", name, version, bytes.NewReader(body), result.Size)

	contentType := a.storageSvc.GetContentType(filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(200, contentType, body)
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
				result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "pypi", pkgName, version, urlBuilder)
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
	if name == "" || version == "" {
		return nil, fmt.Errorf("missing name or version")
	}

	storageKey, err := a.storageSvc.StorePackage(ctx, "pypi", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypePyPI,
		Description:    getDescription(req.Metadata),
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   req.Size,
		PublishedBy: req.UploadedBy,
		Metadata:    marshalMetadata(req.Metadata),
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "pypi", name, version)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
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

var wheelRegex = regexp.MustCompile(`^(.+?)-(.+?)-.*\.(whl|tar\.gz|zip)$`)

func parseWheelFilename(filename string) (name, version string) {
	matches := wheelRegex.FindStringSubmatch(filename)
	if len(matches) >= 3 {
		return matches[1], matches[2]
	}

	parts := strings.SplitN(filename, "-", 2)
	if len(parts) == 2 {
		version = strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
		return parts[0], version
	}
	return "", ""
}

func normalizePackageName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}