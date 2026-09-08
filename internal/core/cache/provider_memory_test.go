package cache

import (
	"context"
	"testing"
	"time"
)

func TestMetadataCacheProviderStatsAndDelete(t *testing.T) {
	mc := NewMetadataCache(time.Hour, time.Minute, 100)
	mc.Set(testKey{id: "a"}, "va")
	mc.Set(testKey{id: "b"}, "vb")
	mc.SetNegative(testKey{id: "missing"})

	p := NewMetadataCacheProvider("test", "memory", "test", mc)
	stats := p.Stats(context.Background())
	if stats.TotalItems != 3 || stats.ActiveItems != 3 || stats.ExpiredItems != 0 {
		t.Fatalf("stats = %+v, want total/active 3, expired 0", stats)
	}
	if stats.MaxItems != 100 || stats.TTLSeconds != 3600 {
		t.Fatalf("stats = %+v, want max_items 100, ttl_seconds 3600", stats)
	}

	// Delete 字符串 key 应同时清除正/负缓存条目
	if err := p.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := mc.Len(); got != 2 {
		t.Fatalf("Len after Delete = %d, want 2", got)
	}

	// Set 拒绝字符串 key 写入（强类型写路径约束）
	if err := p.Set(context.Background(), "x", 1, time.Minute); err == nil {
		t.Fatal("Set should reject string-key writes")
	}

	// Clear 清空全部
	if err := p.Clear(context.Background()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := mc.Len(); got != 0 {
		t.Fatalf("Len after Clear = %d, want 0", got)
	}
}
