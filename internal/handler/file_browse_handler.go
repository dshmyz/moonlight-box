package handler

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type FileBrowseHandler struct {
	basePath string
}

func NewFileBrowseHandler(basePath string) *FileBrowseHandler {
	return &FileBrowseHandler{
		basePath: filepath.Clean(basePath),
	}
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type DirectoryInfo struct {
	Path   string     `json:"path"`
	Files  []FileInfo `json:"files"`
	Total  int        `json:"total"`
	IsRoot bool       `json:"is_root"`
}

func (h *FileBrowseHandler) ListDirectory(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		relativePath = "/"
	}

	relativePath = strings.TrimPrefix(relativePath, "/")
	relativePath = strings.TrimSuffix(relativePath, "/")

	fullPath := filepath.Join(h.basePath, relativePath)

	if !strings.HasPrefix(fullPath, h.basePath) {
		BadRequest(c, "invalid path", "path is outside base directory")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			NotFound(c, "directory not found")
			return
		}
		InternalError(c, err.Error())
		return
	}

	if !info.IsDir() {
		BadRequest(c, "not a directory", "specified path is not a directory")
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		filePath := filepath.Join(relativePath, entry.Name())
		if relativePath == "" || relativePath == "/" {
			filePath = entry.Name()
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filePath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	result := DirectoryInfo{
		Path:   relativePath,
		Files:  files,
		Total:  len(files),
		IsRoot: relativePath == "" || relativePath == "/",
	}

	Success(c, result)
}

func (h *FileBrowseHandler) GetFileStats(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		BadRequest(c, "missing path", "path parameter is required")
		return
	}

	relativePath = strings.TrimPrefix(relativePath, "/")
	fullPath := filepath.Join(h.basePath, relativePath)

	if !strings.HasPrefix(fullPath, h.basePath) {
		BadRequest(c, "invalid path", "path is outside base directory")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			NotFound(c, "file not found")
			return
		}
		InternalError(c, err.Error())
		return
	}

	result := FileInfo{
		Name:    info.Name(),
		Path:    relativePath,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
	}

	Success(c, result)
}

func (h *FileBrowseHandler) DownloadFile(c *gin.Context) {
	relativePath := c.Query("path")
	if relativePath == "" {
		BadRequest(c, "missing path", "path parameter is required")
		return
	}

	relativePath = strings.TrimPrefix(relativePath, "/")
	fullPath := filepath.Join(h.basePath, relativePath)

	if !strings.HasPrefix(fullPath, h.basePath) {
		BadRequest(c, "invalid path", "path is outside base directory")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			NotFound(c, "file not found")
			return
		}
		InternalError(c, err.Error())
		return
	}

	if info.IsDir() {
		BadRequest(c, "not a file", "specified path is a directory")
		return
	}

	c.FileAttachment(fullPath, info.Name())
}
