package cache

import (
	"container/list"
	"hash/fnv"
	"sync"
	"time"
)

type Item struct {
	Value     interface{}
	ExpiresAt time.Time
}

func (i *Item) IsExpired() bool {
	if i.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(i.ExpiresAt)
}

// MemoryCacheOptions 可选构造参数。零值字段表示不启用对应能力。
type MemoryCacheOptions struct {
	NumShards int // 分片数，0 取默认 16
	MaxItems  int // 全局条目上限，>0 时按分片均摊并在超容时 LRU 淘汰
}

type Shard struct {
	mu    sync.RWMutex
	items map[string]*Item

	// 容量淘汰（仅 MaxItems>0 的 shard 启用）：order 按"插入/最近访问"排序的
	// key 链表，orderMap 提供 key→element 的 O(1) 定位。命中即 MoveToBack，
	// 超容时先清扫过期项、仍超则从队首淘汰，整体表现为 LRU。
	order    *list.List
	orderMap map[string]*list.Element
	maxItems int
}

type MemoryCache struct {
	shards    []*Shard
	numShards int
	maxItems  int
	cleaner   *time.Ticker
	stopChan  chan struct{}
	stopOnce  sync.Once
}

func NewMemoryCache() *MemoryCache {
	return NewMemoryCacheWithShards(16)
}

func NewMemoryCacheWithShards(numShards int) *MemoryCache {
	return NewMemoryCacheWithOptions(MemoryCacheOptions{NumShards: numShards})
}

// NewMemoryCacheWithOptions 带可选参数构造。MaxItems>0 时按分片均摊容量，
// 超容时先清扫过期项、仍超则 LRU 淘汰（FIFO 链表 + 命中置尾）。
func NewMemoryCacheWithOptions(opts MemoryCacheOptions) *MemoryCache {
	numShards := opts.NumShards
	if numShards <= 0 {
		numShards = 16
	}
	c := &MemoryCache{
		shards:    make([]*Shard, numShards),
		numShards: numShards,
		maxItems:  opts.MaxItems,
		stopChan:  make(chan struct{}),
	}

	// 容量按分片均摊，余数分给前几片，保证 sum(cap) == MaxItems
	base, remainder := 0, 0
	if opts.MaxItems > 0 {
		base = opts.MaxItems / numShards
		remainder = opts.MaxItems % numShards
	}
	for i := 0; i < numShards; i++ {
		shard := &Shard{
			items: make(map[string]*Item),
		}
		if opts.MaxItems > 0 {
			shard.maxItems = base
			if i < remainder {
				shard.maxItems++
			}
			shard.order = list.New()
			shard.orderMap = make(map[string]*list.Element)
		}
		c.shards[i] = shard
	}

	c.cleaner = time.NewTicker(5 * time.Minute)
	go c.startCleaner()

	return c
}

func (c *MemoryCache) getShard(key string) *Shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	shardIndex := int(h.Sum32()) % c.numShards
	return c.shards[shardIndex]
}

func (c *MemoryCache) startCleaner() {
	for {
		select {
		case <-c.cleaner.C:
			c.deleteExpired()
		case <-c.stopChan:
			c.cleaner.Stop()
			return
		}
	}
}

func (c *MemoryCache) deleteExpired() {
	for _, shard := range c.shards {
		shard.mu.Lock()
		now := time.Now()
		for key, item := range shard.items {
			if !item.ExpiresAt.IsZero() && now.After(item.ExpiresAt) {
				shard.removeItemLocked(key)
			}
		}
		shard.mu.Unlock()
	}
}

// removeItemLocked 从 items 与容量淘汰链表中同步移除。调用方必须持有 shard.mu。
func (s *Shard) removeItemLocked(key string) {
	delete(s.items, key)
	if el, ok := s.orderMap[key]; ok {
		s.order.Remove(el)
		delete(s.orderMap, key)
	}
}

// touchLocked 命中/写入后将 key 置为最近访问（队尾）。调用方必须持有 shard.mu。
func (s *Shard) touchLocked(key string) {
	if s.order == nil {
		return
	}
	if el, ok := s.orderMap[key]; ok {
		s.order.MoveToBack(el)
		return
	}
	s.orderMap[key] = s.order.PushBack(key)
}

// enforceCapacityLocked 超容时先清扫本 shard 过期项，仍超则从队首
// （最久未访问）淘汰。调用方必须持有 shard.mu。
func (s *Shard) enforceCapacityLocked() {
	if s.order == nil || s.maxItems <= 0 {
		return
	}
	for len(s.items) > s.maxItems {
		// 一趟清扫全部过期项（range 中删除安全）
		swept := 0
		for key, item := range s.items {
			if item.IsExpired() {
				s.removeItemLocked(key)
				swept++
			}
		}
		if swept > 0 {
			continue
		}
		el := s.order.Front()
		if el == nil {
			return
		}
		s.removeItemLocked(el.Value.(string))
	}
}

func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	shard := c.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	shard.items[key] = &Item{
		Value:     value,
		ExpiresAt: expiresAt,
	}
	shard.touchLocked(key)
	shard.enforceCapacityLocked()
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	shard := c.getShard(key)

	// 启用容量淘汰时命中要 MoveToBack（写操作），需独占锁
	if shard.order != nil {
		shard.mu.Lock()
		defer shard.mu.Unlock()
	} else {
		shard.mu.RLock()
		defer shard.mu.RUnlock()
	}

	item, exists := shard.items[key]
	if !exists {
		return nil, false
	}

	if item.IsExpired() {
		return nil, false
	}

	shard.touchLocked(key)
	return item.Value, true
}

func (c *MemoryCache) Delete(key string) {
	shard := c.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	shard.removeItemLocked(key)
}

func (c *MemoryCache) Clear() {
	for _, shard := range c.shards {
		shard.mu.Lock()
		shard.items = make(map[string]*Item)
		if shard.order != nil {
			shard.order.Init()
			shard.orderMap = make(map[string]*list.Element)
		}
		shard.mu.Unlock()
	}
}

func (c *MemoryCache) Invalidate(pattern string) {
	for _, shard := range c.shards {
		shard.mu.Lock()
		for key := range shard.items {
			if matchPattern(key, pattern) {
				shard.removeItemLocked(key)
			}
		}
		shard.mu.Unlock()
	}
}

func (c *MemoryCache) Count() int {
	count := 0
	for _, shard := range c.shards {
		shard.mu.RLock()
		for _, item := range shard.items {
			if !item.IsExpired() {
				count++
			}
		}
		shard.mu.RUnlock()
	}
	return count
}

func (c *MemoryCache) Stats() map[string]interface{} {
	total := 0
	expired := 0

	for _, shard := range c.shards {
		shard.mu.RLock()
		for _, item := range shard.items {
			total++
			if item.IsExpired() {
				expired++
			}
		}
		shard.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_items":   total,
		"active_items":  total - expired,
		"expired_items": expired,
		"num_shards":    c.numShards,
		"max_items":     c.maxItems,
	}
}

// MaxItems 返回构造时设置的容量上限（0 表示无上限）。
func (c *MemoryCache) MaxItems() int {
	return c.maxItems
}

func (c *MemoryCache) ListItems(offset, limit int, search string) ([]CacheItem, int) {
	var items []CacheItem
	total := 0

	for _, shard := range c.shards {
		shard.mu.RLock()
		for key, item := range shard.items {
			if search == "" || contains(key, search) {
				total++
				if total > offset && len(items) < limit {
					remainingTTL := int64(0)
					if !item.ExpiresAt.IsZero() {
						remainingTTL = int64(time.Until(item.ExpiresAt).Seconds())
					}
					items = append(items, CacheItem{
						Key:          key,
						IsExpired:    item.IsExpired(),
						Expiry:       item.ExpiresAt,
						RemainingTTL: remainingTTL,
					})
				}
			}
		}
		shard.mu.RUnlock()
	}

	return items, total
}

func (c *MemoryCache) Stop() {
	c.stopOnce.Do(func() {
		c.cleaner.Stop()
		close(c.stopChan)
	})
}

// GetAllItems 返回所有缓存项的详细信息（包含过期状态），返回的是副本
func (c *MemoryCache) GetAllItems() map[string]*Item {
	result := make(map[string]*Item)
	for _, shard := range c.shards {
		shard.mu.RLock()
		for key, item := range shard.items {
			// 返回副本，防止外部修改影响缓存内部数据
			cp := *item
			result[key] = &cp
		}
		shard.mu.RUnlock()
	}
	return result
}

func matchPattern(key, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if len(pattern) == 0 {
		return false
	}

	if pattern[0] == '*' && pattern[len(pattern)-1] == '*' {
		substr := pattern[1 : len(pattern)-1]
		return contains(key, substr)
	}

	if pattern[0] == '*' {
		suffix := pattern[1:]
		return hasSuffix(key, suffix)
	}

	if pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return hasPrefix(key, prefix)
	}

	return key == pattern
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
