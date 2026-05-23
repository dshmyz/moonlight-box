package cache

import (
	"time"
)

// ListItemsPaginator 提供通用的分页逻辑
func ListItemsPaginator(items []CacheItem, offset, limit int) ([]CacheItem, int) {
	total := len(items)

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if offset >= len(items) {
		return []CacheItem{}, total
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	return items[offset:end], total
}

// BuildCacheItem 构建缓存项的通用参数
type CacheItemBuilder struct {
	Key         string
	Size        int64
	ContentType string
	IsNegative  bool
	Expiry      time.Time
	RemainingTTL int64
	IsExpired   bool
	Metadata    interface{}
}

// NewCacheItemBuilder 创建缓存项构建器
func NewCacheItemBuilder(key string) *CacheItemBuilder {
	return &CacheItemBuilder{
		Key: key,
	}
}

// WithSize 设置大小
func (b *CacheItemBuilder) WithSize(size int64) *CacheItemBuilder {
	b.Size = size
	return b
}

// WithContentType 设置内容类型
func (b *CacheItemBuilder) WithContentType(contentType string) *CacheItemBuilder {
	b.ContentType = contentType
	return b
}

// WithExpiry 设置过期时间
func (b *CacheItemBuilder) WithExpiry(expiry time.Time) *CacheItemBuilder {
	b.Expiry = expiry
	return b
}

// WithTTL 根据 TTL 设置过期时间
func (b *CacheItemBuilder) WithTTL(ttl time.Duration) *CacheItemBuilder {
	now := time.Now()
	b.Expiry = now.Add(ttl)
	b.RemainingTTL = int64(ttl.Seconds())
	return b
}

// WithMetadata 设置元数据
func (b *CacheItemBuilder) WithMetadata(metadata interface{}) *CacheItemBuilder {
	b.Metadata = metadata
	return b
}

// Build 构建缓存项
func (b *CacheItemBuilder) Build() CacheItem {
	return CacheItem{
		Key:         b.Key,
		Size:        b.Size,
		ContentType: b.ContentType,
		IsNegative:  b.IsNegative,
		Expiry:      b.Expiry,
		RemainingTTL: b.RemainingTTL,
		IsExpired:   b.IsExpired,
		Metadata:    b.Metadata,
	}
}

// BuildCacheItemsWithTTL 构建带 TTL 的缓存项列表
func BuildCacheItemsWithTTL(key string, contentType string, ttl time.Duration, metadata interface{}) CacheItem {
	now := time.Now()
	return CacheItem{
		Key:         key,
		Size:        0,
		ContentType: contentType,
		IsNegative:  false,
		Expiry:      now.Add(ttl),
		RemainingTTL: int64(ttl.Seconds()),
		IsExpired:   false,
		Metadata:    metadata,
	}
}
