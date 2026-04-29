package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type StorageBackendHandler struct {
	svc *service.StorageBackendService
}

func NewStorageBackendHandler(svc *service.StorageBackendService) *StorageBackendHandler {
	return &StorageBackendHandler{svc: svc}
}

type storageBackendRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        string                 `json:"type" binding:"required"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config" binding:"required"`
	IsDefault   bool                   `json:"is_default"`
	Status      string                 `json:"status"`
	IsActive    bool                   `json:"is_active"`
}

func (h *StorageBackendHandler) List(c *gin.Context) {
	backends, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, backends)
}

func (h *StorageBackendHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	backend, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage backend not found"})
		return
	}
	c.JSON(http.StatusOK, backend)
}

func (h *StorageBackendHandler) Create(c *gin.Context) {
	var req storageBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configData, _ := json.Marshal(req.Config)
	var cfg model.StorageBackendConfig
	if err := json.Unmarshal(configData, &cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config"})
		return
	}

	backend := &model.StorageBackend{
		Name:        req.Name,
		Type:        model.StorageBackendType(req.Type),
		Description: req.Description,
		Config:      cfg,
		IsDefault:   req.IsDefault,
		IsActive:    req.IsActive,
	}
	if req.Status == "" {
		backend.Status = model.StatusActive
	} else {
		backend.Status = model.StorageBackendStatus(req.Status)
	}

	result, err := h.svc.Create(backend)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *StorageBackendHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req storageBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configData, _ := json.Marshal(req.Config)
	var cfg model.StorageBackendConfig
	if err := json.Unmarshal(configData, &cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config"})
		return
	}

	backend := &model.StorageBackend{
		ID:          uint(id),
		Name:        req.Name,
		Type:        model.StorageBackendType(req.Type),
		Description: req.Description,
		Config:      cfg,
		IsDefault:   req.IsDefault,
		IsActive:    req.IsActive,
	}
	if req.Status == "" {
		backend.Status = model.StatusActive
	} else {
		backend.Status = model.StorageBackendStatus(req.Status)
	}

	result, err := h.svc.Update(backend)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *StorageBackendHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.svc.Delete(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *StorageBackendHandler) SetDefault(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.svc.SetDefault(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "default updated"})
}

func (h *StorageBackendHandler) TestConnection(c *gin.Context) {
	var req storageBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configData, _ := json.Marshal(req.Config)
	var cfg model.StorageBackendConfig
	if err := json.Unmarshal(configData, &cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config"})
		return
	}

	backend := &model.StorageBackend{
		Type:   model.StorageBackendType(req.Type),
		Config: cfg,
	}

	if err := h.svc.TestConnection(backend); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "connection successful"})
}
