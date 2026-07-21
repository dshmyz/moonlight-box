package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

// MetadataCache 是带 TTL 和 LRU 淘汰的元数据缓存。
//
// 用双向链表维护访问顺序，Get/Set/Evict 均为 O(1)，
// 避免 sync.Map.Range 全表扫描的反模式（修复前 evictOldest 是 O(n)）。
type MetadataCache struct {
	mu      sync.Mutex
	store   map[string]*list.Element
	ll      *list.List
	ttl     time.Duration
	maxSize int
}

type cachedMetadata struct {
	key        string
	artifact   *runtime.Artifact
	cachedAt   time.Time
	isNegative bool
}

func NewMetadataCache(ttl time.Duration, maxSize int) *MetadataCache {
	return &MetadataCache{
		store:   make(map[string]*list.Element),
		ll:      list.New(),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

func (c *MetadataCache) Get(ctx context.Context, key *runtime.ArtifactKey) (*runtime.Artifact, bool) {
	cacheKey := key.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.store[cacheKey]
	if !ok {
		return nil, false
	}

	cached := el.Value.(*cachedMetadata)
	if time.Since(cached.cachedAt) > c.ttl {
		c.removeElementLocked(el)
		return nil, false
	}

	if cached.isNegative {
		// 负缓存命中：命中后移到队尾（最近访问），但不返回 artifact
		c.ll.MoveToBack(el)
		return nil, false
	}

	// 命中后移到队尾（最近访问），LRU 淘汰时从队首删
	c.ll.MoveToBack(el)
	return cached.artifact, true
}

func (c *MetadataCache) Set(ctx context.Context, key *runtime.ArtifactKey, artifact *runtime.Artifact) {
	c.setLocked(key.String(), &cachedMetadata{
		artifact: artifact,
		cachedAt: time.Now(),
	})
}

func (c *MetadataCache) SetNegative(ctx context.Context, key *runtime.ArtifactKey) {
	c.setLocked(key.String()+":negative", &cachedMetadata{
		cachedAt:   time.Now(),
		isNegative: true,
	})
}

func (c *MetadataCache) setLocked(cacheKey string, cached *cachedMetadata) {
	cached.key = cacheKey
	c.mu.Lock()
	defer c.mu.Unlock()

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

func (c *MetadataCache) Invalidate(ctx context.Context, key *runtime.ArtifactKey) {
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
