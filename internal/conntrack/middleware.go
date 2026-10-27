package conntrack

import (
	"github.com/gin-gonic/gin"
)

// Middleware 返回 gin middleware：登记每个入站请求，请求结束后注销。
// 同时包装 ResponseWriter 统计响应字节数。
//
// 仓库名直接读路由参数：gin 在进入中间件链前已完成路由匹配，c.Param 可用，
// 因此不依赖任何 group 级中间件的执行顺序。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := Global().Register(Entry{
			Kind:       "request",
			Remote:     c.ClientIP(),
			Path:       c.Request.URL.Path,
			Repository: repositoryFromRoute(c),
			Extra: map[string]string{
				"method":     c.Request.Method,
				"user_agent": c.Request.UserAgent(),
			},
		})
		defer h.Unregister()

		c.Writer = &countingResponseWriter{ResponseWriter: c.Writer, handle: h}
		c.Next()
	}
}

// repositoryFromRoute 取仓库名：Nexus 3/2 的 :repoName 与组合仓库的 :groupName。
func repositoryFromRoute(c *gin.Context) string {
	if name := c.Param("repoName"); name != "" {
		return name
	}
	return c.Param("groupName")
}

// countingResponseWriter 包装 gin.ResponseWriter，把写出字节累计到登记项。
type countingResponseWriter struct {
	gin.ResponseWriter
	handle *Handle
}

func (w *countingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.handle.AddWritten(int64(n))
	return n, err
}

// WriteString 必须一并覆写：嵌入的 gin.ResponseWriter 会把 io.StringWriter
// 快路径直接透传，只覆写 Write 会漏掉这部分响应字节。
func (w *countingResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	w.handle.AddWritten(int64(n))
	return n, err
}
