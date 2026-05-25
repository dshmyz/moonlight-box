package http

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"github.com/gin-gonic/gin"
)

type FileBrowseHandler struct {
	storageSvc *service.StorageService
}

func NewFileBrowseHandler(storageSvc *service.StorageService) *FileBrowseHandler {
	return &FileBrowseHandler{storageSvc: storageSvc}
}

type DirectoryInfo struct {
	Path   string                `json:"path"`
	Files  []storage.BrowseEntry `json:"files"`
	Total  int                   `json:"total"`
	IsRoot bool                  `json:"is_root"`
}

func (h *FileBrowseHandler) getBackend(c *gin.Context) (storage.Backend, uint, error) {
	backendIDStr := c.Query("backend_id")
	if backendIDStr == "" || backendIDStr == "0" {
		return h.storageSvc.GetDefaultBackend(), 0, nil
	}

	backendID, err := strconv.ParseUint(backendIDStr, 10, 32)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid backend_id: %s", backendIDStr)
	}

	backend, err := h.storageSvc.GetBackend(uint(backendID))
	if err != nil {
		return nil, 0, err
	}
	return backend, uint(backendID), nil
}

func (h *FileBrowseHandler) ListDirectory(c *gin.Context) {
	backend, _, err := h.getBackend(c)
	if err != nil {
		response.BadRequest(c, "invalid backend", err.Error())
		return
	}

	relativePath := c.Query("path")
	if relativePath == "" {
		relativePath = "/"
	}

	relativePath = strings.TrimPrefix(relativePath, "/")
	relativePath = strings.TrimSuffix(relativePath, "/")

	files, err := backend.Browse(c.Request.Context(), relativePath)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	result := DirectoryInfo{
		Path:   relativePath,
		Files:  files,
		Total:  len(files),
		IsRoot: relativePath == "",
	}

	response.Success(c, result)
}

func (h *FileBrowseHandler) GetFileStats(c *gin.Context) {
	backend, _, err := h.getBackend(c)
	if err != nil {
		response.BadRequest(c, "invalid backend", err.Error())
		return
	}

	relativePath := c.Query("path")
	if relativePath == "" {
		response.BadRequest(c, "missing path", "path parameter is required")
		return
	}

	relativePath = strings.TrimPrefix(relativePath, "/")

	files, err := backend.Browse(c.Request.Context(), filepath.Dir(relativePath))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.NotFound(c, "file not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	fileName := filepath.Base(relativePath)
	for _, f := range files {
		if f.Name == fileName {
			response.Success(c, f)
			return
		}
	}

	response.NotFound(c, "file not found")
}

type BackendOption struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsDefault bool   `json:"is_default"`
}

func (h *FileBrowseHandler) ListBackends(c *gin.Context) {
	backends, _ := h.storageSvc.ListBackends()

	options := make([]BackendOption, 0, len(backends)+1)

	defaultBackend := h.storageSvc.GetDefaultBackend()
	options = append(options, BackendOption{
		ID:        0,
		Name:      "默认存储",
		Type:      defaultBackend.Name(),
		IsDefault: true,
	})

	for _, b := range backends {
		// 排除默认后端（已以ID=0展示）和不可用的后端
		if b.IsDefault || !b.IsActive {
			continue
		}
		options = append(options, BackendOption{
			ID:        b.ID,
			Name:      b.Name,
			Type:      string(b.Type),
			IsDefault: false,
		})
	}

	response.Success(c, options)
}

// localPathResolver 定义本地文件后端的能力接口，
// 用于安全地检测后端是否支持本地文件路径优化下载。
type localPathResolver interface {
	ResolvePathSafe(key string) (string, error)
	BasePath() string
}

func (h *FileBrowseHandler) DownloadFile(c *gin.Context) {
	backend, _, err := h.getBackend(c)
	if err != nil {
		response.BadRequest(c, "invalid backend", err.Error())
		return
	}

	relativePath := c.Query("path")
	if relativePath == "" {
		response.BadRequest(c, "missing path", "path parameter is required")
		return
	}

	relativePath = strings.TrimPrefix(relativePath, "/")

	// 优先使用本地文件路径优化下载（gin.FileAttachment）
	if resolver, ok := backend.(localPathResolver); ok {
		fullPath, err := resolver.ResolvePathSafe(relativePath)
		if err != nil {
			response.BadRequest(c, "invalid path", err.Error())
			return
		}

		c.FileAttachment(fullPath, filepath.Base(fullPath))
		return
	}

	size, err := backend.Size(c.Request.Context(), relativePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
			response.NotFound(c, "file not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	reader, err := backend.Get(c.Request.Context(), relativePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
			response.NotFound(c, "file not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	defer reader.Close()

	fileName := filepath.Base(relativePath)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.Header("Content-Type", "application/octet-stream")
	if size > 0 {
		c.Header("Content-Length", strconv.FormatInt(size, 10))
	}
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}
