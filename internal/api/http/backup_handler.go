package http

import (
	"strconv"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

type BackupHandler struct {
	backupSvc *service.BackupService
}

func NewBackupHandler(backupSvc *service.BackupService) *BackupHandler {
	return &BackupHandler{
		backupSvc: backupSvc,
	}
}

type CreateBackupRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func (h *BackupHandler) Create(c *gin.Context) {
	var req CreateBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	backupType := model.BackupTypeFull
	if req.Type == "incremental" {
		backupType = model.BackupTypeIncremental
	}

	userID := c.GetUint("userID")

	backup, err := h.backupSvc.CreateBackup(req.Name, backupType, req.Description, userID)
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.Created(c, backup)
}

func (h *BackupHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if pageSize > 100 {
		pageSize = 100
	}

	backups, total, err := h.backupSvc.ListBackups(page, pageSize)
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.SuccessWithPagination(c, backups, page, pageSize, total)
}

func (h *BackupHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid backup id", err.Error())
		return
	}

	backup, err := h.backupSvc.GetBackup(uint(id))
	if err != nil {
		response.NotFound(c, "backup not found")
		return
	}

	response.Success(c, backup)
}

func (h *BackupHandler) Restore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid backup id", err.Error())
		return
	}

	if err := h.backupSvc.RestoreBackup(uint(id)); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.Success(c, gin.H{"message": "backup restored successfully"})
}

func (h *BackupHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid backup id", err.Error())
		return
	}

	if err := h.backupSvc.DeleteBackup(uint(id)); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.NoContent(c)
}
