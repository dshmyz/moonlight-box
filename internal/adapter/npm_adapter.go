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

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type NpmAdapter struct {
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	auditSvc    *service.AuditService
	proxyRouter *proxy.ProxyRouter
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

func NewNpmAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	proxyRouter *proxy.ProxyRouter,
) *NpmAdapter {
	return &NpmAdapter{
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
		proxyRouter: proxyRouter,
	}
}

func (a *NpmAdapter) Type() PackageType   { return NpmType }
func (a *NpmAdapter) RoutePrefix() string { return "/npm" }

func (a *NpmAdapter) SetProxyRouter(pr *proxy.ProxyRouter) {
	a.proxyRouter = pr
}

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

	if strings.Contains(fullPath, "/-/tarball/") {
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
				if syncErr := a.syncFromProxy(c.Request.Context(), name); syncErr == nil {
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
	tarballPart := strings.SplitN(fullPath, "/-/tarball/", 2)
	if len(tarballPart) != 2 {
		response.NotFound(c, "tarball not found")
		return
	}

	scope := ""
	filename := tarballPart[1]

	name, version := parseTarballFilename(filename, scope)

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "npm", name, version)
	if err == nil {
		defer content.Close()
		contentType := a.storageSvc.GetContentType(filename)
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.proxyRouter == nil {
		response.NotFound(c, "tarball not found")
		return
	}

	urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
		return fmt.Sprintf("%s/%s/-/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pkgName, filename)
	}

	result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "npm", name, version, urlBuilder)
	if resolveErr != nil {
		response.NotFound(c, "tarball not found")
		return
	}
	defer result.Content.Close()

	storeErr := a.storeProxyContent(c.Request.Context(), result, "npm", name, version)
	if storeErr != nil {
		contentType := a.storageSvc.GetContentType(filename)
		c.DataFromReader(200, result.Size, contentType, result.Content, nil)
		return
	}

	localContent, localSize, localErr := a.storageSvc.GetPackage(c.Request.Context(), "npm", name, version)
	if localErr != nil {
		contentType := a.storageSvc.GetContentType(filename)
		c.DataFromReader(200, result.Size, contentType, result.Content, nil)
		return
	}
	defer localContent.Close()

	contentType := a.storageSvc.GetContentType(filename)
	c.DataFromReader(200, localSize, contentType, localContent, nil)
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

	storageKey, err := a.storageSvc.StorePackage(ctx, "npm", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeNPM,
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
		a.storageSvc.DeletePackage(ctx, "npm", name, version)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *NpmAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "npm", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/octet-stream",
		Size:        size,
	}, nil
}

func (a *NpmAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        NpmType,
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

func (a *NpmAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *NpmAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeNPM)
}

func (a *NpmAdapter) syncFromProxy(ctx context.Context, name string) error {
	urlBuilder := func(repo *model.Repository, pkgName, _ string) string {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pkgName)
	}

	result, err := a.proxyRouter.ResolveProxyOnly(ctx, "npm", name, "", urlBuilder)
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

		_, _, err := a.pkgRepo.CreateOrUpdate(ctx, pkg, &model.PackageVersion{
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

func (a *NpmAdapter) storeProxyContent(ctx context.Context, result *proxy.RouteResult, pkgType, name, version string) error {
	body, err := io.ReadAll(result.Content)
	if err != nil {
		return err
	}
	_, storeErr := a.storageSvc.StorePackage(ctx, pkgType, name, version, bytes.NewReader(body), result.Size)
	return storeErr
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

func parseTarballFilename(filename, scope string) (name, version string) {
	basename := filepath.Base(filename)
	basename = strings.TrimSuffix(basename, ".tgz")

	parts := strings.Split(basename, "-")
	if len(parts) >= 2 {
		version = parts[len(parts)-1]
		name = strings.Join(parts[:len(parts)-1], "-")
	}

	if scope != "" {
		name = scope + "/" + name
	}
	return
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
