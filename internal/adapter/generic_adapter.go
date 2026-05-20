package adapter

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
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

// 路径验证配置
const (
	MaxPathDepth      = 50   // 最大路径深度
	MaxPathLength     = 4096 // 最大路径长度
	MaxFilenameLength = 255  // 最大文件名长度
)

// 允许的文件名字符（符合大多数文件系统）
var safeFilenameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-]*$`)

// validateGenericPath 验证 generic 存储路径，防止路径遍历和其他安全问题
// 返回 (有效路径, 错误)
func validateGenericPath(inputPath string) (string, error) {
	// 检查空路径
	if inputPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// 检查最大长度
	if len(inputPath) > MaxPathLength {
		return "", fmt.Errorf("path too long: maximum %d characters allowed", MaxPathLength)
	}

	// 检查路径遍历攻击（双重编码等）
	cleaned := filepath.Clean(inputPath)

	// 检查是否包含绝对路径标记
	if strings.HasPrefix(cleaned, "/") ||
		strings.HasPrefix(cleaned, "\\") ||
		strings.HasPrefix(cleaned, "..") ||
		strings.Contains(cleaned, ".."+string(filepath.Separator)) ||
		strings.Contains(cleaned, string(filepath.Separator)+"..") {
		return "", fmt.Errorf("invalid path: path traversal detected")
	}

	// 检查 Windows 风格的路径遍历
	if strings.Contains(cleaned, "..\\") ||
		strings.Contains(cleaned, "\\..") ||
		strings.Contains(cleaned, "C:") ||
		strings.Contains(cleaned, "c:") {
		return "", fmt.Errorf("invalid path: path traversal detected")
	}

	// 检查 URL 编码的路径遍历
	decoded, err := url.QueryUnescape(cleaned)
	if err == nil && decoded != cleaned {
		// 再次验证解码后的路径
		decoded = filepath.Clean(decoded)
		if strings.HasPrefix(decoded, "/") ||
			strings.HasPrefix(decoded, "\\") ||
			strings.HasPrefix(decoded, "..") {
			return "", fmt.Errorf("invalid path: encoded path traversal detected")
		}
	}

	// 检查路径深度
	depth := 0
	current := ""
	for _, c := range cleaned {
		if c == filepath.Separator || c == '/' {
			if current != "" {
				depth++
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		depth++
	}

	if depth > MaxPathDepth {
		return "", fmt.Errorf("path too deep: maximum %d levels allowed", MaxPathDepth)
	}

	// 验证每个路径组件
	parts := strings.Split(cleaned, string(filepath.Separator))
	if len(parts) == 0 {
		parts = strings.Split(cleaned, "/")
	}

	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("invalid path: parent directory reference not allowed")
		}

		// 检查文件名长度
		if len(part) > MaxFilenameLength {
			return "", fmt.Errorf("filename too long: maximum %d characters allowed", MaxFilenameLength)
		}

		// 验证文件名安全性（除了版本目录外）
		if i == len(parts)-1 && part != "" {
			// 最后一部分可能是文件名，进行更严格的检查
			ext := path.Ext(part)
			base := strings.TrimSuffix(part, ext)

			// 文件名至少需要一个字符
			if len(base) == 0 && len(ext) > 0 {
				// 允许 .gitignore 等以点开头的文件
				if !strings.HasPrefix(part, ".") {
					return "", fmt.Errorf("invalid filename: must start with alphanumeric character or dot")
				}
			}

			// 不允许的控制字符和特殊字符
			unsafeChars := []string{"<", ">", ":", "\"", "|", "?", "*", "\x00"}
			for _, char := range unsafeChars {
				if strings.Contains(part, char) {
					return "", fmt.Errorf("invalid filename: contains unsafe character")
				}
			}
		}
	}

	// 验证最终的干净路径
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("invalid path")
	}

	return cleaned, nil
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

	return &types.PackagePathInfo{
		Name:           path,
		Version:        "1.0.0",
		Filename:       filename,
		StorageName:    path,
		StorageVersion: "1.0.0",
		RemotePath:     path,
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

	// 验证文件名
	filename := header.Filename
	if validFilename, err := validateGenericPath(filename); err != nil {
		response.BadRequest(c, "invalid filename", err.Error())
		return
	} else {
		filename = validFilename
	}

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
		// 验证并清理目标路径
		if validPath, err := validateGenericPath(targetPath); err != nil {
			response.BadRequest(c, "invalid path", err.Error())
			return
		} else {
			targetPath = validPath
		}
		targetPath = filepath.Join(targetPath, filename)
	} else {
		targetPath = filename
	}

	// 最终验证完整路径
	if validPath, err := validateGenericPath(targetPath); err != nil {
		response.BadRequest(c, "invalid path", err.Error())
		return
	} else {
		targetPath = validPath
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
		var parentPath string
		if idx := strings.LastIndex(dirPath, "/"); idx >= 0 {
			parentPath = "/" + dirPath[:idx] + "/"
		}
		_ = browseHTML.Execute(c.Writer, gin.H{
			"path":       dirPath,
			"entries":    entries,
			"parentPath": parentPath,
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

	// 验证文件名
	filename := req.Filename
	if validFilename, err := validateGenericPath(filename); err != nil {
		return nil, fmt.Errorf("invalid filename: %v", err)
	} else {
		filename = validFilename
	}

	path, _ := req.Metadata["path"].(string)
	if path == "" {
		path = filename
	} else {
		// 验证并清理目标路径
		if validPath, err := validateGenericPath(path); err != nil {
			return nil, fmt.Errorf("invalid path: %v", err)
		} else {
			path = filepath.Join(validPath, filename)
		}
	}

	// 最终验证完整路径
	if validPath, err := validateGenericPath(path); err != nil {
		return nil, fmt.Errorf("invalid path: %v", err)
	} else {
		path = validPath
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

	if path == "" || strings.HasSuffix(path, "/") {
		intent.Type = types.RequestList
		return intent
	}

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
		return a.listRoot(ctx, repo)
	}

	validPath, err := validateGenericPath(filePath)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}, nil
	}

	storageKey := fmt.Sprintf("generic/%s/%s", repo.Name, validPath)

	backend := a.storageSvc.GetDefaultBackend()

	// 先尝试作为文件获取
	exists, err := backend.Exists(ctx, storageKey)
	if err == nil && exists {
		size, err := backend.Size(ctx, storageKey)
		if err == nil {
			// 是文件，返回内容
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
	}

	// 尝试作为目录浏览
	dirKey := strings.TrimSuffix(storageKey, "/") + "/"
	_, entries := a.listDirectory(ctx, backend, dirKey, strings.TrimSuffix(filePath, "/"))
	if len(entries) > 0 {
		repoBasePath := fmt.Sprintf("/repository/%s/", repo.Name)
		for i := range entries {
			entries[i].Path = repoBasePath + strings.TrimPrefix(entries[i].Path, "/")
		}
		var parentPath string
		if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
			parentPath = repoBasePath + filePath[:idx] + "/"
		} else {
			parentPath = repoBasePath
		}
		var buf bytes.Buffer
		err := browseHTML.Execute(&buf, gin.H{
			"path":       filePath,
			"entries":    entries,
			"parentPath": parentPath,
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

	if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		filename := filepath.Base(filePath)

		pathInfo, pathErr := a.ParsePath(filePath)
		if pathErr != nil {
			slog.Warn("failed to resolve generic package path", "path", filePath, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if err == nil && result != nil {
				defer result.Content.Close()

				// 读取到 buffer，同时用于存储和响应
				body, readErr := io.ReadAll(result.Content)
				if readErr != nil {
					slog.Warn("failed to read generic proxy content", "error", readErr)
				} else {
					// 缓存到本地存储，避免后续重复拉取
					storageKey := fmt.Sprintf("generic/%s/%s", repo.Name, filePath)
					_ = storageKey
					a.storageSvc.StorePackageWithBackend(ctx, repo.Name, "generic", filePath, filename, bytes.NewReader(body), int64(len(body)), repositoryStorageBackendID(repo))

					contentType := a.storageSvc.GetContentType(filename)
					headers := map[string]string{
						"Content-Disposition": fmt.Sprintf(`inline; filename="%s"`, url.PathEscape(filename)),
					}
					return &types.ContentResult{
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
						ContentType: contentType,
						StatusCode:  200,
						Headers:     headers,
					}, nil
				}
			}
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "file not found"},
	}, nil
}

// listRoot 列出根目录（不依赖 gin.Context）
func (a *GenericAdapter) listRoot(ctx context.Context, repo *model.Repository) (*types.ContentResult, error) {
	backend := a.storageSvc.GetDefaultBackend()
	storagePrefix := fmt.Sprintf("generic/%s/", repo.Name)
	_, entries := a.listDirectory(ctx, backend, storagePrefix, "")
	if entries != nil {
		repoBasePath := fmt.Sprintf("/repository/%s/", repo.Name)
		for i := range entries {
			entries[i].Path = repoBasePath + entries[i].Name
		}
		var buf bytes.Buffer
		err := browseHTML.Execute(&buf, gin.H{
			"path":       "/",
			"entries":    entries,
			"parentPath": "",
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

		isDir := entry.IsDir || len(parts) > 1

		if isDir {
			versionKey := entry.Key
			if !strings.HasSuffix(versionKey, "/") {
				versionKey = versionKey + "/"
			}
			versionKey = versionKey + "1.0.0"
			versionExists, _ := backend.Exists(ctx, versionKey)
			if versionExists {
				versionSize, versionErr := backend.Size(ctx, versionKey)
				if versionErr == nil {
					isDir = false
					entry.Size = versionSize
				}
			}
		}

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
	targetPath := strings.TrimPrefix(c.Param("path"), "/")
	if targetPath == "" {
		return nil, fmt.Errorf("path is required")
	}

	if validPath, err := validateGenericPath(targetPath); err != nil {
		return nil, fmt.Errorf("invalid path: %v", err)
	} else {
		targetPath = validPath
	}

	filename := filepath.Base(targetPath)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	return &types.PublishResult{
		PackageName:    targetPath,
		StorageName:    targetPath,
		Version:        "1.0.0",
		StorageVersion: "",
		Size:           int64(len(body)),
		Filename:       filename,
		Content:        bytes.NewReader(body),
		Response: &types.GenericPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  targetPath,
				Version:  "1.0.0",
				Filename: filename,
				Size:     int64(len(body)),
			},
		},
	}, nil
}

func (a *GenericAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	filePath := strings.TrimPrefix(c.Param("path"), "/")

	// 验证并清理文件路径
	validPath, err := validateGenericPath(filePath)
	if err != nil {
		return fmt.Errorf("invalid path: %v", err)
	}

	storageKey := "generic/" + validPath

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Delete(c.Request.Context(), storageKey); err != nil {
		return fmt.Errorf("file not found")
	}

	identity := &PackageIdentity{
		Name:    validPath,
		Version: "",
		Type:    GenericType,
	}

	if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), identity); err != nil {
		return err
	}

	return nil
}

var browseHTML = template.Must(template.New("browse").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Browse: {{.path}}</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f5f7fa;color:#333;padding:0}
.header{background:#1a73e8;color:#fff;padding:16px 24px;display:flex;align-items:center;gap:12px}
.header h1{font-size:18px;font-weight:500}
.header .repo{font-size:14px;opacity:.85}
.container{max-width:1100px;margin:24px auto;padding:0 24px}
.list{background:#fff;border-radius:8px;border:1px solid #e0e4ea;overflow:hidden}
.item{display:flex;align-items:center;padding:10px 16px;border-bottom:1px solid #e0e4ea;transition:background .15s}
.item:last-child{border-bottom:none}
.item:hover{background:#f0f4ff}
.item .icon{width:26px;height:26px;display:flex;align-items:center;justify-content:center;font-size:14px;margin-right:10px;border-radius:4px;flex-shrink:0}
.item .icon.dir{background:#e8f0fe;color:#1a73e8}
.item .icon.file{background:#f0f4ff;color:#5f6368}
.item a{color:#1a73e8;text-decoration:none;font-size:14px;flex:1;overflow:hidden;text-overflow:ellipsis}
.item a:hover{text-decoration:underline}
.item .meta{color:#999;font-size:12px;margin-left:8px;white-space:nowrap}
.empty{text-align:center;padding:48px 24px;color:#999;font-size:14px}
.footer{text-align:center;padding:24px;color:#bbb;font-size:12px}
</style>
</head><body>
<div class="header"><h1>Browse</h1><span class="repo">{{.path}}</span></div>
<div class="container">
{{if .entries}}<div class="list">
{{if .parentPath}}<div class="item"><span class="icon dir">📁</span><a href="{{.parentPath}}">..</a></div>{{end}}
{{range .entries}}<div class="item">
{{if .IsDir}}<span class="icon dir">📁</span><a href="{{.Path}}/">{{.Name}}/</a>
{{else}}<span class="icon file">📄</span><a href="{{.Path}}">{{.Name}}</a><span class="meta">{{.Size}} bytes</span>
{{end}}
</div>{{end}}
</div>{{else}}<div class="empty">This directory is empty</div>{{end}}
</div>
<div class="footer">Moonlight Box Artifact Repository</div>
</body></html>`))
