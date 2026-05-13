package adapter

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
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

	"github.com/gin-gonic/gin"
)

type YumAdapter struct {
	*BaseAdapter
	repoRepo *repository.RepositoryRepository
}

type RepoMD struct {
	XMLName  xml.Name     `xml:"repomd"`
	Xmlns    string       `xml:"xmlns,attr"`
	XmlnsRpm string       `xml:"xmlns:rpm,attr"`
	Revision string       `xml:"revision"`
	Data     []RepoMDData `xml:"data"`
}

type RepoMDData struct {
	Type         string         `xml:"type,attr"`
	Checksum     Checksum       `xml:"checksum"`
	OpenChecksum Checksum       `xml:"open-checksum"`
	Location     RepoMDLocation `xml:"location"`
	Timestamp    int64          `xml:"timestamp"`
	Size         int64          `xml:"size"`
	OpenSize     int64          `xml:"open-size"`
}

type Checksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type RepoMDLocation struct {
	Href string `xml:"href,attr"`
}

type PrimaryData struct {
	XMLName  xml.Name     `xml:"metadata"`
	Xmlns    string       `xml:"xmlns,attr"`
	XmlnsRpm string       `xml:"xmlns:rpm,attr"`
	Packages []YumPackage `xml:"package"`
}

type YumPackage struct {
	Type        string      `xml:"type,attr"`
	Name        string      `xml:"name"`
	Arch        string      `xml:"arch"`
	Version     YumVersion  `xml:"version"`
	Checksum    YumChecksum `xml:"checksum"`
	Summary     string      `xml:"summary"`
	Description string      `xml:"description"`
	Packager    string      `xml:"packager"`
	URL         string      `xml:"url"`
	Time        YumTime     `xml:"time"`
	Size        YumSize     `xml:"size"`
	Location    YumLocation `xml:"location"`
	Format      YumFormat   `xml:"format"`
}

type YumVersion struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type YumChecksum struct {
	Type string `xml:"type,attr"`
	Hash string `xml:",chardata"`
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
	License   string          `xml:"license"`
	Vendor    string          `xml:"vendor"`
	Group     string          `xml:"group"`
	Buildhost string          `xml:"buildhost"`
	Requires  []YumDependency `xml:"requires>rpm:entry"`
	Provides  []YumDependency `xml:"provides>rpm:entry"`
}

type YumDependency struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr"`
	Epoch string `xml:"epoch,attr,omitempty"`
	Ver   string `xml:"ver,attr,omitempty"`
	Rel   string `xml:"rel,attr,omitempty"`
	Pre   bool   `xml:"pre,attr,omitempty"`
}

func NewYumAdapter(
	repoRepo *repository.RepositoryRepository,
	storageSvc *service.StorageService,
	pkgCache *cache.PackageCache,
) *YumAdapter {
	adapter := &YumAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
		repoRepo:    repoRepo,
	}
	return adapter
}

func (a *YumAdapter) Type() PackageType { return YumType }

func (a *YumAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, fmt.Errorf("invalid yum path: empty path")
	}

	if strings.Contains(path, ".rpm") {
		return a.resolveRpmPath(path)
	}

	parts := strings.Split(path, "/")

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

func (a *YumAdapter) resolveRpmPath(path string) (*types.PackagePathInfo, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid yum rpm path: %s", path)
	}

	filename := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], "/")
	version := ""

	base := strings.TrimSuffix(filename, ".rpm")
	pkgParts := strings.Split(base, "-")
	if len(pkgParts) >= 2 {
		version = pkgParts[1]
	}

	storageName := name
	storageVersion := filename
	remotePath := name + "/" + filename

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       filename,
		StorageName:    storageName,
		StorageVersion: storageVersion,
		RemotePath:     remotePath,
	}, nil
}

func (a *YumAdapter) RepoMetadata(c *gin.Context, repo string) *types.ContentResult {
	repomdXML, err := a.generateRepomdXML(c.Request.Context(), repo)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}
	}

	return &types.ContentResult{
		ContentType: "application/xml",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader([]byte(repomdXML))),
		Size:        int64(len(repomdXML)),
	}
}

func (a *YumAdapter) RepoDataFile(c *gin.Context, repo string, filePath string) *types.ContentResult {
	storageKey := fmt.Sprintf("repos/%s/repodata/%s", repo, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(c.Request.Context(), storageKey)
		contentType := "application/xml"
		if strings.HasSuffix(filePath, ".gz") {
			contentType = "application/gzip"
		}
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "metadata file not found"},
	}
}

func (a *YumAdapter) DownloadRPM(c *gin.Context, repoName string, filePath string) *types.ContentResult {
	storageKey := fmt.Sprintf("repos/%s/Packages/%s", repoName, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "RPM not found"},
		}
	}
	defer content.Close()

	size, err := backend.Size(c.Request.Context(), storageKey)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "RPM not found"},
		}
	}

	filename := filepath.Base(filePath)
	headers := map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
	}

	return &types.ContentResult{
		Content:     content,
		Size:        size,
		ContentType: "application/x-rpm",
		StatusCode:  200,
		Headers:     headers,
	}
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

	packageName, version, release, arch := parseRpmFilename(rpmName)

	packagesDir := fmt.Sprintf("repos/%s/Packages/%s", repo, arch)
	storageKey := fmt.Sprintf("%s/%s", packagesDir, rpmName)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(rpmData), header.Size); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeYum,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		SizeBytes:   header.Size,
		PublishedBy: userID,
		Metadata:    marshalMetadata(map[string]interface{}{"repo": repo, "arch": arch, "release": release}),
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
			Version:    version,
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

func (a *YumAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.GetPackageRepository().FindByNameAndType(name, model.PackageTypeYum)
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
	return a.GetPackageRepository().DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeYum)
}

func (a *YumAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersions(name, model.PackageTypeYum)
}

// ParseIntent 解析请求路径为意图
func (a *YumAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = strings.TrimPrefix(path, "/")
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	if strings.HasPrefix(path, "repodata/") {
		filePath := strings.TrimPrefix(path, "repodata/")
		intent.Type = types.RequestMetadata
		intent.Filename = filePath
		return intent
	}

	if strings.HasPrefix(path, "Packages/") {
		filePath := strings.TrimPrefix(path, "Packages/")
		intent.Type = types.RequestDownload
		intent.Filename = filepath.Base(filePath)
		if strings.HasSuffix(intent.Filename, ".rpm") {
			name, version, _, _ := parseRpmFilename(intent.Filename)
			intent.Name = name
			intent.Version = version
		}
		return intent
	}

	// Generic storage access
	intent.Type = types.RequestDownload
	pathInfo, _ := a.ParsePath(path)
	if pathInfo != nil {
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
		intent.PkgPathInfo = pathInfo
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *YumAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	path := strings.TrimPrefix(intent.Path, "/")

	if strings.HasPrefix(path, "repodata/") {
		filePath := strings.TrimPrefix(path, "repodata/")
		return a.repoDataFile(ctx, repo.Name, filePath)
	}

	if strings.HasPrefix(path, "Packages/") {
		filePath := strings.TrimPrefix(path, "Packages/")
		return a.downloadRPM(ctx, repo.Name, filePath)
	}

	// Generic storage access
	storageKey := fmt.Sprintf("repos/%s/%s", repo.Name, path)
	backend := a.storageSvc.GetDefaultBackend()

	content, err := backend.Get(ctx, storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(ctx, storageKey)
		contentType := a.storageSvc.GetContentType(path)
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
		}, nil
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "file not found"},
	}, nil
}

// repoDataFile 获取 repodata 文件（不依赖 gin.Context）
func (a *YumAdapter) repoDataFile(ctx context.Context, repo string, filePath string) (*types.ContentResult, error) {
	storageKey := fmt.Sprintf("repos/%s/repodata/%s", repo, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(ctx, storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(ctx, storageKey)
		contentType := "application/xml"
		if strings.HasSuffix(filePath, ".gz") {
			contentType = "application/gzip"
		}
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
		}, nil
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "metadata file not found"},
	}, nil
}

// downloadRPM 下载 RPM 文件（不依赖 gin.Context）
func (a *YumAdapter) downloadRPM(ctx context.Context, repoName string, filePath string) (*types.ContentResult, error) {
	storageKey := fmt.Sprintf("repos/%s/Packages/%s", repoName, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(ctx, storageKey)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "RPM not found"},
		}, nil
	}
	defer content.Close()

	size, err := backend.Size(ctx, storageKey)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "RPM not found"},
		}, nil
	}

	filename := filepath.Base(filePath)
	headers := map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
	}

	return &types.ContentResult{
		Content:     content,
		Size:        size,
		ContentType: "application/x-rpm",
		StatusCode:  200,
		Headers:     headers,
	}, nil
}

func (a *YumAdapter) generateRepomdXML(ctx context.Context, repo string) (string, error) {
	repomd := RepoMD{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		XmlnsRpm: "http://linux.duke.edu/metadata/rpm",
		Revision: fmt.Sprintf("%d", time.Now().Unix()),
	}

	backend := a.storageSvc.GetDefaultBackend()
	entries, err := backend.List(ctx, fmt.Sprintf("repos/%s/repodata/", repo))
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
	packages, _, err := a.GetPackageRepository().List(1, 10000, string(model.PackageTypeYum), "")
	if err != nil {
		return err
	}

	var yumPkgs []YumPackage
	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			release := ""
			if meta := unmarshalMetadata(ver.Metadata); meta != nil {
				if r, ok := meta["release"].(string); ok {
					release = r
				}
			}

			version := ver.Version
			if release != "" && !strings.Contains(version, "-") {
				version = version + "-" + release
			}

			arch := "x86_64"
			if meta := unmarshalMetadata(ver.Metadata); meta != nil {
				if a, ok := meta["arch"].(string); ok {
					arch = a
				}
			}

			yumPkgs = append(yumPkgs, YumPackage{
				Type:    "rpm",
				Name:    pkg.Name,
				Arch:    arch,
				Version: YumVersion{Ver: version},
				Size:    YumSize{Package: ver.SizeBytes},
				Location: YumLocation{
					Href: fmt.Sprintf("Packages/%s", ver.Version),
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
	repodataDir := fmt.Sprintf("repos/%s/repodata/", repo)
	if err := backend.Put(ctx, repodataDir+primaryFilename, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		return err
	}

	return nil
}

func detectRpmArch(filename string) string {
	if strings.Contains(filename, ".x86_64") {
		return "x86_64"
	} else if strings.Contains(filename, ".aarch64") {
		return "aarch64"
	} else if strings.Contains(filename, ".i686") {
		return "i686"
	} else if strings.Contains(filename, ".noarch") {
		return "noarch"
	} else if strings.Contains(filename, ".armv7hl") {
		return "armv7hl"
	}
	return "x86_64"
}

func parseRpmFilename(filename string) (name, version, release, arch string) {
	filename = strings.TrimSuffix(filename, ".rpm")

	arch = detectRpmArch(filename)

	archSuffix := "." + arch
	if idx := strings.LastIndex(filename, archSuffix); idx > 0 {
		filename = filename[:idx]
	}

	parts := strings.Split(filename, "-")
	if len(parts) >= 3 {
		name = strings.Join(parts[:len(parts)-2], "-")
		version = parts[len(parts)-2]
		release = parts[len(parts)-1]
	} else if len(parts) == 2 {
		name = parts[0]
		version = parts[1]
		release = "1"
	} else {
		name = filename
		version = "1.0.0"
		release = "1"
	}

	return name, version, release, arch
}

func unmarshalMetadata(data string) map[string]interface{} {
	if data == "" {
		return nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return nil
	}
	return meta
}

func (a *YumAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	userID := ctx.UserID

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}
	defer file.Close()

	rpmData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	rpmName := header.Filename
	if !strings.HasSuffix(rpmName, ".rpm") {
		return nil, fmt.Errorf("invalid file type: file must be .rpm")
	}

	packageName, version, release, arch := parseRpmFilename(rpmName)

	packagesDir := fmt.Sprintf("repos/%s/Packages/%s", ctx.Repo.Name, arch)
	storageKey := fmt.Sprintf("%s/%s", packagesDir, rpmName)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(rpmData), header.Size); err != nil {
		return nil, err
	}

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeYum,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		SizeBytes:   header.Size,
		PublishedBy: userID,
		Metadata:    marshalMetadata(map[string]interface{}{"repo": ctx.Repo.Name, "arch": arch, "release": release}),
	})
	if err != nil {
		backend.Delete(c.Request.Context(), storageKey)
		return nil, err
	}

	return &types.PublishResult{
		PackageName: packageName,
		Version:     version,
		Size:        header.Size,
		Filename:    rpmName,
		Response: &types.YumPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  packageName,
				Version:  version,
				Filename: rpmName,
				Size:     header.Size,
			},
			Repo:       ctx.Repo.Name,
			Arch:       arch,
			Release:    release,
			StorageKey: storageKey,
			PackageId:  pkg.ID,
		},
	}, nil
}

func (a *YumAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid path: expected name/version")
	}

	name := parts[0]
	version := parts[1]

	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    YumType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	return nil
}
