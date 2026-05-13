package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type NpmAdapter struct {
	*BaseAdapter
}

type NpmPackageMetadata struct {
	ID          string                     `json:"_id"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Versions    map[string]*NpmVersionInfo `json:"versions"`
	DistTags    map[string]string          `json:"dist-tags"`
	Time        map[string]string          `json:"time"`
	Readme      string                     `json:"readme"`
}

type NpmVersionInfo struct {
	ID              string                 `json:"_id"`
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	Description     string                 `json:"description"`
	Main            string                 `json:"main"`
	Homepage        string                 `json:"homepage"`
	License         string                 `json:"license"`
	Author          NpmAuthor              `json:"author"`
	Repository      *NpmRepository         `json:"repository"`
	Dependencies    map[string]interface{} `json:"dependencies"`
	DevDependencies map[string]interface{} `json:"devDependencies"`
	Dist            NpmDist                `json:"dist"`
}

type NpmAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

func (a *NpmAuthor) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	if data[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		a.Name = name
		return nil
	}

	type Alias NpmAuthor
	aux := &struct{ *Alias }{Alias: (*Alias)(a)}
	return json.Unmarshal(data, aux)
}

type NpmDist struct {
	Integrity string `json:"integrity"`
	Tarball   string `json:"tarball"`
	Shasum    string `json:"shasum"`
}

type NpmPerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

type NpmRepository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func NewNpmAdapter(storageSvc *service.StorageService, pkgCache *cache.PackageCache) *NpmAdapter {
	return &NpmAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
	}
}

// NewNpmAdapterCompat keeps backward compatibility with old call sites.
// Supported signatures:
//   - NewNpmAdapter(storageSvc, pkgCache)
//   - NewNpmAdapterCompat(pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache)
func NewNpmAdapterCompat(args ...interface{}) *NpmAdapter {
	if len(args) >= 2 {
		if storageSvc, ok := args[0].(*service.StorageService); ok {
			if pkgCache, ok := args[1].(*cache.PackageCache); ok {
				return NewNpmAdapter(storageSvc, pkgCache)
			}
		}
	}
	if len(args) >= 3 {
		if storageSvc, ok := args[2].(*service.StorageService); ok {
			var pkgCache *cache.PackageCache
			if pkgRepo, ok := args[0].(*repository.PackageRepository); ok {
				pkgCache = cache.NewPackageCache(pkgRepo, 5*time.Minute)
			}
			if len(args) >= 5 {
				if c, ok := args[4].(*cache.PackageCache); ok {
					pkgCache = c
				}
			}
			return NewNpmAdapter(storageSvc, pkgCache)
		}
	}
	return NewNpmAdapter(nil, nil)
}

func (a *NpmAdapter) Type() PackageType { return NpmType }

func (a *NpmAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	if path == "" {
		return nil, fmt.Errorf("invalid npm package path: %s", path)
	}

	if strings.Contains(path, ".tgz") {
		return a.resolveTarballPath(path)
	}

	parts := strings.Split(path, "/")

	if strings.HasPrefix(path, "@") {
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid scoped npm package path: %s", path)
		}

		name := parts[0] + "/" + parts[1]
		version := ""
		if len(parts) >= 3 {
			version = parts[2]
		}

		remotePath := name
		if version != "" {
			remotePath = name + "/" + version
		}

		return &types.PackagePathInfo{
			Name:           name,
			Version:        version,
			Filename:       "",
			StorageName:    name,
			StorageVersion: version,
			RemotePath:     remotePath,
		}, nil
	}

	name := parts[0]
	version := ""
	if len(parts) >= 2 {
		version = parts[1]
	}

	remotePath := name
	if version != "" {
		remotePath = name + "/" + version
	}

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       "",
		StorageName:    name,
		StorageVersion: version,
		RemotePath:     remotePath,
	}, nil
}

func (a *NpmAdapter) resolveTarballPath(path string) (*types.PackagePathInfo, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid npm tarball path: %s", path)
	}

	filename := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], "/")

	version := ""
	base := strings.TrimSuffix(filename, ".tgz")
	if idx := strings.LastIndex(base, "-"); idx != -1 {
		version = base[idx+1:]
	}

	storageName := name + "/" + filename
	remotePath := name + "/-/" + filename

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       filename,
		StorageName:    storageName,
		StorageVersion: version,
		RemotePath:     remotePath,
	}, nil
}

// ParseIntent 解析请求路径为意图
func (a *NpmAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = trimLeadingSlash(path)
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	pathInfo, err := a.ParsePath(path)
	if err != nil {
		// 无法解析，设为未知意图
		intent.Type = types.RequestUnknown
		return intent
	}
	intent.PkgPathInfo = pathInfo

	// 根据路径特征区分意图
	if strings.Contains(path, ".tgz") {
		// tarball 路径是下载请求
		intent.Type = types.RequestDownload
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
	} else if strings.Contains(path, "/-/") {
		// 附件路径也是下载请求
		intent.Type = types.RequestDownload
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
	} else {
		// 其余都是元数据请求
		intent.Type = types.RequestMetadata
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *NpmAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	switch intent.Type {
	case types.RequestDownload:
		return a.handleTarballDownload(ctx, repo, intent)
	case types.RequestMetadata:
		return a.handleMetadataFetch(ctx, repo, intent)
	default:
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "unknown request type"},
		}, nil
	}
}

// handleTarballDownload 处理 tarball 下载
func (a *NpmAdapter) handleTarballDownload(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	filename := intent.Filename

	// NPM tarball 通常从远程 CDN 获取
	if a.fetcher != nil && repo != nil {
		pathInfo := intent.PkgPathInfo
		if pathInfo == nil {
			pathInfo, _ = a.ParsePath(intent.Path)
		}
		if pathInfo != nil {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr == nil && result != nil {
				return &types.ContentResult{
					StatusCode:  200,
					ContentType: "application/octet-stream",
					Content:     result.Content,
					Size:        result.Size,
					Headers:     map[string]string{"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename)},
				}, nil
			}
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "tarball not found"},
	}, nil
}

// handleMetadataFetch 处理元数据获取
func (a *NpmAdapter) handleMetadataFetch(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	name := intent.Name

	switch repo.Type {
	case model.RepoTypeLocal:
		meta, err := a.GetMetadata(ctx, name)
		if err != nil {
			if util.IsErr(err, util.ErrPackageNotFound) {
				return &types.ContentResult{
					StatusCode: 404,
					ExtraData:  map[string]interface{}{"message": "package not found"},
				}, nil
			}
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:  map[string]interface{}{"message": err.Error()},
			}, nil
		}
		metaJSON, marshalErr := json.Marshal(meta)
		if marshalErr != nil {
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:  map[string]interface{}{"message": "failed to marshal metadata"},
			}, nil
		}
		return &types.ContentResult{
			ContentType: "application/json",
			StatusCode:  200,
			Content:     io.NopCloser(bytes.NewReader(metaJSON)),
			Size:        int64(len(metaJSON)),
		}, nil

	case model.RepoTypeProxy:
		return a.handleProxyMetadataFetch(ctx, repo, name)

	case model.RepoTypeVirtual:
		return a.handleVirtualMetadataFetch(ctx, repo, name)

	default:
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "unknown repository type"},
		}, nil
	}
}

// handleProxyMetadataFetch 处理代理仓库元数据请求（不依赖 gin.Context）
func (a *NpmAdapter) handleProxyMetadataFetch(ctx context.Context, repo *model.Repository, name string) (*types.ContentResult, error) {
	pathInfo, err := a.ParsePath(name)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}
	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}

	return &types.ContentResult{
		Content:     result.Content,
		Size:        result.Size,
		ContentType: "application/json",
		StatusCode:  200,
	}, nil
}

// handleVirtualMetadataFetch 处理虚拟仓库元数据请求（不依赖 gin.Context）
func (a *NpmAdapter) handleVirtualMetadataFetch(ctx context.Context, repo *model.Repository, name string) (*types.ContentResult, error) {
	if a.fetcher == nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}

	pathInfo, err := a.ParsePath(name)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}
	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "package not found"},
		}, nil
	}

	return &types.ContentResult{
		Content:     result.Content,
		Size:        result.Size,
		ContentType: "application/json",
		StatusCode:  200,
	}, nil
}

func extractVersionFromTarball(filename, pkgName string) string {
	// package-1.0.0.tgz → 1.0.0
	// @scope/package-1.0.0.tgz → 1.0.0
	base := strings.TrimSuffix(filename, ".tgz")

	// 移除包名部分
	pkgNamePart := filepath.Base(pkgName)
	if strings.HasPrefix(base, pkgNamePart+"-") {
		return strings.TrimPrefix(base, pkgNamePart+"-")
	}

	return base
}

func (a *NpmAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeNPM, NpmType)
}

func (a *NpmAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeNPM)
}

func (a *NpmAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersions(name, model.PackageTypeNPM)
}

func (a *NpmAdapter) syncFromProxy(ctx context.Context, name string, repo *model.Repository) error {
	pathInfo, err := a.ParsePath(name)
	if err != nil {
		return err
	}
	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return err
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return readErr
	}

	var metadata NpmPackageMetadata
	if jsonErr := json.Unmarshal(body, &metadata); jsonErr != nil {
		return jsonErr
	}

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(ctx, &model.Package{
		Name:           metadata.Name,
		Type:           model.PackageTypeNPM,
		RepositoryType: model.RepoTypeProxy,
		Description:    metadata.Description,
	}, nil)
	if err != nil {
		return err
	}

	for version, verInfo := range metadata.Versions {
		publishedAt := parseTime(metadata.Time[version])
		versionMeta := marshalMetadata(map[string]interface{}{
			"version":     version,
			"publishedAt": publishedAt.Format(time.RFC3339),
			"tarball":     verInfo.Dist.Tarball,
		})

		_, _, err := a.GetPackageRepository().CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
			Version:     version,
			Status:      model.StatusPublished,
			PublishedAt: publishedAt,
			Metadata:    versionMeta,
		})
		if err != nil {
			continue
		}
	}

	return nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}

func generateRevision() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func marshalMetadata(meta map[string]interface{}) string {
	data, _ := json.Marshal(meta)
	return string(data)
}

func (a *NpmAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	fullPath := cutBeforeMarker(trimLeadingSlash(c.Param("path")), "/-rev/")
	parts := splitPathN(fullPath, 2)
	scope := ""
	pkgName := fullPath

	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
		scope = parts[0]
		pkgName = parts[1]
	}

	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	file, header, err := c.Request.FormFile("_attachments")
	if err != nil {
		return nil, fmt.Errorf("missing attachment: %v", err)
	}
	defer file.Close()

	metadataRaw := c.PostForm("_attachment")
	var metadata NpmVersionInfo
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		return nil, fmt.Errorf("invalid metadata: %v", err)
	}

	allowOverwrite := c.GetBool("allowOverwrite")
	if !allowOverwrite {
		pkgRepo := a.GetPackageRepository()
		if pkgRepo == nil {
			return nil, fmt.Errorf("package repository not initialized")
		}
		existingPkg, err := pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
		if err == nil {
			for _, ver := range existingPkg.Versions {
				if ver.Version == metadata.Version {
					return nil, fmt.Errorf("版本 %s 已存在，不允许覆盖", metadata.Version)
				}
			}
		}
	}

	return &types.PublishResult{
		PackageName: name,
		Version:     metadata.Version,
		Filename:    header.Filename,
		Content:     file,
		Size:        header.Size,
		FileType:    model.FileTypePrimary,
		Response: &types.NpmPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  name,
				Version:  metadata.Version,
				Filename: header.Filename,
				Size:     header.Size,
			},
			Description: metadata.Description,
		},
	}, nil
}

func (a *NpmAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := cutBeforeMarker(trimLeadingSlash(c.Param("path")), "/-rev/")
	parts := splitPathN(fullPath, 2)
	scope := ""
	pkgName := fullPath

	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
		scope = parts[0]
		pkgName = parts[1]
	}

	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	identity := &PackageIdentity{
		Name: name,
		Type: NpmType,
	}

	if identity.Version == "" {
		versions, err := a.ListVersions(c.Request.Context(), name)
		if err != nil {
			return err
		}
		for _, version := range versions {
			if err := a.Delete(c.Request.Context(), &PackageIdentity{
				Name:    name,
				Version: version,
				Type:    NpmType,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	return nil
}
