package http

import (
	"net/http"
	"strconv"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/response"

	"github.com/gin-gonic/gin"
)

type AuditLogHandler struct {
	auditRepo *repository.AuditRepository
}

func NewAuditLogHandler(auditRepo *repository.AuditRepository) *AuditLogHandler {
	return &AuditLogHandler{auditRepo: auditRepo}
}

func (h *AuditLogHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 100 {
		pageSize = 100
	}
	action := c.Query("action")
	ipAddress := c.Query("ip_address")
	resourceType := c.Query("resource_type")

	var actionFilter *model.AuditAction
	if action != "" {
		a := model.AuditAction(action)
		actionFilter = &a
	}

	logs, total, err := h.auditRepo.List(page, pageSize, ipAddress, resourceType, actionFilter)
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.SuccessWithPagination(c, logs, page, pageSize, total)
}

func (h *AuditLogHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid log ID", "log ID must be a positive integer")
		return
	}

	log, err := h.auditRepo.GetByID(uint(id))
	if err != nil {
		response.NotFound(c, "log not found")
		return
	}

	response.Success(c, log)
}

var _ = http.StatusOK
