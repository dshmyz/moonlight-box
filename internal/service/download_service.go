package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"hash/fnv"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

// PyPI CDN path resolver cache.
// Maps "filename" -> "hash_prefix1/hash_prefix2/full_hash/filename"
// Uses sharded locks to reduce contention under high concurrency.
const (
	pypiCDNNumShards = 16
	pypiCDNCacheMax  = 50000
	pypiCDNCacheTTL  = 1 * time.Hour
)

type pypiCDNShard struct {
	mu    sync.RWMutex
	store map[string]pypiCDNCacheEntry
}

type pypiCDNCacheEntry struct {
	url      string
	expireAt time.Time
}

var pypiCDNCaches [pypiCDNNumShards]*pypiCDNShard

func init() {
	for i := 0; i < pypiCDNNumShards; i++ {
		pypiCDNCaches[i] = &pypiCDNShard{store: make(map[string]pypiCDNCacheEntry)}
	}
}

func pypiCDNShardIndex(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32() % pypiCDNNumShards
}

// resolvePyPIRemoteURL resolves a PyPI filename to a full download URL through the configured proxy.
// Instead of hardcoding files.pythonhosted.org, it queries the upstream simple index via the proxy
// to find the actual download URL, which respects the configured mirror/proxy for internal networks.
func resolvePyPIRemoteURL(fetcher proxy.ProxyFetcher, downloadCtx *types.DownloadContext, pathInfo *types.PackagePathInfo) string {
	filename := pathInfo.Filename
	if filename == "" {
		return ""
	}

	remotePath := pathInfo.RemotePath
	parts := strings.Split(strings.TrimPrefix(remotePath, "packages/"), "/")
	if len(parts) > 1 {
		// Already has hash-prefix directories: construct via RemoteURL
		baseURL := strings.TrimSuffix(downloadCtx.Repo.RemoteURL, "/")
		return baseURL + "/" + remotePath
	}

	// Check cache first
	shardIdx := pypiCDNShardIndex(filename)
	shard := pypiCDNCaches[shardIdx]
	shard.mu.RLock()
	entry, ok := shard.store[filename]
	shard.mu.RUnlock()
	if ok && time.Now().Before(entry.expireAt) {
		return entry.url
	}

	pkgName := pathInfo.Name
	if pkgName == "" {
		pkgName = normalizePyPNameFromFilename(filename)
	}
	baseURL := strings.TrimSuffix(downloadCtx.Repo.RemoteURL, "/")

	// Try multiple name variations (PyPI normalizes: hyphens, underscores, dots)
	nameVariations := []string{pkgName, strings.ReplaceAll(pkgName, "-", "_")}

	for _, tryName := range nameVariations {
		if tryName == "" {
			continue
		}
		simpleURL := baseURL + "/simple/" + tryName + "/"
		result, err := fetcher.FetchFromRemote(context.Background(), downloadCtx.Repo, simpleURL)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(result.Content)
		result.Content.Close()
		if readErr != nil {
			continue
		}

		// Parse HTML to find the filename and extract the download URL
		pattern := regexp.MustCompile(`href="([^"]*` + regexp.QuoteMeta(filename) + `(?:#[^"]*)?)"`)
		matches := pattern.FindAllStringSubmatch(string(body), -1)
		if len(matches) > 0 {
			href := matches[0][1]
			urlNoFragment := href
			if idx := strings.Index(href, "#"); idx != -1 {
				urlNoFragment = href[:idx]
			}

			var fullURL string
			if strings.HasPrefix(urlNoFragment, "http://") || strings.HasPrefix(urlNoFragment, "https://") {
				// Absolute URL from simple index — use it directly
				// (internal mirror returns mirror URLs, pypi.org returns files.pythonhosted.org)
				fullURL = urlNoFragment
			} else {
				// Relative URL: resolve against RemoteURL
				relPath := strings.TrimPrefix(urlNoFragment, "/")
				fullURL = baseURL + "/" + relPath
			}

			// Cache the result
			shard.mu.Lock()
			if len(shard.store) >= pypiCDNCacheMax/pypiCDNNumShards {
				// Evict expired entries
				now := time.Now()
				for k, v := range shard.store {
					if now.After(v.expireAt) {
						delete(shard.store, k)
					}
				}
			}
			shard.store[filename] = pypiCDNCacheEntry{url: fullURL, expireAt: time.Now().Add(pypiCDNCacheTTL)}
			shard.mu.Unlock()

			return fullURL
		}
	}

	// Fallback: try via RemoteURL directly
	return baseURL + "/packages/" + filename
}

func normalizePyPNameFromFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".whl")
	name = strings.TrimSuffix(name, ".tar.gz")
	name = strings.TrimSuffix(name, ".zip")
	// Remove version and platform tags: package-1.0.0-py3-none-any.whl -> package
	if idx := strings.LastIndex(name, "-"); idx != -1 {
		// Check if the part after the last dash looks like a version
		potentialVersion := name[idx+1:]
		if len(potentialVersion) > 0 && (potentialVersion[0] >= '0' && potentialVersion[0] <= '9') {
			name = name[:idx]
		}
	}
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

type componentAssetIDs struct {
	componentID uint
	assetID     uint
	expires     time.Time
}

type DownloadService struct {
	compRepo      *repository.ComponentRepository
	storageSvc   *StorageService
	fetcher      proxy.ProxyFetcher
	logRepo      *repository.ProxyDownloadLogRepository
	logBatcher   *LogBatcher
	countBatcher *DownloadCountBatcher
	inflight     singleflight.Group

	depParseSem    *semaphore.Weighted // 限制依赖解析并发数
	idCache        sync.Map            // key="repoID:name:pkgType:version:filename" → componentAssetIDs
	idCacheTTL     time.Duration
}

func NewDownloadService(
	compRepo *repository.ComponentRepository,
	storageSvc *StorageService,
	fetcher proxy.ProxyFetcher,
	logRepo *repository.ProxyDownloadLogRepository,
	logBatcher *LogBatcher,
	countBatcher *DownloadCountBatcher,
) *DownloadService {
	return &DownloadService{
		compRepo:      compRepo,
		storageSvc:   storageSvc,
		fetcher:      fetcher,
		logRepo:      logRepo,
		logBatcher:   logBatcher,
		countBatcher: countBatcher,
		depParseSem:  semaphore.NewWeighted(10), // 最多10个并发依赖解析任务
		idCacheTTL:   5 * time.Minute,
	}
}

func (s *DownloadService) Download(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	startTime := time.Now()

	if s.storageSvc == nil {
		return nil, fmt.Errorf("storage service not configured")
	}
	if downloadCtx == nil || downloadCtx.Repo == nil {
		return nil, fmt.Errorf("invalid download context")
	}

	pathInfo := downloadCtx.ResolvedPath
	if pathInfo == nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), false, fmt.Errorf("resolved path is nil"))
		return nil, fmt.Errorf("invalid download path")
	}

	backendID := storageBackendID(downloadCtx.Repo)
	contentType := resolveDownloadContentType(downloadCtx.PkgType, pathInfo.Filename)
	if content, size, err := s.storageSvc.GetPackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, pathInfo.StorageVersion, backendID); err == nil {
		s.recordLog(downloadCtx, size, time.Since(startTime), true, nil)
		s.incrementDownloadCount(ctx, downloadCtx)

		return &types.DownloadResult{
			Content:     content,
			Size:        size,
			FromCache:   true,
			RepoID:      downloadCtx.Repo.ID,
			Name:        pathInfo.Name,
			Version:     pathInfo.Version,
			Filename:    pathInfo.Filename,
			ContentType: contentType,
		}, nil
	}

	if s.fetcher == nil || downloadCtx.Repo.Type == model.RepoTypeLocal {
		s.recordLog(downloadCtx, 0, time.Since(startTime), false, fmt.Errorf("package not found"))
		return nil, fmt.Errorf("package not found")
	}

	return s.downloadFromProxy(ctx, downloadCtx, pathInfo, startTime)
}

func (s *DownloadService) downloadFromProxy(ctx context.Context, downloadCtx *types.DownloadContext, pathInfo *types.PackagePathInfo, startTime time.Time) (*types.DownloadResult, error) {
	if s.fetcher == nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), false, fmt.Errorf("fetcher not configured"))
		return nil, fmt.Errorf("fetcher not configured")
	}

	contentType := resolveDownloadContentType(downloadCtx.PkgType, pathInfo.Filename)

	// For PyPI: resolve CDN path from filename via upstream simple index.
	var remoteURL string
	if downloadCtx.PkgType == "pypi" {
		remoteURL = resolvePyPIRemoteURL(s.fetcher, downloadCtx, pathInfo)
		if remoteURL == "" {
			s.recordLog(downloadCtx, 0, time.Since(startTime), false, fmt.Errorf("failed to resolve PyPI download URL"))
			return nil, fmt.Errorf("failed to resolve PyPI download URL for %s", pathInfo.Filename)
		}
	} else {
		remoteURL = fmt.Sprintf("%s/%s", strings.TrimSuffix(downloadCtx.Repo.RemoteURL, "/"), pathInfo.RemotePath)
	}
	storageVersion := pathInfo.StorageVersion
	backendID := storageBackendID(downloadCtx.Repo)

	type proxyFetchOutcome struct {
		repoID       uint
		useStorage   bool
		fromRemote   bool
		contentBytes []byte
		size         int64
	}

	key := fmt.Sprintf("%d|%s|%s|%s", downloadCtx.Repo.ID, downloadCtx.PkgType, pathInfo.StorageName, storageVersion)
	value, err, _ := s.inflight.Do(key, func() (interface{}, error) {
		// Double-check storage to avoid redundant remote fetch during race.
		if _, _, getErr := s.storageSvc.GetPackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, backendID); getErr == nil {
			return &proxyFetchOutcome{repoID: downloadCtx.Repo.ID, useStorage: true, fromRemote: false}, nil
		}

		result, fetchErr := s.fetcher.FetchFromRemote(ctx, downloadCtx.Repo, remoteURL)
		if fetchErr != nil {
			return nil, fetchErr
		}
		defer result.Content.Close()

		if result.IsLarge {
			// 大文件：流式直接写入存储
			storageKey, storeErr := s.storageSvc.StorePackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, result.Content, result.Size, backendID)
			result.Content.Close()
			if storeErr != nil {
				logrus.WithFields(logrus.Fields{
					"module": "download", "pkg_type": string(downloadCtx.PkgType), "pkg_name": pathInfo.Name,
					"pkg_version": pathInfo.Version, "repo": downloadCtx.Repo.Name, "error": storeErr,
				}).Warn("failed to store large proxy package to local storage")
				// 存储失败，流式内容已关闭无法再读取，返回错误
				return nil, fmt.Errorf("failed to store large package and content stream closed: %w", storeErr)
			}
			if storageKey != "" {
				s.storePackageFileRecord(ctx, downloadCtx, result.RepoID, storageKey, result.Size, pathInfo.RemotePath)
			}
			return &proxyFetchOutcome{repoID: result.RepoID, useStorage: true, fromRemote: true, size: result.Size}, nil
		}

		contentBytes, readErr := io.ReadAll(result.Content)
		if readErr != nil {
			return nil, readErr
		}
		size := int64(len(contentBytes))

		storageKey, storeErr := s.storageSvc.StorePackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, bytes.NewReader(contentBytes), size, backendID)
		if storeErr != nil {
			logrus.WithFields(logrus.Fields{
				"module": "download", "pkg_type": string(downloadCtx.PkgType), "pkg_name": pathInfo.Name,
				"pkg_version": pathInfo.Version, "repo": downloadCtx.Repo.Name, "error": storeErr,
			}).Warn("failed to store proxy package to local storage")
			// 存储失败，保留内存数据兜底返回
			return &proxyFetchOutcome{
				repoID:       result.RepoID,
				useStorage:   false,
				contentBytes: contentBytes,
				size:         size,
			}, nil
		}
		if storageKey != "" {
			s.storePackageFileRecord(ctx, downloadCtx, result.RepoID, storageKey, size, pathInfo.RemotePath)
		}
		return &proxyFetchOutcome{repoID: result.RepoID, useStorage: true, fromRemote: true, size: size}, nil
	})
	if err != nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), false, err)
		return nil, err
	}

	outcome := value.(*proxyFetchOutcome)
	if outcome.useStorage {
		storedContent, storedSize, getErr := s.storageSvc.GetPackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, backendID)
		if getErr != nil {
			s.recordLog(downloadCtx, 0, time.Since(startTime), false, fmt.Errorf("failed to read stored proxy package: %w", getErr))
			return nil, fmt.Errorf("failed to read stored proxy package: %w", getErr)
		}
		s.recordLog(downloadCtx, storedSize, time.Since(startTime), !outcome.fromRemote, nil)
		s.incrementDownloadCount(ctx, downloadCtx)
		return &types.DownloadResult{
			Content:     storedContent,
			Size:        storedSize,
			FromCache:   !outcome.fromRemote,
			RepoID:      outcome.repoID,
			Name:        pathInfo.Name,
			Version:     pathInfo.Version,
			Filename:    pathInfo.Filename,
			ContentType: contentType,
		}, nil
	}

	s.recordLog(downloadCtx, outcome.size, time.Since(startTime), false, nil)
	s.incrementDownloadCount(ctx, downloadCtx)
	return &types.DownloadResult{
		Content:     io.NopCloser(bytes.NewReader(outcome.contentBytes)),
		Size:        outcome.size,
		FromCache:   false,
		RepoID:      outcome.repoID,
		Name:        pathInfo.Name,
		Version:     pathInfo.Version,
		Filename:    pathInfo.Filename,
		ContentType: contentType,
	}, nil
}

func resolveDownloadContentType(pkgType model.PackageType, filename string) string {
	switch pkgType {
	case model.PackageTypeGo:
		if strings.HasSuffix(filename, ".info") {
			return "application/json"
		}
		if strings.HasSuffix(filename, ".mod") {
			return "text/plain; charset=utf-8"
		}
		if strings.HasSuffix(filename, ".zip") {
			return "application/zip"
		}
		return "application/octet-stream"
	case model.PackageTypeMaven:
		ext := strings.ToLower(filename)
		if strings.HasSuffix(ext, ".pom") || strings.HasSuffix(ext, ".xml") {
			return "application/xml"
		}
		if strings.HasSuffix(ext, ".jar") {
			return "application/java-archive"
		}
		if strings.HasSuffix(ext, ".war") {
			return "application/java-archive"
		}
		if strings.HasSuffix(ext, ".aar") {
			return "application/java-archive"
		}
		if strings.HasSuffix(ext, ".sha1") || strings.HasSuffix(ext, ".sha256") || strings.HasSuffix(ext, ".md5") {
			return "text/plain"
		}
		return "application/octet-stream"
	case model.PackageTypePyPI:
		ext := strings.ToLower(filename)
		if strings.HasSuffix(ext, ".whl") {
			return "application/octet-stream"
		}
		if strings.HasSuffix(ext, ".tar.gz") || strings.HasSuffix(ext, ".tgz") {
			return "application/gzip"
		}
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

func storageBackendID(repo *model.Repository) uint {
	if repo == nil || repo.StorageBackendID == nil {
		return 0
	}
	return *repo.StorageBackendID
}

func (s *DownloadService) storePackageFileRecord(ctx context.Context, req *types.DownloadContext, repoID uint, storageKey string, size int64, downloadURL string) {
	comp, _, dbErr := s.compRepo.StoreComponentAsset(ctx, &model.Component{
		RepositoryID: repoID,
		Format:       req.PkgType,
		Name:         req.Name,
		Version:      req.Version,
		Status:       model.StatusPublished,
	}, &model.Asset{
		FileName:    req.Filename,
		Kind:        model.AssetKindPrimary,
		Path:        storageKey,
		DownloadURL: downloadURL,
		Blob: model.Blob{
			Ref:       storageKey,
			SizeBytes: size,
		},
	})
	if dbErr != nil {
		logrus.WithFields(logrus.Fields{
			"module": "download", "pkg_type": string(req.PkgType), "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "error": dbErr,
		}).Warn("failed to store proxy package file to database")
		return
	}

	// Maven POM 代理下载后，异步解析并入库依赖
	if req.PkgType == model.PackageTypeMaven && strings.HasSuffix(strings.ToLower(req.Filename), ".pom") {
		go func() {
			if err := s.depParseSem.Acquire(context.Background(), 1); err != nil {
				logrus.WithFields(logrus.Fields{
					"module": "download", "pkg_name": req.Name, "pkg_version": req.Version,
					"repo": req.Repo.Name, "error": err,
				}).Warn("failed to acquire dep parse semaphore")
				return
			}
			defer s.depParseSem.Release(1)
			s.asyncUpsertMavenDeps(req, comp.ID)
		}()
	}
	if req.PkgType == model.PackageTypeGo && strings.HasSuffix(strings.ToLower(req.Filename), ".mod") {
		go func() {
			if err := s.depParseSem.Acquire(context.Background(), 1); err != nil {
				logrus.WithFields(logrus.Fields{
					"module": "download", "pkg_name": req.Name, "pkg_version": req.Version,
					"repo": req.Repo.Name, "error": err,
				}).Warn("failed to acquire dep parse semaphore")
				return
			}
			defer s.depParseSem.Release(1)
			s.asyncUpsertGoDeps(req, comp.ID)
		}()
	}
}

type mavenPOMForDeps struct {
	XMLName      xml.Name            `xml:"project"`
	Dependencies []mavenDepForUpload `xml:"dependencies>dependency"`
}

type mavenDepForUpload struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

func (s *DownloadService) asyncUpsertMavenDeps(req *types.DownloadContext, versionID uint) {
	if versionID == 0 || req == nil || req.Repo == nil {
		return
	}
	if req.ResolvedPath == nil {
		return
	}
	bg, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	content, _, err := s.storageSvc.GetPackageWithBackend(
		bg,
		req.Repo.Name,
		string(req.PkgType),
		req.ResolvedPath.StorageName,
		req.ResolvedPath.StorageVersion,
		storageBackendID(req.Repo),
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "dep_parse", "pkg_type": "maven", "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "error": err,
		}).Warn("failed to load pom content for dependency parse")
		return
	}
	defer content.Close()

	body, err := io.ReadAll(content)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "dep_parse", "pkg_type": "maven", "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "error": err,
		}).Warn("failed to read pom content for dependency parse")
		return
	}

	var pom mavenPOMForDeps
	if err := xml.Unmarshal(body, &pom); err != nil {
		return
	}

	deps := make([]model.ComponentDependency, 0, len(pom.Dependencies))
	seen := make(map[string]struct{}, len(pom.Dependencies))
	for _, dep := range pom.Dependencies {
		if dep.GroupID == "" || dep.ArtifactID == "" || dep.Version == "" {
			continue
		}
		depName := dep.GroupID + ":" + dep.ArtifactID
		key := depName + "|" + dep.Version + "|" + dep.Scope
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deps = append(deps, model.ComponentDependency{
			DepName:              depName,
			DepVersionConstraint: dep.Version,
			DepType:              "direct",
			PackageType:          string(model.PackageTypeMaven),
			IsOptional:           dep.Scope == "test" || dep.Scope == "provided",
		})
	}

	if len(deps) == 0 {
		return
	}
	if err := s.compRepo.UpsertComponentDependencies(bg, versionID, deps); err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "dep_parse", "pkg_type": "maven", "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "dep_count": len(deps), "error": err,
		}).Warn("failed to upsert maven dependencies")
	}
}

func (s *DownloadService) asyncUpsertGoDeps(req *types.DownloadContext, versionID uint) {
	if versionID == 0 || req == nil || req.Repo == nil || req.ResolvedPath == nil {
		return
	}
	bg, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	content, _, err := s.storageSvc.GetPackageWithBackend(
		bg,
		req.Repo.Name,
		string(req.PkgType),
		req.ResolvedPath.StorageName,
		req.ResolvedPath.StorageVersion,
		storageBackendID(req.Repo),
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "dep_parse", "pkg_type": "go", "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "error": err,
		}).Warn("failed to load go.mod content for dependency parse")
		return
	}
	defer content.Close()

	body, err := io.ReadAll(content)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "dep_parse", "pkg_type": "go", "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "error": err,
		}).Warn("failed to read go.mod content for dependency parse")
		return
	}

	parsed, err := modfile.Parse("go.mod", body, nil)
	if err != nil || parsed == nil {
		return
	}

	deps := make([]model.ComponentDependency, 0, len(parsed.Require))
	seen := make(map[string]struct{}, len(parsed.Require))
	for _, reqDep := range parsed.Require {
		if reqDep == nil || reqDep.Mod.Path == "" || reqDep.Mod.Version == "" {
			continue
		}
		key := reqDep.Mod.Path + "|" + reqDep.Mod.Version + "|" + fmt.Sprint(reqDep.Indirect)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deps = append(deps, model.ComponentDependency{
			DepName:              reqDep.Mod.Path,
			DepVersionConstraint: reqDep.Mod.Version,
			DepType:              "direct",
			PackageType:          string(model.PackageTypeGo),
			IsOptional:           reqDep.Indirect,
		})
	}

	if len(deps) == 0 {
		return
	}
	if err := s.compRepo.UpsertComponentDependencies(bg, versionID, deps); err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "dep_parse", "pkg_type": "go", "pkg_name": req.Name,
			"pkg_version": req.Version, "repo": req.Repo.Name, "dep_count": len(deps), "error": err,
		}).Warn("failed to upsert go dependencies")
	}
}

func (s *DownloadService) recordLog(req *types.DownloadContext, sizeBytes int64, duration time.Duration, fromCache bool, err error) {
	if s.logRepo == nil && s.logBatcher == nil {
		// 即使日志后端未配置，也持续上报下载链路指标。
		s.recordDownloadMetrics(req, sizeBytes, duration, fromCache, err)
		return
	}

	status := model.DownloadStatusSuccess
	statusCode := 200
	if err != nil {
		status = model.DownloadStatusFailed
		statusCode = 404
	}

	log := &model.ProxyDownloadLog{
		RepositoryID: req.Repo.ID,
		PackageType:  string(req.PkgType),
		PackageName:  req.Name,
		Version:      req.Version,
		Filename:     req.Filename,
		Status:       status,
		StatusCode:   statusCode,
		SizeBytes:    sizeBytes,
		DurationMs:   int(duration.Milliseconds()),
		FromCache:    fromCache,
		IPAddress:    req.ClientIP,
	}

	if req.UserID != 0 {
		log.UserID = &req.UserID
	}

	if err != nil {
		log.ErrorMessage = err.Error()
	}

	if s.logBatcher != nil {
		s.logBatcher.Record(log)
	} else if s.logRepo != nil {
		s.logRepo.Create(log)
	}

	s.recordDownloadMetrics(req, sizeBytes, duration, fromCache, err)
	metrics.RecordDownload(string(req.PkgType), req.Name, req.Version)
}

func (s *DownloadService) recordDownloadMetrics(req *types.DownloadContext, sizeBytes int64, duration time.Duration, fromCache bool, err error) {
	source := "local"
	if fromCache {
		source = "cache"
	} else if req != nil && req.Repo != nil && req.Repo.Type == model.RepoTypeProxy {
		source = "proxy_remote"
	}
	result := "success"
	if err != nil {
		result = "failed"
	}
	pkgType := ""
	if req != nil {
		pkgType = string(req.PkgType)
	}
	metrics.RecordDownloadStats(pkgType, source, result, sizeBytes, duration.Seconds())
}

func (s *DownloadService) incrementDownloadCount(ctx context.Context, req *types.DownloadContext) {
	if s.countBatcher == nil {
		return
	}

	cacheKey := fmt.Sprintf("%d:%s:%s:%s:%s", req.Repo.ID, req.Name, req.PkgType, req.Version, req.Filename)
	if cached, ok := s.idCache.Load(cacheKey); ok {
		if entry, ok := cached.(componentAssetIDs); ok && time.Now().Before(entry.expires) {
			s.countBatcher.Increment(entry.componentID, entry.assetID, req.Repo.ID)
			return
		}
	}

	comp, err := s.compRepo.FindComponentByRepoNameVersionContext(ctx, req.Repo.ID, req.Name, req.Version, req.PkgType)
	if err != nil {
		return
	}

	asset, err := s.compRepo.FindAssetByComponentAndFilenameContext(ctx, comp.ID, req.Filename)
	if err != nil {
		s.countBatcher.Increment(comp.ID, 0, req.Repo.ID)
		s.idCache.Store(cacheKey, componentAssetIDs{componentID: comp.ID, expires: time.Now().Add(s.idCacheTTL)})
		return
	}

	s.countBatcher.Increment(comp.ID, asset.ID, req.Repo.ID)
	s.idCache.Store(cacheKey, componentAssetIDs{componentID: comp.ID, assetID: asset.ID, expires: time.Now().Add(s.idCacheTTL)})
}
