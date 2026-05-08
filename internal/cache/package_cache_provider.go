package cache

import (
	"context"
	"time"
)

type PackageCacheProvider struct {
	cache       *PackageCache
	name        string
	cacheType   string
	description string
}

func NewPackageCacheProvider(c *PackageCache, name, description string) *PackageCacheProvider {
	return &PackageCacheProvider{
		cache:       c,
		name:        name,
		cacheType:   "package",
		description: description,
	}
}

func (p *PackageCacheProvider) Name() string {
	return p.name
}

func (p *PackageCacheProvider) Type() string {
	return p.cacheType
}

func (p *PackageCacheProvider) Description() string {
	return p.description
}

func (p *PackageCacheProvider) Get(ctx context.Context, key string) (interface{}, error) {
	if val, ok := p.cache.cache.Get(key); ok {
		return val, nil
	}
	return nil, nil
}

func (p *PackageCacheProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (p *PackageCacheProvider) Delete(ctx context.Context, key string) error {
	p.cache.cache.Delete(key)
	return nil
}

func (p *PackageCacheProvider) Invalidate(ctx context.Context, pattern string) error {
	p.cache.cache.Invalidate(pattern)
	return nil
}

func (p *PackageCacheProvider) Clear(ctx context.Context) error {
	p.cache.Clear()
	return nil
}

func (p *PackageCacheProvider) Stats(ctx context.Context) *CacheStats {
	stats := p.cache.cache.Stats()
	return &CacheStats{
		TotalItems:   int64(stats["total_items"].(int)),
		ActiveItems:  int64(stats["active_items"].(int)),
		ExpiredItems: int64(stats["expired_items"].(int)),
		NumShards:    stats["num_shards"].(int),
		TTLSeconds:   int64(p.cache.TTL().Seconds()),
	}
}

func (p *PackageCacheProvider) ListItems(offset, limit int, search string) ([]CacheItem, int) {
	return nil, 0
}
