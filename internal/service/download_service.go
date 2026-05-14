package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

type DownloadService struct {
	pkgRepo      *repository.PackageRepository
	storageSvc   *StorageService
	fetcher      proxy.ProxyFetcher
	logRepo      *repository.ProxyDownloadLogRepository
	logBatcher   *LogBatcher
	countBatcher *DownloadCountBatcher
	inflight     singleflight.Group
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
	if content, size, err := s.storageSvc.GetPackageWithBackend(ctx, downloadCtx.Repo.Name, string(downloadCtx.PkgType), pathInfo.StorageName, pathInfo.StorageVersion, backendID); err == nil {
		s.recordLog(downloadCtx, size, time.Since(startTime), true, nil)
		s.incrementDownloadCount(ctx, downloadCtx)

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
				logrus.Warnf("failed to store large proxy package %s to local storage: %v", pathInfo.Name, storeErr)
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
			logrus.Warnf("failed to store proxy package %s to local storage: %v", pathInfo.Name, storeErr)
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
			Content:   storedContent,
			Size:      storedSize,
			FromCache: !outcome.fromRemote,
			RepoID:    outcome.repoID,
			Name:      pathInfo.Name,
			Version:   pathInfo.Version,
			Filename:  pathInfo.Filename,
		}, nil
	}

	s.recordLog(downloadCtx, outcome.size, time.Since(startTime), false, nil)
	s.incrementDownloadCount(ctx, downloadCtx)
	return &types.DownloadResult{
		Content:   io.NopCloser(bytes.NewReader(outcome.contentBytes)),
		Size:      outcome.size,
		FromCache: false,
		RepoID:    outcome.repoID,
		Name:      pathInfo.Name,
		Version:   pathInfo.Version,
		Filename:  pathInfo.Filename,
	}, nil
}

func storageBackendID(repo *model.Repository) uint {
	if repo == nil || repo.StorageBackendID == nil {
		return 0
	}
	return *repo.StorageBackendID
}

func (s *DownloadService) storePackageFileRecord(ctx context.Context, req *types.DownloadContext, repoID uint, storageKey string, size int64, downloadURL string) {
	_, _, _, dbErr := s.pkgRepo.StorePackageFile(ctx, &model.Package{
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

	pkg, err := s.pkgRepo.FindByRepoNameAndTypeContext(ctx, req.Repo.ID, req.Name, req.PkgType)
	if err != nil {
		return
	}

	ver, err := s.pkgRepo.FindVersionByPackageAndVersionContext(ctx, pkg.ID, req.Version)
	if err != nil {
		s.countBatcher.Increment(pkg.ID, 0, 0, req.Repo.ID)
		return
	}

	file, err := s.pkgRepo.FindFileByVersionAndFilenameContext(ctx, ver.ID, req.Filename)
	if err != nil {
		s.countBatcher.Increment(pkg.ID, ver.ID, 0, req.Repo.ID)
		return
	}

	s.countBatcher.Increment(pkg.ID, ver.ID, file.ID, req.Repo.ID)
}
