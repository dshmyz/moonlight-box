package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 允许的来源 (可从配置读取)
		allowedOrigins := []string{
			"http://localhost:3000",
			"http://localhost:5173", // Vite 默认端口
			"http://localhost:8080",
		}

		isAllowed := false
		for _, allowed := range allowedOrigins {
			if strings.EqualFold(origin, allowed) {
				isAllowed = true
				break
			}
		}

		if isAllowed || origin == "" {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "Content-Length, X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
