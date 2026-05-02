package adapter

import (
	"bytes"
	"context"
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

type GoAdapter struct {
	*BaseAdapter
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	auditSvc    *service.AuditService
	proxyRouter *proxy.ProxyRouter
}

func NewGoAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	proxyRouter *proxy.ProxyRouter,
) *GoAdapter {
	return &GoAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
		proxyRouter: proxyRouter,
	}
}

func (a *GoAdapter) SetProxyRouter(pr *proxy.ProxyRouter) {
	a.proxyRouter = pr
}

func (a *GoAdapter) Type() PackageType   { return GoType }
func (a *GoAdapter) RoutePrefix() string { return "/go" }

func (a *GoAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/*path", a.goProxyHandler)
		r.PUT("/:module/:version/upload", authMw, permMw("go", "write"), a.uploadHandler)
	}
}

func (a *GoAdapter) goProxyHandler(c *gin.Context) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")

	atIdx := strings.Index(fullPath, "/@v/")
	if atIdx == -1 {
		atIdx = strings.Index(fullPath, "/@latest")
	}
	if atIdx == -1 {
		response.NotFound(c, "invalid path")
		return
	}

	module := decodeGoModulePath(fullPath[:atIdx])
	remaining := fullPath[atIdx+1:]

	if remaining == "@latest" || strings.HasPrefix(remaining, "@latest") {
		a.latestHandler(c)
		return
	}

	if remaining == "@v/list" {
		a.handleListVersions(c, module)
		return
	}

	if strings.HasPrefix(remaining, "@v/") {
		file := strings.TrimPrefix(remaining, "@v/")
		lastDot := strings.LastIndex(file, ".")
		if lastDot == -1 {
			response.NotFound(c, "invalid file path")
			return
		}

		version := file[:lastDot]
		ext := file[lastDot+1:]

		switch ext {
		case "info":
			a.handleVersionInfo(c, module, version)
		case "mod":
			a.handleGoMod(c, module, version)
		case "zip":
			a.handleDownloadZip(c, module, version)
		default:
			response.NotFound(c, "unsupported file type")
		}
		return
	}

	response.NotFound(c, "invalid path")
}

func (a *GoAdapter) handleListVersions(c *gin.Context, module string) {
	versions, err := a.ListVersions(c.Request.Context(), module)
	if err != nil {
		if a.proxyRouter != nil {
			urlBuilder := func(repo *model.Repository, pkgName, _ string) string {
				return fmt.Sprintf("%s/%s/@v/list", strings.TrimSuffix(repo.RemoteURL, "/"), encodeGoModulePath(pkgName))
			}

			var repo *model.Repository
			if r, ok := c.Get("repo"); ok {
				repo = r.(*model.Repository)
			}

			result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "go", module, "", urlBuilder)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					remoteVersions := parseVersionList(string(body))
					if len(remoteVersions) > 0 {
						a.syncVersionsToLocal(c.Request.Context(), module, remoteVersions, result.RepoID)
						c.Data(200, "text/plain", []byte(strings.Join(remoteVersions, "\n")+"\n"))
						return
					}
				}
			}
		}
		response.NotFound(c, "module not found")
		return
	}

	c.Data(200, "text/plain", []byte(strings.Join(versions, "\n")+"\n"))
}

func (a *GoAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid go module path")
	}
	return &PackageIdentity{
		Name:    decodeGoModulePath(parts[0]),
		Version: strings.TrimSuffix(parts[1], ".zip"),
		Type:    GoType,
	}, nil
}

func (a *GoAdapter) handleVersionInfo(c *gin.Context, module, version string) {
	pkg, err := a.pkgRepo.FindByNameAndType(module, model.PackageTypeGo)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.proxyRouter != nil {
				urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
					return fmt.Sprintf("%s/%s/@v/%s.info", strings.TrimSuffix(repo.RemoteURL, "/"), encodeGoModulePath(pkgName), pkgVersion)
				}

				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "go", module, version, urlBuilder)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "application/json", body)
						return
					}
				}
			}
			response.NotFound(c, "module not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	for _, ver := range pkg.Versions {
		if ver.Version == version {
			info := gin.H{
				"Version": version,
				"Time":    ver.PublishedAt.Format(time.RFC3339),
				"Origin":  gin.H{"VCS": "unknown", "URL": "", "Hash": ""},
			}
			c.JSON(200, info)
			return
		}
	}

	response.NotFound(c, "version not found")
}

func (a *GoAdapter) handleGoMod(c *gin.Context, module, version string) {
	storageVersion := filepath.Join("@v", version+".mod")
	slog.Info("handleGoMod called", "module", module, "version", version, "storageVersion", storageVersion)

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "go", module, storageVersion)
	if err == nil {
		slog.Info("Found cached go.mod", "module", module, "version", version)
		defer content.Close()
		c.DataFromReader(200, size, "text/plain", content, nil)
		return
	}

	slog.Info("Cache miss, trying proxy", "module", module, "version", version)

	if a.proxyRouter != nil {
		urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
			return fmt.Sprintf("%s/%s/@v/%s.mod", strings.TrimSuffix(repo.RemoteURL, "/"), encodeGoModulePath(pkgName), pkgVersion)
		}

		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "go", module, version, urlBuilder)
		if resolveErr == nil && result != nil {
			slog.Info("Proxy resolved successfully", "source", result.Source, "size", result.Size)
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				slog.Info("Storing go.mod to storage", "module", module, "version", version, "size", len(body))
				storageKey, storeErr := a.storageSvc.StorePackage(c.Request.Context(), "go", module, version, bytes.NewReader(body), result.Size)
				if storeErr == nil {
					slog.Info("Stored successfully", "storageKey", storageKey)
					a.pkgRepo.StorePackageFile(c.Request.Context(), &model.Package{
						Name:           module,
						Type:           model.PackageTypeGo,
						RepositoryID:   result.RepoID,
						RepositoryType: model.RepoTypeProxy,
					}, &model.PackageVersion{
						Version:     version,
						Status:      model.StatusPublished,
						StoragePath: filepath.Dir(storageKey),
					}, &model.PackageFile{
						Filename:    version + ".mod",
						FileType:    model.FileTypeMetadata,
						StoragePath: storageKey,
						SizeBytes:   result.Size,
					})
				} else {
					slog.Error("Failed to store go.mod", "error", storeErr)
				}
				c.Data(200, "text/plain", body)
				return
			} else {
				slog.Error("Failed to read response body", "error", readErr)
			}
		} else {
			slog.Error("Proxy resolution failed", "error", resolveErr)
		}
	} else {
		slog.Warn("proxyRouter is nil")
	}

	response.NotFound(c, "go.mod not found")
}

func (a *GoAdapter) handleDownloadZip(c *gin.Context, module, version string) {
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "go", module, version)
	if err == nil {
		defer content.Close()

		a.IncrementDownloadCountForPackage(module, model.PackageTypeGo, version, version+".zip")

		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, version))
		c.DataFromReader(200, size, "application/zip", content, nil)
		return
	}

	if a.proxyRouter != nil {
		urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
			return fmt.Sprintf("%s/%s/@v/%s.zip", strings.TrimSuffix(repo.RemoteURL, "/"), encodeGoModulePath(pkgName), pkgVersion)
		}

		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "go", module, version, urlBuilder)
		if resolveErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				storageKey, storeErr := a.storageSvc.StorePackage(c.Request.Context(), "go", module, version, bytes.NewReader(body), result.Size)
				if storeErr == nil {
					a.pkgRepo.StorePackageFileAndIncrementDownload(c.Request.Context(), &model.Package{
						Name:           module,
						Type:           model.PackageTypeGo,
						RepositoryID:   result.RepoID,
						RepositoryType: model.RepoTypeProxy,
					}, &model.PackageVersion{
						Version:     version,
						Status:      model.StatusPublished,
						StoragePath: filepath.Dir(storageKey),
					}, &model.PackageFile{
						Filename:    version + ".zip",
						FileType:    model.FileTypePrimary,
						StoragePath: storageKey,
						SizeBytes:   result.Size,
					})
				}
				localContent, localSize, localErr := a.storageSvc.GetPackage(c.Request.Context(), "go", module, version)
				if localErr == nil {
					defer localContent.Close()
					c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, version))
					c.DataFromReader(200, localSize, "application/zip", localContent, nil)
					return
				}
				c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, version))
				c.Data(200, "application/zip", body)
				return
			}
		}
	}

	response.NotFound(c, "module zip not found")
}

func (a *GoAdapter) latestHandler(c *gin.Context) {
	module := decodeGoModulePath(strings.TrimSuffix(c.Param("module"), "@latest"))

	versions, err := a.ListVersions(c.Request.Context(), module)
	if err != nil || len(versions) == 0 {
		response.NotFound(c, "module not found")
		return
	}

	latest := versions[len(versions)-1]
	c.Redirect(302, fmt.Sprintf("/go/%s/@v/%s.info", encodeGoModulePath(module), latest))
}

func (a *GoAdapter) uploadHandler(c *gin.Context) {
	userID := c.GetUint("userID")
	module := decodeGoModulePath(c.Param("module"))
	version := c.Param("version")

	size := c.Request.ContentLength
	reader := c.Request.Body

	req := &UploadRequest{
		Package:  reader,
		Filename: version + ".zip",
		Size:     size,
		Metadata: map[string]interface{}{
			"module":  module,
			"version": version,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(201, gin.H{"success": true, "result": result})
}

func (a *GoAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["module"].(string)
	version, _ := req.Metadata["version"].(string)
	if name == "" || version == "" {
		return nil, fmt.Errorf("missing module name or version")
	}

	storageVersion := filepath.Join("@v", version+".zip")
	storageKey, err := a.storageSvc.StorePackage(ctx, "go", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, ver, _, err := a.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeGo,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
		PublishedBy: req.UploadedBy,
		Metadata:    marshalMetadata(req.Metadata),
	}, &model.PackageFile{
		Filename:    version + ".zip",
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   req.Size,
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "go", name, storageVersion)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		VersionID:  ver.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *GoAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "go", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/zip",
		Size:        size,
	}, nil
}

func (a *GoAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeGo)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        GoType,
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

func (a *GoAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *GoAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeGo)
}

func (a *GoAdapter) HandleRepoRequest(c *gin.Context, repo *model.Repository, path string) {
	c.Set("repo", repo)
	c.Params = append(c.Params, gin.Param{Key: "path", Value: "/" + path})
	a.goProxyHandler(c)
}

func (a *GoAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) {
	response.Forbidden(c, "Go modules cannot be published directly")
}

func (a *GoAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) {
	response.Forbidden(c, "Go modules cannot be deleted directly")
}

func (a *GoAdapter) syncVersionsToLocal(ctx context.Context, module string, versions []string, repoID uint) {
	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           module,
		Type:           model.PackageTypeGo,
		RepositoryID:   repoID,
		RepositoryType: model.RepoTypeProxy,
	}, nil)
	if err != nil {
		return
	}

	for _, v := range versions {
		a.pkgRepo.CreateOrUpdate(ctx, pkg, &model.PackageVersion{
			Version:     v,
			Status:      model.StatusPublished,
			PublishedAt: time.Now(),
		})
	}
}

func parseVersionList(body string) []string {
	versions := strings.Split(strings.TrimSpace(body), "\n")
	var result []string
	for _, v := range versions {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

var goModuleEscapeRegex = regexp.MustCompile(`[A-Z]`)

func encodeGoModulePath(path string) string {
	return goModuleEscapeRegex.ReplaceAllStringFunc(path, func(s string) string {
		return "!" + strings.ToLower(s)
	})
}

func decodeGoModulePath(path string) string {
	decoded := ""
	for i := 0; i < len(path); i++ {
		if path[i] == '!' && i+1 < len(path) {
			decoded += strings.ToUpper(string(path[i+1]))
			i++
		} else {
			decoded += string(path[i])
		}
	}
	return decoded
}
