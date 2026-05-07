package adapter

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type NuGetAdapter struct {
	*BaseAdapter
	pkgRepo    *repository.PackageRepository
	storageSvc *service.StorageService
	auditSvc   *service.AuditService
}

func NewNuGetAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
) *NuGetAdapter {
	return &NuGetAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
	}
}

func (a *NuGetAdapter) Type() PackageType   { return NuGetType }
func (a *NuGetAdapter) RoutePrefix() string { return "/nuget" }

func (a *NuGetAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/v3/index.json", a.ServiceIndex)
		r.GET("/v3-flatcontainer/:id/:version/:filename", a.DownloadNupkg)
		r.GET("/v3/registration/:id/index.json", a.RegistrationIndex)
		r.GET("/v3/registration/:id/:version/index.json", a.RegistrationEntry)
		r.HEAD("/v3-flatcontainer/:id/:version/:filename", a.HeadNupkg)
		r.PUT("/api/v2/package", authMw, permMw("nuget", "write"), a.PushPackage)

		// OData V2 查询接口
		r.GET("/api/v2/Packages", a.ODataQueryPackages)
		r.GET("/api/v2/Packages()", a.ODataQueryPackages)
		r.GET("/api/v2/Packages(Id=':id',Version=':version')", a.ODataGetPackage)
		r.GET("/api/v2/Search()/$count", a.ODataSearchCount)
		r.GET("/api/v2/Search()", a.ODataSearch)
	}
}

func (a *NuGetAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid nuget path")
	}
	return &PackageIdentity{
		Name:    strings.ToLower(parts[0]),
		Version: parts[1],
		Type:    NuGetType,
	}, nil
}

func (a *NuGetAdapter) ServiceIndex(c *gin.Context) {
	baseURL := c.Request.URL.Scheme + "://" + c.Request.Host + "/nuget"

	c.JSON(200, gin.H{
		"version": "3.0.0",
		"resources": []gin.H{
			{
				"@id":   fmt.Sprintf("%s/v3/registration/", baseURL),
				"@type": "RegistrationsBaseUrl",
			},
			{
				"@id":   fmt.Sprintf("%s/v3-flatcontainer/", baseURL),
				"@type": "PackageBaseAddress/3.0.0",
			},
			{
				"@id":   fmt.Sprintf("%s/api/v2/package", baseURL),
				"@type": "PackagePublish/2.0.0",
			},
		},
	})
}

func (a *NuGetAdapter) DownloadNupkg(c *gin.Context) {
	id := strings.ToLower(c.Param("id"))
	version := c.Param("version")

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeNuGet, id, version, id+"."+version+".nupkg")
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "nuget", id, version)
	if err != nil {
		response.NotFound(c, "package not found")
		return
	}
	defer content.Close()

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s.nupkg"`, id, version))
	c.DataFromReader(200, size, "application/zip", content, nil)
}

func (a *NuGetAdapter) HeadNupkg(c *gin.Context) {
	id := strings.ToLower(c.Param("id"))
	version := c.Param("version")

	exists, err := a.storageSvc.Exists(c.Request.Context(), "nuget", id, version)
	if err != nil || !exists {
		response.NotFound(c, "package not found")
		return
	}
	c.Status(200)
}

func (a *NuGetAdapter) RegistrationIndex(c *gin.Context) {
	id := strings.ToLower(c.Param("id"))

	pkg, err := a.pkgRepo.FindByNameAndType(id, model.PackageTypeNuGet)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	items := make([]gin.H, len(pkg.Versions))
	for i, ver := range pkg.Versions {
		items[i] = gin.H{
			"lower": ver.Version,
			"upper": ver.Version,
			"items": []gin.H{
				{
					"catalogEntry": gin.H{
						"version":   ver.Version,
						"published": ver.PublishedAt.Format(time.RFC3339),
					},
				},
			},
		}
	}

	c.JSON(200, gin.H{
		"@id":   fmt.Sprintf("/nuget/v3/registration/%s/index.json", id),
		"@type": "catalog:CatalogRoot",
		"items": items,
	})
}

func (a *NuGetAdapter) RegistrationEntry(c *gin.Context) {
	id := strings.ToLower(c.Param("id"))
	version := c.Param("version")

	pkg, err := a.pkgRepo.FindByNameAndType(id, model.PackageTypeNuGet)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	for _, ver := range pkg.Versions {
		if ver.Version == version {
			c.JSON(200, gin.H{
				"@id":   fmt.Sprintf("/nuget/v3/registration/%s/%s/index.json", id, version),
				"@type": "Package",
				"catalogEntry": gin.H{
					"version":   version,
					"published": ver.PublishedAt.Format(time.RFC3339),
				},
			})
			return
		}
	}

	response.NotFound(c, "version not found")
}

func (a *NuGetAdapter) PushPackage(c *gin.Context) {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("package")
	if err != nil {
		reader := c.Request.Body
		defer reader.Close()
		req := &UploadRequest{
			Package:  reader,
			Filename: "package.nupkg",
			Size:     c.Request.ContentLength,
			Metadata: map[string]interface{}{
				"uploaded_by": userID,
			},
			UploadedBy: userID,
		}

		result, uploadErr := a.Upload(c.Request.Context(), req)
		if uploadErr != nil {
			response.InternalError(c, uploadErr.Error())
			return
		}

		c.JSON(201, gin.H{"success": true, "result": result})
		return
	}
	defer file.Close()

	pkgName := strings.TrimSuffix(filepath.Base(header.Filename), ".nupkg")
	parts := strings.Split(pkgName, ".")
	if len(parts) > 1 {
		pkgName = strings.Join(parts[:len(parts)-1], ".")
	}

	req := &UploadRequest{
		Package:  file,
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"name": pkgName,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(201, gin.H{"success": true, "result": result})
}

func (a *NuGetAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["name"].(string)
	version, _ := req.Metadata["version"].(string)
	if name == "" {
		name = fmt.Sprintf("unknown-%d", time.Now().UnixNano())
	}
	if version == "" {
		version = "1.0.0"
	}

	filename := fmt.Sprintf("%s.%s.nupkg", name, version)
	storageVersion := filepath.Join(version, filename)
	storageKey, err := a.storageSvc.StorePackage(ctx, "nuget", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, ver, _, err := a.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeNuGet,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
		PublishedBy: req.UploadedBy,
		Metadata:    marshalMetadata(req.Metadata),
	}, &model.PackageFile{
		Filename:    filename,
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   req.Size,
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "nuget", name, storageVersion)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		VersionID:  ver.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *NuGetAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	filename := fmt.Sprintf("%s.%s.nupkg", identity.Name, identity.Version)
	storageVersion := filepath.Join(identity.Version, filename)
	reader, size, err := a.storageSvc.GetPackage(ctx, "nuget", identity.Name, storageVersion)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/zip",
		Size:        size,
	}, nil
}

func (a *NuGetAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeNuGet, NuGetType)
}

func (a *NuGetAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeNuGet)
}

func (a *NuGetAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeNuGet)
}
