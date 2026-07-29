package http

import (
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// internalErr 记录内部错误日志并返回通用错误消息给客户端，避免泄漏内部信息。
// msg 参数用于日志上下文（如 "failed to list users"），不会返回给客户端。
func internalErr(c *gin.Context, err error, msg string) {
	logrus.WithError(err).WithField("path", c.Request.URL.Path).Error(msg)
	response.InternalError(c, "internal server error")
}
