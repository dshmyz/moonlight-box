package cache

import (
	"container/list"
	"sync"
	"time"
)

// CacheKey 是 MetadataCache 的最小 key 约束：能自序列化为唯一字符串。
// core/cache 不依赖 core/runtime（避免环），runtime.ArtifactKey 天然满足该接口。
type CacheKey interface {
	String() string
}

// MetadataCache 是带 TTL 和 LRU 淘汰的元数据缓存，支持独立的负缓存 TTL。
//
// 用双向链表维护访问顺序，Get/Set/Evict 均为 O(1)，
// 避免 sync.Map.Range 全表扫描的反模式（修复前 evictOldest 是 O(n)）。
//
// entry 保留原始 CacheKey（而非仅字符串 key）：WarmUp 重放需要完整 key 对象
// （scoped 名称与 Qualifiers），无法从 key.String() 反解。
type MetadataCache struct {
	mu          sync.Mutex
	store       map[string]*list.Element
	ll          *list.List
	ttl         time.Duration
	negativeTTL time.Duration
	maxSize     int
}

type cachedMetadata struct {
	key        string
	cacheKey   CacheKey
	value      any
	expiresAt  time.Time
	isNegative bool
}

// NewMetadataCache 构造元数据缓存。negativeTTL 为负缓存（"查无此物"）的独立
// 默认 TTL，传 0 时与 ttl 相同（保持旧行为）。maxSize<=0 表示不限制容量。
// 正/负条目共享同一 LRU（总容量受 maxSize 约束）；单条负缓存 TTL 可经
// SetNegativeWithTTL 覆盖。
func NewMetadataCache(ttl, negativeTTL time.Duration, maxSize int) *MetadataCache {
	if negativeTTL <= 0 {
		negativeTTL = ttl
	}
	return &MetadataCache{
		store:       make(map[string]*list.Element),
		ll:          list.New(),
		ttl:         ttl,
		negativeTTL: negativeTTL,
		maxSize:     maxSize,
	}
}

func (c *MetadataCache) Get(key CacheKey) (any, bool) {
	cacheKey := key.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	// 负缓存优先：key 的远端查询确认不存在且仍在负缓存 TTL 内 → miss（不回源）
	negativeKey := cacheKey + ":negative"
	if el, ok := c.store[negativeKey]; ok {
		cached := el.Value.(*cachedMetadata)
		if time.Now().After(cached.expiresAt) {
			c.removeElementLocked(el)
		} else {
			c.ll.MoveToBack(el)
			return nil, false
		}
	}

	el, ok := c.store[cacheKey]
	if !ok {
		return nil, false
	}

	cached := el.Value.(*cachedMetadata)
	if time.Now().After(cached.expiresAt) {
		c.removeElementLocked(el)
		return nil, false
	}

	// 命中后移到队尾（最近访问），LRU 淘汰时从队首删
	c.ll.MoveToBack(el)
	return cached.value, true
}

// IsNegative 报告 key 是否处于负缓存 TTL 内（调用方应避免立即回源）。
func (c *MetadataCache) IsNegative(key CacheKey) bool {
	negativeKey := key.String() + ":negative"

	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.store[negativeKey]
	if !ok {
		return false
	}
	cached := el.Value.(*cachedMetadata)
	if time.Now().After(cached.expiresAt) {
		c.removeElementLocked(el)
		return false
	}
	c.ll.MoveToBack(el)
	return true
}

func (c *MetadataCache) Set(key CacheKey, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(key.String(), key, &cachedMetadata{
		cacheKey:  key,
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	})
}

// SetNegative 记录负缓存（key 对应的远端查询确认不存在），TTL 为构造时的 negativeTTL。
func (c *MetadataCache) SetNegative(key CacheKey) {
	c.SetNegativeWithTTL(key, c.negativeTTL)
}

// SetNegativeWithTTL 设置自定义 TTL 的负缓存条目（如故障退避短 TTL）。
// ttl<=0 时忽略（调用方应保证 ttl>0）。
func (c *MetadataCache) SetNegativeWithTTL(key CacheKey, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(key.String()+":negative", key, &cachedMetadata{
		cacheKey:   key,
		expiresAt:  time.Now().Add(ttl),
		isNegative: true,
	})
}

// setLocked 写入/覆盖条目。cached.expiresAt 由调用方按正/负 TTL 计算。
// 调用方必须持有 c.mu。
func (c *MetadataCache) setLocked(cacheKey string, key CacheKey, cached *cachedMetadata) {
	cached.key = cacheKey

	if el, ok := c.store[cacheKey]; ok {
		el.Value = cached
		c.ll.MoveToBack(el)
		return
	}

	el := c.ll.PushBack(cached)
	c.store[cacheKey] = el

	if c.maxSize > 0 && c.ll.Len() > c.maxSize {
		c.evictOldestLocked()
	}
}

func (c *MetadataCache) Invalidate(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheKey := key.String()
	if el, ok := c.store[cacheKey]; ok {
		c.removeElementLocked(el)
	}
	negativeKey := cacheKey + ":negative"
	if el, ok := c.store[negativeKey]; ok {
		c.removeElementLocked(el)
	}
}

// DeleteNegativeByKey 按字符串 key 清除负缓存条目（回源成功后按
// buildNegativeCacheKeys 生成的 key 列表清理时使用）。
func (c *MetadataCache) DeleteNegativeByKey(cacheKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.store[cacheKey+":negative"]; ok {
		c.removeElementLocked(el)
	}
}

// MetadataCacheItem 是 Items() 返回的单条快照。
type MetadataCacheItem struct {
	Key        string
	CacheKey   CacheKey
	Value      any
	IsNegative bool
	ExpiresAt  time.Time
}

// Items 返回缓存内全部条目的快照（含未过期负缓存），供 WarmUp 重放与
// 管理端展示使用。返回副本，遍历期间不持锁。
func (c *MetadataCache) Items() []MetadataCacheItem {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	items := make([]MetadataCacheItem, 0, c.ll.Len())
	for el := c.ll.Front(); el != nil; el = el.Next() {
		cached := el.Value.(*cachedMetadata)
		if now.After(cached.expiresAt) {
			continue
		}
		items = append(items, MetadataCacheItem{
			Key:        cached.key,
			CacheKey:   cached.cacheKey,
			Value:      cached.value,
			IsNegative: cached.isNegative,
			ExpiresAt:  cached.expiresAt,
		})
	}
	return items
}

// Len 返回当前条目数（含尚未被移除的过期项，LRU 淘汰以链表长度为准）。
func (c *MetadataCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// Clear 清空全部条目（含负缓存）。
func (c *MetadataCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]*list.Element)
	c.ll.Init()
}

// evictOldestLocked 删除队首元素（最久未访问），O(1)。
// 调用方必须持有 c.mu。
func (c *MetadataCache) evictOldestLocked() {
	el := c.ll.Front()
	if el == nil {
		return
	}
	c.removeElementLocked(el)
}

// removeElementLocked 从 map 和链表中同步移除元素，O(1)。
// 调用方必须持有 c.mu。
func (c *MetadataCache) removeElementLocked(el *list.Element) {
	cached := el.Value.(*cachedMetadata)
	delete(c.store, cached.key)
	c.ll.Remove(el)
}
