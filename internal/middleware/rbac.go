package middleware

import (
	"strings"

	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

func RequirePermission(permCache *service.PermissionCacheService, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("userID")
		if userID == 0 {
			response.Unauthorized(c, "missing user information")
			c.Abort()
			return
		}

		hasPerm, err := permCache.HasPermission(userID, resource, action)
		if err != nil {
			response.InternalError(c, "failed to load user permissions")
			c.Abort()
			return
		}

		if !hasPerm {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
		}

		c.Next()
	}
}

func RequireRole(roleNames ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles := c.GetStringSlice("roles")

		hasRole := false
		for _, userRole := range roles {
			for _, requiredRole := range roleNames {
				if strings.EqualFold(userRole, requiredRole) {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			response.Forbidden(c, "insufficient role")
			c.Abort()
			return
		}

		c.Next()
	}
}
