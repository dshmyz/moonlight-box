package main

import (
	"embed"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var frontendFS embed.FS

func setupFrontendRouter(r *gin.Engine, staticDir string) {
	if staticDir != "" && dirExists(staticDir) {
		r.NoRoute(serveFilesystemFrontend(staticDir))
	} else {
		r.NoRoute(serveEmbeddedFrontend())
	}
}

func serveEmbeddedFrontend() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		if reqPath == "/" {
			reqPath = "/index.html"
		}

		filePath := path.Join("dist", reqPath)
		data, err := frontendFS.ReadFile(filePath)
		if err != nil {
			fallbackPath := "dist/index.html"
			data, err = frontendFS.ReadFile(fallbackPath)
			if err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Header("Content-Type", getContentType(fallbackPath))
			c.Data(http.StatusOK, getContentType(fallbackPath), data)
			return
		}

		c.Header("Content-Type", getContentType(filePath))
		c.Data(http.StatusOK, getContentType(filePath), data)
	}
}

func serveFilesystemFrontend(staticDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		if reqPath == "/" {
			reqPath = "/index.html"
		}
		filePath := filepath.Join(staticDir, reqPath)

		_, err := os.Stat(filePath)
		if err != nil {
			indexPath := filepath.Join(staticDir, "index.html")
			content, err := os.ReadFile(indexPath)
			if err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, string(content))
			return
		}

		c.File(filePath)
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func getContentType(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff", ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}

func serveEmbeddedDocs(c *gin.Context) {
	reqPath := c.Param("filepath")
	if reqPath == "" || reqPath == "/" {
		reqPath = "/swagger/index.html"
	}

	filePath := path.Join("dist/docs", reqPath)
	data, err := frontendFS.ReadFile(filePath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// 对模板文件动态替换域名占位符，使客户端配置中的地址与当前请求地址一致
	if strings.HasPrefix(reqPath, "/templates/") {
		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}
		content := strings.ReplaceAll(string(data), "your-moonlight-domain", host)
		data = []byte(content)
	}

	c.Header("Content-Type", getContentType(filePath))
	c.Data(http.StatusOK, getContentType(filePath), data)
}
