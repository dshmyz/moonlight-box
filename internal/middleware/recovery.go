package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logrus.WithFields(logrus.Fields{
					"module":     "middleware",
					"request_id": c.GetString("RequestID"),
					"method":     c.Request.Method,
					"path":       c.Request.URL.Path,
					"error":      err,
				}).Error("PANIC recovered")

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
