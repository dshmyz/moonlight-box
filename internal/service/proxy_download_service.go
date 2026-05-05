package service

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
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
	pkgRepo     *repository.PackageRepository
	storageSvc  *StorageService
	proxyRouter *proxy.ProxyRouter
	logRepo     *repository.ProxyDownloadLogRepository
}

func NewProxyDownloadService(
	pkgRepo *repository.PackageRepository,
	storageSvc *StorageService,
	proxyRouter *proxy.ProxyRouter,
	logRepo *repository.ProxyDownloadLogRepository,
) *ProxyDownloadService {
	return &ProxyDownloadService{
		pkgRepo:     pkgRepo,
		storageSvc:  storageSvc,
		proxyRouter: proxyRouter,
		logRepo:     logRepo,
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
		result, resolveErr = s.proxyRouter.ResolveSmart(ctx, req.Repo, req.PkgType, req.Name, req.Version, req.URLBuilder)
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
	}

	if storeErr == nil && storageKey != "" {
		s.pkgRepo.StorePackageFileAndIncrementDownload(ctx, &model.Package{
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
	if s.logRepo == nil {
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

	if createErr := s.logRepo.Create(log); createErr != nil {
		logrus.Warnf("failed to record proxy download log: %v", createErr)
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
