package http

import (
	"github.com/moonlight-box/registry/internal/response"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/core/cache"
)

type CacheHandler struct {
	cacheMgr *cache.CacheManager
}

func NewCacheHandler(cacheMgr *cache.CacheManager) *CacheHandler {
	return &CacheHandler{cacheMgr: cacheMgr}
}

func (h *CacheHandler) ListCaches(c *gin.Context) {
	response.Success(c, h.cacheMgr.List())
}

func (h *CacheHandler) GetStats(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		stats := h.cacheMgr.StatsAll(c.Request.Context())
		response.Success(c, stats)
		return
	}

	p, ok := h.cacheMgr.Get(name)
	if !ok {
		response.NotFound(c, "cache not found")
		return
	}

	response.Success(c, p.Stats(c.Request.Context()))
}

func (h *CacheHandler) List(c *gin.Context) {
	offsetStr := c.DefaultQuery("offset", "0")
	limitStr := c.DefaultQuery("limit", "50")
	search := c.Query("search")
	cacheType := c.Query("type")

	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)

	name := c.Param("name")

	if name != "" {
		p, ok := h.cacheMgr.Get(name)
		if !ok {
			response.NotFound(c, "cache not found")
			return
		}

		items, total := p.ListItems(offset, limit, search)
		response.Success(c, gin.H{
			"items":  items,
			"total":  total,
			"offset": offset,
			"limit":  limit,
		})
		return
	}

	items, total := h.cacheMgr.ListAllItems(offset, limit, search, cacheType)
	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

func (h *CacheHandler) DeleteItem(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		response.BadRequest(c, "key is required", nil)
		return
	}

	name := c.Param("name")
	if name != "" {
		p, ok := h.cacheMgr.Get(name)
		if !ok {
			response.NotFound(c, "cache not found")
			return
		}

		if err := p.Delete(c.Request.Context(), key); err != nil {
			response.InternalError(c, err.Error())
			return
		}

		response.Success(c, gin.H{"message": "Cache item deleted"})
		return
	}

	if err := h.cacheMgr.DeleteKeyFromAll(c.Request.Context(), key); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Cache item deleted from all caches"})
}

func (h *CacheHandler) CleanupExpired(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		response.BadRequest(c, "cache name is required", nil)
		return
	}

	p, ok := h.cacheMgr.Get(name)
	if !ok {
		response.NotFound(c, "cache not found")
		return
	}

	if err := p.Clear(c.Request.Context()); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"cleaned": 0})
}

func (h *CacheHandler) Clear(c *gin.Context) {
	name := c.Param("name")
	if name != "" {
		if err := h.cacheMgr.Clear(c.Request.Context(), name); err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.Success(c, gin.H{"message": "Cache cleared"})
		return
	}

	if err := h.cacheMgr.ClearAll(c.Request.Context()); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "All caches cleared"})
}

func (h *CacheHandler) Invalidate(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Pattern string `json:"pattern" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.cacheMgr.Invalidate(c.Request.Context(), req.Name, req.Pattern); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Cache invalidated"})
}
