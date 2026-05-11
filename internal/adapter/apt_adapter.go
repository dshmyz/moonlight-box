package adapter

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"

	"github.com/gin-gonic/gin"
)

type AptAdapter struct {
	*BaseAdapter
	repoRepo  *repository.RepositoryRepository
	uploadSvc *service.UploadService
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
	repoRepo *repository.RepositoryRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *AptAdapter {
	adapter := &AptAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, auditSvc, pkgCache),
		repoRepo:    repoRepo,
		uploadSvc:   service.NewUploadService(pkgRepo, storageSvc),
	}
	return adapter
}

func (a *AptAdapter) Type() PackageType { return AptType }

func (a *AptAdapter) ResolvePackagePath(path string) (*types.PackagePathInfo, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid apt path: %s", path)
	}

	filename := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], "/")
	version := ""

	if strings.Contains(filename, ".deb") {
		base := strings.TrimSuffix(filename, ".deb")
		pkgParts := strings.Split(base, "_")
		if len(pkgParts) >= 2 {
			version = pkgParts[1]
		}
	}

	storageName := name
	storageVersion := filename
	remotePath := path

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       filename,
		StorageName:    storageName,
		StorageVersion: storageVersion,
		RemotePath:     remotePath,
	}, nil
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

	packages, _, err := a.GetPackageRepository().List(1, 10000, string(model.PackageTypeApt), "")
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

	packages, _, err := a.GetPackageRepository().List(1, 10000, string(model.PackageTypeApt), "")
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

	storageKey := filePath

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err == nil {
		defer content.Close()

		size, err := backend.Size(c.Request.Context(), storageKey)
		if err != nil {
			response.NotFound(c, "DEB not found")
			return
		}

		filename := filepath.Base(filePath)

		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		decision := a.CheckDownloadPermission(c, repo, model.PackageTypeApt, filename, "", filename)
		if !decision.Allow {
			c.JSON(decision.Code, gin.H{"error": decision.Message})
			return
		}

		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
		c.DataFromReader(200, size, "application/vnd.debian.binary-package", content, nil)
		return
	}

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	if a.fetcher != nil && repo != nil && repo.Type == "proxy" {
		slog.Info("APT proxy: fetching from remote", "filePath", filePath)
		pathInfo, pathErr := a.ResolvePackagePath(filePath)
		if pathErr != nil {
			slog.Warn("APT proxy: failed to resolve path", "filePath", filePath, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				filename := filepath.Base(filePath)
				slog.Info("APT proxy: successfully fetched from remote", "filePath", filePath, "size", result.Size)
				decision := a.CheckDownloadPermission(c, repo, model.PackageTypeApt, filename, "", filename)
				if !decision.Allow {
					c.JSON(decision.Code, gin.H{"error": decision.Message})
					return
				}
				c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
				c.DataFromReader(200, result.Size, "application/vnd.debian.binary-package", result.Content, nil)
				return
			}
			slog.Warn("APT proxy: failed to fetch from remote", "filePath", filePath, "error", fetchErr)
		}
	}

	response.NotFound(c, "DEB not found")
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

	firstLetter := string(name[0])
	if len(firstLetter) > 0 {
		firstLetter = strings.ToLower(firstLetter)
	}
	filename := fmt.Sprintf("%s_%s_amd64.deb", name, version)
	storageVersion := fmt.Sprintf("pool/main/%s/%s/%s", firstLetter, name, filename)

	uploadCtx := &service.UploadContext{
		PkgType:        "apt",
		Name:           name,
		Version:        version,
		StorageVersion: storageVersion,
		Filename:       filename,
		Content:        reader,
		Size:           req.Size,
		PackageType:    model.PackageTypeApt,
		RepositoryType: model.RepoTypeLocal,
		UploadedBy:     req.UploadedBy,
		Metadata:       req.Metadata,
		FileType:       model.FileTypePrimary,
	}

	result, err := a.uploadSvc.Upload(ctx, uploadCtx)
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

func (a *AptAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeApt, AptType)
}

func (a *AptAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeApt)
}

func (a *AptAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersions(name, model.PackageTypeApt)
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

func (a *AptAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := a.storageSvc.GetContentType(result.Filename)
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *AptAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	userID := ctx.UserID

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}
	defer file.Close()

	debData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	debName := header.Filename
	if !strings.HasSuffix(debName, ".deb") {
		return nil, fmt.Errorf("invalid file type: file must be .deb")
	}

	packageName := parseDebPackageName(debName)
	packageVersion := parseDebPackageVersion(debName)

	firstLetter := string(packageName[0])
	if len(firstLetter) > 0 {
		firstLetter = strings.ToLower(firstLetter)
	}
	poolDir := fmt.Sprintf("pool/main/%s/%s", firstLetter, packageName)
	storageKey := poolDir + "/" + debName

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(debData), header.Size); err != nil {
		return nil, err
	}

	md5Hash := md5.Sum(debData)
	sha256Hash := sha256.Sum256(debData)

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeApt,
		RepositoryID:   ctx.Repo.ID,
		RepositoryType: ctx.Repo.Type,
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
		return nil, err
	}

	return &types.PublishResult{
		PackageName: packageName,
		Version:     packageVersion,
		Size:        header.Size,
		Filename:    debName,
		Response: &types.AptPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  packageName,
				Version:  packageVersion,
				Filename: debName,
				Size:     header.Size,
			},
			StorageKey: storageKey,
			PackageId:  pkg.ID,
		},
	}, nil
}

func (a *AptAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	filePath := strings.TrimPrefix(c.Param("path"), "/")
	filePath = strings.TrimPrefix(filePath, "pool/")

	packageName := parseDebPackageName(filepath.Base(filePath))
	packageVersion := parseDebPackageVersion(filepath.Base(filePath))

	identity := &PackageIdentity{
		Name:    packageName,
		Version: packageVersion,
		Type:    AptType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	pkg, _ := a.GetPackageRepository().FindByNameAndType(identity.Name, model.PackageTypeApt)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, ctx.Repo.Name, identity.Name, identity.Version, pkgID)

	return nil
}

func (a *AptAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)

	if strings.HasPrefix(ctx.Path, "pool/") {
		a.DownloadDeb(c)
		return
	}

	if strings.HasPrefix(ctx.Path, "dists/") {
		parts := strings.Split(strings.Trim(ctx.Path, "/"), "/")
		if len(parts) >= 3 {
			dist := parts[1]
			if parts[2] == "Release" {
				c.Params = append(c.Params, gin.Param{Key: "dist", Value: dist})
				a.ReleaseFile(c)
				return
			}
			if parts[2] == "InRelease" {
				c.Params = append(c.Params, gin.Param{Key: "dist", Value: dist})
				a.InReleaseFile(c)
				return
			}
			if parts[2] == "Release.gpg" {
				c.Params = append(c.Params, gin.Param{Key: "dist", Value: dist})
				a.ReleaseGPG(c)
				return
			}
		}

		if len(parts) >= 6 {
			dist := parts[1]
			component := parts[2]
			fileName := parts[4]

			if fileName == "Packages" {
				c.Params = append(c.Params,
					gin.Param{Key: "dist", Value: dist},
					gin.Param{Key: "component", Value: component},
				)
				a.PackagesFile(c)
				return
			}
			if fileName == "Packages.gz" {
				c.Params = append(c.Params,
					gin.Param{Key: "dist", Value: dist},
					gin.Param{Key: "component", Value: component},
				)
				a.PackagesFileGz(c)
				return
			}
		}
	}

	response.NotFound(c, "APT resource not found")
}

func (a *AptAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return nil
	}
	defer file.Close()

	debData, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file", err.Error())
		return nil
	}

	debName := header.Filename
	if !strings.HasSuffix(debName, ".deb") {
		response.BadRequest(c, "invalid file type", "file must be .deb")
		return nil
	}

	packageName := parseDebPackageName(debName)
	packageVersion := parseDebPackageVersion(debName)

	firstLetter := string(packageName[0])
	if len(firstLetter) > 0 {
		firstLetter = strings.ToLower(firstLetter)
	}
	poolDir := fmt.Sprintf("pool/main/%s/%s", firstLetter, packageName)
	storageKey := poolDir + "/" + debName

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(debData), header.Size); err != nil {
		response.InternalError(c, err.Error())
		return nil
	}

	md5Hash := md5.Sum(debData)
	sha256Hash := sha256.Sum256(debData)

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeApt,
		RepositoryID:   repo.ID,
		RepositoryType: repo.Type,
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
		return nil
	}

	result := &types.RepoOperationResult{
		PackageName: packageName,
		Version:     packageVersion,
		Size:        header.Size,
		Filename:    debName,
		ExtraData: map[string]interface{}{
			"storageKey": storageKey,
			"packageId":  pkg.ID,
		},
	}

	result.Response = &types.AptPublishResponse{
		PublishResponse: types.PublishResponse{
			Success:  true,
			Message:  "Package published successfully",
			Package:  packageName,
			Version:  packageVersion,
			Filename: debName,
			Size:     header.Size,
		},
		StorageKey: storageKey,
		PackageId:  pkg.ID,
	}

	return result
}

func (a *AptAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
	filePath := strings.TrimPrefix(c.Param("path"), "/")
	filePath = strings.TrimPrefix(filePath, "pool/")

	packageName := parseDebPackageName(filepath.Base(filePath))
	packageVersion := parseDebPackageVersion(filepath.Base(filePath))

	identity := &PackageIdentity{
		Name:    packageName,
		Version: packageVersion,
		Type:    AptType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return nil
	}

	pkg, _ := a.GetPackageRepository().FindByNameAndType(identity.Name, model.PackageTypeApt)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, repo.Name, identity.Name, identity.Version, pkgID)

	c.JSON(200, gin.H{"ok": true})

	return &types.RepoOperationResult{
		PackageName: packageName,
		Version:     packageVersion,
	}
}
