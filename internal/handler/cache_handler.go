package handler

import (
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/gin-gonic/gin"
)

// CacheHandler 缓存管理处理器
type CacheHandler struct {
	cacheSvc *proxy.CacheService
}

// NewCacheHandler 创建缓存管理处理器实例
func NewCacheHandler(cacheSvc *proxy.CacheService) *CacheHandler {
	return &CacheHandler{cacheSvc: cacheSvc}
}

// GetStats 获取缓存统计信息
func (h *CacheHandler) GetStats(c *gin.Context) {
	stats, err := h.cacheSvc.GetStats(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, stats)
}

// Clear 清空所有缓存
func (h *CacheHandler) Clear(c *gin.Context) {
	if err := h.cacheSvc.Clear(c.Request.Context()); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Cache cleared"})
}

// Invalidate 根据模式使指定缓存失效
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
