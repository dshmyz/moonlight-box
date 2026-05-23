package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type ComponentCache struct {
	cache    *MemoryCache
	compRepo *repository.ComponentRepository
	ttl      time.Duration
}

func NewComponentCache(compRepo *repository.ComponentRepository, ttl time.Duration) *ComponentCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &ComponentCache{
		cache:    NewMemoryCache(),
		compRepo: compRepo,
		ttl:      ttl,
	}
}

func (c *ComponentCache) GetByRepoNameAndType(repositoryID uint, name string, format model.PackageType) (*repository.ComponentAggregate, error) {
	key := c.makeRepoKey(repositoryID, name, format)
	if val, ok := c.cache.Get(key); ok {
		if agg, ok := val.(*repository.ComponentAggregate); ok {
			return agg, nil
		}
	}
	agg, err := c.compRepo.FindByRepoNameAndTypeContext(context.Background(), repositoryID, name, format)
	if err != nil {
		return nil, err
	}
	c.cache.Set(key, agg, c.ttl)
	return agg, nil
}

func (c *ComponentCache) Invalidate(name string, format model.PackageType) {
	c.cache.Delete(c.makeRepoKey(0, name, format))
}

func (c *ComponentCache) InvalidateByName(name string) {
	c.cache.Invalidate(fmt.Sprintf("comp:%s:", name))
}

func (c *ComponentCache) InvalidateByType(format model.PackageType) {
	c.cache.Invalidate(fmt.Sprintf("comp:*:%s", format))
}

func (c *ComponentCache) Clear() {
	c.cache.Clear()
}

func (c *ComponentCache) TTL() time.Duration {
	return c.ttl
}

func (c *ComponentCache) GetComponentRepository() *repository.ComponentRepository {
	return c.compRepo
}

func (c *ComponentCache) makeRepoKey(repositoryID uint, name string, format model.PackageType) string {
	return fmt.Sprintf("comp:%d:%s:%s", repositoryID, name, format)
}
