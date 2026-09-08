package cache

import (
	"context"
	"fmt"
	"time"
)

// MemoryCacheProvider 将通用 MemoryCache 适配为 CacheProvider 胖接口，
// 供 CacheManager 注册后在管理页统一展示/检索/清空。
// MemoryCache 的 Get/Set/Delete/Invalidate/Clear/Stats/ListItems 与接口一一对应，纯映射。
type MemoryCacheProvider struct {
	name        string
	typ         string
	description string
	mc          *MemoryCache
}

func NewMemoryCacheProvider(name, typ, description string, mc *MemoryCache) *MemoryCacheProvider {
	return &MemoryCacheProvider{name: name, typ: typ, description: description, mc: mc}
}

func (p *MemoryCacheProvider) Name() string        { return p.name }
func (p *MemoryCacheProvider) Type() string        { return p.typ }
func (p *MemoryCacheProvider) Description() string { return p.description }

func (p *MemoryCacheProvider) Get(ctx context.Context, key string) (interface{}, error) {
	if v, ok := p.mc.Get(key); ok {
		return v, nil
	}
	return nil, fmt.Errorf("cache miss: %s", key)
}

func (p *MemoryCacheProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	p.mc.Set(key, value, ttl)
	return nil
}

func (p *MemoryCacheProvider) Delete(ctx context.Context, key string) error {
	p.mc.Delete(key)
	return nil
}

func (p *MemoryCacheProvider) Invalidate(ctx context.Context, pattern string) error {
	p.mc.Invalidate(pattern)
	return nil
}

func (p *MemoryCacheProvider) Clear(ctx context.Context) error {
	p.mc.Clear()
	return nil
}

func (p *MemoryCacheProvider) Stats(ctx context.Context) *CacheStats {
	m := p.mc.Stats()
	stats := &CacheStats{}
	if v, ok := m["total_items"].(int); ok {
		stats.TotalItems = int64(v)
	}
	if v, ok := m["active_items"].(int); ok {
		stats.ActiveItems = int64(v)
	}
	if v, ok := m["expired_items"].(int); ok {
		stats.ExpiredItems = int64(v)
	}
	if v, ok := m["num_shards"].(int); ok {
		stats.NumShards = v
	}
	stats.MaxItems = int64(p.mc.MaxItems())
	return stats
}

func (p *MemoryCacheProvider) ListItems(offset, limit int, search string) ([]CacheItem, int) {
	return p.mc.ListItems(offset, limit, search)
}

// MetadataCacheProvider 将 MetadataCache 适配为 CacheProvider。
// 写路径是强类型的（必须传 CacheKey 对象），因此 Set 不支持字符串 key 写入；
// Get 通过 Items 快照按 key 精确匹配（管理页低频操作，O(n) 可接受）。
type MetadataCacheProvider struct {
	name        string
	typ         string
	description string
	mc          *MetadataCache
}

func NewMetadataCacheProvider(name, typ, description string, mc *MetadataCache) *MetadataCacheProvider {
	return &MetadataCacheProvider{name: name, typ: typ, description: description, mc: mc}
}

func (p *MetadataCacheProvider) Name() string        { return p.name }
func (p *MetadataCacheProvider) Type() string        { return p.typ }
func (p *MetadataCacheProvider) Description() string { return p.description }

func (p *MetadataCacheProvider) Get(ctx context.Context, key string) (interface{}, error) {
	for _, it := range p.mc.Items() {
		if it.Key == key && !it.IsNegative {
			return it.Value, nil
		}
	}
	return nil, fmt.Errorf("cache miss: %s", key)
}

// Set 不支持：MetadataCache 的写入必须携带 CacheKey 对象（WarmUp 重放依赖
// 完整 key），字符串 key 无法反解，故拒绝。
func (p *MetadataCacheProvider) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return fmt.Errorf("metadata cache 不支持按字符串 key 写入（需经 CacheKey 强类型写路径）")
}

func (p *MetadataCacheProvider) Delete(ctx context.Context, key string) error {
	p.mc.mu.Lock()
	defer p.mc.mu.Unlock()
	if el, ok := p.mc.store[key]; ok {
		p.mc.removeElementLocked(el)
	}
	if el, ok := p.mc.store[key+":negative"]; ok {
		p.mc.removeElementLocked(el)
	}
	return nil
}

func (p *MetadataCacheProvider) Invalidate(ctx context.Context, pattern string) error {
	for _, it := range p.mc.Items() {
		if matchPattern(it.Key, pattern) {
			if err := p.Delete(ctx, it.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *MetadataCacheProvider) Clear(ctx context.Context) error {
	p.mc.Clear()
	return nil
}

func (p *MetadataCacheProvider) Stats(ctx context.Context) *CacheStats {
	// 同包直读内部链表，一次遍历同时得到 active/expired 拆分（管理页低频操作）。
	p.mc.mu.Lock()
	now := time.Now()
	var active, expired int64
	for el := p.mc.ll.Front(); el != nil; el = el.Next() {
		cached := el.Value.(*cachedMetadata)
		if now.After(cached.expiresAt) {
			expired++
		} else {
			active++
		}
	}
	ttl, maxSize := p.mc.ttl, p.mc.maxSize
	p.mc.mu.Unlock()

	return &CacheStats{
		TotalItems:   active + expired,
		ActiveItems:  active,
		ExpiredItems: expired,
		MaxItems:     int64(maxSize),
		TTLSeconds:   int64(ttl.Seconds()),
	}
}

func (p *MetadataCacheProvider) ListItems(offset, limit int, search string) ([]CacheItem, int) {
	all := p.mc.Items()
	var matched []MetadataCacheItem
	for _, it := range all {
		if search == "" || contains(it.Key, search) {
			matched = append(matched, it)
		}
	}
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return nil, total
	}
	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}
	items := make([]CacheItem, 0, end-offset)
	for _, it := range matched[offset:end] {
		items = append(items, CacheItem{
			Key:          it.Key,
			IsNegative:   it.IsNegative,
			Expiry:       it.ExpiresAt,
			RemainingTTL: int64(time.Until(it.ExpiresAt).Seconds()),
		})
	}
	return items, total
}
