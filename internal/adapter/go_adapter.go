package adapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
	"golang.org/x/mod/modfile"
)

type GoAdapter struct {
	*BaseAdapter
}

func NewGoAdapter(
	storageSvc *service.StorageService,
	pkgCache *cache.PackageCache,
) *GoAdapter {
	adapter := &GoAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
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

func (a *GoAdapter) goProxyHandler(ctx context.Context, fullPath string, repo *model.Repository) (*types.ContentResult, error) {
	atIdx := strings.Index(fullPath, "/@v/")
	if atIdx == -1 {
		atIdx = strings.Index(fullPath, "/@latest")
	}
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid path")
	}

	module := decodeGoModulePath(fullPath[:atIdx])
	remaining := fullPath[atIdx+1:]

	if remaining == "@latest" || strings.HasPrefix(remaining, "@latest") {
		return a.handleLatest(ctx, module, repo)
	}

	if remaining == "@v/list" {
		return a.handleListVersions(ctx, module, repo)
	}

	if strings.HasPrefix(remaining, "@v/") {
		file := strings.TrimPrefix(remaining, "@v/")
		lastDot := strings.LastIndex(file, ".")
		if lastDot == -1 {
			return nil, fmt.Errorf("invalid file path")
		}

		version := file[:lastDot]
		ext := file[lastDot+1:]

		switch ext {
		case "info":
			return a.handleVersionInfo(ctx, module, version, repo)
		case "mod":
			return a.handleGoMod(ctx, module, version, repo)
		case "zip":
			return a.handleDownloadZip(ctx, module, version, repo)
		default:
			return nil, fmt.Errorf("unsupported file type")
		}
	}

	return nil, fmt.Errorf("invalid path")
}

func (a *GoAdapter) handleListVersions(ctx context.Context, module string, repo *model.Repository) (*types.ContentResult, error) {
	versions, err := a.ListVersions(ctx, module)
	if err != nil {
		if a.fetcher != nil {
			result, resolveErr := func() (*types.RouteResult, error) {
				pathInfo, err := a.ParsePath(module + "/@v/list")
				if err != nil {
					return nil, err
				}
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				return a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			}()
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					remoteVersions := parseVersionList(string(body))
					if len(remoteVersions) > 0 {
						a.syncVersionsToLocal(ctx, module, remoteVersions, result.RepoID)
						content := strings.Join(remoteVersions, "\n") + "\n"
						return &types.ContentResult{
							Content:     io.NopCloser(strings.NewReader(content)),
							Size:        int64(len(content)),
							ContentType: "text/plain",
							StatusCode:  200,
						}, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("module not found")
	}

	content := strings.Join(versions, "\n") + "\n"
	return &types.ContentResult{
		Content:     io.NopCloser(strings.NewReader(content)),
		Size:        int64(len(content)),
		ContentType: "text/plain",
		StatusCode:  200,
	}, nil
}

func (a *GoAdapter) handleVersionInfo(ctx context.Context, module, version string, repo *model.Repository) (*types.ContentResult, error) {
	pkgRepo := a.GetPackageRepository()
	if pkgRepo == nil {
		return nil, fmt.Errorf("package repository not available")
	}
	pkg, err := pkgRepo.FindByRepoNameAndTypeContext(ctx, repositoryID(repo), module, model.PackageTypeGo)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				result, resolveErr := func() (*types.RouteResult, error) {
					pathInfo, err := a.ParsePath(module + "/@v/" + version + ".info")
					if err != nil {
						return nil, err
					}
					remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
					return a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
				}()
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						return &types.ContentResult{
							Content:     io.NopCloser(bytes.NewReader(body)),
							Size:        int64(len(body)),
							ContentType: "application/json",
							StatusCode:  200,
						}, nil
					}
				}
			}
			return nil, fmt.Errorf("module not found")
		}
		return nil, fmt.Errorf("failed to find package: %v", err)
	}

	for _, ver := range pkg.Versions {
		if ver.Version == version {
			info := map[string]interface{}{
				"Version": version,
				"Time":    ver.PublishedAt.Format(time.RFC3339),
				"Origin":  map[string]interface{}{"VCS": "unknown", "URL": "", "Hash": ""},
			}
			return &types.ContentResult{
				ExtraData:  info,
				StatusCode: 200,
			}, nil
		}
	}

	return nil, fmt.Errorf("version not found")
}

func (a *GoAdapter) handleGoMod(ctx context.Context, module, version string, repo *model.Repository) (*types.ContentResult, error) {
	storageVersion := filepath.Join("@v", version+".mod")
	slog.Info("handleGoMod called", "module", module, "version", version, "storageVersion", storageVersion)

	content, size, err := a.storageSvc.GetPackageWithBackend(ctx, repo.Name, "go", module, storageVersion, repositoryStorageBackendID(repo))
	if err == nil {
		slog.Info("Found cached go.mod", "module", module, "version", version)
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: "text/plain",
			StatusCode:  200,
		}, nil
	}

	if a.fetcher != nil {
		remotePath := fmt.Sprintf("%s/@v/%s.mod", module, version)
		pathInfo, pathErr := a.ParsePath(module + "/@v/" + version + ".mod")
		if pathErr != nil {
			slog.Info("Failed to resolve go.mod path", "module", module, "version", version, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr == nil && result != nil {
				return &types.ContentResult{
					Content:     result.Content,
					Size:        result.Size,
					ContentType: "text/plain",
					StatusCode:  200,
				}, nil
			}
			slog.Info("Failed to fetch go.mod from remote", "module", module, "version", version, "remotePath", remotePath, "error", fetchErr)
		}
	}

	return nil, fmt.Errorf("go.mod not found")
}

func (a *GoAdapter) handleDownloadZip(ctx context.Context, module, version string, repo *model.Repository) (*types.ContentResult, error) {
	storageVersion := filepath.Join("@v", version+".zip")
	content, size, err := a.storageSvc.GetPackageWithBackend(ctx, repo.Name, "go", module, storageVersion, repositoryStorageBackendID(repo))
	if err == nil {
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: "application/zip",
			StatusCode:  200,
			Headers: map[string]string{
				"Content-Disposition": fmt.Sprintf(`attachment; filename="%s.zip"`, version),
			},
		}, nil
	}

	if a.fetcher != nil {
		pathInfo, pathErr := a.ParsePath(module + "/@v/" + version + ".zip")
		if pathErr != nil {
			slog.Info("Failed to resolve zip path", "module", module, "version", version, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr == nil && result != nil {
				return &types.ContentResult{
					Content:     result.Content,
					Size:        result.Size,
					ContentType: "application/zip",
					StatusCode:  200,
					Headers: map[string]string{
						"Content-Disposition": fmt.Sprintf(`attachment; filename="%s.zip"`, version),
					},
				}, nil
			}
			slog.Info("Failed to fetch zip from remote", "module", module, "version", version, "error", fetchErr)
		}
	}

	return nil, fmt.Errorf("module zip not found")
}

func (a *GoAdapter) handleLatest(ctx context.Context, module string, repo *model.Repository) (*types.ContentResult, error) {
	versions, err := a.ListVersions(ctx, module)
	if err != nil || len(versions) == 0 {
		if a.fetcher != nil {
			result, resolveErr := func() (*types.RouteResult, error) {
				pathInfo, err := a.ParsePath(module + "/@latest")
				if err != nil {
					return nil, err
				}
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				return a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			}()
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					return &types.ContentResult{
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
						ContentType: "application/json",
						StatusCode:  200,
					}, nil
				}
			}
		}
		return nil, fmt.Errorf("module not found")
	}

	latest := versions[len(versions)-1]
	latestInfo := map[string]interface{}{
		"Version": latest,
		"Time":    time.Now().UTC().Format(time.RFC3339),
	}
	return &types.ContentResult{
		ExtraData:  latestInfo,
		StatusCode: 200,
	}, nil
}

func (a *GoAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetRepositoryPackageMetadata(ctx, repositoryFromContext(ctx), name, model.PackageTypeGo, GoType)
}

func (a *GoAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	pkgRepo := a.GetPackageRepository()
	if pkgRepo == nil {
		return fmt.Errorf("package repository not available")
	}
	return pkgRepo.DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypeGo)
}

func (a *GoAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	pkgRepo := a.GetPackageRepository()
	if pkgRepo == nil {
		return nil, fmt.Errorf("package repository not available")
	}
	return pkgRepo.ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeGo)
}

// ParseIntent 解析请求路径为意图
func (a *GoAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = strings.TrimPrefix(path, "/")

	var name, version, filename string
	var reqType types.RequestType

	if strings.HasSuffix(path, "/@v/list") {
		name = strings.TrimSuffix(path, "/@v/list")
		reqType = types.RequestList
		filename = "list"
	} else if strings.HasSuffix(path, "/@latest") {
		name = strings.TrimSuffix(path, "/@latest")
		reqType = types.RequestMetadata
		filename = "latest"
	} else if strings.Contains(path, "/@v/") {
		parts := strings.Split(path, "/@v/")
		if len(parts) >= 2 {
			name = parts[0]
			versionFile := parts[1]
			fileParts := strings.Split(versionFile, ".")
			if len(fileParts) >= 2 {
				version = strings.Join(fileParts[:len(fileParts)-1], ".")
				filename = versionFile
			}
		}
		reqType = types.RequestDownload
	} else {
		// Fallback to ParsePath
		pathInfo, err := a.ParsePath(path)
		if err == nil {
			name = pathInfo.Name
			version = pathInfo.Version
			filename = pathInfo.Filename
		}
		reqType = types.RequestDownload
	}

	pathInfo, _ := a.ParsePath(path)

	return &types.RequestIntent{
		Type:        reqType,
		Name:        name,
		Version:     version,
		Filename:    filename,
		Path:        path,
		PkgPathInfo: pathInfo,
		Extra:       make(map[string]interface{}),
	}
}

// FetchContent 根据意图获取内容
func (a *GoAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	name := intent.Name
	version := intent.Version
	filename := intent.Filename

	switch intent.Type {
	case types.RequestList:
		return a.handleListVersions(ctx, name, repo)
	case types.RequestMetadata:
		if filename == "latest" {
			return a.handleLatest(ctx, name, repo)
		}
		// 其他元数据请求
		return a.handleVersionInfo(ctx, name, version, repo)
	case types.RequestDownload:
		switch {
		case strings.HasSuffix(filename, ".info"):
			return a.handleVersionInfo(ctx, name, version, repo)
		case strings.HasSuffix(filename, ".mod"):
			return a.handleGoMod(ctx, name, version, repo)
		case strings.HasSuffix(filename, ".zip"):
			return a.handleDownloadZip(ctx, name, version, repo)
		default:
			// 未知文件扩展，尝试通过 goProxyHandler 处理
			return a.goProxyHandler(ctx, "/"+intent.Path, repo)
		}
	default:
		return nil, fmt.Errorf("unsupported request type: %s", intent.Type)
	}
}

func (a *GoAdapter) syncVersionsToLocal(ctx context.Context, module string, versions []string, repoID uint) {
	pkgRepo := a.GetPackageRepository()
	if pkgRepo == nil {
		slog.Warn("package repository not available, skipping sync")
		return
	}
	pkg, _, err := pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           module,
		Type:           model.PackageTypeGo,
		RepositoryID:   repoID,
		RepositoryType: model.RepoTypeProxy,
	}, nil)
	if err != nil {
		return
	}

	for _, v := range versions {
		pkgRepo.CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
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

func buildGoDependenciesFromMod(content []byte) []model.PackageDependency {
	parsed, err := modfile.Parse("go.mod", content, nil)
	if err != nil || parsed == nil {
		return nil
	}

	deps := make([]model.PackageDependency, 0, len(parsed.Require))
	seen := make(map[string]struct{}, len(parsed.Require))
	for _, req := range parsed.Require {
		if req == nil || req.Mod.Path == "" || req.Mod.Version == "" {
			continue
		}
		key := req.Mod.Path + "|" + req.Mod.Version + "|" + fmt.Sprint(req.Indirect)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deps = append(deps, model.PackageDependency{
			DepName:              req.Mod.Path,
			DepVersionConstraint: req.Mod.Version,
			DepType:              "direct",
			PackageType:          string(model.PackageTypeGo),
			IsOptional:           req.Indirect,
		})
	}
	return deps
}

func (a *GoAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
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

	body, err := io.ReadAll(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read uploaded file: %v", err)
	}
	_ = file.Close()

	var deps []model.PackageDependency
	if strings.HasSuffix(strings.ToLower(header.Filename), ".mod") {
		deps = buildGoDependenciesFromMod(body)
	}

	return &types.PublishResult{
		PackageName: name,
		Version:     version,
		Filename:    header.Filename,
		Content:     bytes.NewReader(body),
		Size:        header.Size,
		FileType:    model.FileTypePrimary,
		Dependencies: deps,
	}, nil
}

func (a *GoAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	return fmt.Errorf("Go modules cannot be deleted directly")
}
