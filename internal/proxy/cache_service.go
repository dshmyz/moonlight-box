package proxy

import (
	"context"
	"fmt"
	"time"
)

// CacheService 缓存服务，管理代理内容的缓存
type CacheService struct {
	store map[string]*cacheEntry
}

type cacheEntry struct {
	content     []byte
	contentType string
	size        int64
	expiry      time.Time
	isNegative  bool
}

// CacheItem 缓存项
type CacheItem struct {
	Key         string
	Content     []byte
	ContentType string
	Size        int64
	IsNegative  bool `json:"is_negative"`
}

// NewCacheService 创建缓存服务
func NewCacheService() *CacheService {
	return &CacheService{
		store: make(map[string]*cacheEntry),
	}
}

// Get 获取缓存项
func (c *CacheService) Get(ctx context.Context, key string) (*CacheItem, error) {
	entry, ok := c.store[key]
	if !ok {
		return nil, fmt.Errorf("cache miss: %s", key)
	}
	if time.Now().After(entry.expiry) {
		delete(c.store, key)
		return nil, fmt.Errorf("cache expired: %s", key)
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

// Set 设置缓存项
func (c *CacheService) Set(ctx context.Context, item *CacheItem, ttl time.Duration) error {
	c.store[item.Key] = &cacheEntry{
		content:     item.Content,
		contentType: item.ContentType,
		size:        item.Size,
		expiry:      time.Now().Add(ttl),
	}
	return nil
}

// SetNegative 设置负向缓存
func (c *CacheService) SetNegative(ctx context.Context, key string, ttl time.Duration) error {
	c.store[key] = &cacheEntry{
		expiry:     time.Now().Add(ttl),
		isNegative: true,
	}
	return nil
}

// Invalidate 使指定模式的缓存失效
func (c *CacheService) Invalidate(ctx context.Context, pattern string) error {
	for key := range c.store {
		if matchPattern(pattern, key) {
			delete(c.store, key)
		}
	}
	return nil
}

// Clear 清空所有缓存
func (c *CacheService) Clear(ctx context.Context) error {
	c.store = make(map[string]*cacheEntry)
	return nil
}

// GetStats 获取缓存统计
func (c *CacheService) GetStats(ctx context.Context) (map[string]interface{}, error) {
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
		"total_items":    positiveCount + negativeCount,
		"positive_items": positiveCount,
		"negative_items": negativeCount,
		"total_size":     totalSize,
	}, nil
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
