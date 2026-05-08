package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
)

type RepositoryCacheProvider struct {
	cache       *RepositoryCache
	name        string
	cacheType   string
	description string
}

func NewRepositoryCacheProvider(c *RepositoryCache, name, description string) *RepositoryCacheProvider {
	return &RepositoryCacheProvider{
		cache:       c,
		name:        name,
		cacheType:   "repository",
		description: description,
	}
}

func (p *RepositoryCacheProvider) Name() string {
	return p.name
}

func (p *RepositoryCacheProvider) Type() string {
	return p.cacheType
}

func (p *RepositoryCacheProvider) Description() string {
	return p.description
}

func (p *RepositoryCacheProvider) Get(ctx context.Context, key string) (interface{}, error) {
	return p.cache.GetByName(key)
}

func (p *RepositoryCacheProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return fmt.Errorf("repository cache is read-through, use Invalidate to refresh")
}

func (p *RepositoryCacheProvider) Delete(ctx context.Context, key string) error {
	p.cache.Invalidate(key)
	return nil
}

func (p *RepositoryCacheProvider) Invalidate(ctx context.Context, pattern string) error {
	p.cache.Invalidate(pattern)
	return nil
}

func (p *RepositoryCacheProvider) Clear(ctx context.Context) error {
	p.cache.Invalidate("*")
	return nil
}

func (p *RepositoryCacheProvider) Stats(ctx context.Context) *cache.CacheStats {
	return &cache.CacheStats{
		TTLSeconds: int64(p.cache.TTL().Seconds()),
	}
}

func (p *RepositoryCacheProvider) ListItems(offset, limit int, search string) ([]cache.CacheItem, int) {
	return nil, 0
}
