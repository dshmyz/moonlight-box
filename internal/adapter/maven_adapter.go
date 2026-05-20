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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"
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
	Extension  string `xml:"extension"`
	Classifier string `xml:"classifier,omitempty"`
	Value      string `xml:"value"`
	Updated    string `xml:"updated"`
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

func NewMavenAdapter(args ...interface{}) *MavenAdapter {
	var storageSvc *service.StorageService
	var pkgCache *cache.PackageCache

	// New signature: (storageSvc, pkgCache)
	if len(args) >= 1 {
		if s, ok := args[0].(*service.StorageService); ok {
			storageSvc = s
		}
	}
	if len(args) >= 2 {
		if c, ok := args[1].(*cache.PackageCache); ok {
			pkgCache = c
		}
	}

	// Legacy signature: (pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache)
	if storageSvc == nil && len(args) >= 3 {
		if s, ok := args[2].(*service.StorageService); ok {
			storageSvc = s
		}
	}
	if pkgCache == nil && len(args) >= 1 {
		if pkgRepo, ok := args[0].(*repository.PackageRepository); ok {
			pkgCache = cache.NewPackageCache(pkgRepo, 5*time.Minute)
		}
	}

	return &MavenAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
	}
}

func (a *MavenAdapter) Type() PackageType { return MavenType }

// validateMavenPath 验证 Maven 路径安全性
func validateMavenPath(path string) error {
	// 检测路径遍历攻击
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal detected")
	}

	// 检测 Windows 绝对路径
	if strings.Contains(path, ":\\") || strings.HasPrefix(path, "\\") {
		return fmt.Errorf("absolute paths not allowed")
	}

	// 检测 URL 编码的路径遍历
	if strings.Contains(path, "%2e%2e") || strings.Contains(path, "%252e") {
		return fmt.Errorf("encoded path traversal detected")
	}

	// 检测 null 字符
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("null characters not allowed")
	}

	return nil
}

func (a *MavenAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	// 路径安全验证
	if err := validateMavenPath(path); err != nil {
		return nil, err
	}

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
			storageVersion := version + "/" + filename
			storageName := groupPath + "/" + artifactID

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

		// 统一使用 version + "/" + filename 作为存储路径，避免路径不一致
		storageVersion := version + "/" + filename

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

func (a *MavenAdapter) handleMetadataXML(ctx context.Context, fullPath string, repo *model.Repository) (*types.ContentResult, error) {
	group := strings.TrimSuffix(fullPath, "/maven-metadata.xml")
	parts := strings.Split(group, "/")
	if len(parts) < 2 {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "metadata not found"},
		}, nil
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

	if isVersionLevelMetadata {
		return a.handleVersionLevelMetadata(ctx, name, version, groupID, artifactID, repo)
	}

	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), name, model.PackageTypeMaven)

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
		if a.metaCache != nil {
			ttl := time.Duration(repo.CacheTTLSeconds) * time.Second
			if ttl <= 0 {
				ttl = 5 * time.Minute
			}

			cacheContent, _, cacheErr := a.metaCache.GetOrFetch(context.Background(), repo.Name, "maven", name+"/maven-metadata.xml", ttl, func() (io.ReadCloser, int64, error) {
				pathInfo, pathErr := a.ParsePath(name)
				if pathErr != nil {
					return nil, 0, pathErr
				}
				remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
				result, fetchErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
				if fetchErr != nil {
					return nil, 0, fetchErr
				}
				return result.Content, result.Size, nil
			})
			if cacheErr == nil {
				body, readErr := io.ReadAll(cacheContent)
				cacheContent.Close()
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

					storageName := groupArtifactToStoragePath(group)
					a.storageSvc.StorePackageWithBackend(context.Background(), repo.Name, "maven", storageName, "maven-metadata.xml", bytes.NewReader(body), int64(len(body)), repositoryStorageBackendID(repo))

					// 生成 ETag
					etag := util.GenerateETag(body)

					return &types.ContentResult{
						StatusCode:  200,
						ContentType: "application/xml",
						ExtraData: map[string]interface{}{
							"xml_body": body,
						},
						Headers: map[string]string{
							"ETag":          etag,
							"Cache-Control": "public, max-age=300",
						},
					}, nil
				}
			}
		}
	}

	// Fallback: 直接远程获取（无需缓存）
	if a.fetcher != nil {
		pathInfo, err := a.ParsePath(name)
		if err != nil {
			logrus.Warnf("failed to resolve package path for %s: %v", name, err)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			remoteCtx := context.Background()
			result, resolveErr := a.fetcher.FetchFromRemote(remoteCtx, repo, remoteURL)
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

					storageName := groupArtifactToStoragePath(group)
					storageKey, storeErr := a.storageSvc.StorePackageWithBackend(context.Background(), repo.Name, "maven", storageName, "maven-metadata.xml", bytes.NewReader(body), int64(len(body)), repositoryStorageBackendID(repo))
					if storeErr == nil {
						_ = storageKey
					}

					// 生成 ETag
					etag := util.GenerateETag(body)

					return &types.ContentResult{
						StatusCode:  200,
						ContentType: "application/xml",
						ExtraData: map[string]interface{}{
							"xml_body": body,
						},
						Headers: map[string]string{
							"ETag":          etag,
							"Cache-Control": "public, max-age=300",
						},
					}, nil
				}
			}
		}
	}

	versions, err := a.ListVersions(context.WithValue(ctx, "repo", repo), name)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "metadata not found"},
		}, nil
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
			pkg, _ = a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), name, model.PackageTypeMaven)
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

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/xml",
		ExtraData: map[string]interface{}{
			"xml_struct": metadata,
		},
	}, nil
}

func (a *MavenAdapter) handleVersionLevelMetadata(ctx context.Context, name, version, groupID, artifactID string, repo *model.Repository) (*types.ContentResult, error) {
	logrus.Infof("handleVersionLevelMetadata: name=%s, version=%s, groupID=%s, artifactID=%s", name, version, groupID, artifactID)

	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), name, model.PackageTypeMaven)
	if err != nil {
		logrus.Warnf("Package not found for SNAPSHOT metadata: %s, will generate from storage", name)
		return a.generateSnapshotMetadataFromStorage(name, version, groupID, artifactID, repo)
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
		return a.generateSnapshotMetadataFromStorage(name, version, groupID, artifactID, repo)
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

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/xml",
		ExtraData: map[string]interface{}{
			"xml_struct": metadata,
		},
	}, nil
}

func (a *MavenAdapter) generateSnapshotMetadataFromStorage(name, version, groupID, artifactID string, repo *model.Repository) (*types.ContentResult, error) {
	logrus.Infof("generateSnapshotMetadataFromStorage: name=%s, version=%s", name, version)

	storageName := strings.ReplaceAll(groupID, ".", "/") + "/" + artifactID

	logrus.Debugf("Listing files with storageName=%s version=%s", storageName, version)
	entries, err := a.storageSvc.ListPackageWithBackend(context.Background(), repo.Name, "maven", storageName, version, repositoryStorageBackendID(repo))
	if err != nil {
		logrus.Warnf("Failed to list SNAPSHOT files: %v", err)
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "SNAPSHOT metadata not found"},
		}, nil
	}

	if len(entries) == 0 {
		logrus.Warnf("No files found for SNAPSHOT version: %s %s", name, version)
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "SNAPSHOT metadata not found"},
		}, nil
	}

	snapshotVersions, latestTimestamp, latestBuildNumber := buildSnapshotVersionsFromEntries(entries, artifactID, version)

	if latestTimestamp == "" {
		logrus.Warnf("No valid SNAPSHOT files found for: %s %s", name, version)
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "SNAPSHOT metadata not found"},
		}, nil
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
	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/xml",
		ExtraData: map[string]interface{}{
			"xml_struct": metadata,
		},
	}, nil
}

func (a *MavenAdapter) generateSnapshotMetadataForChecksum(name, version, groupID, artifactID string, repo *model.Repository) (*MavenMetadata, error) {
	logrus.Infof("generateSnapshotMetadataForChecksum: name=%s, version=%s", name, version)

	storageName := strings.ReplaceAll(groupID, ".", "/") + "/" + artifactID

	logrus.Debugf("Listing files with storageName=%s version=%s", storageName, version)
	entries, err := a.storageSvc.ListPackageWithBackend(context.Background(), repo.Name, "maven", storageName, version, repositoryStorageBackendID(repo))
	if err != nil {
		logrus.Warnf("Failed to list SNAPSHOT files: %v", err)
		return nil, err
	}

	if len(entries) == 0 {
		logrus.Warnf("No files found for SNAPSHOT version: %s %s", name, version)
		return nil, fmt.Errorf("no SNAPSHOT files found")
	}

	_, latestTimestamp, latestBuildNumber := buildSnapshotVersionsFromEntries(entries, artifactID, version)

	if latestTimestamp == "" {
		logrus.Warnf("No valid SNAPSHOT files found for: %s %s", name, version)
		return nil, fmt.Errorf("no valid SNAPSHOT files found")
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
	return metadata, nil
}

type mavenSnapshotArtifact struct {
	extension   string
	classifier  string
	value       string
	updated     string
	timestamp   string
	buildNumber int
}

var mavenSnapshotBuildPattern = regexp.MustCompile(`^(\d{8}\.\d{6})-(\d+)(?:-([^-]+))?\.([^.]+)$`)

func buildSnapshotVersionsFromEntries(entries []storage.Entry, artifactID, snapshotVersion string) ([]MavenSnapshotVersion, string, int) {
	latestByKind := make(map[string]mavenSnapshotArtifact)
	var latestTimestamp string
	var latestBuildNumber int

	for _, entry := range entries {
		if entry.IsDir {
			continue
		}

		artifact, ok := parseMavenSnapshotFilename(filepath.Base(entry.Key), artifactID, snapshotVersion)
		if !ok {
			continue
		}

		if compareSnapshotBuild(artifact.timestamp, artifact.buildNumber, latestTimestamp, latestBuildNumber) > 0 {
			latestTimestamp = artifact.timestamp
			latestBuildNumber = artifact.buildNumber
		}

		key := artifact.extension + ":" + artifact.classifier
		if existing, exists := latestByKind[key]; !exists || compareSnapshotBuild(artifact.timestamp, artifact.buildNumber, existing.timestamp, existing.buildNumber) > 0 {
			latestByKind[key] = artifact
		}
	}

	snapshotVersions := make([]MavenSnapshotVersion, 0, len(latestByKind))
	for _, artifact := range latestByKind {
		snapshotVersions = append(snapshotVersions, MavenSnapshotVersion{
			Extension:  artifact.extension,
			Classifier: artifact.classifier,
			Value:      artifact.value,
			Updated:    artifact.updated,
		})
	}

	sort.Slice(snapshotVersions, func(i, j int) bool {
		if snapshotVersions[i].Extension == snapshotVersions[j].Extension {
			return snapshotVersions[i].Classifier < snapshotVersions[j].Classifier
		}
		return snapshotVersions[i].Extension < snapshotVersions[j].Extension
	})

	return snapshotVersions, latestTimestamp, latestBuildNumber
}

func parseMavenSnapshotFilename(filename, artifactID, snapshotVersion string) (mavenSnapshotArtifact, bool) {
	baseVersion := strings.TrimSuffix(snapshotVersion, "-SNAPSHOT")
	prefix := artifactID + "-" + baseVersion + "-"
	if !strings.HasPrefix(filename, prefix) {
		return mavenSnapshotArtifact{}, false
	}

	rest := strings.TrimPrefix(filename, prefix)
	matches := mavenSnapshotBuildPattern.FindStringSubmatch(rest)
	if matches == nil {
		return mavenSnapshotArtifact{}, false
	}

	timestamp := matches[1]
	buildNumberStr := matches[2]
	classifier := matches[3]
	extension := matches[4]

	buildNumber := 0
	if _, err := fmt.Sscanf(buildNumberStr, "%d", &buildNumber); err != nil || buildNumber <= 0 {
		return mavenSnapshotArtifact{}, false
	}

	// 构建完整的 SNAPSHOT 版本值
	value := baseVersion + "-" + timestamp + "-" + buildNumberStr
	if classifier != "" {
		value += "-" + classifier
	}
	updated := strings.ReplaceAll(timestamp, ".", "")

	return mavenSnapshotArtifact{
		extension:   extension,
		classifier:  classifier,
		value:       value,
		updated:     updated,
		timestamp:   timestamp,
		buildNumber: buildNumber,
	}, true
}

// resolveSnapshotVersion 将 SNAPSHOT 版本解析为具体的 timestamp-buildNumber 版本
// 例如: 1.0-SNAPSHOT -> 1.0-20250119.001234-5
func (a *MavenAdapter) resolveSnapshotVersion(ctx context.Context, name, version, groupID, artifactID string, repo *model.Repository) (string, error) {
	// 如果不是 SNAPSHOT 版本，直接返回
	if !strings.HasSuffix(version, "-SNAPSHOT") {
		return version, nil
	}

	// 先尝试从数据库获取
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), name, model.PackageTypeMaven)
	if err == nil && pkg != nil {
		for _, v := range pkg.Versions {
			if v.Version == version {
				var meta map[string]interface{}
				if v.Metadata != "" {
					if err := json.Unmarshal([]byte(v.Metadata), &meta); err == nil {
						if timestamp, ok := meta["snapshotTimestamp"].(string); ok {
							if buildNumber, ok := meta["snapshotBuildNumber"].(float64); ok {
								baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
								resolved := fmt.Sprintf("%s-%s-%d", baseVersion, timestamp, int(buildNumber))
								logrus.Infof("Resolved SNAPSHOT from DB: %s -> %s", version, resolved)
								return resolved, nil
							}
						}
					}
				}
				break
			}
		}
	}

	// 从存储扫描获取最新的 SNAPSHOT 版本
	metadata, err := a.generateSnapshotMetadataForChecksum(name, version, groupID, artifactID, repo)
	if err == nil && metadata.Versioning.Snapshot != nil {
		baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
		resolved := fmt.Sprintf("%s-%s-%d", baseVersion, metadata.Versioning.Snapshot.Timestamp, metadata.Versioning.Snapshot.BuildNumber)
		logrus.Infof("Resolved SNAPSHOT from storage: %s -> %s", version, resolved)
		return resolved, nil
	}

	// Proxy 模式下：从上游远程仓库获取最新的 SNAPSHOT 元数据
	if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		resolved, fetchErr := a.fetchSnapshotVersionFromRemote(ctx, name, version, groupID, artifactID, repo)
		if fetchErr == nil && resolved != "" {
			logrus.Infof("Resolved SNAPSHOT from remote: %s -> %s", version, resolved)
			return resolved, nil
		}
		logrus.Warnf("Failed to resolve SNAPSHOT from remote: %v", fetchErr)
	}

	return version, nil
}

// fetchSnapshotVersionFromRemote 从上游 Maven 仓库获取 SNAPSHOT 的最新 timestamp-buildNumber
func (a *MavenAdapter) fetchSnapshotVersionFromRemote(ctx context.Context, name, version, groupID, artifactID string, repo *model.Repository) (string, error) {
	// 构建 maven-metadata.xml 的远程 URL
	// 格式: https://repo1.maven.org/maven2/com/example/lib/1.0-SNAPSHOT/maven-metadata.xml
	storagePath := strings.ReplaceAll(groupID, ".", "/") + "/" + artifactID + "/" + version + "/maven-metadata.xml"
	remoteURL := strings.TrimSuffix(repo.RemoteURL, "/") + "/" + storagePath

	logrus.Infof("Fetching SNAPSHOT metadata from remote: %s", remoteURL)

	result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch SNAPSHOT metadata: %w", err)
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return "", fmt.Errorf("failed to read SNAPSHOT metadata: %w", readErr)
	}

	// 解析 maven-metadata.xml
	var metadata MavenMetadata
	if parseErr := xml.Unmarshal(body, &metadata); parseErr != nil {
		return "", fmt.Errorf("failed to parse SNAPSHOT metadata: %w", parseErr)
	}

	if metadata.Versioning.Snapshot == nil || metadata.Versioning.Snapshot.Timestamp == "" {
		return "", fmt.Errorf("no snapshot timestamp in metadata")
	}

	baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
	resolved := fmt.Sprintf("%s-%s-%d", baseVersion, metadata.Versioning.Snapshot.Timestamp, metadata.Versioning.Snapshot.BuildNumber)
	return resolved, nil
}

// resolveSnapshotFilename 解析带 classifier 的 SNAPSHOT 文件名
// 例如: lib-1.0-SNAPSHOT-sources.jar -> lib-1.0-20250119.001234-5-sources.jar
// 例如: lib-1.0-SNAPSHOT.jar -> lib-1.0-20250119.001234-5.jar
func resolveSnapshotFilename(filename, resolvedVersion, artifactID string) (string, string) {
	// 去掉 -SNAPSHOT 后缀
	baseVersion := strings.TrimSuffix(resolvedVersion, "-SNAPSHOT")

	// 尝试提取 classifier
	// 匹配 pattern: artifactID-baseVersion[-classifier].extension
	snapshotPattern := regexp.MustCompile(`^(` + regexp.QuoteMeta(artifactID) + `-(.+?))(?:-([^-]+))?(\.[^.]+)$`)
	matches := snapshotPattern.FindStringSubmatch(filename)

	if matches != nil {
		// 匹配成功，替换版本部分
		newBase := artifactID + "-" + baseVersion
		classifier := matches[3]
		extension := matches[4]

		if classifier != "" {
			return newBase + "-" + classifier + extension, classifier
		}
		return newBase + extension, ""
	}

	// 备选方案：简单替换
	newFilename := strings.Replace(filename, "-SNAPSHOT", "", 1)
	return newFilename, ""
}

func compareSnapshotBuild(timestamp string, buildNumber int, otherTimestamp string, otherBuildNumber int) int {
	if timestamp > otherTimestamp {
		return 1
	}
	if timestamp < otherTimestamp {
		return -1
	}
	if buildNumber > otherBuildNumber {
		return 1
	}
	if buildNumber < otherBuildNumber {
		return -1
	}
	return 0
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

func groupArtifactToStoragePath(groupArtifact string) string {
	return groupArtifact
}

func mavenNameToStoragePath(name string) string {
	if strings.Contains(name, ":") {
		parts := strings.SplitN(name, ":", 2)
		if len(parts) == 2 {
			return strings.ReplaceAll(parts[0], ".", "/") + "/" + parts[1]
		}
	}
	return name
}

func groupArtifactToName(groupArtifact string) string {
	lastSlash := strings.LastIndex(groupArtifact, "/")
	if lastSlash == -1 {
		return groupArtifact
	}
	groupID := strings.ReplaceAll(groupArtifact[:lastSlash], "/", ".")
	artifactID := groupArtifact[lastSlash+1:]
	return groupID + ":" + artifactID
}

func (a *MavenAdapter) getStorageNamesForGroupArtifact(groupArtifact string, parts []string) []string {
	storageNames := make([]string, 0, 2)

	storageNames = append(storageNames, groupArtifactToStoragePath(groupArtifact))

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

func (a *MavenAdapter) findPackageInStorage(ctx context.Context, repoName, pkgType string, storageNames []string, storageVersion string, backendID uint) (io.ReadCloser, int64, string, error) {
	for _, storageName := range storageNames {
		content, size, err := a.storageSvc.GetPackageWithBackend(ctx, repoName, pkgType, storageName, storageVersion, backendID)
		if err == nil {
			return content, size, storageName, nil
		}
	}
	return nil, 0, "", fmt.Errorf("package not found in storage")
}

func (a *MavenAdapter) handleDownloadArtifact(fullPath string, repo *model.Repository) (*types.ContentResult, error) {
	logrus.Infof("handleDownloadArtifact called: fullPath=%s", fullPath)

	if strings.HasSuffix(fullPath, ".sha1") || strings.HasSuffix(fullPath, ".md5") {
		logrus.Infof("Detected checksum file, calling handleChecksumRequest")
		return a.handleChecksumRequest(fullPath, repo)
	}

	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "artifact not found"},
		}, nil
	}

	version := parts[len(parts)-2]
	filename := parts[len(parts)-1]
	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	pkgName := groupArtifactToName(groupArtifact)
	artifactID := parts[len(parts)-3]
	groupID := strings.ReplaceAll(groupArtifact, "/", ".")

	logrus.Infof("Parsed: version=%s, filename=%s, groupArtifact=%s, pkgName=%s", version, filename, groupArtifact, pkgName)

	// SNAPSHOT 版本重写：将 1.0-SNAPSHOT 转换为 1.0-timestamp-buildNumber
	var resolvedVersion string
	var resolvedFilename string
	isSnapshot := strings.HasSuffix(version, "-SNAPSHOT")

	if isSnapshot {
		// 解析 SNAPSHOT 版本
		resolvedVersion, _ = a.resolveSnapshotVersion(context.Background(), pkgName, version, groupID, artifactID, repo)
		// 解析带 classifier 的文件名
		resolvedFilename, _ = resolveSnapshotFilename(filename, resolvedVersion, artifactID)
		logrus.Infof("SNAPSHOT resolved: version %s -> %s, filename %s -> %s", version, resolvedVersion, filename, resolvedFilename)
	} else {
		resolvedVersion = version
		resolvedFilename = filename
	}

	// 使用解析后的版本和文件名构建存储路径
	storageVersion := resolvedVersion + "/" + resolvedFilename

	storageNames := a.getStorageNamesForGroupArtifact(groupArtifact, parts)
	logrus.Infof("Maven download: looking for %s in storage names=%v, version=%s, filename=%s", pkgName, storageNames, storageVersion, resolvedFilename)
	content, size, _, err := a.findPackageInStorage(context.Background(), repo.Name, "maven", storageNames, storageVersion, repositoryStorageBackendID(repo))

	if err == nil {
		logrus.Infof("Maven cache hit: found %s in storage, size=%d", pkgName, size)

		// 读取内容用于计算 ETag
		body, readErr := io.ReadAll(content)
		content.Close()
		if readErr != nil {
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:   map[string]interface{}{"error": "failed to read content"},
			}, nil
		}

		// 生成 ETag（基于内容 SHA256）
		etag := util.GenerateETag(body)
		lastModified := time.Now().UTC().Format(time.RFC1123)

		contentType := a.storageSvc.GetContentType(resolvedFilename)
		// 响应头中使用客户端请求的原始文件名
		returnFilename := filename
		return &types.ContentResult{
			StatusCode:  200,
			ContentType: contentType,
			Content:     io.NopCloser(bytes.NewReader(body)),
			Size:        int64(len(body)),
			Headers: map[string]string{
				"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, returnFilename),
				"ETag":                etag,
				"Last-Modified":       lastModified,
				"Cache-Control":       "public, max-age=86400",
			},
		}, nil
	}

	// 本地存储未命中，尝试代理
	if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		logrus.Infof("Maven proxy: fetching from remote for %s, version=%s, filename=%s", pkgName, version, filename)
		pathInfo, err := a.ParsePath(pkgName + ":" + version + "/" + filename)
		var fetchErr error
		if err != nil {
			logrus.Warnf("failed to resolve package path: %v", err)
			fetchErr = err
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				logrus.Infof("Maven proxy: successfully fetched %s from remote, size=%d", pkgName, result.Size)
				contentType := a.storageSvc.GetContentType(filename)

				// 如果是 SNAPSHOT，代理下载后存储时使用解析后的文件名
				if isSnapshot && resolvedFilename != filename {
					logrus.Infof("Storing SNAPSHOT artifact with resolved filename: %s -> %s", filename, resolvedFilename)
					// 读取内容并存储为解析后的文件名
					body, readErr := io.ReadAll(result.Content)
					result.Content.Close()
					if readErr == nil {
						// 存储解析后的文件
						reader := bytes.NewReader(body)
						_, storeErr := a.storageSvc.StorePackageWithBackend(context.Background(), repo.Name, "maven", groupArtifact, resolvedFilename, reader, int64(len(body)), repositoryStorageBackendID(repo))
						if storeErr != nil {
							logrus.Warnf("Failed to store SNAPSHOT artifact: %v", storeErr)
						}
						// 生成 ETag
						etag := util.GenerateETag(body)
						// 返回内容
						return &types.ContentResult{
							StatusCode:  200,
							ContentType: contentType,
							Content:     io.NopCloser(bytes.NewReader(body)),
							Size:        int64(len(body)),
							Headers: map[string]string{
								"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
								"ETag":               etag,
								"Cache-Control":       "public, max-age=86400",
							},
						}, nil
					}
				}

				// 读取代理内容并计算 ETag
				body, readErr := io.ReadAll(result.Content)
				result.Content.Close()
				if readErr == nil {
					etag := util.GenerateETag(body)
					return &types.ContentResult{
						StatusCode:  200,
						ContentType: contentType,
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
						Headers: map[string]string{
							"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
							"ETag":               etag,
							"Cache-Control":       "public, max-age=86400",
						},
					}, nil
				}

				return &types.ContentResult{
					StatusCode:  200,
					ContentType: contentType,
					Content:     result.Content,
					Size:        result.Size,
					Headers: map[string]string{
						"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
					},
				}, nil
			}
		}
		logrus.Warnf("Maven proxy: failed to fetch %s from remote: %v", pkgName, fetchErr)
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"error": "artifact not found"},
	}, nil
}

func (a *MavenAdapter) handleChecksumRequest(fullPath string, repo *model.Repository) (*types.ContentResult, error) {
	logrus.Infof("handleChecksumRequest called: fullPath=%s", fullPath)
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "checksum not found"},
		}, nil
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
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "checksum not found"},
		}, nil
	}

	logrus.Infof("Checksum type: %s, actualFilename: %s", checksumType, actualFilename)

	if actualFilename == "maven-metadata.xml" {
		return a.handleMetadataChecksum(context.Background(), fullPath, repo, checksumType, actualFilename)
	}

	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	version := parts[len(parts)-2]
	artifactID := parts[len(parts)-3]
	groupID := strings.ReplaceAll(groupArtifact, "/", ".")
	pkgName := groupArtifactToName(groupArtifact)

	logrus.Infof("GroupArtifact: %s, version: %s", groupArtifact, version)

	// SNAPSHOT 版本重写
	var resolvedVersion string
	var resolvedFilename string
	isSnapshot := strings.HasSuffix(version, "-SNAPSHOT")

	if isSnapshot {
		resolvedVersion, _ = a.resolveSnapshotVersion(context.Background(), pkgName, version, groupID, artifactID, repo)
		resolvedFilename, _ = resolveSnapshotFilename(actualFilename, resolvedVersion, artifactID)
		logrus.Infof("SNAPSHOT checksum resolved: version %s -> %s, filename %s -> %s", version, resolvedVersion, actualFilename, resolvedFilename)
	} else {
		resolvedVersion = version
		resolvedFilename = actualFilename
	}

	storageName := groupArtifactToStoragePath(groupArtifact)
	// 统一使用 version + "/" + filename 作为存储路径，与 ParsePath 保持一致
	storageVersion := resolvedVersion + "/" + resolvedFilename

	logrus.Infof("Looking for file in storage: storageName=%s, storageVersion=%s", storageName, storageVersion)

	content, _, err := a.storageSvc.GetPackageWithBackend(context.Background(), repo.Name, "maven", storageName, storageVersion, repositoryStorageBackendID(repo))

	if err != nil {
		if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
			// 使用原始路径构建远程 URL（包含 groupId/artifactId/version/filename.sha1）
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), fullPath)
			result, fetchErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					checksum := calculateChecksum(body, checksumType)
					return &types.ContentResult{
						StatusCode:  200,
						ContentType: "text/plain",
						ExtraData: map[string]interface{}{
							"checksum_string": fmt.Sprintf("%s  %s", checksum, actualFilename),
						},
					}, nil
				}
			}
		}
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "file not found"},
		}, nil
	}
	defer content.Close()

	body, _ := io.ReadAll(content)
	checksum := calculateChecksum(body, checksumType)

	// 返回的文件名使用客户端请求的原始文件名
	returnFilename := actualFilename
	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/plain",
		ExtraData: map[string]interface{}{
			"checksum_string": fmt.Sprintf("%s  %s", checksum, returnFilename),
		},
	}, nil
}

func (a *MavenAdapter) handleMetadataChecksum(ctx context.Context, fullPath string, repo *model.Repository, checksumType, actualFilename string) (*types.ContentResult, error) {
	group := strings.TrimSuffix(fullPath, "/"+actualFilename)
	parts := strings.Split(group, "/")
	if len(parts) < 2 {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "metadata not found"},
		}, nil
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
		pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), name, model.PackageTypeMaven)
		if err != nil {
			logrus.Warnf("Package not found for SNAPSHOT metadata checksum: %s, will generate from storage", name)
			metadata, err = a.generateSnapshotMetadataForChecksum(name, version, groupID, artifactID, repo)
			if err != nil {
				return &types.ContentResult{
					StatusCode: 404,
					ExtraData:  map[string]interface{}{"error": "metadata not found"},
				}, nil
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
				metadata, err = a.generateSnapshotMetadataForChecksum(name, version, groupID, artifactID, repo)
				if err != nil {
					return &types.ContentResult{
						StatusCode: 404,
						ExtraData:  map[string]interface{}{"error": "version not found"},
					}, nil
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
		versions, err := a.ListVersions(context.WithValue(ctx, "repo", repo), name)
		if err != nil {
			return &types.ContentResult{
				StatusCode: 404,
				ExtraData:  map[string]interface{}{"error": "metadata not found"},
			}, nil
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
			pkg, _ := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repo), name, model.PackageTypeMaven)
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
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"error": "failed to generate metadata"},
		}, nil
	}

	metadataBytes := []byte(xml.Header + string(metadataXML))
	checksum := calculateChecksum(metadataBytes, checksumType)

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/plain",
		ExtraData: map[string]interface{}{
			"checksum_string": fmt.Sprintf("%s  %s", checksum, actualFilename),
		},
	}, nil
}

// handleGPGSignature 处理 GPG 签名文件 (.asc) 请求
func (a *MavenAdapter) handleGPGSignature(fullPath string, repo *model.Repository) (*types.ContentResult, error) {
	logrus.Infof("handleGPGSignature called: fullPath=%s", fullPath)

	// GPG 签名文件路径对应其源文件的路径
	// 例如: com/example/lib/1.0/mylib-1.0.jar.asc 对应 com/example/lib/1.0/mylib-1.0.jar
	actualFilename := strings.TrimSuffix(fullPath, ".asc")
	parts := strings.Split(actualFilename, "/")

	if len(parts) < 4 {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:   map[string]interface{}{"error": "GPG signature not found"},
		}, nil
	}

	filename := parts[len(parts)-1]
	version := parts[len(parts)-2]
	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	artifactID := parts[len(parts)-3]
	groupID := strings.ReplaceAll(groupArtifact, "/", ".")
	pkgName := groupArtifactToName(groupArtifact)

	// SNAPSHOT 版本重写
	var resolvedVersion string
	var resolvedFilename string
	isSnapshot := strings.HasSuffix(version, "-SNAPSHOT")

	if isSnapshot {
		resolvedVersion, _ = a.resolveSnapshotVersion(context.Background(), pkgName, version, groupID, artifactID, repo)
		resolvedFilename, _ = resolveSnapshotFilename(filename, resolvedVersion, artifactID)
		logrus.Infof("SNAPSHOT GPG resolved: version %s -> %s, filename %s -> %s", version, resolvedVersion, filename, resolvedFilename)
	} else {
		resolvedVersion = version
		resolvedFilename = filename
	}

	// 统一使用 version + "/" + filename 作为存储路径
	storageVersion := resolvedVersion + "/" + resolvedFilename
	storageName := groupArtifactToStoragePath(groupArtifact)

	logrus.Infof("GPG signature: looking for %s in storage names=%v, version=%s", filename, storageName, storageVersion)

	// 尝试从本地存储获取
	content, _, _, err := a.findPackageInStorage(context.Background(), repo.Name, "maven", []string{storageName}, storageVersion, repositoryStorageBackendID(repo))

	if err == nil {
		defer content.Close()
		body, readErr := io.ReadAll(content)
		if readErr == nil {
			// 生成 ETag
			etag := util.GenerateETag(body)
			lastModified := time.Now().UTC().Format(time.RFC1123)

			return &types.ContentResult{
				StatusCode:  200,
				ContentType: "application/pgp-signature",
				Content:     io.NopCloser(bytes.NewReader(body)),
				Size:        int64(len(body)),
				Headers: map[string]string{
					"Content-Disposition": fmt.Sprintf(`attachment; filename="%s.asc"`, filename),
					"ETag":               etag,
					"Last-Modified":      lastModified,
					"Cache-Control":      "public, max-age=86400",
				},
			}, nil
		}
	}

	// 代理到远程仓库
	if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		pkgName := groupArtifactToName(groupArtifact)
		pathInfo, pathErr := a.ParsePath(pkgName + ":" + version + "/" + filename)
		var fetchErr error
		if pathErr != nil {
			logrus.Warnf("failed to resolve package path for GPG: %v", pathErr)
			fetchErr = pathErr
		} else {
			remoteURL := fmt.Sprintf("%s/%s.asc", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, resultErr := a.fetcher.FetchFromRemote(context.Background(), repo, remoteURL)
			if resultErr != nil {
				fetchErr = resultErr
			} else if result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					etag := util.GenerateETag(body)
					lastModified := time.Now().UTC().Format(time.RFC1123)

					return &types.ContentResult{
						StatusCode:  200,
						ContentType: "application/pgp-signature",
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
						Headers: map[string]string{
							"Content-Disposition": fmt.Sprintf(`attachment; filename="%s.asc"`, filename),
							"ETag":               etag,
							"Last-Modified":      lastModified,
							"Cache-Control":      "public, max-age=86400",
						},
					}, nil
				}
			}
		}
		if fetchErr != nil {
			logrus.Warnf("Maven proxy: failed to fetch GPG signature %s from remote: %v", fullPath, fetchErr)
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:   map[string]interface{}{"error": "GPG signature not found"},
	}, nil
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

func (a *MavenAdapter) handleIndexRequest(repo *model.Repository) (*types.ContentResult, error) {
	if repo == nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "repository not found"},
		}, nil
	}

	var packages []model.Package
	err := a.GetPackageRepository().DB().
		Preload("Versions").
		Where("repository_id = ?", repo.ID).
		Find(&packages).
		Error

	if err != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"error": "failed to query packages"},
		}, nil
	}

	index := a.generateIndex(packages, repo.Name)

	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		ExtraData: map[string]interface{}{
			"index_json": index,
			"index_xml":  index,
		},
	}, nil
}

func (a *MavenAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetRepositoryPackageMetadata(ctx, repositoryFromContext(ctx), name, model.PackageTypeMaven, MavenType)
}

func (a *MavenAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	storageName := mavenNameToStoragePath(identity.Name)
	repo, _ := ctx.Value("repo").(*model.Repository)
	repoName := ""
	if repo != nil {
		repoName = repo.Name
	}
	backendID := repositoryStorageBackendID(repo)

	logrus.Infof("Maven delete: attempting to delete package=%s, version=%s, storageName=%s, repo=%s",
		identity.Name, identity.Version, storageName, repoName)

	entries, err := a.storageSvc.ListPackageWithBackend(ctx, repoName, "maven", storageName, identity.Version, backendID)
	if err != nil {
		logrus.Warnf("Maven delete: failed to list storage entries: %v", err)
	} else {
		logrus.Infof("Maven delete: found %d storage entries to delete", len(entries))
		for _, entry := range entries {
			if delErr := a.storageSvc.DeleteStorageKeyWithBackend(ctx, entry.Key, backendID); delErr != nil {
				logrus.Errorf("Maven delete: failed to delete storage key %s: %v", entry.Key, delErr)
			} else {
				logrus.Infof("Maven delete: successfully deleted storage key %s", entry.Key)
			}
		}
	}

	if len(entries) == 0 {
		logrus.Warnf("Maven delete: no storage entries found, attempting direct path deletion")
		directKey := fmt.Sprintf("maven/%s/%s/%s/%s", repoName, storageName, identity.Version, identity.Version+".jar")
		if delErr := a.storageSvc.DeleteStorageKeyWithBackend(ctx, directKey, backendID); delErr != nil {
			logrus.Warnf("Maven delete: direct path deletion also failed: %v", delErr)
		} else {
			logrus.Infof("Maven delete: successfully deleted using direct path %s", directKey)
		}
	}

	return a.GetPackageRepository().DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypeMaven)
}

func (a *MavenAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeMaven)
}

// ParseIntent 解析请求路径为意图
func (a *MavenAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = trimLeadingSlash(path)

	if path == "index" || strings.HasSuffix(path, "/index") {
		return &types.RequestIntent{
			Type:  types.RequestList,
			Path:  path,
			Extra: make(map[string]interface{}),
		}
	}

	if strings.HasSuffix(path, "maven-metadata.xml") {
		return &types.RequestIntent{
			Type:  types.RequestMetadata,
			Path:  path,
			Name:  path[:len(path)-len("/maven-metadata.xml")],
			Extra: make(map[string]interface{}),
		}
	}

	if strings.HasSuffix(path, ".sha1") || strings.HasSuffix(path, ".md5") {
		return &types.RequestIntent{
			Type:     types.RequestChecksum,
			Path:     path,
			Filename: filepath.Base(path),
			Extra:    make(map[string]interface{}),
		}
	}

	// GPG 签名文件 (.asc)
	if strings.HasSuffix(path, ".asc") {
		return &types.RequestIntent{
			Type:     types.RequestGPG,
			Path:     path,
			Filename: filepath.Base(path),
			Extra:    make(map[string]interface{}),
		}
	}

	pathInfo, err := a.ParsePath(path)
	intent := &types.RequestIntent{
		Type:        types.RequestDownload,
		Path:        path,
		PkgPathInfo: pathInfo,
		Extra:       make(map[string]interface{}),
	}

	if err == nil {
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *MavenAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	switch intent.Type {
	case types.RequestList:
		return a.handleIndexRequest(repo)
	case types.RequestMetadata:
		return a.handleMetadataXML(ctx, intent.Path, repo)
	case types.RequestChecksum:
		return a.handleChecksumRequest(intent.Path, repo)
	case types.RequestGPG:
		return a.handleGPGSignature(intent.Path, repo)
	case types.RequestDownload:
		return a.handleDownloadArtifact(intent.Path, repo)
	default:
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"error": "unknown request type"},
		}, nil
	}
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
	// Maven 版本比较：按点号和连字符分段，数字段按数值比较，非数字段按字典序比较
	// SNAPSHOT 版本低于对应的 release 版本
	s1 := mavenVersionSegments(v1)
	s2 := mavenVersionSegments(v2)

	for i := 0; i < len(s1) && i < len(s2); i++ {
		if s1[i] == s2[i] {
			continue
		}
		// SNAPSHOT 段低于同级的 release 段
		if s1[i] == "SNAPSHOT" {
			return -1
		}
		if s2[i] == "SNAPSHOT" {
			return 1
		}
		// 尝试数值比较
		n1, err1 := strconv.Atoi(s1[i])
		n2, err2 := strconv.Atoi(s2[i])
		if err1 == nil && err2 == nil {
			if n1 < n2 {
				return -1
			}
			return 1
		}
		// 混合情况：数字 < 字母
		if err1 == nil {
			return -1
		}
		if err2 == nil {
			return 1
		}
		// 都是字符串，按字典序
		if s1[i] < s2[i] {
			return -1
		}
		return 1
	}
	// 较短的版本较低，除非短版本以 SNAPSHOT 结尾
	if len(s1) < len(s2) {
		return -1
	}
	if len(s1) > len(s2) {
		return 1
	}
	return 0
}

// mavenVersionSegments 将 Maven 版本字符串拆分为可比较的段
// 例如 "1.2.3-SNAPSHOT" → ["1", "2", "3", "SNAPSHOT"]
// "2.0.0-RC1" → ["2", "0", "0", "RC1"]
func mavenVersionSegments(v string) []string {
	// 将点号和连字符都作为分隔符
	v = strings.ReplaceAll(v, "-", ".")
	parts := strings.Split(v, ".")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
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

func (a *MavenAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	fullPath := trimLeadingSlash(c.Param("path"))
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

	storageVersion := joinVersionFilename(version, filename)

	// 构建标准 Maven 下载路径：/repository/{repo}/{groupId}/{artifactId}/{version}/{filename}
	// groupId 用斜杠分隔：com.google.guava -> com/google/guava
	groupPath := strings.ReplaceAll(groupID, ".", "/")
	downloadURL := "/" + groupPath + "/" + artifactID + "/" + version + "/" + filename

	return &types.PublishResult{
		PackageName:    name,
		StorageName:    groupPath + "/" + artifactID,
		Version:        version,
		Filename:       filename,
		Content:        bytes.NewReader(body),
		Size:           size,
		FileType:       getMavenFileType(filename),
		StorageVersion: storageVersion,
		DownloadURL:    downloadURL,
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
	fullPath := trimLeadingSlash(c.Param("path"))
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		return fmt.Errorf("invalid path: expected group/artifact/version/filename")
	}

	groupID := strings.Join(parts[:len(parts)-3], "/")
	groupID = strings.ReplaceAll(groupID, "/", ".")
	artifactID := parts[len(parts)-3]
	version := parts[len(parts)-2]

	groupArtifact := groupID + "/" + artifactID
	name := groupArtifactToName(groupArtifact)
	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    MavenType,
	}
	deleteCtx := context.WithValue(c.Request.Context(), "repo", ctx.Repo)
	if err := a.Delete(deleteCtx, identity); err != nil {
		return err
	}

	return nil
}
