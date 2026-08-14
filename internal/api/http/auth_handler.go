package http

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService   *service.AuthService
	auditSvc      *service.AuditService
	apiTokenSvc   *service.APITokenService
}

func NewAuthHandler(authService *service.AuthService, auditSvc *service.AuditService) *AuthHandler {
	return &AuthHandler{authService: authService, auditSvc: auditSvc}
}

// SetAPITokenService 注入 API Token 服务
func (h *AuthHandler) SetAPITokenService(svc *service.APITokenService) {
	h.apiTokenSvc = svc
}

// @Summary 用户登录
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body service.LoginRequest true "登录信息"
// @Success 200 {object} response.Response{data=service.AuthResponse}
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	resp, err := h.authService.Login(&req)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	if h.auditSvc != nil {
		userID := resp.User.ID
		_ = h.auditSvc.LogWithRequestAndStatus(
			c.Request.Context(),
			&userID,
			model.ActionLogin,
			"user",
			&userID,
			resp.User.Username,
			"",
			c.ClientIP(),
			c.Request.UserAgent(),
			200,
			0,
		)
	}

	response.Success(c, resp)
}

// @Summary 登出
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	userID := c.GetUint("userID")
	token := extractTokenFromContext(c)
	if token != "" {
		h.authService.Logout(token)
	}

	if h.auditSvc != nil && userID > 0 {
		uid := userID
		_ = h.auditSvc.LogWithRequestAndStatus(
			c.Request.Context(),
			&uid,
			model.ActionLogout,
			"user",
			&uid,
			"",
			"",
			c.ClientIP(),
			c.Request.UserAgent(),
			200,
			0,
		)
	}

	response.Success(c, gin.H{"message": "logged out successfully"})
}

// @Summary 刷新 Token
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body object{refresh_token=string} true "刷新令牌"
// @Success 200 {object} response.Response{data=service.AuthResponse}
// @Router /api/v1/auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	resp, err := h.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.Success(c, resp)
}

// @Summary 获取当前用户信息
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Response{data=model.UserDTO}
// @Router /api/v1/auth/profile [get]
func (h *AuthHandler) Profile(c *gin.Context) {
	userID := c.GetUint("userID")

	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.Success(c, user.ToDTO())
}

// @Summary 更新用户信息
// @Tags Auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object{email=string,display_name=string,avatar_url=string} true "更新信息"
// @Success 200 {object} response.Response{data=model.UserDTO}
// @Router /api/v1/auth/profile [put]
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetUint("userID")

	var req struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		AvatarURL   string `json:"avatar_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	if req.Email != "" {
		user.Email = req.Email
	}
	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	if req.AvatarURL != "" {
		user.AvatarURL = req.AvatarURL
	}

	if err := h.authService.UpdateUser(user); err != nil {
		response.InternalError(c, "failed to update profile")
		return
	}

	response.Success(c, user.ToDTO())
}

// @Summary 修改密码
// @Tags Auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object{old_password=string,new_password=string} true "密码信息"
// @Success 200 {object} response.Response
// @Router /api/v1/auth/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := c.GetUint("userID")

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	if err := h.authService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		response.BadRequest(c, "failed to change password", err.Error())
		return
	}

	response.Success(c, gin.H{"message": "password changed successfully"})
}

func extractTokenFromContext(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

// --- API Token 管理 ---

// @Summary 列出当前用户的访问令牌
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Response{data=[]model.APIToken}
// @Router /api/v1/auth/tokens [get]
func (h *AuthHandler) ListTokens(c *gin.Context) {
	if h.apiTokenSvc == nil {
		response.InternalError(c, "token service not configured")
		return
	}
	userID := c.GetUint("userID")
	tokens, err := h.apiTokenSvc.ListTokens(userID)
	if err != nil {
		response.InternalError(c, "failed to list tokens")
		return
	}
	response.Success(c, tokens)
}

// @Summary 签发新的访问令牌
// @Tags Auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object{name=string,expires_in=string} true "令牌信息"
// @Success 200 {object} response.Response{data=object{token=string,info=model.APIToken}}
// @Router /api/v1/auth/tokens [post]
func (h *AuthHandler) CreateToken(c *gin.Context) {
	if h.apiTokenSvc == nil {
		response.InternalError(c, "token service not configured")
		return
	}

	var req struct {
		Name      string `json:"name" binding:"required"`
		ExpiresIn string `json:"expires_in"` // 可选，如 "30d", "1y"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	userID := c.GetUint("userID")

	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		d, err := parseDurationExtended(req.ExpiresIn)
		if err != nil {
			response.BadRequest(c, "invalid expires_in format", err.Error())
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	token, info, err := h.apiTokenSvc.CreateToken(userID, req.Name, expiresAt)
	if err != nil {
		response.InternalError(c, "failed to create token")
		return
	}

	response.Success(c, gin.H{
		"token": token,
		"info":  info,
	})
}

// @Summary 撤销访问令牌
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Param id path int true "令牌 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/auth/tokens/{id} [delete]
func (h *AuthHandler) DeleteToken(c *gin.Context) {
	if h.apiTokenSvc == nil {
		response.InternalError(c, "token service not configured")
		return
	}

	userID := c.GetUint("userID")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid token id", err.Error())
		return
	}

	if err := h.apiTokenSvc.DeleteToken(uint(id), userID); err != nil {
		response.InternalError(c, "failed to delete token")
		return
	}

	response.Success(c, gin.H{"message": "token deleted"})
}

// parseDurationExtended 解析带天/年的时间段，扩展 time.ParseDuration。
// 支持: "30d" (30天), "1y" (1年), 以及标准格式如 "24h", "30m"。
func parseDurationExtended(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, &time.ParseError{Layout: "duration", Value: s, LayoutElem: "non-empty"}
	}

	// 处理天数 (d)
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		var days float64
		if _, err := fmt.Sscanf(numStr, "%f", &days); err != nil {
			return 0, fmt.Errorf("invalid day duration: %s", s)
		}
		if days < 0 {
			return 0, fmt.Errorf("duration must be positive: %s", s)
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	}

	// 处理年份 (y)
	if strings.HasSuffix(s, "y") {
		numStr := strings.TrimSuffix(s, "y")
		var years float64
		if _, err := fmt.Sscanf(numStr, "%f", &years); err != nil {
			return 0, fmt.Errorf("invalid year duration: %s", s)
		}
		if years < 0 {
			return 0, fmt.Errorf("duration must be positive: %s", s)
		}
		return time.Duration(years * 365 * 24 * float64(time.Hour)), nil
	}

	// 回退到标准 time.ParseDuration
	return time.ParseDuration(s)
}
