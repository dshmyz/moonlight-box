package middleware

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type FrontendConfig struct {
	StaticDir    string
	IndexFile    string
	APIPrefixes  []string
	StaticAssets []string
}

func DefaultFrontendConfig() FrontendConfig {
	return FrontendConfig{
		StaticDir:    "web/dist",
		IndexFile:    "index.html",
		APIPrefixes:  []string{"/api/", "/repo/", "/npm/", "/maven2/", "/health", "/metrics", "/docs"},
		StaticAssets: []string{"/assets/", "/favicon.ico"},
	}
}

func isAPIRequest(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isStaticAsset(path string, assets []string) bool {
	for _, asset := range assets {
		if strings.Contains(path, asset) {
			return true
		}
	}
	return false
}

func ServeFrontend(config FrontendConfig) gin.HandlerFunc {
	staticDir, err := filepath.Abs(config.StaticDir)
	if err != nil {
		panic("invalid frontend static dir: " + config.StaticDir)
	}

	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		panic("frontend build not found: " + staticDir + ". Run 'cd web && npm run build' first.")
	}

	fileServer := http.FileServer(http.Dir(staticDir))
	indexFile := filepath.Join(staticDir, config.IndexFile)

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if isAPIRequest(path, config.APIPrefixes) {
			c.Next()
			return
		}

		if isStaticAsset(path, config.StaticAssets) {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		if _, err := os.Stat(filepath.Join(staticDir, path)); err == nil {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		if _, err := os.Stat(indexFile); err == nil {
			c.File(indexFile)
			return
		}

		c.Status(http.StatusNotFound)
	}
}

func ServeFrontendFS(fsys fs.FS, config FrontendConfig) gin.HandlerFunc {
	if fsys == nil {
		panic("frontend filesystem is nil")
	}

	fileServer := http.FileServer(http.FS(fsys))
	indexFile := config.IndexFile

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if isAPIRequest(path, config.APIPrefixes) {
			c.Next()
			return
		}

		if isStaticAsset(path, config.StaticAssets) {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		f, err := fsys.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		f, err = fsys.Open(indexFile)
		if err == nil {
			f.Close()
			http.ServeFileFS(c.Writer, c.Request, fsys, indexFile)
			return
		}

		c.Status(http.StatusNotFound)
	}
}
