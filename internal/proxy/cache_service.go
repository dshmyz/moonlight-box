package proxy

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

type CacheService struct {
	mu       sync.RWMutex
	store    map[string]*cacheEntry
	lruList  *list.List
	lruIndex map[string]*list.Element
	maxItems int
	maxBytes int64
	usedBytes int64
}

type cacheEntry struct {
	key         string
	content     []byte
	contentType string
	size        int64
	expiry      time.Time
	isNegative  bool
}

type CacheItem struct {
	Key         string
	Content     []byte
	ContentType string
	Size        int64
	IsNegative  bool `json:"is_negative"`
}

func NewCacheService() *CacheService {
	return &CacheService{
		store:    make(map[string]*cacheEntry),
		lruList:  list.New(),
		lruIndex: make(map[string]*list.Element),
		maxItems: 10000,
		maxBytes: 10 * 1024 * 1024 * 1024,
	}
}

func (c *CacheService) SetLimits(maxItems int, maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxItems = maxItems
	c.maxBytes = maxBytes
	c.evictIfNeeded()
}

func (c *CacheService) Get(ctx context.Context, key string) (*CacheItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.store[key]
	if !ok {
		return nil, fmt.Errorf("cache miss: %s", key)
	}

	if time.Now().After(entry.expiry) {
		c.removeEntry(key, entry)
		return nil, fmt.Errorf("cache expired: %s", key)
	}

	if elem, ok := c.lruIndex[key]; ok {
		c.lruList.MoveToFront(elem)
	}

	if entry.isNegative {
		return &CacheItem{
			Key:        key,
			IsNegative: true,
		}, nil
	}

	return &CacheItem{
		Key:         key,
		Content:     entry.content,
		ContentType: entry.contentType,
		Size:        entry.size,
		IsNegative:  false,
	}, nil
}

func (c *CacheService) Set(ctx context.Context, item *CacheItem, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entrySize := int64(len(item.Content))
	if entrySize > c.maxBytes {
		return fmt.Errorf("item size %d exceeds max cache size %d", entrySize, c.maxBytes)
	}

	if oldEntry, exists := c.store[item.Key]; exists {
		c.removeEntry(item.Key, oldEntry)
	}

	c.evictForSpace(entrySize)

	entry := &cacheEntry{
		key:         item.Key,
		content:     item.Content,
		contentType: item.ContentType,
		size:        item.Size,
		expiry:      time.Now().Add(ttl),
		isNegative:  false,
	}

	c.store[item.Key] = entry
	elem := c.lruList.PushFront(item.Key)
	c.lruIndex[item.Key] = elem
	c.usedBytes += entrySize

	return nil
}

func (c *CacheService) SetNegative(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if oldEntry, exists := c.store[key]; exists {
		c.removeEntry(key, oldEntry)
	}

	c.evictForSpace(0)

	entry := &cacheEntry{
		key:        key,
		expiry:     time.Now().Add(ttl),
		isNegative: true,
		size:       0,
	}

	c.store[key] = entry
	elem := c.lruList.PushFront(key)
	c.lruIndex[key] = elem

	return nil
}

func (c *CacheService) Invalidate(ctx context.Context, pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var keysToDelete []string
	for key := range c.store {
		if matchPattern(pattern, key) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		if entry, exists := c.store[key]; exists {
			c.removeEntry(key, entry)
		}
	}

	return nil
}

func (c *CacheService) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = make(map[string]*cacheEntry)
	c.lruList = list.New()
	c.lruIndex = make(map[string]*list.Element)
	c.usedBytes = 0

	return nil
}

func (c *CacheService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalSize := int64(0)
	positiveCount := 0
	negativeCount := 0
	for _, entry := range c.store {
		totalSize += entry.size
		if entry.isNegative {
			negativeCount++
		} else {
			positiveCount++
		}
	}

	return map[string]interface{}{
		"total_items":     positiveCount + negativeCount,
		"positive_items":  positiveCount,
		"negative_items":  negativeCount,
		"total_size":      totalSize,
		"used_bytes":      c.usedBytes,
		"max_bytes":       c.maxBytes,
		"max_items":       c.maxItems,
		"lru_list_length": c.lruList.Len(),
	}, nil
}

func (c *CacheService) removeEntry(key string, entry *cacheEntry) {
	delete(c.store, key)
	if elem, ok := c.lruIndex[key]; ok {
		c.lruList.Remove(elem)
		delete(c.lruIndex, key)
	}
	c.usedBytes -= entry.size
	if c.usedBytes < 0 {
		c.usedBytes = 0
	}
}

func (c *CacheService) evictForSpace(neededBytes int64) {
	for (len(c.store) >= c.maxItems) || (c.usedBytes+neededBytes > c.maxBytes) {
		if c.lruList.Len() == 0 {
			break
		}

		elem := c.lruList.Back()
		if elem == nil {
			break
		}

		key := elem.Value.(string)
		if entry, exists := c.store[key]; exists {
			c.removeEntry(key, entry)
		}
	}
}

func (c *CacheService) evictIfNeeded() {
	c.evictForSpace(0)
}

func (c *CacheService) cleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, entry := range c.store {
		if now.After(entry.expiry) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if entry, exists := c.store[key]; exists {
			c.removeEntry(key, entry)
		}
	}

	return len(expiredKeys)
}

func matchPattern(pattern, key string) bool {
	if pattern == "*" {
		return true
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	}
	return key == pattern
}
