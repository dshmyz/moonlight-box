package cache

import (
	"context"
	"time"
)

type CacheItem struct {
	Key         string      `json:"key"`
	Size        int64       `json:"size"`
	ContentType string      `json:"content_type"`
	IsNegative  bool        `json:"is_negative"`
	Expiry      time.Time   `json:"expiry"`
	RemainingTTL int64      `json:"remaining_ttl"`
	IsExpired   bool        `json:"is_expired"`
	Metadata    interface{} `json:"metadata"`
}

type CacheStats struct {
	TotalItems    int64  `json:"total_items"`
	ActiveItems   int64  `json:"active_items"`
	ExpiredItems  int64  `json:"expired_items"`
	UsedBytes     int64  `json:"used_bytes"`
	MaxBytes      int64  `json:"max_bytes"`
	MaxItems      int64  `json:"max_items"`
	NumShards     int    `json:"num_shards"`
	TTLSeconds    int64  `json:"ttl_seconds"`
	HitRate       float64 `json:"hit_rate"`
}

type CacheProvider interface {
	Name() string
	Type() string
	Description() string
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Invalidate(ctx context.Context, pattern string) error
	Clear(ctx context.Context) error
	Stats(ctx context.Context) *CacheStats
	ListItems(offset, limit int, search string) ([]CacheItem, int)
}

type TypedCacheProvider interface {
	CacheProvider
	SetNegative(ctx context.Context, key string, ttl time.Duration) error
}
