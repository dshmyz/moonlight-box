package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
)

const maxPkgCacheSize = 10000

type DownloadService struct {
	pkgRepo      *repository.PackageRepository
	storageSvc   *StorageService
	fetcher      proxy.ProxyFetcher
	logRepo      *repository.ProxyDownloadLogRepository
	logBatcher   *LogBatcher
	countBatcher *DownloadCountBatcher
	pkgCache     *cache.LRUCache
}

func NewDownloadService(
	pkgRepo *repository.PackageRepository,
	storageSvc *StorageService,
	fetcher proxy.ProxyFetcher,
	logRepo *repository.ProxyDownloadLogRepository,
	logBatcher *LogBatcher,
	countBatcher *DownloadCountBatcher,
) *DownloadService {
	return &DownloadService{
		pkgRepo:      pkgRepo,
		storageSvc:   storageSvc,
		fetcher:      fetcher,
		logRepo:      logRepo,
		logBatcher:   logBatcher,
		countBatcher: countBatcher,
		pkgCache:     cache.NewLRUCache(maxPkgCacheSize),
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

	if content, size, err := s.storageSvc.GetPackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, pathInfo.StorageVersion, 0); err == nil {
		s.recordLog(downloadCtx, size, time.Since(startTime), true, nil)
		s.incrementDownloadCount(downloadCtx)

		return &types.DownloadResult{
			Content:   content,
			Size:      size,
			FromCache: true,
			RepoID:    downloadCtx.Repo.ID,
			Name:      pathInfo.Name,
			Version:   pathInfo.Version,
			Filename:  pathInfo.Filename,
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

	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(downloadCtx.Repo.RemoteURL, "/"), pathInfo.RemotePath)
	result, fetchErr := s.fetcher.FetchFromRemote(ctx, downloadCtx.Repo, remoteURL)
	if fetchErr != nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), false, fetchErr)
		return nil, fetchErr
	}
	defer result.Content.Close()

	storageVersion := pathInfo.StorageVersion

	var backendID uint
	if downloadCtx.Repo.StorageBackendID != nil {
		backendID = *downloadCtx.Repo.StorageBackendID
	}

	if result.IsLarge {
		storageKey, storeErr := s.storageSvc.StorePackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, result.Content, result.Size, backendID)
		if storeErr != nil {
			result.Content.Close()
			s.recordLog(downloadCtx, 0, time.Since(startTime), false, storeErr)
			return nil, storeErr
		}
		result.Content.Close()

		if storageKey != "" {
			s.storePackageFileRecord(ctx, downloadCtx, result.RepoID, storageKey, result.Size, pathInfo.RemotePath)
		}

		storedContent, storedSize, getErr := s.storageSvc.GetPackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, 0)
		if getErr != nil {
			return nil, fmt.Errorf("failed to read stored large package: %w", getErr)
		}

		s.recordLog(downloadCtx, storedSize, time.Since(startTime), true, nil)
		return &types.DownloadResult{
			Content:   storedContent,
			Size:      storedSize,
			FromCache: true,
			RepoID:    result.RepoID,
			Name:      pathInfo.Name,
			Version:   pathInfo.Version,
			Filename:  pathInfo.Filename,
		}, nil
	}

	contentBytes, readErr := io.ReadAll(result.Content)
	result.Content.Close()
	if readErr != nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), false, readErr)
		return nil, readErr
	}
	size := int64(len(contentBytes))

	storageKey, storeErr := s.storageSvc.StorePackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, bytes.NewReader(contentBytes), size, backendID)
	if storeErr != nil {
		logrus.Warnf("failed to store proxy package %s: %v", pathInfo.Name, storeErr)
		s.recordLog(downloadCtx, size, time.Since(startTime), false, nil)
		return &types.DownloadResult{
			Content:   io.NopCloser(bytes.NewReader(contentBytes)),
			Size:      size,
			FromCache: false,
			RepoID:    result.RepoID,
			Name:      pathInfo.Name,
			Version:   pathInfo.Version,
			Filename:  pathInfo.Filename,
		}, nil
	}

	if storageKey != "" {
		s.storePackageFileRecord(ctx, downloadCtx, result.RepoID, storageKey, size, pathInfo.RemotePath)
	}

	s.recordLog(downloadCtx, size, time.Since(startTime), true, nil)

	return &types.DownloadResult{
		Content:   io.NopCloser(bytes.NewReader(contentBytes)),
		Size:      size,
		FromCache: true,
		RepoID:    result.RepoID,
		Name:      pathInfo.Name,
		Version:   pathInfo.Version,
		Filename:  pathInfo.Filename,
	}, nil
}

func (s *DownloadService) storePackageFileRecord(ctx context.Context, req *types.DownloadContext, repoID uint, storageKey string, size int64, downloadURL string) {
	pkg, ver, file, dbErr := s.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           req.Name,
		Type:           req.PkgType,
		RepositoryID:   repoID,
		RepositoryType: req.Repo.Type,
	}, &model.PackageVersion{
		Version: req.Version,
		Status:  model.StatusPublished,
	}, &model.PackageFile{
		Filename:    req.Filename,
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
		DownloadURL: downloadURL,
	})
	if dbErr != nil {
		logrus.Warnf("failed to store proxy package file to database %s: %v", req.Name, dbErr)
		return
	}

	if s.countBatcher == nil {
		return
	}

	fileID := uint(0)
	if file != nil {
		fileID = file.ID
	}
	versionID := uint(0)
	if ver != nil {
		versionID = ver.ID
	}
	s.countBatcher.Increment(pkg.ID, versionID, fileID, repoID)
}

func (s *DownloadService) recordLog(req *types.DownloadContext, sizeBytes int64, duration time.Duration, fromCache bool, err error) {
	if s.logRepo == nil && s.logBatcher == nil {
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

	metrics.RecordDownload(string(req.PkgType), req.Name, req.Version)
}

func (s *DownloadService) incrementDownloadCount(req *types.DownloadContext) {
	if s.countBatcher == nil {
		return
	}

	pkg, err := s.pkgRepo.FindByNameAndType(req.Name, req.PkgType)
	if err != nil {
		return
	}

	ver, err := s.pkgRepo.FindVersionByPackageAndVersion(pkg.ID, req.Version)
	if err != nil {
		s.countBatcher.Increment(pkg.ID, 0, 0, req.Repo.ID)
		return
	}

	file, err := s.pkgRepo.FindFileByVersionAndFilename(ver.ID, req.Filename)
	if err != nil {
		s.countBatcher.Increment(pkg.ID, ver.ID, 0, req.Repo.ID)
		return
	}

	s.countBatcher.Increment(pkg.ID, ver.ID, file.ID, req.Repo.ID)
}
