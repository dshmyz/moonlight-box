package http

import (
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

type CASHandler struct {
	casService *service.CASService
}

func NewCASHandler(casService *service.CASService) *CASHandler {
	return &CASHandler{casService: casService}
}

func (h *CASHandler) Login(c *gin.Context) {
	if !h.casService.IsEnabled() {
		response.BadRequest(c, "CAS login is not enabled", "please configure CAS in server settings")
		return
	}

	redirect := c.Query("redirect")
	loginURL := h.casService.GetLoginURL(redirect)
	c.Redirect(302, loginURL)
}

func (h *CASHandler) Callback(c *gin.Context) {
	if !h.casService.IsEnabled() {
		response.BadRequest(c, "CAS login is not enabled", "please configure CAS in server settings")
		return
	}

	ticket := c.Query("ticket")
	if ticket == "" {
		response.BadRequest(c, "missing ticket parameter", "ticket is required for CAS callback")
		return
	}

	resp, err := h.casService.LoginByTicket(ticket)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.Success(c, resp)
}

func (h *CASHandler) Config(c *gin.Context) {
	response.Success(c, gin.H{
		"enabled":   h.casService.IsEnabled(),
		"login_url": h.casService.GetLoginURL(""),
	})
}

// GetAdminConfig 返回完整 CAS 配置（管理端用）
func (h *CASHandler) GetAdminConfig(c *gin.Context) {
	cfg := h.casService.GetAdminConfig()
	response.Success(c, cfg)
}

// UpdateAdminConfig 更新 CAS 配置
func (h *CASHandler) UpdateAdminConfig(c *gin.Context) {
	var req model.CASConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	if req.Enabled && (req.ServerURL == "" || req.ServiceURL == "") {
		response.BadRequest(c, "validation failed", "server_url and service_url are required when CAS is enabled")
		return
	}

	if req.LoginPath == "" {
		req.LoginPath = service.DefaultCASLoginPath
	}
	if req.ValidatePath == "" {
		req.ValidatePath = service.DefaultCASValidatePath
	}

	userID := c.GetUint("userID")
	if err := h.casService.UpdateAdminConfig(&req, userID); err != nil {
		response.InternalError(c, "failed to update CAS config")
		return
	}

	response.Success(c, gin.H{"message": "CAS configuration updated successfully"})
}

// TestConnection 测试 CAS 服务器连通性
func (h *CASHandler) TestConnection(c *gin.Context) {
	if err := h.casService.TestConnection(); err != nil {
		response.BadRequest(c, "CAS connection test failed", err.Error())
		return
	}
	response.Success(c, gin.H{"message": "CAS server is reachable"})
}
