package middleware

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				stackTrace := string(buf[:n])

				logrus.WithFields(logrus.Fields{
					"module":     "middleware",
					"request_id": c.GetString("RequestID"),
					"method":     c.Request.Method,
					"path":       c.Request.URL.Path,
					"error":      err,
					"stack":      stackTrace,
				}).Error("PANIC recovered")

				fmt.Printf("PANIC: %v\nStack trace:\n%s\n", err, stackTrace)

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