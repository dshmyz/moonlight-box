package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/proxy"
)

type CacheHandler struct {
	cacheSvc *proxy.CacheService
}

func NewCacheHandler(cacheSvc *proxy.CacheService) *CacheHandler {
	return &CacheHandler{cacheSvc: cacheSvc}
}

func (h *CacheHandler) GetStats(c *gin.Context) {
	stats, err := h.cacheSvc.GetStats(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	expiredCount := h.cacheSvc.GetExpiredCount()

	enhancedStats := map[string]interface{}{
		"total_items":     stats["total_items"],
		"positive_items":  stats["positive_items"],
		"negative_items":  stats["negative_items"],
		"total_size":      stats["total_size"],
		"used_bytes":      stats["used_bytes"],
		"max_bytes":       stats["max_bytes"],
		"max_items":       stats["max_items"],
		"num_shards":      stats["num_shards"],
		"expired_entries": expiredCount,
		"max_size_gb":     float64(stats["max_bytes"].(int64)) / (1024 * 1024 * 1024),
	}

	Success(c, enhancedStats)
}

func (h *CacheHandler) List(c *gin.Context) {
	offsetStr := c.DefaultQuery("offset", "0")
	limitStr := c.DefaultQuery("limit", "50")
	search := c.Query("search")

	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)

	items, total := h.cacheSvc.ListItems(offset, limit, search)

	Success(c, gin.H{
		"items": items,
		"total": total,
		"offset": offset,
		"limit": limit,
	})
}

func (h *CacheHandler) DeleteItem(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		BadRequest(c, "key is required", nil)
		return
	}

	if err := h.cacheSvc.DeleteItem(key); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Cache item deleted"})
}

func (h *CacheHandler) CleanupExpired(c *gin.Context) {
	count := h.cacheSvc.CleanupExpired()
	Success(c, gin.H{"cleaned": count})
}

func (h *CacheHandler) Clear(c *gin.Context) {
	if err := h.cacheSvc.Clear(c.Request.Context()); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Cache cleared"})
}

func (h *CacheHandler) Invalidate(c *gin.Context) {
	var req struct {
		Pattern string `json:"pattern" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.cacheSvc.Invalidate(c.Request.Context(), req.Pattern); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Cache invalidated"})
}
