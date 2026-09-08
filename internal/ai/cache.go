package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	corecache "github.com/dshmyz/moonlight-box/internal/core/cache"
)

// ResponseCache 响应缓存，用于缓存相似问题的答案。
// 统一走 core/cache.MemoryCache（TTL + LRU 容量淘汰 + 后台清理），
// 本结构仅保留协议化的 Get/Set/Stats 接口与查询规范化/哈希逻辑。
type ResponseCache struct {
	mc      *corecache.MemoryCache
	maxSize int
	ttl     time.Duration
}

// CacheStats 缓存统计信息
type CacheStats struct {
	TotalEntries    int   `json:"total_entries"`
	MaxSize         int   `json:"max_size"`
	TTLMilliseconds int64 `json:"ttl_milliseconds"`
}

// NewResponseCache 创建一个新的响应缓存
func NewResponseCache(cfg *config.AICacheConfig) *ResponseCache {
	return &ResponseCache{
		mc:      corecache.NewMemoryCacheWithOptions(corecache.MemoryCacheOptions{MaxItems: cfg.MaxSize}),
		maxSize: cfg.MaxSize,
		ttl:     cfg.TTL,
	}
}

// Cache 暴露底层缓存供 main.go 注册进 CacheManager（管理页可见/可清空）。
func (rc *ResponseCache) Cache() *corecache.MemoryCache {
	return rc.mc
}

// Get 获取缓存的响应
func (rc *ResponseCache) Get(query string) (string, bool) {
	v, ok := rc.mc.Get(rc.hashQuery(query))
	if !ok {
		return "", false
	}
	response, ok := v.(string)
	return response, ok
}

// Set 设置缓存
func (rc *ResponseCache) Set(query, response string) {
	rc.mc.Set(rc.hashQuery(query), response, rc.ttl)
}

// Clear 清空缓存
func (rc *ResponseCache) Clear() {
	rc.mc.Clear()
}

// GetStats 获取缓存统计信息
func (rc *ResponseCache) GetStats() *CacheStats {
	return &CacheStats{
		TotalEntries:    rc.mc.Count(),
		MaxSize:         rc.maxSize,
		TTLMilliseconds: rc.ttl.Milliseconds(),
	}
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

// Stop 停止缓存（后台清理协程随 MemoryCache 一并退出）
func (rc *ResponseCache) Stop() {
	rc.mc.Stop()
}
