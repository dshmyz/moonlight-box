package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodySizeLimit limits incoming request body size in bytes.
// It short-circuits large requests early based on Content-Length and
// also enforces hard limit while reading request body via MaxBytesReader.
func BodySizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"code":    http.StatusRequestEntityTooLarge,
				"message": "request body too large",
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

