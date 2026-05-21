package cache

import (
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

type Shard struct {
	mu       sync.RWMutex
	items    map[string]*Item
}

type MemoryCache struct {
	shards    []*Shard
	numShards int
	cleaner   *time.Ticker
	stopChan  chan struct{}
}

func NewMemoryCache() *MemoryCache {
	return NewMemoryCacheWithShards(16)
}

func NewMemoryCacheWithShards(numShards int) *MemoryCache {
	if numShards <= 0 {
		numShards = 16
	}
	c := &MemoryCache{
		shards:    make([]*Shard, numShards),
		numShards: numShards,
		stopChan:  make(chan struct{}),
	}

	for i := 0; i < numShards; i++ {
		c.shards[i] = &Shard{
			items: make(map[string]*Item),
		}
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
				delete(shard.items, key)
			}
		}
		shard.mu.Unlock()
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
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	shard := c.getShard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	item, exists := shard.items[key]
	if !exists {
		return nil, false
	}

	if item.IsExpired() {
		return nil, false
	}

	return item.Value, true
}

func (c *MemoryCache) Delete(key string) {
	shard := c.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	delete(shard.items, key)
}

func (c *MemoryCache) Clear() {
	for _, shard := range c.shards {
		shard.mu.Lock()
		shard.items = make(map[string]*Item)
		shard.mu.Unlock()
	}
}

func (c *MemoryCache) Invalidate(pattern string) {
	for _, shard := range c.shards {
		shard.mu.Lock()
		for key := range shard.items {
			if matchPattern(key, pattern) {
				delete(shard.items, key)
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
	}
}

func (c *MemoryCache) Stop() {
	close(c.stopChan)
}

// GetAllItems 返回所有缓存项的详细信息（包含过期状态）
func (c *MemoryCache) GetAllItems() map[string]*Item {
	result := make(map[string]*Item)
	for _, shard := range c.shards {
		shard.mu.RLock()
		for key, item := range shard.items {
			result[key] = item
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
