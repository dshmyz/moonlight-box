package middleware

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// AccessLog 记录每个 HTTP 请求的访问日志。
// 启用分文件日志时写入 access.log；未启用时走主 logger（stdout）。
// 替代 gin.Logger()，让访问日志受统一日志配置控制（级别/格式/轮转）。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		util.GetLogger(util.LogTypeAccess).WithFields(logrus.Fields{
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
			"status":      c.Writer.Status(),
			"duration_ms": time.Since(start).Milliseconds(),
			"request_id":  c.GetString("RequestID"),
			"client_ip":   c.ClientIP(),
			"user_agent":  c.Request.UserAgent(),
		}).Info("access")
	}
}
