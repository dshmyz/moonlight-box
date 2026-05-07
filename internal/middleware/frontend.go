package middleware

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// FrontendConfig 前端配置
type FrontendConfig struct {
	StaticDir string // 静态文件目录
	IndexFile string // 默认首页文件
}

// DefaultFrontendConfig 返回默认前端配置
func DefaultFrontendConfig() FrontendConfig {
	return FrontendConfig{
		StaticDir: "./cmd/registry/dist",
		IndexFile: "index.html",
	}
}

// ServeFrontend 创建前端静态文件服务中间件
func ServeFrontend(cfg FrontendConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取请求路径
		path := c.Request.URL.Path

		// 构建文件路径
		filePath := filepath.Join(cfg.StaticDir, path)

		// 检查文件是否存在
		_, err := os.Stat(filePath)
		if err != nil {
			// 文件不存在，返回 index.html（SPA 路由）
			serveIndex(c, cfg)
			return
		}

		// 文件存在，直接提供静态文件
		c.File(filePath)
	}
}

// serveIndex 返回首页
func serveIndex(c *gin.Context, cfg FrontendConfig) {
	indexPath := filepath.Join(cfg.StaticDir, cfg.IndexFile)
	
	// 读取并渲染 index.html
	content, err := os.ReadFile(indexPath)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, string(content))
}

// RenderIndex 渲染首页模板（用于 SSR）
func RenderIndex(c *gin.Context, cfg FrontendConfig, data map[string]interface{}) {
	indexPath := filepath.Join(cfg.StaticDir, cfg.IndexFile)
	
	tmpl, err := template.ParseFiles(indexPath)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}
