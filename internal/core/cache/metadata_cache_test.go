package cache

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

func TestMetadataCacheSetNegativeCountsTowardMaxSize(t *testing.T) {
	cache := NewMetadataCache(time.Hour, 1)
	ctx := context.Background()

	cache.SetNegative(ctx, &runtime.ArtifactKey{RepositoryID: "1", Format: "npm", Name: "missing-a"})
	cache.SetNegative(ctx, &runtime.ArtifactKey{RepositoryID: "1", Format: "npm", Name: "missing-b"})

	if got := countMetadataCacheEntries(cache); got != 1 {
		t.Fatalf("entry count = %d, want 1", got)
	}
	if cache.ll.Len() != 1 {
		t.Fatalf("tracked size = %d, want 1", cache.ll.Len())
	}
}

func countMetadataCacheEntries(cache *MetadataCache) int {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.ll.Len()
}
