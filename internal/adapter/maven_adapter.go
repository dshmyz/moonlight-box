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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
)

type MavenAdapter struct {
	*BaseAdapter
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
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *MavenAdapter {
	return &MavenAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, auditSvc, pkgCache),
	}
}

func (a *MavenAdapter) Type() PackageType { return MavenType }

func (a *MavenAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	// 处理 groupId:artifactId 格式（元数据请求）
	if strings.Contains(path, ":") && !strings.Contains(path, "/") {
		parts := strings.Split(path, ":")
		if len(parts) == 2 {
			groupPath := strings.ReplaceAll(parts[0], ".", "/")
			artifactID := parts[1]
			remotePath := groupPath + "/" + artifactID + "/maven-metadata.xml"
			return &types.PackagePathInfo{
				Name:        path,
				Version:     "",
				Filename:    "maven-metadata.xml",
				StorageName: groupPath + "/" + artifactID,
				RemotePath:  remotePath,
			}, nil
		}
	}

	// 处理 groupId:artifactId/version/filename 格式
	if strings.Contains(path, ":") {
		colonIdx := strings.Index(path, ":")
		groupArtifact := path[:colonIdx]
		rest := path[colonIdx+1:]

		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			filename := parts[len(parts)-1]
			version := parts[0]
			if len(parts) > 2 {
				version = strings.Join(parts[:len(parts)-1], "/")
			}

			groupPath := strings.ReplaceAll(groupArtifact, ".", "/")
			artifactID := ""
			if idx := strings.LastIndex(groupPath, "/"); idx != -1 {
				artifactID = groupPath[idx+1:]
			} else {
				artifactID = groupArtifact
			}

			name := groupArtifact + ":" + artifactID
			storageName := groupPath + "/" + artifactID
			storageVersion := version + "/" + filename

			remotePath := groupPath + "/" + artifactID + "/" + version + "/" + filename

			return &types.PackagePathInfo{
				Name:           name,
				Version:        version,
				Filename:       filename,
				StorageName:    storageName,
				StorageVersion: storageVersion,
				RemotePath:     remotePath,
			}, nil
		}
	}

	// 处理 groupId/artifactId/version/filename 格式（标准 Maven 路径）
	parts := strings.Split(path, "/")
	if len(parts) >= 4 {
		filename := parts[len(parts)-1]
		version := parts[len(parts)-2]
		artifactId := parts[len(parts)-3]
		groupId := strings.Join(parts[:len(parts)-3], ".")
		name := groupId + ":" + artifactId

		groupPath := strings.Join(parts[:len(parts)-3], "/")
		storageName := groupPath + "/" + artifactId

		storageVersion := version + "/" + filename
		if strings.HasSuffix(version, "-SNAPSHOT") {
			baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
			if strings.Contains(filename, baseVersion) {
				idx := strings.Index(filename, baseVersion)
				if idx != -1 {
					actualVersion := strings.TrimSuffix(filename[idx:], filepath.Ext(filename))
					storageVersion = version + "/" + actualVersion + "/" + filename
				}
			}
		}

		remotePath := groupPath + "/" + artifactId + "/" + version + "/" + filename

		return &types.PackagePathInfo{
			Name:           name,
			Version:        version,
			Filename:       filename,
			StorageName:    storageName,
			StorageVersion: storageVersion,
			RemotePath:     remotePath,
		}, nil
	}

	return nil, fmt.Errorf("invalid maven path: %s", path)
}

func (a *MavenAdapter) handleMetadataXML(c *gin.Context, fullPath string) {
	group := strings.TrimSuffix(fullPath, "/maven-metadata.xml")
	parts := strings.Split(group, "/")
	if len(parts) < 2 {
		response.NotFound(c, "metadata not found")
		return
	}

	var groupID, artifactID, name, version string
	var isVersionLevelMetadata bool

	if len(parts) >= 3 {
		lastPart := parts[len(parts)-1]

		if strings.Contains(lastPart, "-SNAPSHOT") || containsDigit(lastPart) {
			version = lastPart
			groupID = strings.Join(parts[:len(parts)-2], ".")
			artifactID = parts[len(parts)-2]
			name = groupID + ":" + artifactID
			isVersionLevelMetadata = true
		}
	}

	if !isVersionLevelMetadata {
		groupID = strings.Join(parts[:len(parts)-1], ".")
		artifactID = parts[len(parts)-1]
		name = groupID + ":" + artifactID
	}

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	if isVersionLevelMetadata {
		a.handleVersionLevelMetadata(c, name, version, groupID, artifactID, repo)
		return
	}

	pkg, err := a.GetPackageRepository().FindByNameAndType(name, model.PackageTypeMaven)

	needRefresh := false
	if err != nil {
		needRefresh = true
	} else if repo != nil && repo.Type == model.RepoTypeProxy {
		cacheTTL := time.Duration(repo.CacheTTLSeconds) * time.Second
		if time.Since(pkg.UpdatedAt) > cacheTTL {
			needRefresh = true
		}
	}

	if needRefresh && repo != nil && repo.Type == model.RepoTypeProxy && a.fetcher != nil {
		pathInfo, err := a.ParsePath(name)
		if err != nil {
			logrus.Warnf("failed to resolve package path for %s: %v", name, err)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								logrus.Errorf("panic in updatePackageMetadata for %s: %v", name, r)
							}
						}()
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						if err := a.updatePackageMetadata(ctx, name, body, repo.ID); err != nil {
							logrus.Warnf("failed to update package metadata for %s: %v", name, err)
						}
					}()

					storageName := groupArtifactToStorageName(group)
					storageKey, storeErr := a.storageSvc.StorePackage(c.Request.Context(), "maven", storageName, "maven-metadata.xml", bytes.NewReader(body), int64(len(body)))
					if storeErr == nil {
						_ = storageKey
					}

					c.Data(200, "application/xml", body)
					return
				}
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
			pkg, _ = a.GetPackageRepository().FindByNameAndType(name, model.PackageTypeMaven)
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

func (a *MavenAdapter) handleVersionLevelMetadata(c *gin.Context, name, version, groupID, artifactID string, repo *model.Repository) {
	logrus.Infof("handleVersionLevelMetadata: name=%s, version=%s, groupID=%s, artifactID=%s", name, version, groupID, artifactID)

	pkg, err := a.GetPackageRepository().FindByNameAndType(name, model.PackageTypeMaven)
	if err != nil {
		logrus.Warnf("Package not found for SNAPSHOT metadata: %s, will generate from storage", name)
		a.generateSnapshotMetadataFromStorage(c, name, version, groupID, artifactID, repo)
		return
	}

	var pkgVersion *model.PackageVersion
	for _, v := range pkg.Versions {
		if v.Version == version {
			pkgVersion = &v
			break
		}
	}

	if pkgVersion == nil {
		logrus.Warnf("Version not found for SNAPSHOT metadata: %s %s, will generate from storage", name, version)
		a.generateSnapshotMetadataFromStorage(c, name, version, groupID, artifactID, repo)
		return
	}

	metadata := &MavenMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Versioning: MavenVersioning{
			LastUpdated: time.Now().Format("20060102150405"),
		},
	}

	var meta map[string]interface{}
	if pkgVersion.Metadata != "" {
		if err := json.Unmarshal([]byte(pkgVersion.Metadata), &meta); err == nil {
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

	c.XML(200, metadata)
}

func (a *MavenAdapter) generateSnapshotMetadataFromStorage(c *gin.Context, name, version, groupID, artifactID string, repo *model.Repository) {
	logrus.Infof("generateSnapshotMetadataFromStorage: name=%s, version=%s", name, version)

	groupPath := strings.ReplaceAll(groupID, ".", "/") + "/" + artifactID + "/" + version
	storagePrefix := "maven2/" + groupPath + "/"

	logrus.Debugf("Listing files with prefix: %s", storagePrefix)
	entries, err := a.storageSvc.GetDefaultBackend().List(c.Request.Context(), storagePrefix)
	if err != nil {
		logrus.Warnf("Failed to list SNAPSHOT files: %v", err)
		response.NotFound(c, "SNAPSHOT metadata not found")
		return
	}

	if len(entries) == 0 {
		logrus.Warnf("No files found for SNAPSHOT version: %s %s", name, version)
		response.NotFound(c, "SNAPSHOT metadata not found")
		return
	}

	var latestTimestamp string
	var latestBuildNumber int
	var snapshotVersions []MavenSnapshotVersion

	for _, entry := range entries {
		if entry.IsDir {
			continue
		}

		filename := filepath.Base(entry.Key)
		logrus.Debugf("Found SNAPSHOT file: %s", filename)

		if strings.Contains(filename, artifactID+"-"+strings.TrimSuffix(version, "-SNAPSHOT")) {
			parts := strings.Split(filename, "-")
			if len(parts) >= 4 {
				timestamp := parts[2]
				buildNumStr := strings.Split(parts[3], ".")[0]
				buildNum := 0
				fmt.Sscanf(buildNumStr, "%d", &buildNum)

				if timestamp > latestTimestamp {
					latestTimestamp = timestamp
					latestBuildNumber = buildNum
				}

				ext := filepath.Ext(filename)
				snapshotVersions = append(snapshotVersions, MavenSnapshotVersion{
					Extension: strings.TrimPrefix(ext, "."),
					Value:     strings.TrimSuffix(filename, ext),
					Updated:   time.Now().Format("20060102150405"),
				})
			}
		}
	}

	if latestTimestamp == "" {
		logrus.Warnf("No valid SNAPSHOT files found for: %s %s", name, version)
		response.NotFound(c, "SNAPSHOT metadata not found")
		return
	}

	metadata := &MavenMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Versioning: MavenVersioning{
			LastUpdated: time.Now().Format("20060102150405"),
			Snapshot: &MavenSnapshot{
				Timestamp:   latestTimestamp,
				BuildNumber: latestBuildNumber,
			},
			SnapshotVersions: snapshotVersions,
		},
	}

	logrus.Infof("Generated SNAPSHOT metadata: timestamp=%s, buildNumber=%d", latestTimestamp, latestBuildNumber)
	c.XML(200, metadata)
}

func (a *MavenAdapter) generateSnapshotMetadataForChecksum(c *gin.Context, name, version, groupID, artifactID string) *MavenMetadata {
	logrus.Infof("generateSnapshotMetadataForChecksum: name=%s, version=%s", name, version)

	groupPath := strings.ReplaceAll(groupID, ".", "/") + "/" + artifactID + "/" + version
	storagePrefix := "maven2/" + groupPath + "/"

	logrus.Debugf("Listing files with prefix: %s", storagePrefix)
	entries, err := a.storageSvc.GetDefaultBackend().List(c.Request.Context(), storagePrefix)
	if err != nil {
		logrus.Warnf("Failed to list SNAPSHOT files: %v", err)
		return nil
	}

	if len(entries) == 0 {
		logrus.Warnf("No files found for SNAPSHOT version: %s %s", name, version)
		return nil
	}

	var latestTimestamp string
	var latestBuildNumber int

	for _, entry := range entries {
		if entry.IsDir {
			continue
		}

		filename := filepath.Base(entry.Key)
		logrus.Debugf("Found SNAPSHOT file: %s", filename)

		if strings.Contains(filename, artifactID+"-"+strings.TrimSuffix(version, "-SNAPSHOT")) {
			parts := strings.Split(filename, "-")
			if len(parts) >= 4 {
				timestamp := parts[2]
				buildNumStr := strings.Split(parts[3], ".")[0]
				buildNum := 0
				fmt.Sscanf(buildNumStr, "%d", &buildNum)

				if timestamp > latestTimestamp {
					latestTimestamp = timestamp
					latestBuildNumber = buildNum
				}
			}
		}
	}

	if latestTimestamp == "" {
		logrus.Warnf("No valid SNAPSHOT files found for: %s %s", name, version)
		return nil
	}

	metadata := &MavenMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Versioning: MavenVersioning{
			LastUpdated: time.Now().Format("20060102150405"),
			Snapshot: &MavenSnapshot{
				Timestamp:   latestTimestamp,
				BuildNumber: latestBuildNumber,
			},
		},
	}

	logrus.Infof("Generated SNAPSHOT metadata for checksum: timestamp=%s, buildNumber=%d", latestTimestamp, latestBuildNumber)
	return metadata
}

func (a *MavenAdapter) updatePackageMetadata(ctx context.Context, name string, metadataXML []byte, repoID uint) error {
	var metadata MavenMetadata
	if err := xml.Unmarshal(metadataXML, &metadata); err != nil {
		return err
	}

	now := time.Now()
	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeMaven,
		RepositoryID:   repoID,
		RepositoryType: model.RepoTypeProxy,
	}, nil)

	if err != nil {
		return err
	}

	if err := a.GetPackageRepository().DB().Model(pkg).Update("updated_at", now).Error; err != nil {
		return err
	}

	for _, version := range metadata.Versioning.Versions.Version {
		a.GetPackageRepository().CreateOrUpdateMetadata(ctx, pkg, &model.PackageVersion{
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

// groupArtifactToStorageName 将groupArtifact转换为存储名称格式
// 例如: "com/google/guava/guava" -> "com.google.guava:guava"
func groupArtifactToStorageName(groupArtifact string) string {
	return groupArtifactToName(groupArtifact)
}

func (a *MavenAdapter) getStorageNamesForGroupArtifact(groupArtifact string, parts []string) []string {
	storageNames := make([]string, 0, 2)

	storageNames = append(storageNames, groupArtifactToStorageName(groupArtifact))

	if len(parts) >= 4 {
		legacyGroupID := strings.Join(parts[:len(parts)-3], ".")
		legacyArtifactID := parts[len(parts)-3]
		legacyName := legacyGroupID + "/" + legacyArtifactID
		if legacyName != storageNames[0] {
			storageNames = append(storageNames, legacyName)
		}
	}

	return storageNames
}

func (a *MavenAdapter) findPackageInStorage(ctx context.Context, pkgType string, storageNames []string, storageVersion string) (io.ReadCloser, int64, string, error) {
	for _, storageName := range storageNames {
		content, size, err := a.storageSvc.GetPackage(ctx, pkgType, storageName, storageVersion)
		if err == nil {
			return content, size, storageName, nil
		}
	}
	return nil, 0, "", fmt.Errorf("package not found in storage")
}

func (a *MavenAdapter) handleDownloadArtifact(c *gin.Context, fullPath string) {
	logrus.Infof("handleDownloadArtifact called: fullPath=%s", fullPath)

	if strings.HasSuffix(fullPath, ".sha1") || strings.HasSuffix(fullPath, ".md5") {
		logrus.Infof("Detected checksum file, calling handleChecksumRequest")
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
	pkgName := groupArtifactToName(groupArtifact)

	logrus.Infof("Parsed: version=%s, filename=%s, groupArtifact=%s, pkgName=%s", version, filename, groupArtifact, pkgName)

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
		logrus.Infof("Repository info: name=%s, type=%s, remote_url=%s", repo.Name, repo.Type, repo.RemoteURL)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeMaven, pkgName, version, filename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	var storageVersion string
	isSnapshot := strings.HasSuffix(version, "-SNAPSHOT")

	if isSnapshot {
		baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
		if strings.Contains(filename, baseVersion) {
			idx := strings.Index(filename, baseVersion)
			if idx != -1 {
				actualVersion := strings.TrimSuffix(filename[idx:], filepath.Ext(filename))
				storageVersion = version + "/" + actualVersion + "/" + filename
				logrus.Infof("SNAPSHOT file: actualVersion=%s, storageVersion=%s", actualVersion, storageVersion)
			} else {
				storageVersion = version + "/" + filename
			}
		} else {
			storageVersion = version + "/" + filename
		}
	} else {
		storageVersion = version + "/" + filename
	}

	storageNames := a.getStorageNamesForGroupArtifact(groupArtifact, parts)
	logrus.Infof("Maven download: looking for %s in storage names=%v, version=%s", pkgName, storageNames, storageVersion)
	content, size, _, err := a.findPackageInStorage(c.Request.Context(), "maven", storageNames, storageVersion)

	if err == nil {
		logrus.Infof("Maven cache hit: found %s in storage, size=%d", pkgName, size)
		defer content.Close()

		contentType := a.storageSvc.GetContentType(filename)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.fetcher != nil && repo != nil && repo.Type == "proxy" {
		logrus.Infof("Maven proxy: fetching from remote for %s, version=%s, filename=%s", pkgName, version, filename)
		pathInfo, err := a.ParsePath(pkgName + ":" + version + "/" + filename)
		var fetchErr error
		if err != nil {
			logrus.Warnf("failed to resolve package path: %v", err)
			fetchErr = err
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				logrus.Infof("Maven proxy: successfully fetched %s from remote, size=%d", pkgName, result.Size)
				contentType := a.storageSvc.GetContentType(filename)
				c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
				c.DataFromReader(200, result.Size, contentType, result.Content, nil)
				return
			}
		}
		logrus.Warnf("Maven proxy: failed to fetch %s from remote: %v", pkgName, fetchErr)
	}

	response.NotFound(c, "artifact not found")
}

func (a *MavenAdapter) handleChecksumRequest(c *gin.Context, fullPath string) {
	logrus.Infof("handleChecksumRequest called: fullPath=%s", fullPath)
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "checksum not found")
		return
	}

	filename := parts[len(parts)-1]
	logrus.Infof("Checksum filename: %s", filename)

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

	logrus.Infof("Checksum type: %s, actualFilename: %s", checksumType, actualFilename)

	if actualFilename == "maven-metadata.xml" {
		a.handleMetadataChecksum(c, fullPath, checksumType, actualFilename)
		return
	}

	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	version := parts[len(parts)-2]
	pkgName := groupArtifactToName(groupArtifact)

	logrus.Infof("GroupArtifact: %s, version: %s, pkgName: %s", groupArtifact, version, pkgName)

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeMaven, pkgName, version, actualFilename)
	if !decision.Allow {
		logrus.Warnf("Download permission denied: %s", decision.Message)
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	storageName := groupArtifactToStorageName(groupArtifact)
	var storageVersion string
	isSnapshot := strings.HasSuffix(version, "-SNAPSHOT")

	if isSnapshot {
		if strings.Contains(actualFilename, strings.TrimSuffix(version, "-SNAPSHOT")) {
			actualVersion := strings.TrimSuffix(actualFilename, filepath.Ext(actualFilename))
			storageVersion = version + "/" + actualVersion + "/" + actualFilename
			logrus.Infof("SNAPSHOT checksum file: actualVersion=%s, storageVersion=%s", actualVersion, storageVersion)
		} else {
			storageVersion = version + "/" + actualFilename
		}
	} else {
		storageVersion = version + "/" + actualFilename
	}

	logrus.Infof("Looking for file in storage: storageName=%s, storageVersion=%s", storageName, storageVersion)

	content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", storageName, storageVersion)

	if err != nil {
		if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
			pathInfo, resolveErr := a.ParsePath(groupArtifactToStorageName(groupArtifact) + ":" + version)
			if resolveErr == nil {
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, remoteURL)
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
		}
		response.NotFound(c, "file not found")
		return
	}
	defer content.Close()

	body, _ := io.ReadAll(content)
	checksum := calculateChecksum(body, checksumType)

	c.String(200, "%s  %s", checksum, actualFilename)
}

func (a *MavenAdapter) handleMetadataChecksum(c *gin.Context, fullPath, checksumType, actualFilename string) {
	group := strings.TrimSuffix(fullPath, "/"+actualFilename)
	parts := strings.Split(group, "/")
	if len(parts) < 2 {
		response.NotFound(c, "metadata not found")
		return
	}

	var groupID, artifactID, name, version string
	var isVersionLevelMetadata bool

	if len(parts) >= 3 && (strings.Contains(parts[len(parts)-1], "-SNAPSHOT") ||
		!strings.Contains(parts[len(parts)-1], ".")) {
		version = parts[len(parts)-1]
		groupID = strings.Join(parts[:len(parts)-2], ".")
		artifactID = parts[len(parts)-2]
		name = groupID + ":" + artifactID
		isVersionLevelMetadata = true
	} else {
		groupID = strings.Join(parts[:len(parts)-1], ".")
		artifactID = parts[len(parts)-1]
		name = groupID + ":" + artifactID
		isVersionLevelMetadata = false
	}

	var metadata *MavenMetadata

	if isVersionLevelMetadata {
		pkg, err := a.GetPackageRepository().FindByNameAndType(name, model.PackageTypeMaven)
		if err != nil {
			logrus.Warnf("Package not found for SNAPSHOT metadata checksum: %s, will generate from storage", name)
			metadata = a.generateSnapshotMetadataForChecksum(c, name, version, groupID, artifactID)
			if metadata == nil {
				response.NotFound(c, "metadata not found")
				return
			}
		} else {
			var pkgVersion *model.PackageVersion
			for _, v := range pkg.Versions {
				if v.Version == version {
					pkgVersion = &v
					break
				}
			}

			if pkgVersion == nil {
				logrus.Warnf("Version not found for SNAPSHOT metadata checksum: %s %s, will generate from storage", name, version)
				metadata = a.generateSnapshotMetadataForChecksum(c, name, version, groupID, artifactID)
				if metadata == nil {
					response.NotFound(c, "version not found")
					return
				}
			} else {
				metadata = &MavenMetadata{
					GroupID:    groupID,
					ArtifactID: artifactID,
					Version:    version,
					Versioning: MavenVersioning{
						LastUpdated: time.Now().Format("20060102150405"),
					},
				}

				var meta map[string]interface{}
				if pkgVersion.Metadata != "" {
					if err := json.Unmarshal([]byte(pkgVersion.Metadata), &meta); err == nil {
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
			}
		}
	} else {
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

		metadata = &MavenMetadata{
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
			pkg, _ := a.GetPackageRepository().FindByNameAndType(name, model.PackageTypeMaven)
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
	}

	metadataXML, err := xml.Marshal(metadata)
	if err != nil {
		response.InternalError(c, "failed to generate metadata")
		return
	}

	metadataBytes := []byte(xml.Header + string(metadataXML))
	checksum := calculateChecksum(metadataBytes, checksumType)

	c.String(200, "%s  %s", checksum, actualFilename)
}

func (a *MavenAdapter) generateIndex(packages []model.Package, repoName string) *MavenPackageIndex {
	index := &MavenPackageIndex{
		Repository:  repoName,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Packages:    make([]MavenPackageSummary, 0),
	}

	for _, pkg := range packages {
		parts := strings.Split(pkg.Name, "/")
		if len(parts) < 2 {
			continue
		}

		groupID := parts[0]
		artifactID := parts[1]

		versions := make([]string, 0, len(pkg.Versions))
		var latest, release string

		for _, ver := range pkg.Versions {
			versions = append(versions, ver.Version)

			if latest == "" || compareVersions(ver.Version, latest) > 0 {
				latest = ver.Version
			}

			if isRelease(ver.Version) {
				if release == "" || compareVersions(ver.Version, release) > 0 {
					release = ver.Version
				}
			}
		}

		index.Packages = append(index.Packages, MavenPackageSummary{
			GroupID:    groupID,
			ArtifactID: artifactID,
			Versions:   versions,
			Latest:     latest,
			Release:    release,
		})
	}

	return index
}

func (a *MavenAdapter) handleIndexRequest(c *gin.Context) {
	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	if repo == nil {
		response.NotFound(c, "repository not found")
		return
	}

	var packages []model.Package
	err := a.GetPackageRepository().DB().
		Preload("Versions").
		Where("repository_id = ?", repo.ID).
		Find(&packages).
		Error

	if err != nil {
		response.InternalError(c, "failed to query packages")
		return
	}

	index := a.generateIndex(packages, repo.Name)

	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/xml") {
		c.XML(200, index)
	} else {
		c.JSON(200, index)
	}
}

func (a *MavenAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypeMaven, MavenType)
}

func (a *MavenAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	storageName := groupArtifactToStorageName(identity.Name)
	prefix := fmt.Sprintf("maven2/%s/%s/", storageName, identity.Version)
	entries, err := a.storageSvc.GetDefaultBackend().List(ctx, prefix)
	if err == nil {
		for _, entry := range entries {
			a.storageSvc.GetDefaultBackend().Delete(ctx, entry.Key)
		}
	}

	return a.GetPackageRepository().DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypeMaven)
}

func (a *MavenAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersions(name, model.PackageTypeMaven)
}

func (a *MavenAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	logrus.Infof("=== MavenAdapter.HandleRepoRequest START ===")
	logrus.Infof("Maven HandleRepoRequest: path=%s, repo=%s, repo.Type=%s, repo.PackageType=%s", ctx.Path, ctx.Repo.Name, ctx.Repo.Type, ctx.Repo.PackageType)
	c.Set("repo", ctx.Repo)

	if strings.HasSuffix(ctx.Path, "/index") || ctx.Path == "index" {
		a.handleIndexRequest(c)
		return
	}

	if strings.HasSuffix(ctx.Path, "maven-metadata.xml") {
		a.handleMetadataXML(c, ctx.Path)
		return
	}

	a.handleDownloadArtifact(c, ctx.Path)
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

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
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

func calculateChecksum(data []byte, checksumType string) string {
	if checksumType == "sha1" {
		hash := sha1.Sum(data)
		return hex.EncodeToString(hash[:])
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

func (a *MavenAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := a.storageSvc.GetContentType(result.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *MavenAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid path: expected group/artifact/version/file")
	}

	groupID := strings.Join(parts[:len(parts)-3], "/")
	groupID = strings.ReplaceAll(groupID, "/", ".")
	artifactID := parts[len(parts)-3]
	version := parts[len(parts)-2]
	filename := parts[len(parts)-1]

	name := groupID + ":" + artifactID
	size := c.Request.ContentLength

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	hash := sha1.Sum(body)
	storageVersion := hex.EncodeToString(hash[:])

	return &types.PublishResult{
		PackageName:    name,
		Version:        version,
		Filename:       filename,
		Content:        bytes.NewReader(body),
		Size:           size,
		FileType:       getMavenFileType(filename),
		StorageVersion: storageVersion,
		Response: &types.MavenPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  name,
				Version:  version,
				Filename: filename,
				Size:     size,
			},
			Packaging: getPackaging(filename),
		},
	}, nil
}

func (a *MavenAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		return fmt.Errorf("invalid path: expected group/artifact/version/filename")
	}

	groupID := strings.Join(parts[:len(parts)-3], "/")
	groupID = strings.ReplaceAll(groupID, "/", ".")
	artifactID := parts[len(parts)-3]
	version := parts[len(parts)-2]

	groupArtifact := groupID + "/" + artifactID
	name := groupArtifactToStorageName(groupArtifact)
	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    MavenType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	pkg, _ := a.GetPackageRepository().FindByNameAndType(identity.Name, model.PackageTypeMaven)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, ctx.Repo.Name, identity.Name, identity.Version, pkgID)

	return nil
}
