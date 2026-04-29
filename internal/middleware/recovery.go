package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC recovered: %v\n%s", err, debug.Stack())

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":       http.StatusInternalServerError,
					"message":    "Internal server error",
					"request_id": c.GetString("RequestID"),
				})
			}
		}()

		c.Next()
	}
}
