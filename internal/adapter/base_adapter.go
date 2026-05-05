package adapter

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
)

type BaseAdapter struct {
	pkgRepo    *repository.PackageRepository
	storageSvc *service.StorageService
	webhookSvc *service.WebhookService
}

func NewBaseAdapter(pkgRepo *repository.PackageRepository, storageSvc *service.StorageService) *BaseAdapter {
	return &BaseAdapter{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
	}
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

func (b *BaseAdapter) StoreProxyPackageFromResult(ctx context.Context, pkgType model.PackageType, name, version string, result *proxy.RouteResult, metadata map[string]interface{}) (string, error) {
	body, err := io.ReadAll(result.Content)
	if err != nil {
		return "", err
	}

	info := &ProxyPackageInfo{
		PackageType: pkgType,
		Name:        name,
		Version:     version,
		RepoID:      result.RepoID,
		Content:     body,
		Size:        result.Size,
		Metadata:    metadata,
	}

	return b.StoreProxyPackage(ctx, info)
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
