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
		if token == "" {
			response.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		claims, err := authService.ValidateToken(token)
		if err != nil {
			response.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)

		c.Next()
	}
}

func OptionalAuth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.Next()
			return
		}

		claims, err := authService.ValidateToken(token)
		if err != nil {
			c.Next() // Token 无效但不阻断，作为匿名用户继续
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)

		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}
