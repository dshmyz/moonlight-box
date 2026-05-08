package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
)

type CacheServiceProvider struct {
	svc         *CacheService
	name        string
	cacheType   string
	description string
}

func NewCacheServiceProvider(svc *CacheService, name, description string) *CacheServiceProvider {
	return &CacheServiceProvider{
		svc:         svc,
		name:        name,
		cacheType:   "content",
		description: description,
	}
}

func (p *CacheServiceProvider) Name() string {
	return p.name
}

func (p *CacheServiceProvider) Type() string {
	return p.cacheType
}

func (p *CacheServiceProvider) Description() string {
	return p.description
}

func (p *CacheServiceProvider) Get(ctx context.Context, key string) (interface{}, error) {
	return p.svc.Get(ctx, key)
}

func (p *CacheServiceProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	item, ok := value.(*CacheItem)
	if !ok {
		return fmt.Errorf("invalid value type, expected *CacheItem")
	}
	return p.svc.Set(ctx, item, ttl)
}

func (p *CacheServiceProvider) Delete(ctx context.Context, key string) error {
	return p.svc.DeleteItem(key)
}

func (p *CacheServiceProvider) Invalidate(ctx context.Context, pattern string) error {
	return p.svc.Invalidate(ctx, pattern)
}

func (p *CacheServiceProvider) Clear(ctx context.Context) error {
	return p.svc.Clear(ctx)
}

func (p *CacheServiceProvider) Stats(ctx context.Context) *cache.CacheStats {
	stats, err := p.svc.GetStats(ctx)
	if err != nil {
		return &cache.CacheStats{}
	}
	return &cache.CacheStats{
		TotalItems:   int64(stats["total_items"].(int)),
		ActiveItems:  int64(stats["positive_items"].(int)),
		ExpiredItems: int64(stats["negative_items"].(int)),
		UsedBytes:    stats["used_bytes"].(int64),
		MaxBytes:     stats["max_bytes"].(int64),
		MaxItems:     int64(stats["max_items"].(int)),
		NumShards:    stats["num_shards"].(int),
	}
}

func (p *CacheServiceProvider) ListItems(offset, limit int, search string) ([]cache.CacheItem, int) {
	items, total := p.svc.ListItems(offset, limit, search)
	result := make([]cache.CacheItem, len(items))
	for i, item := range items {
		expiry := time.Time{}
		if expiryStr, ok := item["expiry"].(string); ok {
			expiry, _ = time.Parse(time.RFC3339, expiryStr)
		}
		result[i] = cache.CacheItem{
			Key:         item["key"].(string),
			Size:        item["size"].(int64),
			ContentType: item["content_type"].(string),
			IsNegative:  item["is_negative"].(bool),
			Expiry:      expiry,
			RemainingTTL: int64(item["remaining_ttl"].(int64)),
			IsExpired:   item["is_expired"].(bool),
		}
	}
	return result, total
}
