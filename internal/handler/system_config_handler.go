package handler

import (
	"runtime"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type SystemConfigHandler struct {
	configSvc *service.SystemConfigService
	auditSvc  *service.AuditService
}

func NewSystemConfigHandler(configSvc *service.SystemConfigService, auditSvc *service.AuditService) *SystemConfigHandler {
	return &SystemConfigHandler{
		configSvc: configSvc,
		auditSvc:  auditSvc,
	}
}

type SetConfigRequest struct {
	Key         string `json:"key" binding:"required"`
	Value       string `json:"value" binding:"required"`
	ValueType   string `json:"value_type"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IsSensitive bool   `json:"is_sensitive"`
}

type BatchUpdateConfigRequest struct {
	Configs []struct {
		Key   string `json:"key" binding:"required"`
		Value string `json:"value" binding:"required"`
	} `json:"configs" binding:"required"`
}

func (h *SystemConfigHandler) BatchUpdate(c *gin.Context) {
	var req BatchUpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	userID := c.GetUint("userID")

	for _, cfg := range req.Configs {
		existing, err := h.configSvc.Get(cfg.Key)
		valueType := "string"
		category := ""
		description := ""
		isSensitive := false
		if err == nil && existing != nil {
			valueType = existing.ValueType
			category = existing.Category
			description = existing.Description
			isSensitive = existing.IsSensitive
		}
		if err := h.configSvc.Set(cfg.Key, cfg.Value, valueType, category, description, isSensitive, userID); err != nil {
			response.InternalError(c, "failed to update config: "+cfg.Key)
			return
		}
	}

	response.Success(c, gin.H{"message": "configs updated successfully"})
}

func (h *SystemConfigHandler) Get(c *gin.Context) {
	key := c.Param("key")

	config, err := h.configSvc.Get(key)
	if err != nil {
		response.NotFound(c, "config not found")
		return
	}

	response.Success(c, config)
}

func (h *SystemConfigHandler) List(c *gin.Context) {
	configs, err := h.configSvc.GetAll()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, configs)
}

func (h *SystemConfigHandler) Set(c *gin.Context) {
	var req SetConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	userID := c.GetUint("userID")

	valueType := req.ValueType
	if valueType == "" {
		valueType = "string"
	}

	if err := h.configSvc.Set(req.Key, req.Value, valueType, req.Category, req.Description, req.IsSensitive, userID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.auditSvc != nil && userID > 0 {
		uid := userID
		_ = h.auditSvc.LogWithRequestAndStatus(
			c.Request.Context(),
			&uid,
			model.ActionConfigChange,
			"config",
			nil,
			req.Key,
			`{"action":"set"}`,
			c.ClientIP(),
			c.Request.UserAgent(),
			200,
			0,
		)
	}

	response.Success(c, gin.H{"message": "config saved successfully"})
}

func (h *SystemConfigHandler) Delete(c *gin.Context) {
	key := c.Param("key")

	if err := h.configSvc.Delete(key); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.NoContent(c)
}

type SystemInfoHandler struct {
	version   string
	buildTime string
	gitCommit string
	startTime int64

	cacheMu    sync.RWMutex
	cachedInfo gin.H
	cacheTime  time.Time
	cacheTTL   time.Duration
}

func NewSystemInfoHandler(version, buildTime, gitCommit string, startTime int64) *SystemInfoHandler {
	return &SystemInfoHandler{
		version:   version,
		buildTime: buildTime,
		gitCommit: gitCommit,
		startTime: startTime,
		cacheTTL:  5 * time.Second,
	}
}

func (h *SystemInfoHandler) GetInfo(c *gin.Context) {
	h.cacheMu.RLock()
	if h.cachedInfo != nil && time.Since(h.cacheTime) < h.cacheTTL {
		info := h.cachedInfo
		h.cacheMu.RUnlock()
		response.Success(c, info)
		return
	}
	h.cacheMu.RUnlock()

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	if h.cachedInfo != nil && time.Since(h.cacheTime) < h.cacheTTL {
		response.Success(c, h.cachedInfo)
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Now().Unix() - h.startTime

	info := gin.H{
		"version":         h.version,
		"build_time":      h.buildTime,
		"git_commit":      h.gitCommit,
		"uptime":          uptime,
		"go_version":      runtime.Version(),
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"cpu_count":       runtime.NumCPU(),
		"goroutine_count": runtime.NumGoroutine(),
		"memory_usage":    calculateMemoryUsage(&memStats),
		"database_pool":   getDatabasePoolStats(),
	}

	h.cachedInfo = info
	h.cacheTime = time.Now()

	response.Success(c, info)
}

func calculateMemoryUsage(ms *runtime.MemStats) float64 {
	if ms.Sys == 0 {
		return 0
	}
	usage := float64(ms.Alloc) / float64(ms.Sys) * 100
	if usage > 100 {
		return 100
	}
	return float64(int(usage*10)) / 10
}

func getDatabasePoolStats() gin.H {
	stats := database.GetPoolStats()
	if stats == nil {
		return gin.H{
			"max_open_connections": 0,
			"open_connections":     0,
			"in_use":               0,
			"idle":                 0,
			"wait_count":           0,
			"wait_duration_ms":     0,
			"max_idle_closed":      0,
			"max_idle_time_closed": 0,
			"max_lifetime_closed":  0,
		}
	}

	return gin.H{
		"max_open_connections": stats.MaxOpenConnections,
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration_ms":     stats.WaitDuration.Milliseconds(),
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_idle_time_closed": stats.MaxIdleTimeClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}
}

func (h *SystemInfoHandler) Health(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "ok",
		"version": h.version,
	})
}
