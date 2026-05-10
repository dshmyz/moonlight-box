package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
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

type GoAdapter struct {
	*BaseAdapter
	uploadSvc *service.UploadService
}

func NewGoAdapter(
	pkgRepo *repository.PackageRepository,
	repoRepo *repository.RepositoryRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *GoAdapter {
	adapter := &GoAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache),
		uploadSvc:   service.NewUploadService(pkgRepo, storageSvc),
	}
	return adapter
}

func (a *GoAdapter) Type() PackageType   { return GoType }
func (a *GoAdapter) RoutePrefix() string { return "/go" }

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
		if a.fetcher != nil {
			var repo *model.Repository
			if r, ok := c.Get("repo"); ok {
				repo = r.(*model.Repository)
			}

			result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "go", module, "")
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

func (a *GoAdapter) BuildRemotePath(name, version, filename string) string {
	if filename != "" {
		return fmt.Sprintf("%s/@v/%s", name, filename)
	}
	if version == "" {
		return fmt.Sprintf("%s/@v/list", name)
	}
	if strings.HasSuffix(version, ".info") || strings.HasSuffix(version, ".mod") || strings.HasSuffix(version, ".zip") {
		return fmt.Sprintf("%s/@v/%s", name, version)
	}
	return fmt.Sprintf("%s/@v/%s", name, version)
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
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "go", module, version)
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

	if a.fetcher != nil {
		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		remotePath := fmt.Sprintf("%s/@v/%s.mod", module, version)
		result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "go", module, version+".mod")
		if fetchErr == nil && result != nil {
			defer result.Content.Close()
			c.DataFromReader(200, result.Size, "text/plain", result.Content, nil)
			return
		}
		slog.Info("Failed to fetch go.mod from remote", "module", module, "version", version, "remotePath", remotePath, "error", fetchErr)
	}

	response.NotFound(c, "go.mod not found")
}

func (a *GoAdapter) handleDownloadZip(c *gin.Context, module, version string) {
	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeGo, module, version, version+".zip")
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	storageVersion := filepath.Join("@v", version+".zip")
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "go", module, storageVersion)
	if err == nil {
		defer content.Close()

		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, version))
		c.DataFromReader(200, size, "application/zip", content, nil)
		return
	}

	if a.fetcher != nil {
		result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "go", module, version)
		if fetchErr == nil && result != nil {
			defer result.Content.Close()
			c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, version))
			c.DataFromReader(200, result.Size, "application/zip", result.Content, nil)
			return
		}
		slog.Info("Failed to fetch zip from remote", "module", module, "version", version, "error", fetchErr)
	}

	response.NotFound(c, "module zip not found")
}

func (a *GoAdapter) latestHandler(c *gin.Context) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	atIdx := strings.Index(fullPath, "/@latest")
	module := decodeGoModulePath(fullPath[:atIdx])

	versions, err := a.ListVersions(c.Request.Context(), module)
	if err != nil || len(versions) == 0 {
		if a.fetcher != nil {
			var repo *model.Repository
			if r, ok := c.Get("repo"); ok {
				repo = r.(*model.Repository)
			}

			result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "go", module, "@latest")
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

	latest := versions[len(versions)-1]
	latestInfo := map[string]interface{}{
		"Version": latest,
		"Time":    time.Now().UTC().Format(time.RFC3339),
	}
	c.JSON(200, latestInfo)
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

	uploadCtx := &service.UploadContext{
		PkgType:        "go",
		Name:           name,
		Version:        version,
		Filename:       version + ".zip",
		Content:        reader,
		Size:           req.Size,
		PackageType:    model.PackageTypeGo,
		RepositoryType: model.RepoTypeLocal,
		UploadedBy:     req.UploadedBy,
		Metadata:       req.Metadata,
		FileType:       model.FileTypePrimary,
	}

	result, err := a.uploadSvc.Upload(ctx, uploadCtx)
	if err != nil {
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  result.PackageID,
		VersionID:  result.VersionID,
		Version:    result.Version,
		StorageKey: result.StorageKey,
		Size:       result.Size,
	}, nil
}

func (a *GoAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeGo, GoType)
}

func (a *GoAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeGo)
}

func (a *GoAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeGo)
}

func (a *GoAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	c.Params = append(c.Params, gin.Param{Key: "path", Value: "/" + ctx.Path})
	a.goProxyHandler(c)
}

func (a *GoAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
	response.Forbidden(c, "Go modules cannot be published directly")
	return nil
}

func (a *GoAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
	response.Forbidden(c, "Go modules cannot be deleted directly")
	return nil
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
		a.pkgRepo.CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
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

func (a *GoAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := "application/octet-stream"
	if strings.HasSuffix(result.Filename, ".mod") {
		contentType = "text/plain"
	} else if strings.HasSuffix(result.Filename, ".zip") {
		contentType = "application/zip"
	} else if strings.HasSuffix(result.Filename, ".info") {
		contentType = "application/json"
	}
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *GoAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	name := ctx.Name
	version := ctx.Version
	filename := ctx.Filename

	storageName := name + "/" + version + "/" + filename
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "go", storageName, version)
	if err == nil {
		return &types.DownloadResult{
			Content:   content,
			Size:      size,
			FromCache: false,
			RepoID:    ctx.Repo.ID,
			Filename:  filename,
			Name:      name,
			Version:   version,
		}, nil
	}

	if a.fetcher != nil && ctx.Repo.Type == "proxy" {
		result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), ctx.Repo, "go", name, version+"/"+filename)
		if fetchErr == nil && result != nil {
			return &types.DownloadResult{
				Content:   result.Content,
				Size:      result.Size,
				FromCache: result.FromCache,
				RepoID:    result.RepoID,
				Filename:  filename,
				Name:      name,
				Version:   version,
			}, nil
		}
	}

	return nil, fmt.Errorf("module not found")
}

func (a *GoAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	return nil, fmt.Errorf("Go modules cannot be published directly")
}

func (a *GoAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	return fmt.Errorf("Go modules cannot be deleted directly")
}
