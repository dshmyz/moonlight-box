package service

import (
	"context"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
)

type PermissionCacheProvider struct {
	svc         *PermissionCacheService
	name        string
	cacheType   string
	description string
}

func NewPermissionCacheProvider(svc *PermissionCacheService, name, description string) *PermissionCacheProvider {
	return &PermissionCacheProvider{
		svc:         svc,
		name:        name,
		cacheType:   "permission",
		description: description,
	}
}

func (p *PermissionCacheProvider) Name() string {
	return p.name
}

func (p *PermissionCacheProvider) Type() string {
	return p.cacheType
}

func (p *PermissionCacheProvider) Description() string {
	return p.description
}

func (p *PermissionCacheProvider) Get(ctx context.Context, key string) (interface{}, error) {
	return nil, nil
}

func (p *PermissionCacheProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (p *PermissionCacheProvider) Delete(ctx context.Context, key string) error {
	return nil
}

func (p *PermissionCacheProvider) Invalidate(ctx context.Context, pattern string) error {
	p.svc.InvalidateAll()
	return nil
}

func (p *PermissionCacheProvider) Clear(ctx context.Context) error {
	p.svc.InvalidateAll()
	return nil
}

func (p *PermissionCacheProvider) Stats(ctx context.Context) *cache.CacheStats {
	stats := p.svc.GetStats()
	return &cache.CacheStats{
		TotalItems:   int64(stats["total_items"].(int)),
		ActiveItems:  int64(stats["active_items"].(int)),
		ExpiredItems: int64(stats["expired_items"].(int)),
		NumShards:    stats["num_shards"].(int),
	}
}

func (p *PermissionCacheProvider) ListItems(offset, limit int, search string) ([]cache.CacheItem, int) {
	now := time.Now()
	allCacheItems := p.svc.GetAllItems()

	var items []cache.CacheItem
	for key, item := range allCacheItems {
		if search != "" && !containsIgnoreCase(key, search) {
			continue
		}

		isExpired := item.IsExpired()
		var remainingTTL int64
		var expiry time.Time

		if !item.ExpiresAt.IsZero() {
			expiry = item.ExpiresAt
			if !isExpired {
				remainingTTL = int64(item.ExpiresAt.Sub(now).Seconds())
			}
		}

		items = append(items, cache.CacheItem{
			Key:          key,
			Size:         0,
			ContentType:  "permission",
			IsNegative:   false,
			Expiry:       expiry,
			RemainingTTL: remainingTTL,
			IsExpired:    isExpired,
		})
	}

	return cache.ListItemsPaginator(items, offset, limit)
}

func containsIgnoreCase(s, substr string) bool {
	if substr == "" {
		return true
	}
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Contains(sLower, substrLower)
}
