package adapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
)

type BaseAdapter struct {
	pkgRepo     *repository.PackageRepository
	storageSvc  *service.StorageService
	webhookSvc  *service.WebhookService
	proxyRouter *proxy.ProxyRouter
}

func NewBaseAdapter(pkgRepo *repository.PackageRepository, storageSvc *service.StorageService) *BaseAdapter {
	return &BaseAdapter{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
	}
}

func (b *BaseAdapter) SetProxyRouter(pr *proxy.ProxyRouter) {
	b.proxyRouter = pr
}

func (b *BaseAdapter) SetWebhookService(webhookSvc *service.WebhookService) {
	b.webhookSvc = webhookSvc
}

func (b *BaseAdapter) TriggerWebhook(event model.WebhookEvent, pkgName, version, repoName string, extraData map[string]interface{}) {
	if b.webhookSvc == nil {
		return
	}

	payload := &service.WebhookPayload{
		Event:       string(event),
		Timestamp:   time.Now().Format(time.RFC3339),
		PackageName: pkgName,
		Version:     version,
		Repository:  repoName,
		Data:        extraData,
	}
	b.webhookSvc.TriggerEvent(event, payload)
}

type ProxyPackageInfo struct {
	PackageType model.PackageType
	Name        string
	Version     string
	RepoID      uint
	Content     []byte
	Size        int64
	Metadata    map[string]interface{}
}

type LocalPackageResult struct {
	Content   io.ReadCloser
	Size      int64
	Found     bool
	PkgID     uint
	VersionID uint
	FileID    uint
}

func (b *BaseAdapter) GetLocalPackage(ctx context.Context, pkgType, name, version string) (*LocalPackageResult, error) {
	content, size, err := b.storageSvc.GetPackage(ctx, pkgType, name, version)
	if err != nil {
		return &LocalPackageResult{Found: false}, nil
	}

	return &LocalPackageResult{
		Content: content,
		Size:    size,
		Found:   true,
	}, nil
}

func (b *BaseAdapter) GetLocalPackageWithIDs(ctx context.Context, pkgType model.PackageType, name, version, filename string) (*LocalPackageResult, error) {
	content, size, err := b.storageSvc.GetPackage(ctx, string(pkgType), name, version)
	if err != nil {
		return &LocalPackageResult{Found: false}, nil
	}

	pkg, err := b.pkgRepo.FindByNameAndType(name, pkgType)
	if err != nil {
		return &LocalPackageResult{
			Content: content,
			Size:    size,
			Found:   true,
		}, nil
	}

	var versionID uint
	var fileID uint
	for _, v := range pkg.Versions {
		if v.Version == version {
			versionID = v.ID
			for _, f := range v.Files {
				if f.Filename == filename {
					fileID = f.ID
					break
				}
			}
			break
		}
	}

	return &LocalPackageResult{
		Content:   content,
		Size:      size,
		Found:     true,
		PkgID:     pkg.ID,
		VersionID: versionID,
		FileID:    fileID,
	}, nil
}

func (b *BaseAdapter) StoreProxyPackage(ctx context.Context, info *ProxyPackageInfo) (string, error) {
	storageKey, err := b.storageSvc.StorePackage(ctx, string(info.PackageType), info.Name, info.Version, bytes.NewReader(info.Content), info.Size)
	if err != nil {
		return "", err
	}

	_, _, dbErr := b.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           info.Name,
		Type:           info.PackageType,
		RepositoryID:   info.RepoID,
		RepositoryType: model.RepoTypeProxy,
	}, &model.PackageVersion{
		Version:     info.Version,
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   info.Size,
		Metadata:    marshalMetadata(info.Metadata),
	})

	if dbErr != nil {
		return storageKey, dbErr
	}

	return storageKey, nil
}

func (b *BaseAdapter) StoreProxyPackageFromReader(ctx context.Context, pkgType model.PackageType, name, version string, content io.Reader, size int64, repoID uint, metadata map[string]interface{}) (string, error) {
	storageKey, err := b.storageSvc.StorePackage(ctx, string(pkgType), name, version, content, size)
	if err != nil {
		return "", err
	}

	_, _, dbErr := b.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           pkgType,
		RepositoryID:   repoID,
		RepositoryType: model.RepoTypeProxy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   size,
		Metadata:    marshalMetadata(metadata),
	})

	if dbErr != nil {
		return storageKey, dbErr
	}

	return storageKey, nil
}

func (b *BaseAdapter) StoreProxyPackageFromResult(ctx context.Context, pkgType model.PackageType, name, version string, result *proxy.RouteResult, metadata map[string]interface{}) (string, error) {
	defer result.Content.Close()
	return b.StoreProxyPackageFromReader(ctx, pkgType, name, version, result.Content, result.Size, result.RepoID, metadata)
}

func (b *BaseAdapter) IncrementDownloadCountForPackage(pkgName string, pkgType model.PackageType, version string, filename string) {
	pkg, err := b.pkgRepo.FindByNameAndType(pkgName, pkgType)
	if err != nil {
		return
	}

	for _, v := range pkg.Versions {
		if v.Version == version {
			for _, f := range v.Files {
				if f.Filename == filename {
					b.pkgRepo.IncrementDownloadCount(pkg.ID, v.ID, f.ID)
					return
				}
			}
			return
		}
	}
}

func (b *BaseAdapter) GetPackageMetadata(ctx context.Context, name string, pkgType model.PackageType, typeStr types.PackageType) (*types.PackageMeta, error) {
	pkg, err := b.pkgRepo.FindByNameAndType(name, pkgType)
	if err != nil {
		return nil, err
	}

	meta := &types.PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        typeStr,
		Description: pkg.Description,
	}

	for _, ver := range pkg.Versions {
		var totalSize int64
		for _, f := range ver.Files {
			totalSize += f.SizeBytes
		}
		meta.Versions = append(meta.Versions, types.VersionInfo{
			Version:       ver.Version,
			PublishedAt:   ver.PublishedAt.Format(time.RFC3339),
			Size:          totalSize,
			DownloadCount: int64(ver.DownloadCount),
		})
	}

	return meta, nil
}

func (b *BaseAdapter) GetPackageRepository() *repository.PackageRepository {
	return b.pkgRepo
}

func (b *BaseAdapter) GetStorageService() *service.StorageService {
	return b.storageSvc
}

type UploadHelperOpts struct {
	PkgType        string
	Name           string
	Version        string
	StorageVersion string
	Filename       string
	PackageType    model.PackageType
	RepositoryType model.RepositoryType
	RepositoryID   uint
	UploadedBy     uint
	Metadata       map[string]interface{}
	FileType       model.PackageFileType
}

func (b *BaseAdapter) ExecuteUpload(ctx context.Context, opts *UploadHelperOpts, reader io.Reader, size int64, uploadSvc *service.UploadService) (*PackageVersionResult, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	storageVersion := opts.StorageVersion
	if storageVersion == "" {
		storageVersion = opts.Version
	}

	result, err := uploadSvc.Upload(ctx, &service.UploadContext{
		PkgType:        opts.PkgType,
		Name:           opts.Name,
		Version:        opts.Version,
		StorageVersion: storageVersion,
		Filename:       opts.Filename,
		Content:        content,
		Size:           size,
		PackageType:    opts.PackageType,
		RepositoryType: opts.RepositoryType,
		RepositoryID:   opts.RepositoryID,
		UploadedBy:     opts.UploadedBy,
		Metadata:       opts.Metadata,
		FileType:       opts.FileType,
	})

	if err != nil {
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  result.PackageID,
		VersionID:  result.VersionID,
		Version:    result.Version,
		StorageKey: result.StorageKey,
		Size:       result.Size,
		Checksum:   result.ChecksumSHA256,
	}, nil
}

type ProxyDownloadAndCacheOpts struct {
	PkgType     model.PackageType
	Name        string
	Version     string
	Filename    string
	ContentType string
	Repo        *model.Repository
	URLBuilder  proxy.URLBuilder
}

func (b *BaseAdapter) DownloadFromProxyAndCache(c *gin.Context, opts *ProxyDownloadAndCacheOpts) bool {
	if b.proxyRouter == nil {
		return false
	}

	result, err := b.proxyRouter.ResolveSmart(c.Request.Context(), opts.Repo, string(opts.PkgType), opts.Name, opts.Version, opts.URLBuilder)
	if err != nil {
		return false
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		return false
	}

	storageKey, storeErr := b.storageSvc.StorePackage(c.Request.Context(), string(opts.PkgType), opts.Name, opts.Version, bytes.NewReader(body), result.Size)
	if storeErr == nil {
		b.pkgRepo.StorePackageFileAndIncrementDownload(c.Request.Context(), &model.Package{
			Name:           opts.Name,
			Type:           opts.PkgType,
			RepositoryID:   result.RepoID,
			RepositoryType: model.RepoTypeProxy,
		}, &model.PackageVersion{
			Version:     opts.Version,
			Status:      model.StatusPublished,
			StoragePath: storageKey,
		}, &model.PackageFile{
			Filename:    opts.Filename,
			FileType:    model.FileTypePrimary,
			StoragePath: storageKey,
			SizeBytes:   result.Size,
		})
	}

	contentType := opts.ContentType
	if contentType == "" {
		contentType = b.storageSvc.GetContentType(opts.Filename)
	}
	c.Data(200, contentType, body)
	return true
}

func (b *BaseAdapter) GetMetadataFromProxy(c *gin.Context, repo *model.Repository, urlBuilder proxy.URLBuilder) bool {
	if b.proxyRouter == nil {
		return false
	}

	result, err := b.proxyRouter.ResolveProxyOnlyForRepo(c.Request.Context(), repo, "", "", urlBuilder)
	if err != nil {
		return false
	}
	defer result.Content.Close()

	c.DataFromReader(200, result.Size, "application/json", result.Content, nil)
	return true
}
