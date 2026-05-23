package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/core/cache"
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
	now := time.Now()

	p.cache.mu.RLock()
	defer p.cache.mu.RUnlock()

	var items []cache.CacheItem

	for name, entry := range p.cache.repos {
		if search != "" && !containsIgnoreCaseRepo(name, search) {
			continue
		}

		isExpired := now.After(entry.expiresAt)
		var remainingTTL int64
		if !isExpired {
			remainingTTL = int64(entry.expiresAt.Sub(now).Seconds())
		}

		items = append(items, cache.CacheItem{
			Key:          fmt.Sprintf("repo:name:%s", name),
			Size:         0,
			ContentType:  "repository",
			IsNegative:   false,
			Expiry:       entry.expiresAt,
			RemainingTTL: remainingTTL,
			IsExpired:    isExpired,
			Metadata: map[string]interface{}{
				"repo_id":   entry.repo.ID,
				"repo_type": entry.repo.Type,
			},
		})
	}

	for repoID, entries := range p.cache.members {
		key := fmt.Sprintf("repo:members:%d", repoID)
		if search != "" && !containsIgnoreCaseRepo(key, search) {
			continue
		}

		var expiry time.Time
		isExpired := true
		if len(entries) > 0 {
			expiry = entries[0].expiresAt
			isExpired = now.After(expiry)
		}

		var remainingTTL int64
		if !isExpired {
			remainingTTL = int64(expiry.Sub(now).Seconds())
		}

		items = append(items, cache.CacheItem{
			Key:          key,
			Size:         0,
			ContentType:  "repository",
			IsNegative:   false,
			Expiry:       expiry,
			RemainingTTL: remainingTTL,
			IsExpired:    isExpired,
			Metadata: map[string]interface{}{
				"member_count": len(entries),
			},
		})
	}

	return cache.ListItemsPaginator(items, offset, limit)
}

func containsIgnoreCaseRepo(s, substr string) bool {
	if substr == "" {
		return true
	}
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Contains(sLower, substrLower)
}
