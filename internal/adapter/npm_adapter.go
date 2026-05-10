package adapter

import (
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
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NpmAdapter struct {
	*BaseAdapter
	uploadSvc *service.UploadService
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

func NewNpmAdapter(pkgRepo *repository.PackageRepository, repoRepo *repository.RepositoryRepository, storageSvc *service.StorageService, auditSvc *service.AuditService, pkgCache *cache.PackageCache) *NpmAdapter {
	return &NpmAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache),
		uploadSvc:   service.NewUploadService(pkgRepo, storageSvc),
	}
}

func (a *NpmAdapter) Type() PackageType   { return NpmType }
func (a *NpmAdapter) RoutePrefix() string { return "/npm" }

func (a *NpmAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	a.getPackageForRepo(c, ctx.Repo, ctx.Path)
}

func (a *NpmAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
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
		return nil
	}
	defer file.Close()

	metadataRaw := c.PostForm("_attachment")
	var metadata NpmVersionInfo
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		response.BadRequest(c, "invalid metadata", err.Error())
		return nil
	}

	allowOverwrite := c.GetBool("allowOverwrite")
	if !allowOverwrite {
		existingPkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
		if err == nil {
			for _, ver := range existingPkg.Versions {
				if ver.Version == metadata.Version {
					response.Conflict(c, fmt.Sprintf("版本 %s 已存在，不允许覆盖", metadata.Version))
					return nil
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

	_, err = a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return nil
	}

	result := &types.RepoOperationResult{
		PackageName: name,
		Version:     metadata.Version,
		Size:        header.Size,
		Filename:    header.Filename,
		ExtraData: map[string]interface{}{
			"size":        req.Size,
			"description": metadata.Description,
		},
	}

	result.Response = &types.NpmPublishResponse{
		PublishResponse: types.PublishResponse{
			Success:  true,
			Message:  "Package published successfully",
			Package:  name,
			Version:  metadata.Version,
			Filename: header.Filename,
			Size:     header.Size,
		},
		Description: metadata.Description,
	}

	return result
}

func (a *NpmAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
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
		return nil
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return nil
	}

	pkg, _ := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, repo.Name, name, identity.Version, pkgID)

	c.JSON(200, gin.H{"ok": true})

	return &types.RepoOperationResult{
		PackageName: name,
		Version:     identity.Version,
	}
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
	result, err := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "npm", name, "")
	if err != nil {
		response.NotFound(c, "package not found")
		return
	}
	defer result.Content.Close()

	c.DataFromReader(200, result.Size, "application/json", result.Content, nil)
}

func (a *NpmAdapter) handleVirtualMetadata(c *gin.Context, repo *model.Repository, name string) {
	if a.fetcher == nil {
		response.NotFound(c, "package not found")
		return
	}

	result, err := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "npm", name, "")
	if err != nil {
		response.NotFound(c, "package not found")
		return
	}
	defer result.Content.Close()

	c.DataFromReader(200, result.Size, "application/json", result.Content, nil)
}

func (a *NpmAdapter) BuildRemotePath(name, version, filename string) string {
	if version != "" {
		return fmt.Sprintf("%s/-/%s-%s.tgz", name, name, version)
	}
	return name
}

func (a *NpmAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// 处理 tarball 路径：@scope/package/-/package-1.0.0.tgz 或 package/-/package-1.0.0.tgz
	if strings.Contains(path, "/-/") {
		// 找到 "-" 的位置
		dashIdx := -1
		for i, part := range parts {
			if part == "-" {
				dashIdx = i
				break
			}
		}

		if dashIdx > 0 && dashIdx < len(parts)-1 {
			// 提取包名
			name := strings.Join(parts[:dashIdx], "/")

			// 从文件名提取版本：package-1.0.0.tgz → 1.0.0
			filename := parts[len(parts)-1]
			version := extractVersionFromTarball(filename, name)

			return &PackageIdentity{
				Name:    name,
				Version: version,
				Type:    NpmType,
			}, nil
		}
	}

	// 处理 metadata 路径：@scope/package 或 package
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

func (a *NpmAdapter) HandleNpmPath(c *gin.Context) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")

	if fullPath == "-/v1/search" {
		a.HandleSearch(c)
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
			if a.fetcher != nil {
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

	result, err := a.uploadSvc.Upload(ctx, &service.UploadContext{
		PkgType:        "npm",
		Name:           name,
		Version:        version,
		StorageVersion: filepath.Join(version, "package.tgz"),
		Filename:       "package.tgz",
		Content:        reader,
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
	result, err := a.fetcher.FetchFromRemote(ctx, repo, "npm", name, "")
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

func (a *NpmAdapter) searchPackages(ctx context.Context, query string, size, from int) ([]model.Package, int, error) {
	var packages []model.Package
	var total int64

	searchTerm := "%" + query + "%"
	db := a.pkgRepo.DB().Model(&model.Package{}).
		Where("type = ?", model.PackageTypeNPM).
		Where("name LIKE ? OR description LIKE ?", searchTerm, searchTerm)

	db.Count(&total)

	err := db.Preload("Versions", func(db *gorm.DB) *gorm.DB {
		return db.Order("published_at DESC").Limit(1)
	}).
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

func (a *NpmAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := a.storageSvc.GetContentType(result.Filename)
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *NpmAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	name := ctx.Name
	version := ctx.Version
	filename := ctx.Filename

	if strings.Contains(filename, ".tgz") {
		storageName := name + "/" + filename
		content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "npm", storageName, version)
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
			result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), ctx.Repo, "npm", name, version+"/"+filename)
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
	}

	return nil, fmt.Errorf("package not found")
}

func (a *NpmAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	userID := ctx.UserID

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
		existingPkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
		if err == nil {
			for _, ver := range existingPkg.Versions {
				if ver.Version == metadata.Version {
					return nil, fmt.Errorf("版本 %s 已存在，不允许覆盖", metadata.Version)
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
			"repo_name":   ctx.Repo.Name,
		},
		UploadedBy: userID,
	}

	_, err = a.Upload(c.Request.Context(), req)
	if err != nil {
		return nil, err
	}

	return &types.PublishResult{
		PackageName: name,
		Version:     metadata.Version,
		Size:        header.Size,
		Filename:    header.Filename,
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
		return fmt.Errorf("invalid package path: %v", err)
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	pkg, _ := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, ctx.Repo.Name, name, identity.Version, pkgID)

	return nil
}
