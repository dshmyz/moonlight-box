package handler

import (
	"strconv"

	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type SystemConfigHandler struct {
	configSvc *service.SystemConfigService
}

func NewSystemConfigHandler(configSvc *service.SystemConfigService) *SystemConfigHandler {
	return &SystemConfigHandler{
		configSvc: configSvc,
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
	startTime int64
}

func NewSystemInfoHandler(version, buildTime string, startTime int64) *SystemInfoHandler {
	return &SystemInfoHandler{
		version:   version,
		buildTime: buildTime,
		startTime: startTime,
	}
}

func (h *SystemInfoHandler) GetInfo(c *gin.Context) {
	info := gin.H{
		"version":    h.version,
		"build_time": h.buildTime,
		"start_time": h.startTime,
		"go_version": "go1.26.2",
		"os":         "linux",
		"arch":       "amd64",
		"cpu_cores":  strconv.Itoa(4),
		"goroutines": strconv.Itoa(10),
	}

	response.Success(c, info)
}

func (h *SystemInfoHandler) Health(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "ok",
		"version": h.version,
	})
}
