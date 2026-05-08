package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

type CacheRegistryInfo struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
}

type CacheManager struct {
	mu      sync.RWMutex
	caches  map[string]CacheProvider
}

func NewCacheManager() *CacheManager {
	return &CacheManager{
		caches: make(map[string]CacheProvider),
	}
}

func (m *CacheManager) Register(provider CacheProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.caches[provider.Name()] = provider
}

func (m *CacheManager) Get(name string) (CacheProvider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.caches[name]
	return p, ok
}

func (m *CacheManager) List() []CacheRegistryInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]CacheRegistryInfo, 0, len(m.caches))
	for _, p := range m.caches {
		result = append(result, CacheRegistryInfo{
			Name:        p.Name(),
			Type:        p.Type(),
			Description: p.Description(),
		})
	}
	return result
}

func (m *CacheManager) GetByName(ctx context.Context, name string) (interface{}, error) {
	m.mu.RLock()
	p, ok := m.caches[name]
	m.mu.RUnlock()

	if !ok {
		return nil, nil
	}
	return p.Get(ctx, name)
}

func (m *CacheManager) Invalidate(ctx context.Context, name string, pattern string) error {
	m.mu.RLock()
	p, ok := m.caches[name]
	m.mu.RUnlock()

	if !ok {
		return nil
	}
	return p.Invalidate(ctx, pattern)
}

func (m *CacheManager) Clear(ctx context.Context, name string) error {
	m.mu.RLock()
	p, ok := m.caches[name]
	m.mu.RUnlock()

	if !ok {
		return nil
	}
	return p.Clear(ctx)
}

func (m *CacheManager) ClearAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.caches {
		p.Clear(ctx)
	}
	return nil
}

func (m *CacheManager) StatsAll(ctx context.Context) map[string]*CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*CacheStats)
	for name, p := range m.caches {
		result[name] = p.Stats(ctx)
	}
	return result
}

func (m *CacheManager) ListAllItems(offset, limit int, search string, cacheType string) ([]CacheItemWithSource, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allItems []CacheItemWithSource
	for name, p := range m.caches {
		if cacheType != "" && p.Type() != cacheType {
			continue
		}

		items, total := p.ListItems(offset, limit, search)
		for _, item := range items {
			allItems = append(allItems, CacheItemWithSource{
				CacheItem:   item,
				SourceCache: name,
				CacheType:   p.Type(),
			})
		}
		_ = total
	}

	total := len(allItems)

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if offset >= len(allItems) {
		return []CacheItemWithSource{}, total
	}

	end := offset + limit
	if end > len(allItems) {
		end = len(allItems)
	}

	return allItems[offset:end], total
}

type CacheItemWithSource struct {
	CacheItem
	SourceCache string `json:"source_cache"`
	CacheType   string `json:"cache_type"`
}

type MemoryCacheWrapper struct {
	cache       *MemoryCache
	name        string
	cacheType   string
	description string
}

func NewMemoryCacheWrapper(cache *MemoryCache, name, cacheType, description string) *MemoryCacheWrapper {
	return &MemoryCacheWrapper{
		cache:       cache,
		name:        name,
		cacheType:   cacheType,
		description: description,
	}
}

func (w *MemoryCacheWrapper) Name() string {
	return w.name
}

func (w *MemoryCacheWrapper) Type() string {
	return w.cacheType
}

func (w *MemoryCacheWrapper) Description() string {
	return w.description
}

func (w *MemoryCacheWrapper) Get(ctx context.Context, key string) (interface{}, error) {
	val, ok := w.cache.Get(key)
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (w *MemoryCacheWrapper) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	w.cache.Set(key, value, ttl)
	return nil
}

func (w *MemoryCacheWrapper) Delete(ctx context.Context, key string) error {
	w.cache.Delete(key)
	return nil
}

func (w *MemoryCacheWrapper) Invalidate(ctx context.Context, pattern string) error {
	w.cache.Invalidate(pattern)
	return nil
}

func (w *MemoryCacheWrapper) Clear(ctx context.Context) error {
	w.cache.Clear()
	return nil
}

func (w *MemoryCacheWrapper) Stats(ctx context.Context) *CacheStats {
	stats := w.cache.Stats()
	return &CacheStats{
		TotalItems:   int64(stats["total_items"].(int)),
		ActiveItems:  int64(stats["active_items"].(int)),
		ExpiredItems: int64(stats["expired_items"].(int)),
		NumShards:    stats["num_shards"].(int),
	}
}

func (w *MemoryCacheWrapper) ListItems(offset, limit int, search string) ([]CacheItem, int) {
	return nil, 0
}

func containsIgnoreCase(s, substr string) bool {
	if substr == "" {
		return true
	}
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Contains(sLower, substrLower)
}
