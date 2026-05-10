package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

const (
	maxPkgCacheSize = 10000 // 最大缓存条目数
)

type ProxyDownloadService struct {
	pkgRepo      *repository.PackageRepository
	storageSvc   *StorageService
	downloader   *proxy.ProxyDownloader
	logRepo      *repository.ProxyDownloadLogRepository
	logBatcher   *LogBatcher
	countBatcher *DownloadCountBatcher

	pkgCache *cache.LRUCache
}

type pkgCacheEntry struct {
	PackageID uint
	Versions  map[string]*versionCacheEntry
}

type versionCacheEntry struct {
	VersionID uint
	Files     map[string]uint
}

func NewProxyDownloadService(
	pkgRepo *repository.PackageRepository,
	storageSvc *StorageService,
	downloader *proxy.ProxyDownloader,
	logRepo *repository.ProxyDownloadLogRepository,
	logBatcher *LogBatcher,
	countBatcher *DownloadCountBatcher,
) *ProxyDownloadService {
	return &ProxyDownloadService{
		pkgRepo:      pkgRepo,
		storageSvc:   storageSvc,
		downloader:   downloader,
		logRepo:      logRepo,
		logBatcher:   logBatcher,
		countBatcher: countBatcher,
		pkgCache:     cache.NewLRUCache(maxPkgCacheSize),
	}
}

type ProxyDownloadRequest struct {
	PkgType        string
	Name           string
	Version        string
	Filename       string
	Repo           *model.Repository
	PackageType    model.PackageType
	RepositoryType model.RepositoryType
	FileType       model.PackageFileType
	Metadata       map[string]interface{}
	IPAddress      string
	UserAgent      string
	UserID         *uint
}

type ProxyDownloadResult struct {
	Content    io.ReadCloser
	Size       int64
	StorageKey string
	FromCache  bool
	RepoID     uint
}

type readCloserWrapper struct {
	io.Reader
}

func (r *readCloserWrapper) Close() error {
	return nil
}

func (s *ProxyDownloadService) Download(ctx context.Context, req *proxy.DownloadRequest) (*proxy.DownloadResult, error) {
	result, err := s.downloadInternal(ctx, &ProxyDownloadRequest{
		PkgType:   req.PkgType,
		Name:      req.Name,
		Version:   req.Version,
		Filename:  req.Filename,
		Repo:      req.Repo,
		IPAddress: req.IPAddress,
		UserAgent: req.UserAgent,
		UserID:    req.UserID,
	})
	if err != nil {
		return nil, err
	}
	return &proxy.DownloadResult{
		Content:   result.Content,
		Size:      result.Size,
		FromCache: result.FromCache,
		RepoID:    result.RepoID,
		Filename:  result.StorageKey,
	}, nil
}

func (s *ProxyDownloadService) downloadInternal(ctx context.Context, req *ProxyDownloadRequest) (*ProxyDownloadResult, error) {
	startTime := time.Now()

	cacheVersion := s.storageSvc.NormalizeVersion(req.PkgType, req.Version, req.Filename)

	if result, ok := s.checkLocalCache(ctx, req, cacheVersion, startTime); ok {
		return result, nil
	}

	if s.downloader == nil || req.Repo == nil || req.Repo.Type == model.RepoTypeLocal {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, proxy.ErrPackageNotFound)
		return nil, proxy.ErrPackageNotFound
	}

	return s.fetchAndStoreRemote(ctx, req, startTime)
}

func (s *ProxyDownloadService) checkLocalCache(ctx context.Context, req *ProxyDownloadRequest, cacheVersion string, startTime time.Time) (*ProxyDownloadResult, bool) {
	content, size, err := s.storageSvc.GetPackage(ctx, req.PkgType, req.Name, cacheVersion)
	if err != nil {
		logrus.Debugf("Cache miss for %s/%s/%s: %v", req.PkgType, req.Name, cacheVersion, err)
		return nil, false
	}

	logrus.Debugf("Cache hit for %s/%s/%s, size=%d", req.PkgType, req.Name, cacheVersion, size)
	metrics.RecordDownload(req.PkgType, req.Name, req.Version)
	s.incrementDownloadCount(req)
	s.recordLog(req, model.DownloadStatusCached, 0, size, int(time.Since(startTime).Milliseconds()), true, nil)
	return &ProxyDownloadResult{
		Content:   content,
		Size:      size,
		FromCache: true,
	}, true
}

func (s *ProxyDownloadService) fetchAndStoreRemote(ctx context.Context, req *ProxyDownloadRequest, startTime time.Time) (*ProxyDownloadResult, error) {
	result, resolveErr := s.downloader.FetchFromRemote(ctx, req.Repo, req.PkgType, req.Name, req.Version)
	if resolveErr != nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, resolveErr)
		return nil, resolveErr
	}
	defer result.Content.Close()

	storageVersion := s.storageSvc.NormalizeVersion(req.PkgType, req.Version, req.Filename)
	if req.PkgType == "go" && req.Filename != "" {
		req.Version = strings.TrimSuffix(req.Filename, filepath.Ext(req.Filename))
	}

	var backendID uint
	if req.Repo != nil && req.Repo.StorageBackendID != nil {
		backendID = *req.Repo.StorageBackendID
	}
	storageKey, storeErr := s.storageSvc.StorePackageWithBackend(ctx, req.PkgType, req.Name, storageVersion, result.Content, result.Size, backendID)
	if storeErr != nil {
		logrus.Warnf("failed to store proxy package %s: %v", req.Name, storeErr)
		s.recordLog(req, model.DownloadStatusSuccess, 200, result.Size, int(time.Since(startTime).Milliseconds()), result.FromCache, nil)
		return &ProxyDownloadResult{
			Content:    nil,
			Size:       0,
			StorageKey: "",
			FromCache:  false,
			RepoID:     result.RepoID,
		}, storeErr
	}

	if storageKey != "" {
		s.storePackageFileRecord(ctx, req, result.RepoID, storageKey, result.Size)
	}

	s.recordLog(req, model.DownloadStatusSuccess, 200, result.Size, int(time.Since(startTime).Milliseconds()), result.FromCache, nil)

	storedContent, storedSize, getErr := s.storageSvc.GetPackage(ctx, req.PkgType, req.Name, storageVersion)
	if getErr != nil {
		return nil, fmt.Errorf("failed to get stored package: %w", getErr)
	}

	return &ProxyDownloadResult{
		Content:    storedContent,
		Size:       storedSize,
		StorageKey: storageKey,
		FromCache:  true,
		RepoID:     result.RepoID,
	}, nil
}

func (s *ProxyDownloadService) storePackageFileRecord(ctx context.Context, req *ProxyDownloadRequest, repoID uint, storageKey string, size int64) {
	pkgType := req.PackageType
	if pkgType == "" {
		pkgType = model.PackageType(req.PkgType)
	}
	repoType := req.RepositoryType
	if repoType == "" && req.Repo != nil {
		repoType = req.Repo.Type
	}
	fileType := req.FileType
	if fileType == "" {
		fileType = model.FileTypePrimary
	}
	_, _, _, dbErr := s.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           req.Name,
		Type:           pkgType,
		RepositoryID:   repoID,
		RepositoryType: repoType,
	}, &model.PackageVersion{
		Version:     req.Version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
	}, &model.PackageFile{
		Filename:    req.Filename,
		FileType:    fileType,
		StoragePath: storageKey,
		SizeBytes:   size,
	})
	if dbErr != nil {
		logrus.Warnf("failed to store proxy package file to database %s: %v", req.Name, dbErr)
	}
}

func (s *ProxyDownloadService) recordLog(req *ProxyDownloadRequest, status string, statusCode int, sizeBytes int64, durationMs int, fromCache bool, err error) {
	if s.logRepo == nil && s.logBatcher == nil {
		return
	}

	var repoID uint
	if req.Repo != nil {
		repoID = req.Repo.ID
	}

	log := &model.ProxyDownloadLog{
		RepositoryID: repoID,
		PackageType:  req.PkgType,
		PackageName:  req.Name,
		Version:      req.Version,
		Filename:     req.Filename,
		Status:       status,
		StatusCode:   statusCode,
		SizeBytes:    sizeBytes,
		DurationMs:   durationMs,
		FromCache:    fromCache,
		IPAddress:    req.IPAddress,
		UserAgent:    req.UserAgent,
		UserID:       req.UserID,
	}

	if err != nil {
		log.ErrorMessage = err.Error()
	}

	if s.logBatcher != nil {
		s.logBatcher.Record(log)
	} else if s.logRepo != nil {
		if createErr := s.logRepo.Create(log); createErr != nil {
			logrus.Warnf("failed to record proxy download log: %v", createErr)
		}
	}
}

func (s *ProxyDownloadService) incrementDownloadCount(req *ProxyDownloadRequest) {
	if req.Name == "" || req.PackageType == "" {
		return
	}

	cacheKey := string(req.PackageType) + "/" + req.Name

	var entry *pkgCacheEntry
	if cached, ok := s.pkgCache.Get(cacheKey); ok {
		entry = cached.(*pkgCacheEntry)
	}

	if entry == nil {
		pkg, err := s.pkgRepo.FindByNameAndType(req.Name, req.PackageType)
		if err != nil {
			return
		}

		entry = &pkgCacheEntry{
			PackageID: pkg.ID,
			Versions:  make(map[string]*versionCacheEntry),
		}

		for _, v := range pkg.Versions {
			verEntry := &versionCacheEntry{
				VersionID: v.ID,
				Files:     make(map[string]uint),
			}
			for _, f := range v.Files {
				verEntry.Files[f.Filename] = f.ID
			}
			entry.Versions[v.Version] = verEntry
		}

		s.pkgCache.Set(cacheKey, entry)
	}

	var versionID uint
	var fileID uint
	var repoID uint

	if verEntry, ok := entry.Versions[req.Version]; ok {
		versionID = verEntry.VersionID
		if req.Filename != "" {
			fileID = verEntry.Files[req.Filename]
		}
	}

	if req.Repo != nil {
		repoID = req.Repo.ID
	}

	s.countBatcher.Increment(entry.PackageID, versionID, fileID, repoID)
}
