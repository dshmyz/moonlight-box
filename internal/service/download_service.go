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
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
)

const maxPkgCacheSize = 10000

type DownloadService struct {
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	adapters  map[string]types.Adapter

	pkgRepo      *repository.PackageRepository
	storageSvc   *StorageService
	fetcher      proxy.ProxyFetcher
	logRepo      *repository.ProxyDownloadLogRepository
	logBatcher   *LogBatcher
	countBatcher *DownloadCountBatcher
	pkgCache     *cache.LRUCache
}

func NewDownloadService(
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	adapters map[string]types.Adapter,
	pkgRepo *repository.PackageRepository,
	storageSvc *StorageService,
	fetcher proxy.ProxyFetcher,
	logRepo *repository.ProxyDownloadLogRepository,
	logBatcher *LogBatcher,
	countBatcher *DownloadCountBatcher,
) *DownloadService {
	return &DownloadService{
		repoRepo:     repoRepo,
		groupRepo:    groupRepo,
		adapters:     adapters,
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

	adp := s.adapters[string(downloadCtx.PkgType)]
	if adp == nil {
		return nil, fmt.Errorf("unsupported package type: %s", downloadCtx.PkgType)
	}

	pathInfo, err := adp.ResolvePackagePath(downloadCtx.Name + "/" + downloadCtx.Version + "/" + downloadCtx.Filename)
	if err != nil {
		return nil, err
	}

	if content, size, err := s.storageSvc.GetPackage(ctx, string(downloadCtx.PkgType), pathInfo.StorageName, pathInfo.StorageVersion); err == nil {
		s.recordLog(downloadCtx, size, time.Since(startTime), nil)
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

	switch downloadCtx.Repo.Type {
	case model.RepoTypeLocal:
		s.recordLog(downloadCtx, 0, time.Since(startTime), fmt.Errorf("package not found"))
		return nil, fmt.Errorf("package not found")

	case model.RepoTypeProxy:
		return s.downloadFromProxy(ctx, downloadCtx, pathInfo, startTime)

	case model.RepoTypeVirtual:
		return s.downloadFromVirtual(ctx, downloadCtx, startTime)

	default:
		return nil, fmt.Errorf("unsupported repository type: %s", downloadCtx.Repo.Type)
	}
}

func (s *DownloadService) downloadFromProxy(ctx context.Context, downloadCtx *types.DownloadContext, pathInfo *types.PackagePathInfo, startTime time.Time) (*types.DownloadResult, error) {
	if s.fetcher == nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), fmt.Errorf("fetcher not configured"))
		return nil, fmt.Errorf("fetcher not configured")
	}

	adp, ok := s.adapters[string(downloadCtx.PkgType)]
	if !ok {
		s.recordLog(downloadCtx, 0, time.Since(startTime), fmt.Errorf("no adapter for package type: %s", downloadCtx.PkgType))
		return nil, fmt.Errorf("no adapter for package type: %s", downloadCtx.PkgType)
	}

	resolvedPath, resolveErr := adp.ResolvePackagePath(pathInfo.Name + "/" + pathInfo.Version + "/" + pathInfo.Filename)
	if resolveErr != nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), resolveErr)
		return nil, resolveErr
	}

	remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(downloadCtx.Repo.RemoteURL, "/"), resolvedPath.RemotePath)
	result, fetchErr := s.fetcher.FetchFromRemote(ctx, downloadCtx.Repo, remoteURL)
	if fetchErr != nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), fetchErr)
		return nil, fetchErr
	}
	defer result.Content.Close()

	storageVersion := s.storageSvc.NormalizeVersion(string(downloadCtx.PkgType), pathInfo.Version, pathInfo.Filename)

	var backendID uint
	if downloadCtx.Repo.StorageBackendID != nil {
		backendID = *downloadCtx.Repo.StorageBackendID
	}

	storageKey, storeErr := s.storageSvc.StorePackageWithBackend(ctx, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion, result.Content, result.Size, backendID)
	if storeErr != nil {
		logrus.Warnf("failed to store proxy package %s: %v", pathInfo.Name, storeErr)
		s.recordLog(downloadCtx, result.Size, time.Since(startTime), nil)
		return &types.DownloadResult{
			Content:   nil,
			Size:      0,
			FromCache: false,
			RepoID:    result.RepoID,
			Name:      pathInfo.Name,
			Version:   pathInfo.Version,
			Filename:  pathInfo.Filename,
		}, storeErr
	}

	if storageKey != "" {
		s.storePackageFileRecord(ctx, downloadCtx, result.RepoID, storageKey, result.Size)
	}

	s.recordLog(downloadCtx, result.Size, time.Since(startTime), nil)

	storedContent, storedSize, getErr := s.storageSvc.GetPackage(ctx, string(downloadCtx.PkgType), pathInfo.StorageName, storageVersion)
	if getErr != nil {
		return nil, fmt.Errorf("failed to get stored package: %w", getErr)
	}

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

func (s *DownloadService) downloadFromVirtual(ctx context.Context, downloadCtx *types.DownloadContext, startTime time.Time) (*types.DownloadResult, error) {
	members, err := s.groupRepo.GetMembersByVirtualRepo(downloadCtx.Repo.ID)
	if err != nil {
		s.recordLog(downloadCtx, 0, time.Since(startTime), err)
		return nil, err
	}

	for _, member := range members {
		if string(member.MemberRepo.PackageType) != string(downloadCtx.PkgType) {
			continue
		}

		memberCtx := *downloadCtx
		memberCtx.Repo = &member.MemberRepo

		result, err := s.Download(ctx, &memberCtx)
		if err == nil {
			result.RepoID = member.MemberRepo.ID
			return result, nil
		}
	}

	s.recordLog(downloadCtx, 0, time.Since(startTime), fmt.Errorf("package not found in virtual repository"))
	return nil, fmt.Errorf("package not found in virtual repository")
}

func (s *DownloadService) storePackageFileRecord(ctx context.Context, req *types.DownloadContext, repoID uint, storageKey string, size int64) {
	_, _, _, dbErr := s.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           req.Name,
		Type:           req.PkgType,
		RepositoryID:   repoID,
		RepositoryType: req.Repo.Type,
	}, &model.PackageVersion{
		Version:     req.Version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
	}, &model.PackageFile{
		Filename:    req.Filename,
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})
	if dbErr != nil {
		logrus.Warnf("failed to store proxy package file to database %s: %v", req.Name, dbErr)
	}
}

func (s *DownloadService) recordLog(req *types.DownloadContext, sizeBytes int64, duration time.Duration, err error) {
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
		FromCache:    false,
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
	// TODO: implement download count increment
	// Need to get PackageID, VersionID, FileID from database or cache
}

type readCloserWrapper struct {
	io.Reader
}

func (r *readCloserWrapper) Close() error {
	return nil
}
