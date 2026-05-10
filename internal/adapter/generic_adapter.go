package adapter

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/moonlight-box/registry/internal/types"

	"github.com/gin-gonic/gin"
)

type GenericAdapter struct {
	*BaseAdapter
	repoRepo *repository.RepositoryRepository
}

type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
	Path    string `json:"path"`
}

func NewGenericAdapter(
	pkgRepo *repository.PackageRepository,
	repoRepo *repository.RepositoryRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *GenericAdapter {
	adapter := &GenericAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache),
		repoRepo:    repoRepo,
	}
	return adapter
}

func (a *GenericAdapter) Type() PackageType   { return GenericType }
func (a *GenericAdapter) RoutePrefix() string { return "/files" }

func (a *GenericAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	name := strings.TrimPrefix(path, "/")
	if name == "" {
		return nil, fmt.Errorf("invalid generic path: %s", path)
	}

	return &PackageIdentity{Name: name, Version: "", Type: GenericType}, nil
}

func (a *GenericAdapter) DownloadOrBrowse(c *gin.Context) {
	filePath := c.Param("path")
	filePath = strings.TrimPrefix(filePath, "/")

	if filePath == "" {
		a.listRoot(c)
		return
	}

	storageKey := "generic/" + filePath

	backend := a.storageSvc.GetDefaultBackend()
	exists, err := backend.Exists(c.Request.Context(), storageKey)
	if err == nil && exists {
		size, err := backend.Size(c.Request.Context(), storageKey)
		if err != nil {
			content, entries := a.listDirectory(c, backend, storageKey, filePath)
			if content != nil {
				c.Header("Content-Type", "text/html; charset=utf-8")
				_ = browseHTML.Execute(c.Writer, gin.H{
					"path":    filePath,
					"entries": entries,
				})
				return
			}
			response.NotFound(c, "path not found")
			return
		}

		filename := filepath.Base(filePath)

		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		decision := a.CheckDownloadPermission(c, repo, model.PackageTypeGeneric, filePath, "", filename)
		if !decision.Allow {
			c.JSON(decision.Code, gin.H{"error": decision.Message})
			return
		}

		content, err := backend.Get(c.Request.Context(), storageKey)
		if err != nil {
			response.NotFound(c, "file not found")
			return
		}
		defer content.Close()

		contentType := a.storageSvc.GetContentType(filename)

		c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, url.PathEscape(filename)))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.fetcher != nil {
		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		if repo != nil && repo.Type == "proxy" {
			filename := filepath.Base(filePath)
			decision := a.CheckDownloadPermission(c, repo, model.PackageTypeGeneric, filePath, "", filename)
			if !decision.Allow {
				c.JSON(decision.Code, gin.H{"error": decision.Message})
				return
			}

			result, err := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "generic", filePath, "")
			if err == nil && result != nil {
				defer result.Content.Close()
				contentType := a.storageSvc.GetContentType(filename)
				c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, url.PathEscape(filename)))
				c.DataFromReader(200, result.Size, contentType, result.Content, nil)
				return
			}
		}
	}

	response.NotFound(c, "file not found")
}

func (a *GenericAdapter) UploadFile(c *gin.Context) {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return
	}
	defer file.Close()

	repoName := c.PostForm("repository")
	var repositoryID uint
	if repoName != "" {
		repo, err := a.repoRepo.FindByName(repoName)
		if err != nil {
			response.BadRequest(c, "invalid repository", "repository not found: "+repoName)
			return
		}
		if repo.Type != model.RepoTypeLocal {
			response.BadRequest(c, "invalid repository", "only local repositories support uploading")
			return
		}
		repositoryID = repo.ID
	}

	targetPath := c.PostForm("path")
	if targetPath != "" {
		targetPath = filepath.Join(targetPath, header.Filename)
	} else {
		targetPath = header.Filename
	}

	storageKey := "generic/" + filepath.Clean(targetPath)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, file, header.Size); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           targetPath,
		Type:           model.PackageTypeGeneric,
		RepositoryID:   repositoryID,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   header.Size,
		PublishedBy: userID,
	})
	if err != nil {
		backend.Delete(c.Request.Context(), storageKey)
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success":    true,
		"path":       targetPath,
		"storageKey": storageKey,
		"result": &PackageVersionResult{
			PackageID:  pkg.ID,
			Version:    "1.0.0",
			StorageKey: storageKey,
			Size:       header.Size,
		},
	})
}

func (a *GenericAdapter) BrowseDirectory(c *gin.Context) {
	dirPath := c.Param("path")
	dirPath = strings.TrimPrefix(dirPath, "/")

	backend := a.storageSvc.GetDefaultBackend()
	content, entries := a.listDirectory(c, backend, "generic/"+dirPath, dirPath)
	if content != nil {
		c.Header("Content-Type", "text/html; charset=utf-8")
		_ = browseHTML.Execute(c.Writer, gin.H{
			"path":    dirPath,
			"entries": entries,
		})
		return
	}

	response.NotFound(c, "directory not found")
}

func (a *GenericAdapter) DeleteFile(c *gin.Context) {
	filePath := c.Param("path")
	filePath = strings.TrimPrefix(filePath, "/")

	storageKey := "generic/" + filePath

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Delete(c.Request.Context(), storageKey); err != nil {
		response.NotFound(c, "file not found")
		return
	}

	c.JSON(200, gin.H{"success": true, "path": filePath})
}

func (a *GenericAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	path, _ := req.Metadata["path"].(string)
	if path == "" {
		path = req.Filename
	}

	storageKey := "generic/" + filepath.Clean(path)

	_, err := a.storageSvc.StorePackage(ctx, "generic", path, "1.0.0", reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, ver, _, err := a.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           path,
		Type:           model.PackageTypeGeneric,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
		PublishedBy: req.UploadedBy,
		Metadata:    marshalMetadata(req.Metadata),
	}, &model.PackageFile{
		Filename:    filepath.Base(path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   req.Size,
	})
	if err != nil {
		a.storageSvc.DeletePackage(ctx, "generic", path, "1.0.0")
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		VersionID:  ver.ID,
		Version:    "1.0.0",
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *GenericAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeGeneric, GenericType)
}

func (a *GenericAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeGeneric)
}

func (a *GenericAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeGeneric)
}

func (a *GenericAdapter) listRoot(c *gin.Context) {
	backend := a.storageSvc.GetDefaultBackend()
	_, entries := a.listDirectory(c, backend, "generic/", "")
	if entries != nil {
		c.Header("Content-Type", "text/html; charset=utf-8")
		_ = browseHTML.Execute(c.Writer, gin.H{
			"path":    "/",
			"entries": entries,
		})
		return
	}

	c.JSON(200, gin.H{"message": "Generic file storage"})
}

func (a *GenericAdapter) listDirectory(c *gin.Context, backend storage.Backend, storagePrefix string, displayPath string) (io.ReadCloser, []FileEntry) {
	entries, err := backend.List(c.Request.Context(), storagePrefix)
	if err != nil {
		return nil, nil
	}

	if len(entries) == 0 {
		return nil, nil
	}

	var firstEntry storage.Entry
	for _, e := range entries {
		firstEntry = e
		break
	}

	if !firstEntry.IsDir && firstEntry.Key == storagePrefix {
		return nil, nil
	}

	var fileEntries []FileEntry
	seen := make(map[string]bool)

	for _, entry := range entries {
		relPath := strings.TrimPrefix(entry.Key, storagePrefix)
		if relPath == "" || relPath == "/" {
			continue
		}

		parts := strings.SplitN(relPath, "/", 2)
		name := parts[0]

		if seen[name] {
			continue
		}
		seen[name] = true

		isDir := len(parts) > 1

		fileEntries = append(fileEntries, FileEntry{
			Name:  name,
			Size:  entry.Size,
			IsDir: isDir,
			Path:  displayPath + "/" + name,
		})
	}

	return nil, fileEntries
}

func (a *GenericAdapter) BuildRemotePath(name, version, filename string) string {
	if filename != "" {
		return name + "/" + filename
	}
	return name
}

func (a *GenericAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	a.DownloadOrBrowse(c)
}

func (a *GenericAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := a.storageSvc.GetContentType(result.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *GenericAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	name := ctx.Name
	version := ctx.Version
	filename := ctx.Filename

	storageName := name + "/" + filename
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "generic", storageName, version)
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
		result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), ctx.Repo, "generic", name, version+"/"+filename)
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

	return nil, fmt.Errorf("file not found")
}

func (a *GenericAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	userID := ctx.UserID

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}
	defer file.Close()

	targetPath := c.PostForm("path")
	if targetPath != "" {
		targetPath = filepath.Join(targetPath, header.Filename)
	} else {
		targetPath = header.Filename
	}

	storageKey := "generic/" + filepath.Clean(targetPath)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, file, header.Size); err != nil {
		return nil, err
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           targetPath,
		Type:           model.PackageTypeGeneric,
		RepositoryID:   ctx.Repo.ID,
		RepositoryType: ctx.Repo.Type,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   header.Size,
		PublishedBy: userID,
	})
	if err != nil {
		backend.Delete(c.Request.Context(), storageKey)
		return nil, err
	}

	return &types.PublishResult{
		PackageName: targetPath,
		Version:     "1.0.0",
		Size:        header.Size,
		Filename:    header.Filename,
		Response: &types.GenericPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  targetPath,
				Version:  "1.0.0",
				Filename: header.Filename,
				Size:     header.Size,
			},
			StorageKey: storageKey,
			PackageId:  pkg.ID,
		},
	}, nil
}

func (a *GenericAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	filePath := strings.TrimPrefix(c.Param("path"), "/")

	storageKey := "generic/" + filePath

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Delete(c.Request.Context(), storageKey); err != nil {
		return fmt.Errorf("file not found")
	}

	identity := &PackageIdentity{
		Name:    filePath,
		Version: "",
		Type:    GenericType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	pkg, _ := a.pkgRepo.FindByNameAndType(identity.Name, model.PackageTypeGeneric)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, ctx.Repo.Name, identity.Name, identity.Version, pkgID)

	return nil
}

var browseHTML = template.Must(template.New("browse").Parse(`<!DOCTYPE html>
<html><head><title>Browse: {{.path}}</title>
<style>
body { font-family: monospace; margin: 20px; }
a { color: #333; text-decoration: none; }
a:hover { text-decoration: underline; }
.dir { color: #0066cc; font-weight: bold; }
.size { color: #999; margin-left: 20px; }
</style>
</head><body>
<h2>Browse: {{.path}}</h2>
{{if .entries}}<table>
{{range .entries}}<tr><td>
{{if .IsDir}}<a class="dir" href="/files/browse{{.Path}}/">[DIR] {{.Name}}/</a>
{{else}}<a href="/files{{.Path}}">{{.Name}}</a><span class="size">{{.Size}} bytes</span>
{{end}}
</td></tr>{{end}}
</table>{{else}}<p>Empty directory</p>{{end}}
</body></html>`))
