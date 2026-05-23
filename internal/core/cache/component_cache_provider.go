package cache

import (
	"context"
	"fmt"
	"time"
)

type ComponentCacheProvider struct {
	cache       *ComponentCache
	name        string
	description string
}

func NewComponentCacheProvider(c *ComponentCache, name, description string) *ComponentCacheProvider {
	return &ComponentCacheProvider{
		cache:       c,
		name:        name,
		description: description,
	}
}

func (p *ComponentCacheProvider) Name() string { return p.name }

func (p *ComponentCacheProvider) Type() string { return "component-metadata" }

func (p *ComponentCacheProvider) Description() string { return p.description }

func (p *ComponentCacheProvider) Get(ctx context.Context, key string) (interface{}, error) {
	val, ok := p.cache.cache.Get(key)
	if !ok {
		return nil, fmt.Errorf("cache miss: %s", key)
	}
	return val, nil
}

func (p *ComponentCacheProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	p.cache.cache.Set(key, value, ttl)
	return nil
}

func (p *ComponentCacheProvider) Delete(ctx context.Context, key string) error {
	p.cache.cache.Delete(key)
	return nil
}

func (p *ComponentCacheProvider) Invalidate(ctx context.Context, pattern string) error {
	p.cache.cache.Invalidate(pattern)
	return nil
}

func (p *ComponentCacheProvider) Clear(ctx context.Context) error {
	p.cache.Clear()
	return nil
}

func (p *ComponentCacheProvider) Stats(ctx context.Context) *CacheStats {
	stats := p.cache.cache.Stats()
	return &CacheStats{
		TotalItems:  int64(stats["total_items"].(int)),
		ActiveItems: int64(stats["active_items"].(int)),
		ExpiredItems: int64(stats["expired_items"].(int)),
		NumShards:   stats["num_shards"].(int),
	}
}

func (p *ComponentCacheProvider) ListItems(offset, limit int, search string) ([]CacheItem, int) {
	return p.cache.cache.ListItems(offset, limit, search)
}
