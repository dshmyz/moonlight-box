package http

import (
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/sirupsen/logrus"

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
	loginURL, err := h.casService.GetLoginURL(c, redirect)
	if err != nil {
		response.BadRequest(c, "failed to build CAS login URL", err.Error())
		return
	}
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

	resp, err := h.casService.LoginByTicket(c, ticket)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.Success(c, resp)
}

func (h *CASHandler) Config(c *gin.Context) {
	// enabled 反映全局配置是否可完成登录（IsEnabled 已保证 service 可解析）。
	// login_url 只在当前请求可解析时给出：动态模式命中非白名单 Host 时为缺省，
	// 避免出现 "enabled=true 但 login_url=''" 的误导性死字段。
	enabled := h.casService.IsEnabled()
	payload := gin.H{"enabled": enabled}
	if loginURL, err := h.casService.GetLoginURL(c, ""); err == nil && loginURL != "" {
		payload["login_url"] = loginURL
	} else if err != nil {
		logrus.WithError(err).Warn("CAS: failed to resolve login url for config probe")
	}
	response.Success(c, payload)
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

	// service_url 可选：留空时按请求 Host 动态推导（多域名场景），需配合 allowed_hosts 白名单。
	// 启用时二者至少配置其一，保证 IsEnabled 的不变量 "enabled ⟹ service 可解析" 成立。
	if req.Enabled {
		if req.ServerURL == "" {
			response.BadRequest(c, "validation failed", "server_url is required when CAS is enabled")
			return
		}
		if req.ServiceURL == "" && len(req.AllowedHosts) == 0 {
			response.BadRequest(c, "validation failed", "CAS requires service_url or allowed_hosts when enabled")
			return
		}
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
