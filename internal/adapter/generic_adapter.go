package adapter

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log/slog"
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
	storageSvc *service.StorageService,
	pkgCache *cache.PackageCache,
) *GenericAdapter {
	adapter := &GenericAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
	}
	return adapter
}

func (a *GenericAdapter) Type() PackageType { return GenericType }

func (a *GenericAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid generic path: %s", path)
	}

	filename := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], "/")
	if name == "" {
		name = filename
	}

	storageName := name
	storageVersion := filename
	remotePath := path

	return &types.PackagePathInfo{
		Name:           name,
		Version:        "",
		Filename:       filename,
		StorageName:    storageName,
		StorageVersion: storageVersion,
		RemotePath:     remotePath,
	}, nil
}

func (a *GenericAdapter) DownloadOrBrowse(c *gin.Context, filePath string) *types.ContentResult {
	result, err := a.HandleGet(c.Request.Context(), nil, &types.RequestIntent{
		Path:  strings.TrimPrefix(filePath, "/"),
		Extra: make(map[string]interface{}),
	})
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "file not found"},
		}
	}
	return result
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

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           targetPath,
		Type:           model.PackageTypeGeneric,
		RepositoryID:   repositoryID,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
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

	pkg, ver, _, err := a.GetPackageRepository().StorePackageFile(ctx, &model.Package{
		Name:           path,
		Type:           model.PackageTypeGeneric,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
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
	return a.BaseAdapter.GetRepositoryPackageMetadata(ctx, repositoryFromContext(ctx), name, model.PackageTypeGeneric, GenericType)
}

func (a *GenericAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypeGeneric)
}

func (a *GenericAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeGeneric)
}

// ParseIntent 解析请求路径为意图
func (a *GenericAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = strings.TrimPrefix(path, "/")
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	if path == "" {
		intent.Type = types.RequestList
		return intent
	}

	// For generic, all paths are downloads or directory listings
	pathInfo, _ := a.ParsePath(path)
	intent.Type = types.RequestDownload
	if pathInfo != nil {
		intent.Name = pathInfo.Name
		intent.Filename = pathInfo.Filename
		intent.PkgPathInfo = pathInfo
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *GenericAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	filePath := intent.Path

	if filePath == "" {
		return a.listRoot(ctx)
	}

	storageKey := "generic/" + filePath

	backend := a.storageSvc.GetDefaultBackend()
	exists, err := backend.Exists(ctx, storageKey)
	if err == nil && exists {
		size, err := backend.Size(ctx, storageKey)
		if err != nil {
			content, entries := a.listDirectory(ctx, backend, storageKey, filePath)
			if content != nil {
				var buf bytes.Buffer
				err := browseHTML.Execute(&buf, gin.H{
					"path":    filePath,
					"entries": entries,
				})
				if err != nil {
					return &types.ContentResult{
						StatusCode: 500,
						ExtraData:  map[string]interface{}{"message": "failed to render directory listing"},
					}, nil
				}
				htmlContent := buf.String()
				return &types.ContentResult{
					ContentType: "text/html; charset=utf-8",
					StatusCode:  200,
					Content:     io.NopCloser(bytes.NewReader([]byte(htmlContent))),
					Size:        int64(len(htmlContent)),
				}, nil
			}
			return &types.ContentResult{
				StatusCode: 404,
				ExtraData:  map[string]interface{}{"message": "path not found"},
			}, nil
		}

		filename := filepath.Base(filePath)

		content, err := backend.Get(ctx, storageKey)
		if err != nil {
			return &types.ContentResult{
				StatusCode: 404,
				ExtraData:  map[string]interface{}{"message": "file not found"},
			}, nil
		}

		contentType := a.storageSvc.GetContentType(filename)
		headers := map[string]string{
			"Content-Disposition": fmt.Sprintf(`inline; filename="%s"`, url.PathEscape(filename)),
		}

		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
			Headers:     headers,
		}, nil
	}

	if a.fetcher != nil && repo != nil && repo.Type == "proxy" {
		filename := filepath.Base(filePath)

		pathInfo, pathErr := a.ParsePath(filePath)
		if pathErr != nil {
			slog.Warn("failed to resolve generic package path", "path", filePath, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if err == nil && result != nil {
				defer result.Content.Close()
				contentType := a.storageSvc.GetContentType(filename)
				headers := map[string]string{
					"Content-Disposition": fmt.Sprintf(`inline; filename="%s"`, url.PathEscape(filename)),
				}
				return &types.ContentResult{
					Content:     result.Content,
					Size:        result.Size,
					ContentType: contentType,
					StatusCode:  200,
					Headers:     headers,
				}, nil
			}
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "file not found"},
	}, nil
}

// listRoot 列出根目录（不依赖 gin.Context）
func (a *GenericAdapter) listRoot(ctx context.Context) (*types.ContentResult, error) {
	backend := a.storageSvc.GetDefaultBackend()
	_, entries := a.listDirectory(ctx, backend, "generic/", "")
	if entries != nil {
		var buf bytes.Buffer
		err := browseHTML.Execute(&buf, gin.H{
			"path":    "/",
			"entries": entries,
		})
		if err != nil {
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:  map[string]interface{}{"message": "failed to render directory listing"},
			}, nil
		}
		htmlContent := buf.String()
		return &types.ContentResult{
			ContentType: "text/html; charset=utf-8",
			StatusCode:  200,
			Content:     io.NopCloser(bytes.NewReader([]byte(htmlContent))),
			Size:        int64(len(htmlContent)),
		}, nil
	}

	message := "Generic file storage"
	return &types.ContentResult{
		StatusCode: 200,
		ExtraData:  map[string]interface{}{"message": message},
	}, nil
}

// listDirectory 列出目录（不依赖 gin.Context）
func (a *GenericAdapter) listDirectory(ctx context.Context, backend storage.Backend, storagePrefix string, displayPath string) (io.ReadCloser, []FileEntry) {
	entries, err := backend.List(ctx, storagePrefix)
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

func (a *GenericAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
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

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           targetPath,
		Type:           model.PackageTypeGeneric,
		RepositoryID:   ctx.Repo.ID,
		RepositoryType: ctx.Repo.Type,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
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

	if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), identity); err != nil {
		return err
	}

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
