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
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type GoAdapter struct {
	*BaseAdapter
}

func NewGoAdapter(
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *GoAdapter {
	adapter := &GoAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, auditSvc, pkgCache),
	}
	return adapter
}

func (a *GoAdapter) Type() PackageType { return GoType }

func (a *GoAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	// 处理元数据请求：name/@v/list
	if strings.HasSuffix(path, "/@v/list") {
		name := strings.TrimSuffix(path, "/@v/list")
		return &types.PackagePathInfo{
			Name:        name,
			Version:     "",
			Filename:    "list",
			StorageName: name,
			RemotePath:  name + "/@v/list",
		}, nil
	}

	// 处理 @v/ 路径（包含 version.info, version.mod, version.zip）
	if strings.Contains(path, "/@v/") {
		parts := strings.Split(path, "/@v/")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid go module path: %s", path)
		}

		name := parts[0]
		versionFile := parts[1]

		fileParts := strings.Split(versionFile, ".")
		if len(fileParts) < 2 {
			return nil, fmt.Errorf("invalid go module version file: %s", versionFile)
		}

		version := strings.Join(fileParts[:len(fileParts)-1], ".")
		filename := versionFile

		storageName := name
		storageVersion := "@v/" + filename
		remotePath := name + "/@v/" + filename

		return &types.PackagePathInfo{
			Name:           name,
			Version:        version,
			Filename:       filename,
			StorageName:    storageName,
			StorageVersion: storageVersion,
			RemotePath:     remotePath,
		}, nil
	}

	// 处理 name/version 格式（version 可能包含 .info/.mod/.zip）
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid go module path: %s", path)
	}

	filename := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], "/")
	version := filename

	// 如果 filename 包含 .info/.mod/.zip，说明是完整的版本文件
	if strings.HasSuffix(filename, ".info") || strings.HasSuffix(filename, ".mod") || strings.HasSuffix(filename, ".zip") {
		storageName := name
		storageVersion := "@v/" + filename
		remotePath := name + "/@v/" + filename

		return &types.PackagePathInfo{
			Name:           name,
			Version:        version,
			Filename:       filename,
			StorageName:    storageName,
			StorageVersion: storageVersion,
			RemotePath:     remotePath,
		}, nil
	}

	return nil, fmt.Errorf("invalid go module path: %s", path)
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
		if a.fetcher != nil {
			var repo *model.Repository
			if r, ok := c.Get("repo"); ok {
				repo = r.(*model.Repository)
			}

			result, resolveErr := func() (*types.RouteResult, error) {
				pathInfo, err := a.ParsePath(module + "/@v/list")
				if err != nil {
					return nil, err
				}
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				return a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			}()
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

func (a *GoAdapter) handleVersionInfo(c *gin.Context, module, version string) {
	pkg, err := a.GetPackageRepository().FindByNameAndType(module, model.PackageTypeGo)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := func() (*types.RouteResult, error) {
					pathInfo, err := a.ParsePath(module + "/@v/" + version + ".info")
					if err != nil {
						return nil, err
					}
					remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
					return a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
				}()
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
		pathInfo, pathErr := a.ParsePath(module + "/@v/" + version + ".mod")
		if pathErr != nil {
			slog.Info("Failed to resolve go.mod path", "module", module, "version", version, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				c.DataFromReader(200, result.Size, "text/plain", result.Content, nil)
				return
			}
			slog.Info("Failed to fetch go.mod from remote", "module", module, "version", version, "remotePath", remotePath, "error", fetchErr)
		}
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
		pathInfo, pathErr := a.ParsePath(module + "/@v/" + version + ".zip")
		if pathErr != nil {
			slog.Info("Failed to resolve zip path", "module", module, "version", version, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, version))
				c.DataFromReader(200, result.Size, "application/zip", result.Content, nil)
				return
			}
			slog.Info("Failed to fetch zip from remote", "module", module, "version", version, "error", fetchErr)
		}
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

			result, resolveErr := func() (*types.RouteResult, error) {
				pathInfo, err := a.ParsePath(module + "/@latest")
				if err != nil {
					return nil, err
				}
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				return a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			}()
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

func (a *GoAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeGo, GoType)
}

func (a *GoAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeGo)
}

func (a *GoAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersions(name, model.PackageTypeGo)
}

func (a *GoAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	c.Params = append(c.Params, gin.Param{Key: "path", Value: "/" + ctx.Path})
	a.goProxyHandler(c)
}

func (a *GoAdapter) syncVersionsToLocal(ctx context.Context, module string, versions []string, repoID uint) {
	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(ctx, &model.Package{
		Name:           module,
		Type:           model.PackageTypeGo,
		RepositoryID:   repoID,
		RepositoryType: model.RepoTypeProxy,
	}, nil)
	if err != nil {
		return
	}

	for _, v := range versions {
		a.GetPackageRepository().CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
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

func (a *GoAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}

	name := c.PostForm("name")
	version := c.PostForm("version")
	if name == "" || version == "" {
		file.Close()
		return nil, fmt.Errorf("missing module name or version")
	}

	return &types.PublishResult{
		PackageName: name,
		Version:     version,
		Filename:    header.Filename,
		Content:     file,
		Size:        header.Size,
		FileType:    model.FileTypePrimary,
	}, nil
}

func (a *GoAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	return fmt.Errorf("Go modules cannot be deleted directly")
}
