package proxy

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

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

type CacheShard struct {
	mu        sync.RWMutex
	store     map[string]*cacheEntry
	lruList   *list.List
	lruIndex  map[string]*list.Element
	maxItems  int
	maxBytes  int64
	usedBytes int64
}

type CacheService struct {
	shards    []*CacheShard
	numShards int
	maxItems  int
	maxBytes  int64
}

type CacheServiceOptions struct {
	MaxItems  int
	MaxBytes  int64
	NumShards int
}

func NewCacheService() *CacheService {
	return NewCacheServiceWithOptions(CacheServiceOptions{
		MaxItems:  10000,
		MaxBytes:  2 * 1024 * 1024 * 1024,
		NumShards: 16,
	})
}

func NewCacheServiceWithOptions(opts CacheServiceOptions) *CacheService {
	if opts.NumShards <= 0 {
		opts.NumShards = 16
	}
	if opts.MaxItems <= 0 {
		opts.MaxItems = 10000
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 2 * 1024 * 1024 * 1024
	}

	c := &CacheService{
		shards:    make([]*CacheShard, opts.NumShards),
		numShards: opts.NumShards,
		maxItems:  opts.MaxItems,
		maxBytes:  opts.MaxBytes,
	}

	for i := 0; i < opts.NumShards; i++ {
		c.shards[i] = &CacheShard{
			store:    make(map[string]*cacheEntry),
			lruList:  list.New(),
			lruIndex: make(map[string]*list.Element),
			maxItems: opts.MaxItems / opts.NumShards,
			maxBytes: opts.MaxBytes / int64(opts.NumShards),
		}
	}

	return c
}

func (c *CacheService) getShard(key string) *CacheShard {
	hash := sha256.Sum256([]byte(key))
	shardIndex := int(hash[0]) % c.numShards
	return c.shards[shardIndex]
}

func (c *CacheService) SetLimits(maxItems int, maxBytes int64) {
	c.maxItems = maxItems
	c.maxBytes = maxBytes

	shardMaxItems := maxItems / c.numShards
	shardMaxBytes := maxBytes / int64(c.numShards)

	for _, shard := range c.shards {
		shard.mu.Lock()
		shard.maxItems = shardMaxItems
		shard.maxBytes = shardMaxBytes
		shard.mu.Unlock()
		shard.evictIfNeeded()
	}
}

func (c *CacheService) Get(ctx context.Context, key string) (*CacheItem, error) {
	shard := c.getShard(key)
	return shard.get(key)
}

func (c *CacheService) Set(ctx context.Context, item *CacheItem, ttl time.Duration) error {
	shard := c.getShard(item.Key)
	return shard.set(item, ttl)
}

func (c *CacheService) SetNegative(ctx context.Context, key string, ttl time.Duration) error {
	shard := c.getShard(key)
	return shard.setNegative(key, ttl)
}

func (c *CacheService) Invalidate(ctx context.Context, pattern string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, c.numShards)

	for _, shard := range c.shards {
		wg.Add(1)
		go func(s *CacheShard) {
			defer wg.Done()
			if err := s.invalidate(pattern); err != nil {
				errChan <- err
			}
		}(shard)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}
	return nil
}

func (c *CacheService) Clear(ctx context.Context) error {
	for _, shard := range c.shards {
		shard.clear()
	}
	return nil
}

func (c *CacheService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	totalItems := 0
	positiveItems := 0
	negativeItems := 0
	totalSize := int64(0)
	usedBytes := int64(0)

	for _, shard := range c.shards {
		shard.mu.RLock()
		for _, entry := range shard.store {
			totalSize += entry.size
			if entry.isNegative {
				negativeItems++
			} else {
				positiveItems++
			}
		}
		usedBytes += shard.usedBytes
		totalItems += shard.lruList.Len()
		shard.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_items":    totalItems,
		"positive_items": positiveItems,
		"negative_items": negativeItems,
		"total_size":     totalSize,
		"used_bytes":     usedBytes,
		"max_bytes":      c.maxBytes,
		"max_items":      c.maxItems,
		"num_shards":     c.numShards,
	}, nil
}

func (c *CacheService) cleanupExpired() int {
	totalExpired := 0
	for _, shard := range c.shards {
		totalExpired += shard.cleanupExpired()
	}
	return totalExpired
}

func (c *CacheService) GetExpiredCount() int {
	totalExpired := 0
	now := time.Now()
	for _, shard := range c.shards {
		shard.mu.RLock()
		for _, entry := range shard.store {
			if now.After(entry.expiry) {
				totalExpired++
			}
		}
		shard.mu.RUnlock()
	}
	return totalExpired
}

func (c *CacheService) ListItems(offset, limit int, search string) ([]map[string]interface{}, int) {
	var allItems []map[string]interface{}
	now := time.Now()

	for _, shard := range c.shards {
		shard.mu.RLock()
		for key, entry := range shard.store {
			if search != "" && !containsIgnoreCase(key, search) {
				continue
			}

			formattedExpiry := entry.expiry.Format(time.RFC3339)
			remainingTTL := int64(0)
			if entry.expiry.After(now) {
				remainingTTL = int64(time.Until(entry.expiry).Seconds())
			}

			item := map[string]interface{}{
				"key":           key,
				"size":          entry.size,
				"content_type":  entry.contentType,
				"is_negative":   entry.isNegative,
				"expiry":        formattedExpiry,
				"remaining_ttl": remainingTTL,
				"is_expired":    now.After(entry.expiry),
			}
			allItems = append(allItems, item)
		}
		shard.mu.RUnlock()
	}

	total := len(allItems)

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if offset >= len(allItems) {
		return []map[string]interface{}{}, total
	}

	end := offset + limit
	if end > len(allItems) {
		end = len(allItems)
	}

	return allItems[offset:end], total
}

func (c *CacheService) DeleteItem(key string) error {
	shard := c.getShard(key)
	return shard.deleteItem(key)
}

func (c *CacheService) CleanupExpired() int {
	return c.cleanupExpired()
}

func containsIgnoreCase(s, substr string) bool {
	if substr == "" {
		return true
	}
	sLower := make([]byte, len(s))
	copy(sLower, s)
	for i := range sLower {
		if sLower[i] >= 'A' && sLower[i] <= 'Z' {
			sLower[i] = sLower[i] + 32
		}
	}
	substrLower := make([]byte, len(substr))
	copy(substrLower, substr)
	for i := range substrLower {
		if substrLower[i] >= 'A' && substrLower[i] <= 'Z' {
			substrLower[i] = substrLower[i] + 32
		}
	}
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if string(sLower[i:i+len(substrLower)]) == string(substrLower) {
			return true
		}
	}
	return false
}

func (s *CacheShard) get(key string) (*CacheItem, error) {
	s.mu.RLock()
	entry, ok := s.store[key]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("cache miss: %s", key)
	}

	if time.Now().After(entry.expiry) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.removeEntry(key, entry)
		s.mu.Unlock()
		return nil, fmt.Errorf("cache expired: %s", key)
	}

	if elem, ok := s.lruIndex[key]; ok {
		s.mu.RUnlock()
		s.mu.Lock()
		s.lruList.MoveToFront(elem)
		s.mu.Unlock()
	} else {
		s.mu.RUnlock()
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

func (s *CacheShard) set(item *CacheItem, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entrySize := int64(len(item.Content))
	if entrySize > s.maxBytes {
		return fmt.Errorf("item size %d exceeds max cache size %d", entrySize, s.maxBytes)
	}

	if oldEntry, exists := s.store[item.Key]; exists {
		s.removeEntry(item.Key, oldEntry)
	}

	s.evictForSpace(entrySize)

	entry := &cacheEntry{
		key:         item.Key,
		content:     item.Content,
		contentType: item.ContentType,
		size:        item.Size,
		expiry:      time.Now().Add(ttl),
		isNegative:  false,
	}

	s.store[item.Key] = entry
	elem := s.lruList.PushFront(item.Key)
	s.lruIndex[item.Key] = elem
	s.usedBytes += entrySize

	return nil
}

func (s *CacheShard) setNegative(key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldEntry, exists := s.store[key]; exists {
		s.removeEntry(key, oldEntry)
	}

	s.evictForSpace(0)

	entry := &cacheEntry{
		key:        key,
		expiry:     time.Now().Add(ttl),
		isNegative: true,
		size:       0,
	}

	s.store[key] = entry
	elem := s.lruList.PushFront(key)
	s.lruIndex[key] = elem

	return nil
}

func (s *CacheShard) invalidate(pattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keysToDelete []string
	for key := range s.store {
		if matchPattern(pattern, key) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		if entry, exists := s.store[key]; exists {
			s.removeEntry(key, entry)
		}
	}

	return nil
}

func (s *CacheShard) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store = make(map[string]*cacheEntry)
	s.lruList = list.New()
	s.lruIndex = make(map[string]*list.Element)
	s.usedBytes = 0
}

func (s *CacheShard) removeEntry(key string, entry *cacheEntry) {
	delete(s.store, key)
	if elem, ok := s.lruIndex[key]; ok {
		s.lruList.Remove(elem)
		delete(s.lruIndex, key)
	}
	s.usedBytes -= entry.size
	if s.usedBytes < 0 {
		s.usedBytes = 0
	}
}

func (s *CacheShard) evictForSpace(neededBytes int64) {
	for (len(s.store) >= s.maxItems) || (s.usedBytes+neededBytes > s.maxBytes) {
		if s.lruList.Len() == 0 {
			break
		}

		elem := s.lruList.Back()
		if elem == nil {
			break
		}

		key := elem.Value.(string)
		if entry, exists := s.store[key]; exists {
			s.removeEntry(key, entry)
		}
	}
}

func (s *CacheShard) evictIfNeeded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictForSpace(0)
}

func (s *CacheShard) deleteItem(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.store[key]; exists {
		s.removeEntry(key, entry)
		return nil
	}
	return fmt.Errorf("cache key not found: %s", key)
}

func (s *CacheShard) cleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, entry := range s.store {
		if now.After(entry.expiry) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if entry, exists := s.store[key]; exists {
			s.removeEntry(key, entry)
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
