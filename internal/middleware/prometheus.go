package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/metrics"
)

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		metrics.RecordHTTPRequest(c.Request.Method, path, status, duration)
	}
}
