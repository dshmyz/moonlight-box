package cache

import (
	"testing"
	"time"
)

// testKey 满足 CacheKey 接口的测试 key 类型（cache 包不依赖 core/runtime）。
type testKey struct{ id string }

func (k testKey) String() string { return k.id }

// TestMemoryCacheCapacityEviction 验证 MaxItems>0 时超容 LRU 淘汰：
// 无容量时全部保留；有容量时最久未访问者被淘汰。
func TestMemoryCacheCapacityEviction(t *testing.T) {
	mc := NewMemoryCacheWithOptions(MemoryCacheOptions{NumShards: 1, MaxItems: 3})
	defer mc.Stop()

	for i := 0; i < 5; i++ {
		mc.Set(string(rune('a'+i)), i, time.Hour)
	}
	// a b c d e 依次写入，超容淘汰 a b（FIFO 队首），剩 c d e
	for _, missing := range []string{"a", "b"} {
		if _, ok := mc.Get(missing); ok {
			t.Fatalf("%s 应已被淘汰", missing)
		}
	}
	for _, present := range []string{"c", "d", "e"} {
		if _, ok := mc.Get(present); !ok {
			t.Fatalf("%s 应保留", present)
		}
	}
	if got := mc.Count(); got != 3 {
		t.Fatalf("Count = %d, want 3", got)
	}
}

// TestMemoryCacheLRUOnGet 验证命中会刷新访问序：访问 c 后再写入新 key，
// 被淘汰的是 d（最久未访问）而非 c。
func TestMemoryCacheLRUOnGet(t *testing.T) {
	mc := NewMemoryCacheWithOptions(MemoryCacheOptions{NumShards: 1, MaxItems: 3})
	defer mc.Stop()

	for _, k := range []string{"a", "b", "c"} {
		mc.Set(k, 1, time.Hour)
	}
	if _, ok := mc.Get("c"); !ok {
		t.Fatal("c 应存在")
	}
	mc.Set("d", 1, time.Hour)

	if _, ok := mc.Get("d"); !ok {
		t.Fatal("d 应存在（新写入）")
	}
	if _, ok := mc.Get("c"); !ok {
		t.Fatal("c 应存在（刚被访问过，不应淘汰）")
	}
	if _, ok := mc.Get("a"); ok {
		t.Fatal("a 应被 LRU 淘汰")
	}
}

// TestMemoryCacheCapacitySweepsExpiredFirst 验证超容时优先清扫过期项：
// 写入 3 条短 TTL，过期后再写新 key，被清掉的是过期项而非 LRU 队首。
func TestMemoryCacheCapacitySweepsExpiredFirst(t *testing.T) {
	mc := NewMemoryCacheWithOptions(MemoryCacheOptions{NumShards: 1, MaxItems: 3})
	defer mc.Stop()

	mc.Set("x1", 1, 10*time.Millisecond)
	mc.Set("x2", 2, 10*time.Millisecond)
	mc.Set("live", 3, time.Hour)
	time.Sleep(30 * time.Millisecond)

	mc.Set("fresh", 4, time.Hour)

	if _, ok := mc.Get("live"); !ok {
		t.Fatal("live 应存在（过期清扫优先于 LRU 淘汰）")
	}
	if _, ok := mc.Get("fresh"); !ok {
		t.Fatal("fresh 应存在")
	}
}

// TestMemoryCacheNoCapacityKeepsAll 验证默认构造（无上限）行为不变。
func TestMemoryCacheNoCapacityKeepsAll(t *testing.T) {
	mc := NewMemoryCache()
	defer mc.Stop()

	for i := 0; i < 100; i++ {
		mc.Set(string(rune(i)), i, time.Hour)
	}
	if got := mc.Count(); got != 100 {
		t.Fatalf("Count = %d, want 100（无上限不应淘汰）", got)
	}
}

// TestMetadataCacheNegativeTTL 独立负缓存 TTL：负缓存先于正缓存过期。
func TestMetadataCacheNegativeTTL(t *testing.T) {
	mc := NewMetadataCache(time.Hour, 20*time.Millisecond, 0)
	key := testKey{"npm:missing"}

	mc.SetNegative(key)
	if _, hit := mc.Get(key); hit {
		t.Fatal("负缓存不应返回 artifact")
	}
	if !mc.IsNegative(key) {
		t.Fatal("负缓存 TTL 内 IsNegative 应为 true")
	}
	// 负缓存 TTL 过期后应彻底 miss（触发回源），而正缓存 TTL 尚未到期
	time.Sleep(30 * time.Millisecond)
	if mc.IsNegative(key) {
		t.Fatal("负缓存过期后 IsNegative 应为 false")
	}
	if _, hit := mc.Get(key); hit {
		t.Fatal("负缓存过期后不应命中")
	}
	if got := mc.Len(); got != 0 {
		t.Fatalf("负缓存过期后 Len = %d, want 0（惰性移除）", got)
	}
}

// TestMetadataCacheSetNegativeWithTTL 验证单条负缓存可用自定义 TTL 覆盖。
func TestMetadataCacheSetNegativeWithTTL(t *testing.T) {
	mc := NewMetadataCache(time.Hour, time.Hour, 0)
	key := testKey{"npm:flaky"}

	mc.SetNegativeWithTTL(key, 20*time.Millisecond)
	if !mc.IsNegative(key) {
		t.Fatal("自定义 TTL 负缓存应生效")
	}
	time.Sleep(30 * time.Millisecond)
	if mc.IsNegative(key) {
		t.Fatal("自定义 TTL 过期后 IsNegative 应为 false")
	}
}

// TestMetadataCacheItemsSnapshot 验证 Items 快照包含 CacheKey 与负缓存标记。
func TestMetadataCacheItemsSnapshot(t *testing.T) {
	mc := NewMetadataCache(time.Hour, time.Hour, 0)

	posKey := testKey{"npm:pkg:1.0.0"}
	negKey := testKey{"npm:missing"}

	mc.Set(posKey, "cached-artifact")
	mc.SetNegative(negKey)

	items := mc.Items()
	if len(items) != 2 {
		t.Fatalf("Items len = %d, want 2", len(items))
	}
	byKey := make(map[string]MetadataCacheItem, len(items))
	for _, it := range items {
		byKey[it.Key] = it
	}
	pos, ok := byKey[posKey.String()]
	if !ok || pos.IsNegative || pos.CacheKey == nil || pos.Value != "cached-artifact" {
		t.Fatalf("正缓存条目快照异常: %+v", pos)
	}
	neg, ok := byKey[negKey.String()+":negative"]
	if !ok || !neg.IsNegative {
		t.Fatalf("负缓存条目快照异常: %+v", neg)
	}
}

// TestMetadataCacheInvalidateClearsBoth 验证 Invalidate 同时清正/负缓存。
func TestMetadataCacheInvalidateClearsBoth(t *testing.T) {
	mc := NewMetadataCache(time.Hour, time.Hour, 0)
	key := testKey{"npm:pkg"}

	mc.Set(key, "artifact")
	mc.SetNegative(key)
	if got := mc.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
	mc.Invalidate(key)
	if got := mc.Len(); got != 0 {
		t.Fatalf("Invalidate 后 Len = %d, want 0", got)
	}
}

// TestMetadataCacheDeleteNegativeByKey 验证按字符串 key 清除负缓存。
func TestMetadataCacheDeleteNegativeByKey(t *testing.T) {
	mc := NewMetadataCache(time.Hour, time.Hour, 0)
	key := testKey{"npm:pkg"}

	mc.SetNegative(key)
	mc.DeleteNegativeByKey(key.String())
	if got := mc.Len(); got != 0 {
		t.Fatalf("DeleteNegativeByKey 后 Len = %d, want 0", got)
	}
}
