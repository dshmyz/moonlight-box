package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type PackageCache struct {
	cache   *MemoryCache
	pkgRepo *repository.PackageRepository
	ttl     time.Duration
}

func NewPackageCache(pkgRepo *repository.PackageRepository, ttl time.Duration) *PackageCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &PackageCache{
		cache:   NewMemoryCache(),
		pkgRepo: pkgRepo,
		ttl:     ttl,
	}
}

func (c *PackageCache) GetByNameAndType(name string, pkgType model.PackageType) (*model.Package, error) {
	return c.GetByRepoNameAndType(0, name, pkgType)
}

func (c *PackageCache) GetByRepoNameAndType(repositoryID uint, name string, pkgType model.PackageType) (*model.Package, error) {
	key := c.makeRepoKey(repositoryID, name, pkgType)

	if val, ok := c.cache.Get(key); ok {
		if pkg, ok := val.(*model.Package); ok {
			return pkg, nil
		}
	}

	pkg, err := c.pkgRepo.FindByRepoNameAndTypeContext(context.Background(), repositoryID, name, pkgType)
	if err != nil {
		return nil, err
	}

	c.cache.Set(key, pkg, c.ttl)
	return pkg, nil
}

func (c *PackageCache) Invalidate(name string, pkgType model.PackageType) {
	key := c.makeKey(name, pkgType)
	c.cache.Delete(key)
}

func (c *PackageCache) InvalidateByName(name string) {
	c.cache.Invalidate(fmt.Sprintf("pkg:%s:", name))
}

func (c *PackageCache) InvalidateByType(pkgType model.PackageType) {
	c.cache.Invalidate(fmt.Sprintf("pkg:*:%s", pkgType))
}

func (c *PackageCache) Clear() {
	c.cache.Clear()
}

func (c *PackageCache) TTL() time.Duration {
	return c.ttl
}

func (c *PackageCache) makeKey(name string, pkgType model.PackageType) string {
	return c.makeRepoKey(0, name, pkgType)
}

func (c *PackageCache) makeRepoKey(repositoryID uint, name string, pkgType model.PackageType) string {
	return fmt.Sprintf("pkg:%d:%s:%s", repositoryID, name, pkgType)
}

func (c *PackageCache) GetPackageRepository() *repository.PackageRepository {
	return c.pkgRepo
}
