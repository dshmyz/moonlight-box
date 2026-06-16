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
	if cache.size != 1 {
		t.Fatalf("tracked size = %d, want 1", cache.size)
	}
}

func countMetadataCacheEntries(cache *MetadataCache) int {
	count := 0
	cache.store.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
