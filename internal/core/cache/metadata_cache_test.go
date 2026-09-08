package cache

import (
	"testing"
	"time"
)

func TestMetadataCacheSetNegativeCountsTowardMaxSize(t *testing.T) {
	cache := NewMetadataCache(time.Hour, 0, 1)

	cache.SetNegative(testKey{"npm:missing-a"})
	cache.SetNegative(testKey{"npm:missing-b"})

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
