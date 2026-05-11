package middleware

import (
	"strings"

	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

func Auth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token != "" {
			claims, err := authService.ValidateToken(token)
			if err != nil {
				response.Unauthorized(c, "invalid or expired token")
				c.Abort()
				return
			}

			c.Set("userID", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("roles", claims.Roles)
			c.Next()
			return
		}

		userID, username, roles, ok := extractBasicAuth(c, authService)
		if ok {
			c.Set("userID", userID)
			c.Set("username", username)
			c.Set("roles", roles)
			c.Next()
			return
		}

		response.Unauthorized(c, "missing authorization header")
		c.Abort()
	}
}

func OptionalAuth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token != "" {
			claims, err := authService.ValidateToken(token)
			if err == nil {
				c.Set("userID", claims.UserID)
				c.Set("username", claims.Username)
				c.Set("roles", claims.Roles)
				c.Next()
				return
			}
		}

		userID, username, roles, ok := extractBasicAuth(c, authService)
		if ok {
			c.Set("userID", userID)
			c.Set("username", username)
			c.Set("roles", roles)
		}

		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}

	if strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}

	return ""
}

func extractBasicAuth(c *gin.Context, authService *service.AuthService) (uint, string, []string, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return 0, "", nil, false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return 0, "", nil, false
	}

	username, password, ok := parseBasicAuth(c, parts[1])
	if !ok {
		return 0, "", nil, false
	}

	claims, err := authService.Login(&service.LoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return 0, "", nil, false
	}

	return claims.User.ID, claims.User.Username, claims.User.Roles, true
}

func parseBasicAuth(c *gin.Context, auth string) (username, password string, ok bool) {
	return c.Request.BasicAuth()
}
