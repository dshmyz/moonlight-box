package proxy

import (
	"context"
	"testing"
	"time"
)

func TestCacheServiceUsedBytesMatchesContentLength(t *testing.T) {
	c := NewCacheServiceWithOptions(CacheServiceOptions{
		MaxItems:  1000,
		MaxBytes:  1024 * 1024,
		NumShards: 4,
	})
	ctx := context.Background()

	// 模拟客户端传入的 Size 和实际 Content 长度不一致的情况
	content := []byte("hello-world")
	mismatchedSize := int64(999999)

	err := c.Set(ctx, &CacheItem{
		Key:         "test-key",
		Content:     content,
		ContentType: "text/plain",
		Size:        mismatchedSize, // 客户端传了错误的大小
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	stats, err := c.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	totalSize := stats["total_size"].(int64)
	usedBytes := stats["used_bytes"].(int64)
	expectedSize := int64(len(content))

	if totalSize != expectedSize {
		t.Fatalf("total_size = %d, want %d (content length, not mismatched Size)", totalSize, expectedSize)
	}
	if usedBytes != expectedSize {
		t.Fatalf("used_bytes = %d, want %d (content length, not mismatched Size)", usedBytes, expectedSize)
	}
	if totalSize != usedBytes {
		t.Fatalf("total_size (%d) and used_bytes (%d) must be consistent", totalSize, usedBytes)
	}
}

func TestCacheServiceUsedBytesAfterEviction(t *testing.T) {
	c := NewCacheServiceWithOptions(CacheServiceOptions{
		MaxItems:  100,
		MaxBytes:  100, // 极小容量，触发 eviction
		NumShards: 1,
	})
	ctx := context.Background()

	// 塞入多个条目，触发 LRU 驱逐
	content := []byte("1234567890") // 10 bytes
	for i := 0; i < 20; i++ {
		key := string(rune('a' + i))
		err := c.Set(ctx, &CacheItem{
			Key:     key,
			Content: content,
			Size:    int64(len(content)),
		}, 5*time.Minute)
		if err != nil {
			t.Fatalf("Set(%q) failed: %v", key, err)
		}
	}

	stats, err := c.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	totalSize := stats["total_size"].(int64)
	usedBytes := stats["used_bytes"].(int64)

	if totalSize < 0 || usedBytes < 0 {
		t.Fatalf("negative values: total_size=%d, used_bytes=%d", totalSize, usedBytes)
	}
	if totalSize != usedBytes {
		t.Fatalf("total_size (%d) != used_bytes (%d) after eviction", totalSize, usedBytes)
	}
	if totalSize > int64(100) {
		t.Fatalf("total_size (%d) exceeds max_bytes after eviction", totalSize)
	}
}

func TestCacheServiceUsedBytesAfterDelete(t *testing.T) {
	c := NewCacheServiceWithOptions(CacheServiceOptions{
		MaxItems:  100,
		MaxBytes:  1024 * 1024,
		NumShards: 1,
	})
	ctx := context.Background()

	content := []byte("some-data")
	err := c.Set(ctx, &CacheItem{
		Key:     "to-delete",
		Content: content,
		Size:    int64(len(content)),
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err = c.DeleteItem("to-delete")
	if err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}

	stats, err := c.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	totalSize := stats["total_size"].(int64)
	usedBytes := stats["used_bytes"].(int64)

	if totalSize != 0 {
		t.Fatalf("total_size = %d, want 0 after delete", totalSize)
	}
	if usedBytes != 0 {
		t.Fatalf("used_bytes = %d, want 0 after delete", usedBytes)
	}
}