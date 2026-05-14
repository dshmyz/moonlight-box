package adapter

import (
	"context"
	"fmt"

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
	metaCache  *service.MetadataCache
}

func NewBaseAdapter(storageSvc *service.StorageService, pkgCache *cache.PackageCache) *BaseAdapter {
	return &BaseAdapter{
		storageSvc: storageSvc,
		pkgCache:   pkgCache,
	}
}

func (b *BaseAdapter) SetMetadataCache(mc *service.MetadataCache) {
	b.metaCache = mc
}

func (b *BaseAdapter) GetMetadataCache() *service.MetadataCache {
	return b.metaCache
}

func (b *BaseAdapter) SetFetcher(fetcher proxy.ProxyFetcher) {
	b.fetcher = fetcher
}

func (b *BaseAdapter) GetPackageMetadata(ctx context.Context, name string, pkgType model.PackageType, typeStr types.PackageType) (*types.PackageMeta, error) {
	return b.GetRepositoryPackageMetadata(ctx, nil, name, pkgType, typeStr)
}

func (b *BaseAdapter) GetRepositoryPackageMetadata(ctx context.Context, repo *model.Repository, name string, pkgType model.PackageType, typeStr types.PackageType) (*types.PackageMeta, error) {
	if b.pkgCache == nil {
		return nil, fmt.Errorf("package cache not initialized")
	}

	var repositoryID uint
	if repo != nil {
		repositoryID = repo.ID
	}

	pkg, err := b.pkgCache.GetByRepoNameAndType(repositoryID, name, pkgType)
	if err != nil {
		return nil, err
	}

	return packageMetaFromModel(pkg, typeStr), nil
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
