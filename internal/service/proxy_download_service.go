package service

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type ResolutionMode int

const (
	ResolutionModeSmart ResolutionMode = iota
	ResolutionModeProxyOnly
	ResolutionModeVirtualRepo
)

type ProxyDownloadService struct {
	pkgRepo      *repository.PackageRepository
	storageSvc   *StorageService
	proxyRouter  *proxy.ProxyRouter
	logRepo      *repository.ProxyDownloadLogRepository
	logBatcher   *LogBatcher
	countBatcher *DownloadCountBatcher

	pkgCacheMu sync.RWMutex
	pkgCache   map[string]*pkgCacheEntry
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
	proxyRouter *proxy.ProxyRouter,
	logRepo *repository.ProxyDownloadLogRepository,
	logBatcher *LogBatcher,
	countBatcher *DownloadCountBatcher,
) *ProxyDownloadService {
	return &ProxyDownloadService{
		pkgRepo:      pkgRepo,
		storageSvc:   storageSvc,
		proxyRouter:  proxyRouter,
		logRepo:      logRepo,
		logBatcher:   logBatcher,
		countBatcher: countBatcher,
		pkgCache:     make(map[string]*pkgCacheEntry),
	}
}

type ProxyDownloadRequest struct {
	PkgType        string
	Name           string
	Version        string
	Filename       string
	Repo           *model.Repository
	URLBuilder     proxy.URLBuilder
	PackageType    model.PackageType
	RepositoryType model.RepositoryType
	FileType       model.PackageFileType
	Metadata       map[string]interface{}
	ResolutionMode ResolutionMode
	IPAddress      string
	UserAgent      string
	UserID         *uint
	RemoteURL      string
}

type ProxyDownloadResult struct {
	Content    []byte
	Size       int64
	StorageKey string
	FromCache  bool
	RepoID     uint
}

func (s *ProxyDownloadService) Download(ctx context.Context, req *ProxyDownloadRequest) (*ProxyDownloadResult, error) {
	startTime := time.Now()

	content, size, err := s.storageSvc.GetPackage(ctx, req.PkgType, req.Name, req.Version)
	if err == nil {
		body, readErr := io.ReadAll(content)
		content.Close()
		if readErr == nil {
			s.incrementDownloadCount(req)
			s.recordLog(req, model.DownloadStatusCached, 0, size, int(time.Since(startTime).Milliseconds()), true, nil)
			return &ProxyDownloadResult{
				Content:   body,
				Size:      size,
				FromCache: true,
			}, nil
		}
	}

	if s.proxyRouter == nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, proxy.ErrPackageNotFound)
		return nil, proxy.ErrPackageNotFound
	}

	var result *proxy.RouteResult
	var resolveErr error

	switch req.ResolutionMode {
	case ResolutionModeProxyOnly:
		result, resolveErr = s.proxyRouter.ResolveProxyOnlyForRepo(ctx, req.Repo, req.Name, req.Version, req.URLBuilder)
	case ResolutionModeVirtualRepo:
		result, resolveErr = s.proxyRouter.ResolveForVirtualRepo(ctx, req.Repo, req.PkgType, req.Name, req.Version, req.URLBuilder)
	default:
		if req.URLBuilder != nil && req.Repo != nil {
			result, resolveErr = s.proxyRouter.ResolveProxyOnlyForRepo(ctx, req.Repo, req.Name, req.Version, req.URLBuilder)
		} else {
			result, resolveErr = s.proxyRouter.ResolveSmart(ctx, req.Repo, req.PkgType, req.Name, req.Version, req.URLBuilder)
		}
	}

	if resolveErr != nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, resolveErr)
		return nil, resolveErr
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, readErr)
		return nil, readErr
	}

	// 对于 Maven 等需要包含文件名的包类型，将文件名添加到版本路径中
	storageVersion := req.Version
	if (req.PkgType == "maven" || req.PkgType == "maven2") && req.Filename != "" {
		storageVersion = req.Version + "/" + req.Filename
	}

	storageKey, storeErr := s.storageSvc.StorePackage(ctx, req.PkgType, req.Name, storageVersion, bytes.NewReader(body), result.Size)
	if storeErr != nil {
		logrus.Warnf("failed to store proxy package %s: %v", req.Name, storeErr)
	} else if storageKey != "" {
		_, _, _, dbErr := s.pkgRepo.StorePackageFile(ctx, &model.Package{
			Name:           req.Name,
			Type:           req.PackageType,
			RepositoryID:   result.RepoID,
			RepositoryType: req.RepositoryType,
		}, &model.PackageVersion{
			Version:     req.Version,
			Status:      model.StatusPublished,
			StoragePath: filepath.Dir(storageKey),
		}, &model.PackageFile{
			Filename:    req.Filename,
			FileType:    req.FileType,
			StoragePath: storageKey,
			SizeBytes:   result.Size,
		})
		if dbErr != nil {
			logrus.Warnf("failed to store proxy package file to database %s: %v", req.Name, dbErr)
		}
	}

	s.recordLog(req, model.DownloadStatusSuccess, 200, result.Size, int(time.Since(startTime).Milliseconds()), result.FromCache, nil)

	return &ProxyDownloadResult{
		Content:    body,
		Size:       result.Size,
		StorageKey: storageKey,
		FromCache:  result.FromCache,
		RepoID:     result.RepoID,
	}, nil
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
		RemoteURL:    req.RemoteURL,
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

func (s *ProxyDownloadService) DownloadAndStore(ctx context.Context, req *ProxyDownloadRequest, postProcess func(ctx context.Context, content []byte, name, version string) error) (*ProxyDownloadResult, error) {
	result, err := s.Download(ctx, req)
	if err != nil {
		return nil, err
	}

	if postProcess != nil {
		if processErr := postProcess(ctx, result.Content, req.Name, req.Version); processErr != nil {
			logrus.Warnf("post-process failed for %s: %v", req.Name, processErr)
		}
	}

	return result, nil
}

func (s *ProxyDownloadService) GetStorageService() *StorageService {
	return s.storageSvc
}

func (s *ProxyDownloadService) GetPackageRepository() *repository.PackageRepository {
	return s.pkgRepo
}

func (s *ProxyDownloadService) SetProxyRouter(pr *proxy.ProxyRouter) {
	s.proxyRouter = pr
}

func (s *ProxyDownloadService) incrementDownloadCount(req *ProxyDownloadRequest) {
	if req.Name == "" || req.PackageType == "" {
		return
	}

	cacheKey := string(req.PackageType) + "/" + req.Name

	s.pkgCacheMu.RLock()
	entry, exists := s.pkgCache[cacheKey]
	s.pkgCacheMu.RUnlock()

	if !exists {
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

		s.pkgCacheMu.Lock()
		s.pkgCache[cacheKey] = entry
		s.pkgCacheMu.Unlock()
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
