package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
)

type BaseAdapter struct {
	storageSvc *service.StorageService
	fetcher    proxy.ProxyFetcher
	pkgCache   *cache.PackageCache
}

func NewBaseAdapter(storageSvc *service.StorageService, pkgCache *cache.PackageCache) *BaseAdapter {
	return &BaseAdapter{
		storageSvc: storageSvc,
		pkgCache:   pkgCache,
	}
}

func (b *BaseAdapter) SetFetcher(fetcher proxy.ProxyFetcher) {
	b.fetcher = fetcher
}

func (b *BaseAdapter) GetPackageMetadata(ctx context.Context, name string, pkgType model.PackageType, typeStr types.PackageType) (*types.PackageMeta, error) {
	if b.pkgCache == nil {
		return nil, fmt.Errorf("package cache not initialized")
	}

	pkg, err := b.pkgCache.GetByNameAndType(name, pkgType)
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
	if b.pkgCache == nil {
		return nil
	}
	return b.pkgCache.GetPackageRepository()
}

func (b *BaseAdapter) GetStorageService() *service.StorageService {
	return b.storageSvc
}