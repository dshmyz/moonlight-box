package cache

import (
	"context"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/core/runtime"
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
		c.store.Delete(cacheKey)
		return nil, false
	}

	if cached.isNegative {
		return nil, false
	}

	return cached.artifact, true
}

func (c *MetadataCache) Set(ctx context.Context, key *runtime.ArtifactKey, artifact *runtime.Artifact) {
	c.mu.Lock()
	if c.size >= c.maxSize {
		c.evictOldest()
	}
	c.size++
	c.mu.Unlock()

	cacheKey := key.String()
	c.store.Store(cacheKey, &cachedMetadata{
		artifact: artifact,
		cachedAt: time.Now(),
	})
}

func (c *MetadataCache) SetNegative(ctx context.Context, key *runtime.ArtifactKey) {
	cacheKey := key.String() + ":negative"
	c.store.Store(cacheKey, &cachedMetadata{
		cachedAt:   time.Now(),
		isNegative: true,
	})
}

func (c *MetadataCache) Invalidate(ctx context.Context, key *runtime.ArtifactKey) {
	cacheKey := key.String()
	c.store.Delete(cacheKey)
	c.store.Delete(cacheKey + ":negative")
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
		c.store.Delete(oldestKey)
		c.size--
	}
}
