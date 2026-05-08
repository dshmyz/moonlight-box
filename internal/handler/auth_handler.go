package handler

import (
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
	auditSvc    *service.AuditService
}

func NewAuthHandler(authService *service.AuthService, auditSvc *service.AuditService) *AuthHandler {
	return &AuthHandler{authService: authService, auditSvc: auditSvc}
}

// @Summary 用户登录
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body service.LoginRequest true "登录信息"
// @Success 200 {object} Response{data=service.AuthResponse}
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
// @Success 200 {object} Response
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
// @Success 200 {object} Response{data=service.AuthResponse}
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
// @Success 200 {object} Response{data=model.UserDTO}
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
// @Param body body UpdateProfileRequest true "更新信息"
// @Success 200 {object} Response{data=model.UserDTO}
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
// @Param body body ChangePasswordRequest true "密码信息"
// @Success 200 {object} Response
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
