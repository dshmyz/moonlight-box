package adapter

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type AptAdapter struct {
	pkgRepo    *repository.PackageRepository
	storageSvc *service.StorageService
	auditSvc   *service.AuditService
}

type AptReleaseFile struct {
	Origin     string
	Label      string
	Arch       string
	Date       string
	Suites     string
	Components string
	MD5Sum     []AptHashEntry
	SHA1       []AptHashEntry
	SHA256     []AptHashEntry
}

type AptHashEntry struct {
	MD5    string
	SHA1   string
	SHA256 string
	Size   int64
	Name   string
}

type AptPackageEntry struct {
	Package       string
	Version       string
	Architecture  string
	Maintainer    string
	Description   string
	Section       string
	Priority      string
	InstalledSize string
	Filename      string
	Size          int64
	MD5Sum        string
	SHA1          string
	SHA256        string
}

func NewAptAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
) *AptAdapter {
	return &AptAdapter{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
		auditSvc:   auditSvc,
	}
}

func (a *AptAdapter) Type() PackageType { return AptType }
func (a *AptAdapter) RoutePrefix() string { return "/apt" }

func (a *AptAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	{
		r.GET("/dists/:dist/Release", a.ReleaseFile)
		r.GET("/dists/:dist/InRelease", a.InReleaseFile)
		r.GET("/dists/:dist/Release.gpg", a.ReleaseGPG)
		r.GET("/dists/:dist/:component/:arch", a.PackagesFile)
		r.GET("/dists/:dist/:component/binary-:arch/Packages", a.PackagesFile)
		r.GET("/dists/:dist/:component/binary-:arch/Packages.gz", a.PackagesFileGz)
		r.GET("/pool/*path", a.DownloadDeb)

		upload := r.Group("")
		upload.Use(authMw, permMw("apt", "write"))
		{
			upload.POST("/upload", a.UploadDeb)
		}
	}
}

func (a *AptAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid apt path: %s", path)
	}

	name := parts[0]
	version := ""
	if len(parts) >= 2 {
		version = parts[1]
	}

	return &PackageIdentity{Name: name, Version: version, Type: AptType}, nil
}

func (a *AptAdapter) ReleaseFile(c *gin.Context) {
	dist := c.Param("dist")

	release := fmt.Sprintf(
		`Origin: Moonlight Registry
Label: Moonlight
Suite: %s
Codename: %s
Architectures: amd64 arm64 i386
Components: main
Date: %s
Description: Moonlight Registry APT Repository
`,
		dist, dist, time.Now().UTC().Format(time.RFC1123),
	)

	release += "\nMD5Sum:\n"
	release += "SHA1:\n"
	release += "SHA256:\n"

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(200, release)
}

func (a *AptAdapter) InReleaseFile(c *gin.Context) {
	dist := c.Param("dist")

	release := fmt.Sprintf(
		`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Origin: Moonlight Registry
Label: Moonlight
Suite: %s
Codename: %s
Architectures: amd64 arm64 i386
Components: main
Date: %s
Description: Moonlight Registry APT Repository
-----BEGIN PGP SIGNATURE-----
(placeholder - not signed)
-----END PGP SIGNATURE-----
`,
		dist, dist, time.Now().UTC().Format(time.RFC1123),
	)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(200, release)
}

func (a *AptAdapter) ReleaseGPG(c *gin.Context) {
	response.NotFound(c, "GPG signature not available")
}

func (a *AptAdapter) PackagesFile(c *gin.Context) {
	_ = c.Param("dist")
	_ = c.Param("component")

	packages, _, err := a.pkgRepo.List(1, 10000, string(model.PackageTypeApt), "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var output strings.Builder
	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			filename := filepath.Base(ver.StoragePath)
			entry := AptPackageEntry{
				Package:       pkg.Name,
				Version:       ver.Version,
				Architecture:  "amd64",
				Maintainer:    "Moonlight Registry",
				Description:   pkg.Description,
				Section:       "misc",
				Priority:      "optional",
				InstalledSize: fmt.Sprintf("%d", ver.SizeBytes/1024),
				Filename:      fmt.Sprintf("pool/%s", filename),
				Size:          ver.SizeBytes,
				MD5Sum:        ver.ChecksumMD5,
				SHA256:        ver.ChecksumSHA256,
			}
			output.WriteString(formatPackageEntry(entry))
		}
	}

	if output.Len() == 0 {
		response.NotFound(c, "packages not found")
		return
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(200, output.String())
}

func (a *AptAdapter) PackagesFileGz(c *gin.Context) {
	_ = c.Param("dist")
	_ = c.Param("component")

	packages, _, err := a.pkgRepo.List(1, 10000, string(model.PackageTypeApt), "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var output strings.Builder
	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			filename := filepath.Base(ver.StoragePath)
			entry := AptPackageEntry{
				Package:       pkg.Name,
				Version:       ver.Version,
				Architecture:  "amd64",
				Maintainer:    "Moonlight Registry",
				Description:   pkg.Description,
				Section:       "misc",
				Priority:      "optional",
				InstalledSize: fmt.Sprintf("%d", ver.SizeBytes/1024),
				Filename:      fmt.Sprintf("pool/%s", filename),
				Size:          ver.SizeBytes,
			}
			output.WriteString(formatPackageEntry(entry))
		}
	}

	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", `attachment; filename="Packages.gz"`)
	c.String(200, output.String())
}

func (a *AptAdapter) DownloadDeb(c *gin.Context) {
	filePath := c.Param("path")
	filePath = strings.TrimPrefix(filePath, "/")

	storageKey := "apt/" + filePath

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err != nil {
		response.NotFound(c, "DEB not found")
		return
	}
	defer content.Close()

	size, err := backend.Size(c.Request.Context(), storageKey)
	if err != nil {
		response.NotFound(c, "DEB not found")
		return
	}

	filename := filepath.Base(filePath)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
	c.DataFromReader(200, size, "application/vnd.debian.binary-package", content, nil)
}

func (a *AptAdapter) UploadDeb(c *gin.Context) {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return
	}
	defer file.Close()

	debData, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file", err.Error())
		return
	}

	debName := header.Filename
	if !strings.HasSuffix(debName, ".deb") {
		response.BadRequest(c, "invalid file type", "file must be .deb")
		return
	}

	packageName := parseDebPackageName(debName)
	packageVersion := parseDebPackageVersion(debName)

	poolDir := fmt.Sprintf("pool/%s", packageName)
	storageKey := fmt.Sprintf("apt/%s/%s", poolDir, debName)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(debData), header.Size); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	md5Hash := md5.Sum(debData)
	sha256Hash := sha256.Sum256(debData)

	pkg, _, err := a.pkgRepo.CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeApt,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:        packageVersion,
		Status:         model.StatusPublished,
		StoragePath:    storageKey,
		SizeBytes:      header.Size,
		ChecksumMD5:    hex.EncodeToString(md5Hash[:]),
		ChecksumSHA256: hex.EncodeToString(sha256Hash[:]),
		PublishedBy:    userID,
	})
	if err != nil {
		backend.Delete(c.Request.Context(), storageKey)
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success":    true,
		"package":    packageName,
		"version":    packageVersion,
		"storageKey": storageKey,
		"result": &PackageVersionResult{
			PackageID:  pkg.ID,
			Version:    packageVersion,
			StorageKey: storageKey,
			Size:       header.Size,
		},
	})
}

func (a *AptAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["name"].(string)
	version, _ := req.Metadata["version"].(string)
	if name == "" || version == "" {
		return nil, fmt.Errorf("missing name or version in metadata")
	}

	storageKey := fmt.Sprintf("apt/pool/%s/%s_%s.deb", name, name, version)

	_, err := a.storageSvc.StorePackage(ctx, "apt", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeApt,
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
		a.storageSvc.DeletePackage(ctx, "apt", name, version)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *AptAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "apt", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/vnd.debian.binary-package",
		Size:        size,
	}, nil
}

func (a *AptAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeApt)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        AptType,
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

func (a *AptAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *AptAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeApt)
}

func formatPackageEntry(entry AptPackageEntry) string {
	return fmt.Sprintf(
		`Package: %s
Version: %s
Architecture: %s
Maintainer: %s
Description: %s
Section: %s
Priority: %s
Installed-Size: %s
Filename: %s
Size: %d
`,
		entry.Package,
		entry.Version,
		entry.Architecture,
		entry.Maintainer,
		entry.Description,
		entry.Section,
		entry.Priority,
		entry.InstalledSize,
		entry.Filename,
		entry.Size,
	)
}

func parseDebPackageName(filename string) string {
	base := strings.TrimSuffix(filename, ".deb")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return base
}

func parseDebPackageVersion(filename string) string {
	base := strings.TrimSuffix(filename, ".deb")
	parts := strings.SplitN(base, "_", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return "1.0.0"
}
