package adapter

import (
	"bytes"
	"context"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
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

type YumAdapter struct {
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	auditSvc    *service.AuditService
	proxyRouter *proxy.ProxyRouter
}

type RepoMD struct {
	XMLName  xml.Name     `xml:"repomd"`
	Xmlns    string       `xml:"xmlns,attr"`
	XmlnsRpm string       `xml:"xmlns:rpm,attr"`
	Revision string       `xml:"revision"`
	Data     []RepoMDData `xml:"data"`
}

type RepoMDData struct {
	Type       string        `xml:"type,attr"`
	Checksum   Checksum      `xml:"checksum"`
	OpenChecksum Checksum    `xml:"open-checksum"`
	Location   RepoMDLocation `xml:"location"`
	Timestamp  int64         `xml:"timestamp"`
	Size       int64         `xml:"size"`
	OpenSize   int64         `xml:"open-size"`
}

type Checksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type RepoMDLocation struct {
	Href string `xml:"href,attr"`
}

type PrimaryData struct {
	XMLName  xml.Name      `xml:"metadata"`
	Xmlns    string        `xml:"xmlns,attr"`
	XmlnsRpm string        `xml:"xmlns:rpm,attr"`
	Packages []YumPackage  `xml:"package"`
}

type YumPackage struct {
	Type        string          `xml:"type,attr"`
	Name        string          `xml:"name"`
	Arch        string          `xml:"arch"`
	Version     YumVersion      `xml:"version"`
	Checksum    YumChecksum     `xml:"checksum"`
	Summary     string          `xml:"summary"`
	Description string          `xml:"description"`
	Packager    string          `xml:"packager"`
	URL         string          `xml:"url"`
	Time        YumTime         `xml:"time"`
	Size        YumSize         `xml:"size"`
	Location    YumLocation     `xml:"location"`
	Format      YumFormat       `xml:"format"`
}

type YumVersion struct {
	Epoch   string `xml:"epoch,attr"`
	Ver     string `xml:"ver,attr"`
	Rel     string `xml:"rel,attr"`
}

type YumChecksum struct {
	Type  string `xml:"type,attr"`
	Hash  string `xml:",chardata"`
}

type YumTime struct {
	File  int64 `xml:"file,attr"`
	Build int64 `xml:"build,attr"`
}

type YumSize struct {
	Package   int64 `xml:"package,attr"`
	Installed int64 `xml:"installed,attr"`
	Archive   int64 `xml:"archive,attr"`
}

type YumLocation struct {
	Href string `xml:"href,attr"`
}

type YumFormat struct {
	License   string            `xml:"license"`
	Vendor    string            `xml:"vendor"`
	Group     string            `xml:"group"`
	Buildhost string            `xml:"buildhost"`
	Requires  []YumDependency   `xml:"requires>rpm:entry"`
	Provides  []YumDependency   `xml:"provides>rpm:entry"`
}

type YumDependency struct {
	Name    string `xml:"name,attr"`
	Flags   string `xml:"flags,attr"`
	Epoch   string `xml:"epoch,attr,omitempty"`
	Ver     string `xml:"ver,attr,omitempty"`
	Rel     string `xml:"rel,attr,omitempty"`
	Pre     bool   `xml:"pre,attr,omitempty"`
}

func NewYumAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	proxyRouter *proxy.ProxyRouter,
) *YumAdapter {
	return &YumAdapter{
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		auditSvc:    auditSvc,
		proxyRouter: proxyRouter,
	}
}

func (a *YumAdapter) SetProxyRouter(pr *proxy.ProxyRouter) {
	a.proxyRouter = pr
}

func (a *YumAdapter) Type() PackageType { return YumType }
func (a *YumAdapter) RoutePrefix() string { return "/yum" }

func (a *YumAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/:repo/repodata/*path", a.RepoDataFile)
		r.GET("/:repo/Packages/*path", a.DownloadRPM)

		upload := r.Group("")
		upload.Use(authMw, permMw("yum", "write"))
		{
			upload.POST("/:repo/upload", a.UploadRPM)
			upload.POST("/:repo/regenerate", a.RegenerateMetadata)
		}
	}
}

func (a *YumAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid yum path: %s", path)
	}

	name := parts[0]
	version := ""
	if len(parts) >= 2 {
		version = parts[1]
	}

	return &PackageIdentity{Name: name, Version: version, Type: YumType}, nil
}

func (a *YumAdapter) RepoMetadata(c *gin.Context) {
	repo := c.Param("repo")

	repomdXML, err := a.generateRepomdXML(c.Request.Context(), repo)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.Header("Content-Type", "application/xml")
	c.String(200, repomdXML)
}

func (a *YumAdapter) RepoDataFile(c *gin.Context) {
	repo := c.Param("repo")
	filePath := c.Param("path")
	filePath = strings.TrimPrefix(filePath, "/")

	storageKey := fmt.Sprintf("yum/%s/repodata/%s", repo, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(c.Request.Context(), storageKey)
		contentType := "application/xml"
		if strings.HasSuffix(filePath, ".gz") {
			contentType = "application/gzip"
		}
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.proxyRouter != nil {
		urlBuilder := func(r *model.Repository, pkgName, pkgVersion string) string {
			baseURL := strings.TrimSuffix(r.RemoteURL, "/")
			return fmt.Sprintf("%s/repodata/%s", baseURL, filePath)
		}
		result, resolveErr := a.proxyRouter.ResolveProxyOnly(c.Request.Context(), "yum", repo+"/repodata/"+filePath, "", urlBuilder)
		if resolveErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				backend.Put(c.Request.Context(), storageKey, bytes.NewReader(body), result.Size)
				contentType := "application/xml"
				if strings.HasSuffix(filePath, ".gz") {
					contentType = "application/gzip"
				}
				c.Data(200, contentType, body)
				return
			}
		}
	}

	response.NotFound(c, "metadata file not found")
}

func (a *YumAdapter) DownloadRPM(c *gin.Context) {
	repo := c.Param("repo")
	filePath := c.Param("path")
	filePath = strings.TrimPrefix(filePath, "/")

	storageKey := fmt.Sprintf("yum/%s/Packages/%s", repo, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err != nil {
		response.NotFound(c, "RPM not found")
		return
	}
	defer content.Close()

	size, err := backend.Size(c.Request.Context(), storageKey)
	if err != nil {
		response.NotFound(c, "RPM not found")
		return
	}

	filename := filepath.Base(filePath)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.DataFromReader(200, size, "application/x-rpm", content, nil)
}

func (a *YumAdapter) UploadRPM(c *gin.Context) {
	userID := c.GetUint("userID")
	repo := c.Param("repo")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return
	}
	defer file.Close()

	rpmData, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file", err.Error())
		return
	}

	rpmName := header.Filename
	if !strings.HasSuffix(rpmName, ".rpm") {
		response.BadRequest(c, "invalid file type", "file must be .rpm")
		return
	}

	arch := detectRpmArch(rpmName)
	packagesDir := fmt.Sprintf("yum/%s/Packages/%s", repo, arch)
	storageKey := fmt.Sprintf("%s/%s", packagesDir, rpmName)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(rpmData), header.Size); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	packageName := strings.TrimSuffix(rpmName, ".rpm")
	pkg, _, err := a.pkgRepo.CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeYum,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     "1",
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   header.Size,
		PublishedBy: userID,
		Metadata:    marshalMetadata(map[string]interface{}{"repo": repo, "arch": arch}),
	})
	if err != nil {
		backend.Delete(c.Request.Context(), storageKey)
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success":    true,
		"repo":       repo,
		"filename":   rpmName,
		"storageKey": storageKey,
		"result": &PackageVersionResult{
			PackageID:  pkg.ID,
			Version:    "1",
			StorageKey: storageKey,
			Size:       header.Size,
		},
	})
}

func (a *YumAdapter) RegenerateMetadata(c *gin.Context) {
	repo := c.Param("repo")

	if err := a.regenerateRepodata(c.Request.Context(), repo); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "metadata regenerated",
		"repo":    repo,
	})
}

func (a *YumAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["name"].(string)
	repo, _ := req.Metadata["repo"].(string)
	if name == "" || repo == "" {
		return nil, fmt.Errorf("missing name or repo in metadata")
	}

	arch := detectRpmArch(name)
	storageKey := fmt.Sprintf("yum/%s/Packages/%s/%s", repo, arch, name)

	_, err := a.storageSvc.StorePackage(ctx, "yum", repo+"/"+name, "1", reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeYum,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     "1",
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   req.Size,
		PublishedBy: req.UploadedBy,
		Metadata:    marshalMetadata(req.Metadata),
	})
	if err != nil {
		a.storageSvc.DeletePackage(ctx, "yum", repo+"/"+name, "1")
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    "1",
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *YumAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "yum", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/x-rpm",
		Size:        size,
	}, nil
}

func (a *YumAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeYum)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        YumType,
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

func (a *YumAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *YumAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeYum)
}

func (a *YumAdapter) generateRepomdXML(ctx context.Context, repo string) (string, error) {
	repomd := RepoMD{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		XmlnsRpm: "http://linux.duke.edu/metadata/rpm",
		Revision: fmt.Sprintf("%d", time.Now().Unix()),
	}

	backend := a.storageSvc.GetDefaultBackend()
	entries, err := backend.List(ctx, fmt.Sprintf("yum/%s/repodata/", repo))
	if err != nil {
		entries = nil
	}

	for _, entry := range entries {
		if strings.Contains(entry.Key, "primary.xml") {
			repomd.Data = append(repomd.Data, RepoMDData{
				Type: "primary",
				Checksum: Checksum{
					Type:  "sha256",
					Value: "placeholder",
				},
				Location: RepoMDLocation{
					Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
				},
			})
		} else if strings.Contains(entry.Key, "filelists.xml") {
			repomd.Data = append(repomd.Data, RepoMDData{
				Type: "filelists",
				Location: RepoMDLocation{
					Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
				},
			})
		} else if strings.Contains(entry.Key, "other.xml") {
			repomd.Data = append(repomd.Data, RepoMDData{
				Type: "other",
				Location: RepoMDLocation{
					Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
				},
			})
		}
	}

	output, err := xml.MarshalIndent(repomd, "", "  ")
	if err != nil {
		return "", err
	}

	return xml.Header + string(output), nil
}

func (a *YumAdapter) regenerateRepodata(ctx context.Context, repo string) error {
	packages, _, err := a.pkgRepo.List(1, 10000, string(model.PackageTypeYum), "")
	if err != nil {
		return err
	}

	var yumPkgs []YumPackage
	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			yumPkgs = append(yumPkgs, YumPackage{
				Type:    "rpm",
				Name:    pkg.Name,
				Arch:    "x86_64",
				Version: YumVersion{Ver: ver.Version},
				Size:    YumSize{Package: ver.SizeBytes},
				Location: YumLocation{
					Href: fmt.Sprintf("Packages/%s", filepath.Base(ver.StoragePath)),
				},
			})
		}
	}

	primaryData := PrimaryData{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		XmlnsRpm: "http://linux.duke.edu/metadata/rpm",
		Packages: yumPkgs,
	}

	primaryXML, err := xml.MarshalIndent(primaryData, "", "  ")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(primaryXML); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	checksum := sha256.Sum256(buf.Bytes())
	primaryFilename := fmt.Sprintf("%s-primary.xml.gz", hex.EncodeToString(checksum[:8]))

	backend := a.storageSvc.GetDefaultBackend()
	repodataDir := fmt.Sprintf("yum/%s/repodata/", repo)
	if err := backend.Put(ctx, repodataDir+primaryFilename, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		return err
	}

	return nil
}

func detectRpmArch(filename string) string {
	if strings.Contains(filename, ".x86_64.") {
		return "x86_64"
	} else if strings.Contains(filename, ".aarch64.") {
		return "aarch64"
	} else if strings.Contains(filename, ".i686.") {
		return "i686"
	} else if strings.Contains(filename, ".noarch.") {
		return "noarch"
	} else if strings.Contains(filename, ".armv7hl.") {
		return "armv7hl"
	}
	return "x86_64"
}
