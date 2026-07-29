package middleware

import (
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

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
			return
		}

		c.Next()
	}
}


