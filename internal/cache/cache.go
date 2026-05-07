package cache

import (
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

type MemoryCache struct {
	mu       sync.RWMutex
	items    map[string]*Item
	cleaner  *time.Ticker
	stopChan chan struct{}
}

func NewMemoryCache() *MemoryCache {
	c := &MemoryCache{
		items:    make(map[string]*Item),
		stopChan: make(chan struct{}),
	}

	c.cleaner = time.NewTicker(5 * time.Minute)
	go c.startCleaner()

	return c
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
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if !item.ExpiresAt.IsZero() && now.After(item.ExpiresAt) {
			delete(c.items, key)
		}
	}
}

func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	c.items[key] = &Item{
		Value:     value,
		ExpiresAt: expiresAt,
	}
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false
	}

	if item.IsExpired() {
		return nil, false
	}

	return item.Value, true
}

func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*Item)
}

func (c *MemoryCache) Invalidate(pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.items {
		if matchPattern(key, pattern) {
			delete(c.items, key)
		}
	}
}

func (c *MemoryCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, item := range c.items {
		if !item.IsExpired() {
			count++
		}
	}
	return count
}

func (c *MemoryCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := len(c.items)
	expired := 0

	for _, item := range c.items {
		if item.IsExpired() {
			expired++
		}
	}

	return map[string]interface{}{
		"total_items":  total,
		"active_items": total - expired,
		"expired_items": expired,
	}
}

func (c *MemoryCache) Stop() {
	close(c.stopChan)
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
