package adapter

import (
	"bytes"
	"context"
	"encoding/xml"
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

	"github.com/gin-gonic/gin"
)

type MavenAdapter struct {
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	auditSvc    *service.AuditService
	proxyRouter *proxy.ProxyRouter
}

type MavenMetadata struct {
	XMLName    xml.Name        `xml:"metadata"`
	GroupID    string          `xml:"groupId"`
	ArtifactID string          `xml:"artifactId"`
	Versioning MavenVersioning `xml:"versioning"`
}

type MavenVersioning struct {
	Release     string        `xml:"release"`
	Latest      string        `xml:"latest"`
	Versions    MavenVersions `xml:"versions"`
	LastUpdated string        `xml:"lastUpdated"`
}

type MavenVersions struct {
	Version          []string `xml:"version"`
	SnapshotVersions []string `xml:"snapshotVersions>snapshotVersion"`
}

type MavenProject struct {
	XMLName      xml.Name          `xml:"project"`
	GroupID      string            `xml:"groupId"`
	ArtifactID   string            `xml:"artifactId"`
	Version      string            `xml:"version"`
	Packaging    string            `xml:"packaging"`
	Description  string            `xml:"description"`
	Dependencies []MavenDependency `xml:"dependencies>dependency"`
}

type MavenDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

func NewMavenAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	proxyRouter *proxy.ProxyRouter,
) *MavenAdapter {
	return &MavenAdapter{
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
		proxyRouter: proxyRouter,
	}
}

func (a *MavenAdapter) SetProxyRouter(pr *proxy.ProxyRouter) {
	a.proxyRouter = pr
}

func (a *MavenAdapter) Type() PackageType   { return MavenType }
func (a *MavenAdapter) RoutePrefix() string { return "/maven2" }

func (a *MavenAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/*path", a.handleRequest)

		upload := r.Group("")
		upload.Use(authMw, permMw("maven", "write"))
		{
			upload.PUT("/*path", a.UploadArtifact)
		}
	}
}

func (a *MavenAdapter) handleRequest(c *gin.Context) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")

	if strings.HasSuffix(fullPath, "maven-metadata.xml") {
		a.handleMetadataXML(c, fullPath)
		return
	}

	a.handleDownloadArtifact(c, fullPath)
}

func (a *MavenAdapter) handleMetadataXML(c *gin.Context, fullPath string) {
	group := strings.TrimSuffix(fullPath, "/maven-metadata.xml")
	parts := strings.Split(group, "/")
	if len(parts) < 2 {
		response.NotFound(c, "metadata not found")
		return
	}

	groupID := strings.Join(parts[:len(parts)-1], ".")
	artifactID := parts[len(parts)-1]
	name := groupID + "/" + artifactID

	versions, err := a.ListVersions(c.Request.Context(), name)
	if err != nil {
		if a.proxyRouter != nil {
			urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
				baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
				return fmt.Sprintf("%s/%s/maven-metadata.xml", baseURL, group)
			}
			result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "maven", name, "", urlBuilder)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					c.Data(200, "application/xml", body)
					return
				}
			}
		}
		response.NotFound(c, "metadata not found")
		return
	}

	var latest, release string
	for _, ver := range versions {
		if latest == "" || compareVersions(ver, latest) > 0 {
			latest = ver
		}
		if release == "" || isRelease(ver) {
			release = ver
		}
	}

	metadata := &MavenMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Versioning: MavenVersioning{
			Release:     release,
			Latest:      latest,
			Versions:    MavenVersions{Version: versions},
			LastUpdated: time.Now().Format("20060102150405"),
		},
	}

	c.XML(200, metadata)
}

func (a *MavenAdapter) handleDownloadArtifact(c *gin.Context, fullPath string) {
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "artifact not found")
		return
	}

	version := parts[len(parts)-2]
	filename := parts[len(parts)-1]
	groupArtifact := strings.Join(parts[:len(parts)-2], "/")

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, version)
	if err == nil {
		defer content.Close()
		contentType := a.storageSvc.GetContentType(filename)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.proxyRouter != nil {
		urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
			baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
			return fmt.Sprintf("%s/%s", baseURL, fullPath)
		}
		result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "maven", groupArtifact, version, urlBuilder)
		if resolveErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				a.storageSvc.StorePackage(c.Request.Context(), "maven", groupArtifact, version, bytes.NewReader(body), result.Size)
				localContent, localSize, localErr := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, version)
				if localErr == nil {
					defer localContent.Close()
					c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
					c.DataFromReader(200, localSize, a.storageSvc.GetContentType(filename), localContent, nil)
					return
				}
				c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
				c.Data(200, a.storageSvc.GetContentType(filename), body)
				return
			}
		}
	}

	response.NotFound(c, "artifact not found")
}

func (a *MavenAdapter) UploadArtifact(c *gin.Context) {
	userID := c.GetUint("userID")

	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.BadRequest(c, "invalid path", "expected group/artifact/version/file")
		return
	}

	groupID := strings.Join(parts[:len(parts)-3], "/")
	groupID = strings.ReplaceAll(groupID, "/", ".")
	artifactID := parts[len(parts)-3]
	version := parts[len(parts)-2]
	filename := parts[len(parts)-1]

	size := c.Request.ContentLength
	reader := c.Request.Body

	req := &UploadRequest{
		Package:  reader,
		Filename: filename,
		Size:     size,
		Metadata: map[string]interface{}{
			"groupId":    groupID,
			"artifactId": artifactID,
			"version":    version,
			"packaging":  getPackaging(filename),
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	_ = result
	c.Status(200)
}

func (a *MavenAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	groupID, _ := req.Metadata["groupId"].(string)
	artifactID, _ := req.Metadata["artifactId"].(string)
	version, _ := req.Metadata["version"].(string)

	name := groupID + "/" + artifactID

	storageKey, err := a.storageSvc.StorePackage(ctx, "maven", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeMaven,
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
		a.storageSvc.DeletePackage(ctx, "maven", name, version)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *MavenAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "maven", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/octet-stream",
		Size:        size,
	}, nil
}

func (a *MavenAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeMaven)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        name,
		Type:        MavenType,
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

func (a *MavenAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *MavenAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeMaven)
}

func (a *MavenAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid maven path: %s", path)
	}

	groupID := strings.ReplaceAll(strings.Join(parts[:len(parts)-2], "."), ".", "/")
	artifactID := parts[len(parts)-2]
	version := ""
	if len(parts) >= 3 {
		version = parts[len(parts)-1]
	}

	name := groupID + "/" + artifactID
	return &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    MavenType,
	}, nil
}

func getPackaging(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".jar":
		return "jar"
	case ".pom":
		return "pom"
	case "-sources.jar":
		return "jar"
	case "-javadoc.jar":
		return "jar"
	default:
		return "jar"
	}
}

func isRelease(version string) bool {
	return !strings.HasSuffix(version, "-SNAPSHOT")
}

func compareVersions(v1, v2 string) int {
	return strings.Compare(v1, v2)
}
