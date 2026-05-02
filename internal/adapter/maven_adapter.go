package adapter

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
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
	*BaseAdapter
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	auditSvc    *service.AuditService
	proxyRouter *proxy.ProxyRouter
}

type MavenMetadata struct {
	XMLName    xml.Name        `xml:"metadata"`
	GroupID    string          `xml:"groupId"`
	ArtifactID string          `xml:"artifactId"`
	Version    string          `xml:"version,omitempty"`
	Versioning MavenVersioning `xml:"versioning"`
}

type MavenVersioning struct {
	Release          string                 `xml:"release,omitempty"`
	Latest           string                 `xml:"latest"`
	Versions         MavenVersions          `xml:"versions"`
	LastUpdated      string                 `xml:"lastUpdated"`
	Snapshot         *MavenSnapshot         `xml:"snapshot,omitempty"`
	SnapshotVersions []MavenSnapshotVersion `xml:"snapshotVersions>snapshotVersion,omitempty"`
}

type MavenSnapshot struct {
	Timestamp   string `xml:"timestamp"`
	BuildNumber int    `xml:"buildNumber"`
	LocalCopy   bool   `xml:"localCopy,omitempty"`
}

type MavenSnapshotVersion struct {
	Extension string `xml:"extension"`
	Value     string `xml:"value"`
	Updated   string `xml:"updated"`
}

type MavenPackageIndex struct {
	XMLName     xml.Name              `xml:"index"`
	Repository  string                `xml:"repository" json:"repository"`
	GeneratedAt string                `xml:"generatedAt" json:"generatedAt"`
	Packages    []MavenPackageSummary `xml:"packages>package" json:"packages"`
}

type MavenPackageSummary struct {
	GroupID    string   `xml:"groupId" json:"groupId"`
	ArtifactID string   `xml:"artifactId" json:"artifactId"`
	Versions   []string `xml:"versions>version" json:"versions"`
	Latest     string   `xml:"latest" json:"latest"`
	Release    string   `xml:"release" json:"release"`
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
		BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
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

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeMaven)

	needRefresh := false
	if err != nil {
		needRefresh = true
	} else if repo != nil && repo.Type == model.RepoTypeProxy {
		cacheTTL := time.Duration(repo.CacheTTLSeconds) * time.Second
		if time.Since(pkg.UpdatedAt) > cacheTTL {
			needRefresh = true
		}
	}

	if needRefresh && repo != nil && repo.Type == model.RepoTypeProxy && a.proxyRouter != nil {
		urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
			baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
			return fmt.Sprintf("%s/%s/maven-metadata.xml", baseURL, group)
		}

		result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "maven", name, "", urlBuilder)
		if resolveErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				go a.updatePackageMetadata(context.Background(), name, body, repo.ID)

				c.Data(200, "application/xml", body)
				return
			}
		}
	}

	versions, err := a.ListVersions(c.Request.Context(), name)
	if err != nil {
		response.NotFound(c, "metadata not found")
		return
	}

	var latest, release string
	var snapshotVersions []string
	for _, ver := range versions {
		if latest == "" || compareVersions(ver, latest) > 0 {
			latest = ver
		}
		if isRelease(ver) {
			if release == "" || compareVersions(ver, release) > 0 {
				release = ver
			}
		} else {
			snapshotVersions = append(snapshotVersions, ver)
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

	if len(snapshotVersions) > 0 {
		latestSnapshot := snapshotVersions[len(snapshotVersions)-1]
		if pkg == nil {
			pkg, _ = a.pkgRepo.FindByNameAndType(name, model.PackageTypeMaven)
		}
		if pkg != nil {
			for _, ver := range pkg.Versions {
				if ver.Version == latestSnapshot {
					metadata.Version = latestSnapshot

					var meta map[string]interface{}
					if ver.Metadata != "" {
						if err := json.Unmarshal([]byte(ver.Metadata), &meta); err == nil {
							if timestamp, ok := meta["snapshotTimestamp"].(string); ok {
								if buildNumber, ok := meta["snapshotBuildNumber"].(float64); ok {
									metadata.Versioning.Snapshot = &MavenSnapshot{
										Timestamp:   timestamp,
										BuildNumber: int(buildNumber),
									}
								}
							}
						}
					}
					break
				}
			}
		}
	}

	c.XML(200, metadata)
}

func (a *MavenAdapter) updatePackageMetadata(ctx context.Context, name string, metadataXML []byte, repoID uint) error {
	var metadata MavenMetadata
	if err := xml.Unmarshal(metadataXML, &metadata); err != nil {
		return err
	}

	now := time.Now()
	pkg, _, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeMaven,
		RepositoryID:   repoID,
		RepositoryType: model.RepoTypeProxy,
		MetadataSynced: true,
		MetadataSyncAt: &now,
	}, nil)

	if err != nil {
		return err
	}

	if err := a.pkgRepo.DB().Model(pkg).Update("updated_at", now).Error; err != nil {
		return err
	}

	for _, version := range metadata.Versioning.Versions.Version {
		a.pkgRepo.CreateOrUpdate(ctx, pkg, &model.PackageVersion{
			Version: version,
			Status:  model.StatusPublished,
		})
	}

	return nil
}

func groupArtifactToName(groupArtifact string) string {
	parts := strings.Split(groupArtifact, "/")
	if len(parts) < 2 {
		return groupArtifact
	}
	groupId := strings.Join(parts[:len(parts)-1], ".")
	artifactId := parts[len(parts)-1]
	return groupId + ":" + artifactId
}

func (a *MavenAdapter) handleDownloadArtifact(c *gin.Context, fullPath string) {
	// 检查是否是校验文件请求
	if strings.HasSuffix(fullPath, ".sha1") || strings.HasSuffix(fullPath, ".md5") {
		a.handleChecksumRequest(c, fullPath)
		return
	}

	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "artifact not found")
		return
	}

	version := parts[len(parts)-2]
	filename := parts[len(parts)-1]
	groupArtifact := strings.Join(parts[:len(parts)-2], "/")

	storageVersion := version + "/" + filename
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, storageVersion)
	if err == nil {
		defer content.Close()

		pkgName := groupArtifactToName(groupArtifact)
		a.IncrementDownloadCountForPackage(pkgName, model.PackageTypeMaven, version, filename)

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

		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "maven", groupArtifact, version, urlBuilder)
		if resolveErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				storageKey, storeErr := a.storageSvc.StorePackage(c.Request.Context(), "maven", groupArtifact, version, bytes.NewReader(body), result.Size)
				if storeErr == nil {
					pkgName := groupArtifactToName(groupArtifact)
					a.pkgRepo.StorePackageFileAndIncrementDownload(c.Request.Context(), &model.Package{
						Name:           pkgName,
						Type:           model.PackageTypeMaven,
						RepositoryID:   result.RepoID,
						RepositoryType: model.RepoTypeProxy,
					}, &model.PackageVersion{
						Version:     version,
						Status:      model.StatusPublished,
						StoragePath: filepath.Dir(storageKey),
					}, &model.PackageFile{
						Filename:    filename,
						FileType:    getMavenFileType(filename),
						StoragePath: storageKey,
						SizeBytes:   result.Size,
					})
				}
				localContent, localSize, localErr := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, storageVersion)
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

func (a *MavenAdapter) handleChecksumRequest(c *gin.Context, fullPath string) {
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "checksum not found")
		return
	}

	filename := parts[len(parts)-1]

	var checksumType string
	var actualFilename string

	if strings.HasSuffix(filename, ".sha1") {
		checksumType = "sha1"
		actualFilename = strings.TrimSuffix(filename, ".sha1")
	} else if strings.HasSuffix(filename, ".md5") {
		checksumType = "md5"
		actualFilename = strings.TrimSuffix(filename, ".md5")
	} else {
		response.NotFound(c, "checksum not found")
		return
	}

	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	version := parts[len(parts)-2]
	storageVersion := version + "/" + actualFilename

	content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, storageVersion)

	if err != nil {
		if a.proxyRouter != nil {
			urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
				baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
				return fmt.Sprintf("%s/%s/%s/%s", baseURL, groupArtifact, version, actualFilename)
			}

			var repo *model.Repository
			if r, ok := c.Get("repo"); ok {
				repo = r.(*model.Repository)
			}

			result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "maven", groupArtifact, version, urlBuilder)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					checksum := calculateChecksum(body, checksumType)
					c.String(200, "%s  %s", checksum, actualFilename)
					return
				}
			}
		}
		response.NotFound(c, "file not found")
		return
	}
	defer content.Close()

	body, _ := io.ReadAll(content)
	checksum := calculateChecksum(body, checksumType)

	c.String(200, "%s  %s", checksum, actualFilename)
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
			"filename":   filename,
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
	filename, _ := req.Metadata["filename"].(string)
	if filename == "" {
		filename = req.Filename
	}

	name := groupID + "/" + artifactID

	isSnapshot := strings.HasSuffix(version, "-SNAPSHOT")
	var storageVersion string
	var actualVersion string

	if isSnapshot {
		timestamp, buildNumber := generateSnapshotTimestamp()
		baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
		actualVersion = fmt.Sprintf("%s-%s-%d", baseVersion, timestamp, buildNumber)
		storageVersion = version + "/" + actualVersion + "/" + filename

		req.Metadata["snapshotTimestamp"] = timestamp
		req.Metadata["snapshotBuildNumber"] = buildNumber
		req.Metadata["actualVersion"] = actualVersion
	} else {
		storageVersion = version + "/" + filename
		actualVersion = version
	}

	storageKey, err := a.storageSvc.StorePackage(ctx, "maven", name, actualVersion, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, ver, _, err := a.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeMaven,
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
		FileType:    getMavenFileType(filename),
		StoragePath: storageKey,
		SizeBytes:   req.Size,
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "maven", name, storageVersion)
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
		var totalSize int64
		for _, f := range ver.Files {
			totalSize += f.SizeBytes
		}
		meta.Versions = append(meta.Versions, VersionInfo{
			Version:       ver.Version,
			PublishedAt:   ver.PublishedAt.Format(time.RFC3339),
			Size:          totalSize,
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

func (a *MavenAdapter) HandleRepoRequest(c *gin.Context, repo *model.Repository, path string) {
	c.Set("repo", repo)

	if strings.HasSuffix(path, "maven-metadata.xml") {
		a.handleMetadataXML(c, path)
		return
	}

	a.handleDownloadArtifact(c, path)
}

func (a *MavenAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) {
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
			"filename":   filename,
			"repo_name":  repo.Name,
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

func (a *MavenAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 3 {
		response.BadRequest(c, "invalid path", "expected group/artifact/version")
		return
	}

	groupID := strings.Join(parts[:len(parts)-2], ".")
	artifactID := parts[len(parts)-2]
	version := parts[len(parts)-1]

	name := groupID + "/" + artifactID
	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    MavenType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{"ok": true})
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

func getMavenFileType(filename string) model.PackageFileType {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".pom") {
		return model.FileTypePom
	}
	if strings.Contains(lower, "-sources") {
		return model.FileTypeSources
	}
	if strings.Contains(lower, "-javadoc") {
		return model.FileTypeJavadoc
	}
	if strings.Contains(lower, "maven-metadata.xml") {
		return model.FileTypeMetadata
	}
	return model.FileTypePrimary
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

func generateSnapshotTimestamp() (string, int) {
	now := time.Now()
	timestamp := now.Format("20060102.150405")
	buildNumber := 1
	return timestamp, buildNumber
}

func parseSnapshotVersion(version string) (baseVersion, timestamp string, buildNumber int) {
	if !strings.HasSuffix(version, "-SNAPSHOT") {
		return version, "", 0
	}

	baseVersion = strings.TrimSuffix(version, "-SNAPSHOT")
	return baseVersion, "", 0
}

func isSnapshotTimestampVersion(version string) bool {
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+-\d{8}\.\d{6}-\d+$`, version)
	return matched
}

func calculateChecksum(data []byte, checksumType string) string {
	if checksumType == "sha1" {
		hash := sha1.Sum(data)
		return hex.EncodeToString(hash[:])
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}
