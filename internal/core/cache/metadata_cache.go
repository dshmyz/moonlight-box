package cache

import (
	"context"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

type MetadataCache struct {
	store   sync.Map
	ttl     time.Duration
	maxSize int
	size    int
	mu      sync.Mutex
}

type cachedMetadata struct {
	artifact   *runtime.Artifact
	cachedAt   time.Time
	isNegative bool
}

func NewMetadataCache(ttl time.Duration, maxSize int) *MetadataCache {
	return &MetadataCache{
		ttl:     ttl,
		maxSize: maxSize,
	}
}

func (c *MetadataCache) Get(ctx context.Context, key *runtime.ArtifactKey) (*runtime.Artifact, bool) {
	cacheKey := key.String()

	value, ok := c.store.Load(cacheKey)
	if !ok {
		return nil, false
	}

	cached := value.(*cachedMetadata)

	if time.Since(cached.cachedAt) > c.ttl {
		c.mu.Lock()
		c.deleteKeyLocked(cacheKey)
		c.mu.Unlock()
		return nil, false
	}

	if cached.isNegative {
		return nil, false
	}

	return cached.artifact, true
}

func (c *MetadataCache) Set(ctx context.Context, key *runtime.ArtifactKey, artifact *runtime.Artifact) {
	cacheKey := key.String()
	c.mu.Lock()
	if _, exists := c.store.Load(cacheKey); !exists {
		c.evictIfNeededLocked()
		c.size++
	}
	c.store.Store(cacheKey, &cachedMetadata{
		artifact: artifact,
		cachedAt: time.Now(),
	})
	c.mu.Unlock()
}

func (c *MetadataCache) SetNegative(ctx context.Context, key *runtime.ArtifactKey) {
	cacheKey := key.String() + ":negative"
	c.mu.Lock()
	if _, exists := c.store.Load(cacheKey); !exists {
		c.evictIfNeededLocked()
		c.size++
	}
	c.store.Store(cacheKey, &cachedMetadata{
		cachedAt:   time.Now(),
		isNegative: true,
	})
	c.mu.Unlock()
}

func (c *MetadataCache) Invalidate(ctx context.Context, key *runtime.ArtifactKey) {
	cacheKey := key.String()
	c.mu.Lock()
	c.deleteKeyLocked(cacheKey)
	c.deleteKeyLocked(cacheKey + ":negative")
	c.mu.Unlock()
}

func (c *MetadataCache) evictIfNeededLocked() {
	if c.maxSize <= 0 {
		return
	}
	for c.size >= c.maxSize {
		c.evictOldest()
	}
}

func (c *MetadataCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	c.store.Range(func(key, value interface{}) bool {
		cached := value.(*cachedMetadata)
		if oldestKey == "" || cached.cachedAt.Before(oldestTime) {
			oldestKey = key.(string)
			oldestTime = cached.cachedAt
		}
		return true
	})

	if oldestKey != "" {
		c.deleteKeyLocked(oldestKey)
	}
}

func (c *MetadataCache) deleteKeyLocked(key string) {
	if _, loaded := c.store.LoadAndDelete(key); loaded && c.size > 0 {
		c.size--
	}
}
