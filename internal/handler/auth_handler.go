package handler

import (
	"strings"

	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
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
		BadRequest(c, "invalid request body", err.Error())
		return
	}

	resp, err := h.authService.Login(&req)
	if err != nil {
		Unauthorized(c, err.Error())
		return
	}

	Success(c, resp)
}

// @Summary 登出
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} Response
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	token := extractTokenFromContext(c)
	if token != "" {
		h.authService.Logout(token)
	}

	Success(c, gin.H{"message": "logged out successfully"})
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
		BadRequest(c, "invalid request body", err.Error())
		return
	}

	resp, err := h.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		Unauthorized(c, err.Error())
		return
	}

	Success(c, resp)
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
		NotFound(c, "user not found")
		return
	}

	Success(c, user.ToDTO())
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
