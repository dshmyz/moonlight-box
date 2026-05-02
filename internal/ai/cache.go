package ai

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/config"
)

// ResponseCache 响应缓存，用于缓存相似问题的答案
type ResponseCache struct {
	entries   map[string]*cacheEntry
	lruList   *list.List
	lruIndex  map[string]*list.Element
	mu        sync.RWMutex
	maxSize   int
	ttl       time.Duration
	stopClean chan struct{}
	stopOnce  sync.Once
}

// cacheEntry 缓存条目
type cacheEntry struct {
	query    string
	response string
	createdAt time.Time
}

// CacheStats 缓存统计信息
type CacheStats struct {
	TotalEntries    int   `json:"total_entries"`
	MaxSize         int   `json:"max_size"`
	TTLMilliseconds int64 `json:"ttl_milliseconds"`
}

// NewResponseCache 创建一个新的响应缓存
func NewResponseCache(cfg *config.AICacheConfig) *ResponseCache {
	rc := &ResponseCache{
		entries:   make(map[string]*cacheEntry),
		lruList:   list.New(),
		lruIndex:  make(map[string]*list.Element),
		maxSize:   cfg.MaxSize,
		ttl:       cfg.TTL,
		stopClean: make(chan struct{}),
	}

	// 启动定期清理协程
	go rc.cleanupLoop()

	return rc
}

// Get 获取缓存的响应
func (rc *ResponseCache) Get(query string) (string, bool) {
	key := rc.hashQuery(query)

	rc.mu.RLock()
	entry, exists := rc.entries[key]
	rc.mu.RUnlock()

	if !exists {
		return "", false
	}

	// 检查是否过期
	if time.Since(entry.createdAt) > rc.ttl {
		rc.mu.Lock()
		delete(rc.entries, key)
		if elem, ok := rc.lruIndex[key]; ok {
			rc.lruList.Remove(elem)
			delete(rc.lruIndex, key)
		}
		rc.mu.Unlock()
		return "", false
	}

	// 更新LRU
	rc.mu.Lock()
	if elem, ok := rc.lruIndex[key]; ok {
		rc.lruList.MoveToFront(elem)
	}
	rc.mu.Unlock()

	return entry.response, true
}

// Set 设置缓存
func (rc *ResponseCache) Set(query, response string) {
	key := rc.hashQuery(query)

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// 如果已存在，更新
	if entry, exists := rc.entries[key]; exists {
		entry.response = response
		entry.createdAt = time.Now()
		if elem, ok := rc.lruIndex[key]; ok {
			rc.lruList.MoveToFront(elem)
		}
		return
	}

	// 检查是否需要淘汰
	if rc.maxSize > 0 && len(rc.entries) >= rc.maxSize {
		rc.evict()
	}

	// 添加新条目
	entry := &cacheEntry{
		query:     query,
		response:  response,
		createdAt: time.Now(),
	}
	rc.entries[key] = entry
	elem := rc.lruList.PushFront(key)
	rc.lruIndex[key] = elem
}

// Clear 清空缓存
func (rc *ResponseCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.entries = make(map[string]*cacheEntry)
	rc.lruList = list.New()
	rc.lruIndex = make(map[string]*list.Element)
}

// Delete 删除指定查询的缓存
func (rc *ResponseCache) Delete(query string) {
	key := rc.hashQuery(query)

	rc.mu.Lock()
	defer rc.mu.Unlock()

	delete(rc.entries, key)
	if elem, ok := rc.lruIndex[key]; ok {
		rc.lruList.Remove(elem)
		delete(rc.lruIndex, key)
	}
}

// GetStats 获取缓存统计信息
func (rc *ResponseCache) GetStats() *CacheStats {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return &CacheStats{
		TotalEntries:    len(rc.entries),
		MaxSize:         rc.maxSize,
		TTLMilliseconds: rc.ttl.Milliseconds(),
	}
}

// Size 获取当前缓存大小
func (rc *ResponseCache) Size() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.entries)
}

// evict 淘汰最久未使用的条目
func (rc *ResponseCache) evict() {
	if rc.lruList.Len() == 0 {
		return
	}

	// 获取最久未使用的key
	elem := rc.lruList.Back()
	if elem == nil {
		return
	}

	key := elem.Value.(string)

	// 删除条目
	delete(rc.entries, key)
	rc.lruList.Remove(elem)
	delete(rc.lruIndex, key)
}

// hashQuery 对查询进行哈希，生成缓存key
func (rc *ResponseCache) hashQuery(query string) string {
	// 对查询进行规范化处理（去除多余空格、转小写等）
	normalized := normalizeQuery(query)

	// 计算SHA256哈希
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// normalizeQuery 规范化查询文本
func normalizeQuery(query string) string {
	// 简单的规范化：去除首尾空格，转小写
	// 可以根据需要扩展更复杂的规范化逻辑
	result := make([]byte, 0, len(query))
	inSpace := false

	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !inSpace {
				result = append(result, ' ')
				inSpace = true
			}
		} else {
			// 转小写
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			result = append(result, c)
			inSpace = false
		}
	}

	return string(result)
}

// cleanupLoop 定期清理过期条目
func (rc *ResponseCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rc.cleanupExpired()
		case <-rc.stopClean:
			return
		}
	}
}

// cleanupExpired 清理过期条目
func (rc *ResponseCache) cleanupExpired() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := time.Now()
	for key, entry := range rc.entries {
		if now.Sub(entry.createdAt) > rc.ttl {
			delete(rc.entries, key)
			if elem, ok := rc.lruIndex[key]; ok {
				rc.lruList.Remove(elem)
				delete(rc.lruIndex, key)
			}
		}
	}
}

// Stop 停止缓存
func (rc *ResponseCache) Stop() {
	rc.stopOnce.Do(func() {
		close(rc.stopClean)
	})
}
