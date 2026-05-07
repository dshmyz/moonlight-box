package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type NpmAdapter struct {
	*BaseAdapter
	proxyDownloadSvc *service.ProxyDownloadService
	uploadSvc        *service.UploadService
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

type NpmSearchRequest struct {
	Text string `form:"text" binding:"required"`
	Size int    `form:"size" binding:"omitempty,min=1,max=100"`
	From int    `form:"from" binding:"omitempty,min=0"`
}

type NpmSearchResponse struct {
	Objects []NpmSearchObject `json:"objects"`
	Total   int               `json:"total"`
	Time    string            `json:"time"`
}

type NpmSearchObject struct {
	Package NpmSearchPackage `json:"package"`
	Score   NpmSearchScore   `json:"score"`
}

type NpmSearchPackage struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Date        string `json:"date"`
}

type NpmSearchScore struct {
	Detail NpmSearchScoreDetail `json:"detail"`
	Final  float64              `json:"final"`
}

type NpmSearchScoreDetail struct {
	Quality     float64 `json:"quality"`
	Popularity  float64 `json:"popularity"`
	Maintenance float64 `json:"maintenance"`
}

func NewNpmAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	proxyRouter *proxy.ProxyRouter,
	logRepo *repository.ProxyDownloadLogRepository,
	proxyDownloadSvc *service.ProxyDownloadService,
) *NpmAdapter {
	adapter := &NpmAdapter{
		BaseAdapter:      NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
		proxyDownloadSvc: proxyDownloadSvc,
		uploadSvc:        service.NewUploadService(pkgRepo, storageSvc),
	}
	adapter.SetProxyRouter(proxyRouter)
	adapter.SetLogRepo(logRepo)
	return adapter
}

func (a *NpmAdapter) Type() PackageType   { return NpmType }
func (a *NpmAdapter) RoutePrefix() string { return "/npm" }

func (a *NpmAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/*path", a.HandleNpmPath)

		publish := r.Group("")
		publish.Use(authMw, permMw("npm", "write"))
		{
			publish.PUT("/*revision", a.Publish)
		}

		unpublish := r.Group("")
		unpublish.Use(authMw, permMw("npm", "delete"))
		{
			unpublish.DELETE("/*revision", a.Unpublish)
		}
	}
}

func (a *NpmAdapter) HandleRepoRequest(c *gin.Context, repo *model.Repository, path string) {
	c.Set("repo", repo)
	if strings.Contains(path, "/-/") && strings.HasSuffix(path, ".tgz") {
		a.downloadTarballForRepo(c, repo, path)
		return
	}

	a.getPackageForRepo(c, repo, path)
}

func (a *NpmAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) {
	userID := c.GetUint("userID")

	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.SplitN(fullPath, "/", 2)
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
		response.BadRequest(c, "missing attachment", err.Error())
		return
	}
	defer file.Close()

	metadataRaw := c.PostForm("_attachment")
	var metadata NpmVersionInfo
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		response.BadRequest(c, "invalid metadata", err.Error())
		return
	}

	allowOverwrite := c.GetBool("allowOverwrite")
	if !allowOverwrite {
		existingPkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
		if err == nil {
			for _, ver := range existingPkg.Versions {
				if ver.Version == metadata.Version {
					response.Conflict(c, fmt.Sprintf("版本 %s 已存在，不允许覆盖", metadata.Version))
					return
				}
			}
		}
	}

	req := &UploadRequest{
		Package:  file,
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"npm":         metadata,
			"name":        name,
			"version":     metadata.Version,
			"description": metadata.Description,
			"repo_name":   repo.Name,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	metrics.RecordUpload("npm", name, metadata.Version)

	a.TriggerWebhook(model.WebhookEventPackageUploaded, name, metadata.Version, repo.Name, map[string]interface{}{
		"size":        req.Size,
		"description": metadata.Description,
	})

	c.JSON(201, gin.H{
		"ok":      true,
		"id":      name,
		"rev":     "1-" + generateRevision(),
		"success": true,
		"result":  result,
	})
}

func (a *NpmAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.SplitN(fullPath, "/", 2)
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

	identity, err := a.ParsePackagePath(name)
	if err != nil {
		response.BadRequest(c, "invalid package path", err.Error())
		return
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	pkg, _ := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, repo.Name, name, identity.Version, pkgID)

	a.TriggerWebhook(model.WebhookEventPackageDeleted, name, identity.Version, repo.Name, nil)

	c.JSON(200, gin.H{"ok": true})
}

func (a *NpmAdapter) getPackageForRepo(c *gin.Context, repo *model.Repository, fullPath string) {
	parts := strings.SplitN(fullPath, "/", 2)
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

	switch repo.Type {
	case model.RepoTypeLocal:
		meta, err := a.GetMetadata(c.Request.Context(), name)
		if err != nil {
			if util.IsErr(err, util.ErrPackageNotFound) {
				response.NotFound(c, "package not found")
				return
			}
			response.InternalError(c, err.Error())
			return
		}
		c.JSON(200, meta)

	case model.RepoTypeProxy:
		a.handleProxyMetadata(c, repo, name)

	case model.RepoTypeVirtual:
		a.handleVirtualMetadata(c, repo, name)

	default:
		response.NotFound(c, "unknown repository type")
	}
}

func (a *NpmAdapter) handleProxyMetadata(c *gin.Context, repo *model.Repository, name string) {
	urlBuilder := func(r *model.Repository, pkgName, _ string) string {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(r.RemoteURL, "/"), pkgName)
	}

	if !a.GetMetadataFromProxy(c, repo, name, urlBuilder) {
		response.NotFound(c, "package not found")
	}
}

func (a *NpmAdapter) handleVirtualMetadata(c *gin.Context, repo *model.Repository, name string) {
	urlBuilder := func(r *model.Repository, pkgName, _ string) string {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(r.RemoteURL, "/"), pkgName)
	}

	if a.proxyRouter == nil {
		response.NotFound(c, "package not found")
		return
	}

	result, err := a.proxyRouter.ResolveForVirtualRepo(c.Request.Context(), repo, "npm", name, "", urlBuilder)
	if err != nil {
		response.NotFound(c, "package not found")
		return
	}
	defer result.Content.Close()

	c.DataFromReader(200, result.Size, "application/json", result.Content, nil)
}

func (a *NpmAdapter) downloadTarballForRepo(c *gin.Context, repo *model.Repository, fullPath string) {
	parts := strings.SplitN(fullPath, "/-/", 2)
	if len(parts) != 2 {
		response.NotFound(c, "tarball not found")
		return
	}

	pkgName := parts[0]
	filename := parts[1]

	filenameWithoutExt := strings.TrimSuffix(filename, ".tgz")
	version := strings.TrimPrefix(filenameWithoutExt, pkgName+"-")

	if version == filenameWithoutExt {
		scope := ""
		if strings.HasPrefix(pkgName, "@") {
			scopeParts := strings.SplitN(pkgName, "/", 2)
			if len(scopeParts) == 2 {
				scope = scopeParts[0]
				pkgName = scopeParts[1]
				version = strings.TrimPrefix(filenameWithoutExt, pkgName+"-")
			}
		}
		if scope != "" {
			pkgName = scope + "/" + pkgName
		}
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeNPM, pkgName, version, filename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	switch repo.Type {
	case model.RepoTypeLocal:
		content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "npm", pkgName, version)
		if err != nil {
			response.NotFound(c, "tarball not found")
			return
		}
		defer content.Close()

		contentType := a.storageSvc.GetContentType(filename)
		c.DataFromReader(200, size, contentType, content, nil)

	case model.RepoTypeProxy:
		a.downloadFromProxy(c, repo, pkgName, version, filename)

	case model.RepoTypeVirtual:
		a.downloadFromVirtual(c, repo, pkgName, version, filename)

	default:
		response.NotFound(c, "unknown repository type")
	}
}

func (a *NpmAdapter) downloadFromProxy(c *gin.Context, repo *model.Repository, name, version, filename string) {
	urlBuilder := func(r *model.Repository, pkgName, pkgVersion string) string {
		return fmt.Sprintf("%s/%s/-/%s", strings.TrimSuffix(r.RemoteURL, "/"), pkgName, filename)
	}

	result, err := a.proxyDownloadSvc.Download(c.Request.Context(), &service.ProxyDownloadRequest{
		PkgType:        "npm",
		Name:           name,
		Version:        version,
		Filename:       filename,
		Repo:           repo,
		URLBuilder:     urlBuilder,
		PackageType:    model.PackageTypeNPM,
		RepositoryType: repo.Type,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeSmart,
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		UserID:         getUintPtr(c.GetUint("userID")),
	})

	if err != nil {
		response.NotFound(c, "tarball not found")
		return
	}

	contentType := a.storageSvc.GetContentType(filename)
	c.Data(200, contentType, result.Content)
}

func (a *NpmAdapter) downloadFromVirtual(c *gin.Context, repo *model.Repository, name, version, filename string) {
	urlBuilder := func(r *model.Repository, pkgName, pkgVersion string) string {
		return fmt.Sprintf("%s/%s/-/%s", strings.TrimSuffix(r.RemoteURL, "/"), pkgName, filename)
	}

	result, err := a.proxyDownloadSvc.Download(c.Request.Context(), &service.ProxyDownloadRequest{
		PkgType:        "npm",
		Name:           name,
		Version:        version,
		Filename:       filename,
		Repo:           repo,
		URLBuilder:     urlBuilder,
		PackageType:    model.PackageTypeNPM,
		RepositoryType: repo.Type,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeVirtualRepo,
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		UserID:         getUintPtr(c.GetUint("userID")),
	})

	if err != nil {
		response.NotFound(c, "tarball not found")
		return
	}

	contentType := a.storageSvc.GetContentType(filename)
	c.Data(200, contentType, result.Content)
}

func (a *NpmAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
		name := parts[0] + "/" + parts[1]
		version := ""
		if len(parts) >= 3 {
			version = parts[2]
		}
		return &PackageIdentity{Name: name, Version: version, Type: NpmType}, nil
	}

	name := parts[0]
	version := ""
	if len(parts) >= 2 {
		version = parts[1]
	}
	return &PackageIdentity{Name: name, Version: version, Type: NpmType}, nil
}

func (a *NpmAdapter) HandleNpmPath(c *gin.Context) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")

	if fullPath == "-/v1/search" {
		a.HandleSearch(c)
		return
	}

	if strings.Contains(fullPath, "/-/") {
		a.DownloadTarballPath(c, fullPath)
		return
	}

	a.GetPackageByPath(c, fullPath)
}

func (a *NpmAdapter) GetPackageByPath(c *gin.Context, fullPath string) {
	parts := strings.SplitN(fullPath, "/", 2)
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

	meta, err := a.GetMetadata(c.Request.Context(), name)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.proxyRouter != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}
				if syncErr := a.syncFromProxy(c.Request.Context(), name, repo); syncErr == nil {
					meta, err = a.GetMetadata(c.Request.Context(), name)
					if err == nil {
						c.JSON(200, meta)
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

	c.JSON(200, meta)
}

func (a *NpmAdapter) DownloadTarballPath(c *gin.Context, fullPath string) {
	parts := strings.SplitN(fullPath, "/-/", 2)
	if len(parts) != 2 {
		response.NotFound(c, "tarball not found")
		return
	}

	pkgName := parts[0]
	filename := parts[1]

	filenameWithoutExt := strings.TrimSuffix(filename, ".tgz")
	version := strings.TrimPrefix(filenameWithoutExt, pkgName+"-")

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeNPM, pkgName, version, filename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	urlBuilder := func(r *model.Repository, name, ver string) string {
		return fmt.Sprintf("%s/%s/-/%s", strings.TrimSuffix(r.RemoteURL, "/"), name, filename)
	}

	result, err := a.proxyDownloadSvc.Download(c.Request.Context(), &service.ProxyDownloadRequest{
		PkgType:        "npm",
		Name:           pkgName,
		Version:        version,
		Filename:       filename,
		Repo:           repo,
		URLBuilder:     urlBuilder,
		PackageType:    model.PackageTypeNPM,
		RepositoryType: model.RepoTypeProxy,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeSmart,
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		UserID:         getUintPtr(c.GetUint("userID")),
	})

	if err != nil {
		response.NotFound(c, "tarball not found")
		return
	}

	contentType := a.storageSvc.GetContentType(filename)
	c.Data(200, contentType, result.Content)
}

func (a *NpmAdapter) Publish(c *gin.Context) {
	userID := c.GetUint("userID")

	scope := c.Param("scope")
	pkgName := c.Param("package")

	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	file, header, err := c.Request.FormFile("_attachments")
	if err != nil {
		response.BadRequest(c, "missing attachment", err.Error())
		return
	}
	defer file.Close()

	metadataRaw := c.PostForm("_attachment")
	var metadata NpmVersionInfo
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		response.BadRequest(c, "invalid metadata", err.Error())
		return
	}

	req := &UploadRequest{
		Package:  file,
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"npm":         metadata,
			"name":        name,
			"version":     metadata.Version,
			"description": metadata.Description,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(201, gin.H{
		"ok":      true,
		"id":      name,
		"rev":     "1-" + generateRevision(),
		"success": true,
		"result":  result,
	})
}

func (a *NpmAdapter) Unpublish(c *gin.Context) {
	scope := c.Param("scope")
	pkgName := c.Param("package")

	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	identity, err := a.ParsePackagePath(name)
	if err != nil {
		response.BadRequest(c, "invalid package path", err.Error())
		return
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{"ok": true})
}

func (a *NpmAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["name"].(string)
	version, _ := req.Metadata["version"].(string)
	if name == "" || version == "" {
		return nil, fmt.Errorf("missing name or version in metadata")
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	result, err := a.uploadSvc.Upload(ctx, &service.UploadContext{
		PkgType:        "npm",
		Name:           name,
		Version:        version,
		StorageVersion: filepath.Join(version, "package.tgz"),
		Filename:       "package.tgz",
		Content:        content,
		Size:           req.Size,
		PackageType:    model.PackageTypeNPM,
		RepositoryType: model.RepoTypeLocal,
		UploadedBy:     req.UploadedBy,
		Metadata:       req.Metadata,
		FileType:       model.FileTypePrimary,
	})

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

func (a *NpmAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	storageVersion := filepath.Join(identity.Version, "package.tgz")
	reader, size, err := a.storageSvc.GetPackage(ctx, "npm", identity.Name, storageVersion)
	if err != nil {
		return nil, err
	}

	metrics.RecordDownload("npm", identity.Name, identity.Version)

	return &PackageContent{
		Content:     reader,
		ContentType: "application/octet-stream",
		Size:        size,
	}, nil
}

func (a *NpmAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeNPM, NpmType)
}

func (a *NpmAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeNPM)
}

func (a *NpmAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeNPM)
}

func (a *NpmAdapter) syncFromProxy(ctx context.Context, name string, repo *model.Repository) error {
	urlBuilder := func(repo *model.Repository, pkgName, _ string) string {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pkgName)
	}

	result, err := a.proxyRouter.ResolveSmart(ctx, repo, "npm", name, "", urlBuilder)
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

	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
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

		_, _, err := a.pkgRepo.CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
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

func (a *NpmAdapter) storeProxyContent(ctx context.Context, result *proxy.RouteResult, pkgType, name, version string, filename string) error {
	body, err := io.ReadAll(result.Content)
	if err != nil {
		return err
	}

	storageKey, storeErr := a.storageSvc.StorePackage(ctx, "npm", name, version, bytes.NewReader(body), result.Size)
	if storeErr != nil {
		return storeErr
	}

	_, _, _, dbErr := a.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeNPM,
		RepositoryID:   result.RepoID,
		RepositoryType: model.RepoTypeProxy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
	}, &model.PackageFile{
		Filename:    filename,
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   result.Size,
	})

	if dbErr != nil {
		return dbErr
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

func getDescription(meta map[string]interface{}) string {
	if desc, ok := meta["description"]; ok {
		if s, ok := desc.(string); ok {
			return s
		}
	}
	return ""
}

func marshalMetadata(meta map[string]interface{}) string {
	data, _ := json.Marshal(meta)
	return string(data)
}

// SyncMetadata 实现 MetadataSyncer 接口，同步 NPM 仓库元数据
func (a *NpmAdapter) SyncMetadata(ctx context.Context, repo interface{}) (*types.SyncResult, error) {
	result := &types.SyncResult{}

	// 类型断言，获取 Repository 对象
	r, ok := repo.(*model.Repository)
	if !ok {
		return nil, fmt.Errorf("invalid repository type")
	}

	// 构建请求 URL
	url := fmt.Sprintf("%s/-/all", strings.TrimSuffix(r.RemoteURL, "/"))

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 设置认证头
	if r.AuthType != "none" {
		authConfig, _ := r.GetAuthConfig()
		a.setAuthHeader(req, authConfig)
	}

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch metadata: status code %d", resp.StatusCode)
	}

	// 解析响应
	var allPackages map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&allPackages); err != nil {
		return nil, err
	}

	// 处理每个包
	for pkgName, rawMeta := range allPackages {
		// 跳过以 _ 开头的特殊字段
		if strings.HasPrefix(pkgName, "_") {
			continue
		}

		result.Total++

		// 解析包元数据
		var meta NpmPackageMetadata
		if err := json.Unmarshal(rawMeta, &meta); err != nil {
			result.Failed++
			continue
		}

		// 创建或更新包记录
		now := time.Now()
		pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
			Name:           pkgName,
			Type:           model.PackageTypeNPM,
			RepositoryID:   r.ID,
			RepositoryType: model.RepoTypeProxy,
			Description:    meta.Description,
			MetadataSynced: true,
			MetadataSyncAt: &now,
		}, nil)

		if err != nil {
			result.Failed++
			continue
		}

		// 创建或更新版本记录
		for version, verInfo := range meta.Versions {
			publishedAt := parseTime(meta.Time[version])
			versionMeta := marshalMetadata(map[string]interface{}{
				"version":     version,
				"publishedAt": publishedAt.Format(time.RFC3339),
				"tarball":     verInfo.Dist.Tarball,
				"shasum":      verInfo.Dist.Shasum,
			})

			a.pkgRepo.CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
				Version:     version,
				Status:      model.StatusPublished,
				PublishedAt: publishedAt,
				Metadata:    versionMeta,
			})
		}

		result.Synced++
	}

	return result, nil
}

// setAuthHeader 设置 HTTP 请求的认证头
func (a *NpmAdapter) setAuthHeader(req *http.Request, cfg *model.ProxyAuthConfig) {
	switch cfg.Type {
	case "basic":
		if cfg.Basic != nil {
			req.SetBasicAuth(cfg.Basic.Username, cfg.Basic.Password)
		}
	case "bearer":
		if cfg.Bearer != nil {
			req.Header.Set("Authorization", "Bearer "+cfg.Bearer.Token)
		}
	case "api_key":
		if cfg.APIKey != nil {
			if cfg.APIKey.HeaderName != "" {
				req.Header.Set(cfg.APIKey.HeaderName, cfg.APIKey.KeyValue)
			}
		}
	}
}

func (a *NpmAdapter) searchPackages(ctx context.Context, query string, size, from int) ([]model.Package, int, error) {
	var packages []model.Package
	var total int64

	searchTerm := "%" + query + "%"
	db := a.pkgRepo.DB().Model(&model.Package{}).
		Where("type = ?", model.PackageTypeNPM).
		Where("name LIKE ? OR description LIKE ?", searchTerm, searchTerm)

	db.Count(&total)

	err := db.Preload("Versions").
		Order("updated_at DESC").
		Offset(from).
		Limit(size).
		Find(&packages).Error

	return packages, int(total), err
}

func (a *NpmAdapter) formatSearchResponse(packages []model.Package, total int) *NpmSearchResponse {
	objects := make([]NpmSearchObject, 0, len(packages))

	for _, pkg := range packages {
		var latestVersion string
		var updatedAt string
		if len(pkg.Versions) > 0 {
			latestVersion = pkg.Versions[0].Version
			updatedAt = pkg.Versions[0].PublishedAt.Format(time.RFC3339)
		}

		objects = append(objects, NpmSearchObject{
			Package: NpmSearchPackage{
				Name:        pkg.Name,
				Version:     latestVersion,
				Description: pkg.Description,
				Date:        updatedAt,
			},
			Score: NpmSearchScore{
				Detail: NpmSearchScoreDetail{
					Quality:     1.0,
					Popularity:  1.0,
					Maintenance: 1.0,
				},
				Final: 1.0,
			},
		})
	}

	return &NpmSearchResponse{
		Objects: objects,
		Total:   total,
		Time:    time.Now().Format("Mon Jan 02 2006 15:04:05 GMT-0700"),
	}
}

func (a *NpmAdapter) HandleSearch(c *gin.Context) {
	var req NpmSearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid search parameters", err.Error())
		return
	}

	if req.Size == 0 {
		req.Size = 20
	}

	packages, total, err := a.searchPackages(c.Request.Context(), req.Text, req.Size, req.From)
	if err != nil {
		response.InternalError(c, "search failed")
		return
	}

	resp := a.formatSearchResponse(packages, total)
	c.JSON(200, resp)
}
