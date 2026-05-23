package http

import (
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type CASAdminHandler struct {
	casConfigSvc *service.CASConfigService
}

func NewCASAdminHandler(casConfigSvc *service.CASConfigService) *CASAdminHandler {
	return &CASAdminHandler{casConfigSvc: casConfigSvc}
}

func (h *CASAdminHandler) GetConfig(c *gin.Context) {
	config, err := h.casConfigSvc.GetConfig()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, config)
}

func (h *CASAdminHandler) UpdateConfig(c *gin.Context) {
	var req model.CASConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	userID := c.GetUint("userID")
	if err := h.casConfigSvc.SaveConfig(&req, &userID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, req)
}

func (h *CASAdminHandler) DeleteConfig(c *gin.Context) {
	if err := h.casConfigSvc.DeleteConfig(); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "CAS configuration deleted"})
}
