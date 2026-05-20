# 文件浏览支持多存储后端 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 重构文件浏览功能使其支持多存储后端（local/S3/OBS），用户可在前端切换后端浏览不同存储的文件。

**架构：** 在 `storage.Backend` 接口新增 `Browse` 方法，各后端（LocalStorage/S3Storage）独立实现浏览逻辑。`FileBrowseHandler` 改为注入 `StorageService`，根据 `backend_id` 参数调度到对应后端。前端新增存储后端选择器。

**技术栈：** Go (gin)、Vue 3 + TypeScript + Element Plus

---

## 文件结构

| 文件 | 职责 | 变更类型 |
|---|---|---|
| `internal/storage/backend.go` | Backend 接口定义、BrowseEntry 结构体 | 修改 |
| `internal/storage/local_storage.go` | LocalStorage.Browse() 实现 | 修改 |
| `internal/storage/s3_storage.go` | S3Storage.Browse() 实现 | 修改 |
| `internal/service/storage_service.go` | 新增 ListBackends 方法 | 修改 |
| `internal/handler/file_browse_handler.go` | 重构为使用 StorageService，新增 ListBackends handler | 重构 |
| `cmd/registry/main.go` | FileBrowseHandler 初始化改为注入 StorageService | 修改 |
| `cmd/registry/router.go` | 新增 /files/backends 路由 | 修改 |
| `web/src/api/file.ts` | 新增 getBackends，所有方法增加 backend_id | 修改 |
| `web/src/views/FileBrowser.vue` | 新增后端选择器，传递 backend_id | 修改 |

---

### 任务 1：扩展 Backend 接口和 BrowseEntry 结构体

**文件：**
- 修改：`internal/storage/backend.go`

- [ ] **步骤 1：在 backend.go 中新增 BrowseEntry 结构体和 Browse 方法**

当前 `backend.go` 内容（完整）：

```go
package storage

import (
	"context"
	"io"
)

type Backend interface {
	Name() string
	Init(basePath string) error
	Put(ctx context.Context, key string, reader io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Size(ctx context.Context, key string) (int64, error)
	List(ctx context.Context, prefix string) ([]Entry, error)
	Close() error
	BasePath() string
}

type Entry struct {
	Key   string
	IsDir bool
	Size  int64
}
```

修改为：

```go
package storage

import (
	"context"
	"io"
)

type Backend interface {
	Name() string
	Init(basePath string) error
	Put(ctx context.Context, key string, reader io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Size(ctx context.Context, key string) (int64, error)
	List(ctx context.Context, prefix string) ([]Entry, error)
	Close() error
	BasePath() string
	Browse(ctx context.Context, path string) ([]BrowseEntry, error)
}

type Entry struct {
	Key   string
	IsDir bool
	Size  int64
}

type BrowseEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}
```

- [ ] **步骤 2：编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./internal/storage/...`
预期：编译失败，因为 LocalStorage 和 S3Storage 还没有实现 Browse 方法

- [ ] **步骤 3：Commit**

```bash
git add internal/storage/backend.go
git commit -m "feat: add BrowseEntry struct and Browse method to Backend interface"
```

---

### 任务 2：实现 LocalStorage.Browse()

**文件：**
- 修改：`internal/storage/local_storage.go`

- [ ] **步骤 1：在 local_storage.go 中新增 Browse 方法**

在 `local_storage.go` 文件末尾（`BasePath()` 方法之后）添加以下方法：

```go
func (s *LocalStorage) Browse(ctx context.Context, path string) ([]BrowseEntry, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	cleanPath = strings.TrimSuffix(cleanPath, "/")

	fullPath, err := s.resolvePathSafe(cleanPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []BrowseEntry{}, nil
		}
		return nil, err
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	result := make([]BrowseEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := filepath.Join(cleanPath, entry.Name())
		if cleanPath == "" || cleanPath == "/" {
			entryPath = entry.Name()
		}

		result = append(result, BrowseEntry{
			Name:    entry.Name(),
			Path:    entryPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return result, nil
}
```

注意：此方法复用了 `FileBrowseHandler.ListDirectory()` 中的逻辑（os.ReadDir + os.Stat），但使用 `resolvePathSafe()` 进行路径安全检查，与 LocalStorage 的其他方法保持一致。

- [ ] **步骤 2：编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./internal/storage/...`
预期：编译通过（LocalStorage 已实现 Browse）

- [ ] **步骤 3：Commit**

```bash
git add internal/storage/local_storage.go
git commit -m "feat: implement LocalStorage.Browse() method"
```

---

### 任务 3：实现 S3Storage.Browse()

**文件：**
- 修改：`internal/storage/s3_storage.go`

- [ ] **步骤 1：在 s3_storage.go 中新增 Browse 方法**

在 `s3_storage.go` 文件的 `BasePath()` 方法之后添加以下方法：

```go
func (s *S3Storage) Browse(ctx context.Context, path string) ([]BrowseEntry, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	cleanPath = strings.TrimSuffix(cleanPath, "/")

	var prefix string
	if cleanPath == "" {
		prefix = s.basePath
	} else {
		if s.basePath != "" {
			prefix = s.basePath + "/" + cleanPath
		} else {
			prefix = cleanPath
		}
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}

	result := make([]BrowseEntry, 0, len(resp.CommonPrefixes)+len(resp.Contents))

	for _, cp := range resp.CommonPrefixes {
		p := *cp.Prefix
		var name string
		if s.basePath != "" {
			name = strings.TrimPrefix(p, s.basePath+"/")
		} else {
			name = p
		}
		name = strings.TrimSuffix(name, "/")

		var entryPath string
		if cleanPath == "" {
			entryPath = name
		} else {
			entryPath = cleanPath + "/" + name
		}

		result = append(result, BrowseEntry{
			Name:    name,
			Path:    entryPath,
			IsDir:   true,
			Size:    0,
			ModTime: "-",
		})
	}

	for _, obj := range resp.Contents {
		key := *obj.Key
		if key == prefix {
			continue
		}

		var name string
		if s.basePath != "" {
			name = strings.TrimPrefix(key, s.basePath+"/")
		} else {
			name = key
		}
		name = strings.TrimPrefix(name, cleanPath+"/")

		var entryPath string
		if cleanPath == "" {
			entryPath = name
		} else {
			entryPath = cleanPath + "/" + name
		}

		modTime := "-"
		if obj.LastModified != nil {
			modTime = obj.LastModified.Format("2006-01-02 15:04:05")
		}

		var size int64
		if obj.Size != nil {
			size = *obj.Size
		}

		result = append(result, BrowseEntry{
			Name:    name,
			Path:    entryPath,
			IsDir:   false,
			Size:    size,
			ModTime: modTime,
		})
	}

	return result, nil
}
```

关键细节：
- S3 使用 `Delimiter: "/"` + `Prefix` 来模拟目录结构
- `CommonPrefixes` 对应目录，`Contents` 对应文件
- 跳过 `Contents` 中 key == prefix 的条目（这是 S3 返回的目录自身的"占位对象"，不是真正的文件）
- 目录的 ModTime 为 "-"，文件的 ModTime 来自 `LastModified`
- `basePath` 为 S3 配置中的存储前缀，需要从 key 中剥离

- [ ] **步骤 2：编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./internal/storage/...`
预期：编译通过（S3Storage 已实现 Browse）

- [ ] **步骤 3：Commit**

```bash
git add internal/storage/s3_storage.go
git commit -m "feat: implement S3Storage.Browse() method"
```

---

### 任务 4：重构 FileBrowseHandler

**文件：**
- 重构：`internal/handler/file_browse_handler.go`

- [ ] **步骤 1：重构 FileBrowseHandler 结构体和方法**

将 `file_browse_handler.go` 的完整内容替换为：

```go
package handler

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
)

type FileBrowseHandler struct {
	storageSvc *service.StorageService
}

func NewFileBrowseHandler(storageSvc *service.StorageService) *FileBrowseHandler {
	return &FileBrowseHandler{storageSvc: storageSvc}
}

type DirectoryInfo struct {
	Path   string              `json:"path"`
	Files  []storage.BrowseEntry `json:"files"`
	Total  int                 `json:"total"`
	IsRoot bool                `json:"is_root"`
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
		IsRoot: relativePath == "" || relativePath == "/",
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
	backends, err := h.storageSvc.ListBackends()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	options := make([]BackendOption, 0, len(backends)+1)

	defaultBackend := h.storageSvc.GetDefaultBackend()
	options = append(options, BackendOption{
		ID:        0,
		Name:      "默认存储",
		Type:      defaultBackend.Name(),
		IsDefault: true,
	})

	for _, b := range backends {
		options = append(options, BackendOption{
			ID:        b.ID,
			Name:      b.Name,
			Type:      string(b.Type),
			IsDefault: b.IsDefault,
		})
	}

	response.Success(c, options)
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

	if backend.Name() == "local" {
		localBackend := backend.(*storage.LocalStorage)
		fullPath, err := localBackend.resolvePathSafe(relativePath)
		if err != nil {
			response.BadRequest(c, "invalid path", err.Error())
			return
		}

		info, err := filepath.Abs(fullPath)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}

		basePath := localBackend.BasePath()
		if !strings.HasPrefix(info, basePath) {
			response.BadRequest(c, "invalid path", "path is outside base directory")
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
```

关键变更：
- `basePath string` → `storageSvc *service.StorageService`
- 新增 `getBackend()` 辅助方法解析 `backend_id` query 参数
- `ListDirectory` 和 `GetFileStats` 使用 `backend.Browse()` 替代本地文件系统操作
- `DownloadFile` 对 local 后端保留 `c.FileAttachment()` 优化，对 S3/OBS 使用 `Backend.Get()` 流式下载
- 新增 `ListBackends` 方法返回可用存储后端列表
- `DirectoryInfo.Files` 类型从 `[]FileInfo`（handler 层结构）改为 `[]storage.BrowseEntry`（storage 层结构）
- 移除了旧的 `FileInfo` 结构体（已被 `BrowseEntry` 替代）

注意：`resolvePathSafe` 是 LocalStorage 的私有方法。为了在 DownloadFile 中使用它，需要在 local_storage.go 中暴露一个公开方法（见任务 2 补充步骤）。

- [ ] **步骤 2：在 LocalStorage 中暴露 ResolvePathSafe 公开方法**

在 `local_storage.go` 的 `Browse()` 方法之后添加：

```go
func (s *LocalStorage) ResolvePathSafe(key string) (string, error) {
	return s.resolvePathSafe(key)
}
```

这样 `FileBrowseHandler.DownloadFile()` 可以调用 `localBackend.ResolvePathSafe(relativePath)` 进行路径安全检查。

- [ ] **步骤 3：编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：编译失败，因为 `main.go` 中 `NewFileBrowseHandler` 的调用方式需要更新（见任务 5），`StorageService.ListBackends` 方法还未实现（见任务 5）

- [ ] **步骤 4：Commit**

```bash
git add internal/handler/file_browse_handler.go internal/storage/local_storage.go
git commit -m "refactor: FileBrowseHandler uses StorageService instead of basePath"
```

---

### 任务 5：新增 StorageService.ListBackends 方法和更新 main.go 初始化

**文件：**
- 修改：`internal/service/storage_service.go`
- 修改：`cmd/registry/main.go`

- [ ] **步骤 1：在 StorageService 中新增 ListBackends 方法**

在 `storage_service.go` 的 `RefreshBackends()` 方法之后添加：

```go
func (s *StorageService) ListBackends() ([]model.StorageBackend, error) {
	if s.storageBackendRepo == nil {
		return nil, fmt.Errorf("storage backend repository not available")
	}
	return s.storageBackendRepo.List()
}
```

此方法复用已有的 `StorageBackendRepository.List()`，返回数据库中所有存储后端配置。

- [ ] **步骤 2：更新 main.go 中 FileBrowseHandler 的初始化**

当前 main.go 第 405 行：
```go
fileBrowseHandler := handler.NewFileBrowseHandler(cfg.Storage.Local.BasePath)
```

修改为：
```go
fileBrowseHandler := handler.NewFileBrowseHandler(storageSvc)
```

其中 `storageSvc` 变量已在 main.go 中更早的位置初始化（当前代码已有 `storageSvc` 变量）。

- [ ] **步骤 3：编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./cmd/registry/...`
预期：编译通过

- [ ] **步骤 4：Commit**

```bash
git add internal/service/storage_service.go cmd/registry/main.go
git commit -m "feat: add StorageService.ListBackends and update FileBrowseHandler init"
```

---

### 任务 6：新增 /files/backends 路由

**文件：**
- 修改：`cmd/registry/router.go`

- [ ] **步骤 1：在 router.go 的 files 路由组中新增 backends 路由**

当前 router.go 第 426-432 行：
```go
files := protected.Group("/files")
files.Use(ctx.requirePermission("system", "admin"))
{
	files.GET("/browse", ctx.Handlers.FileBrowse.ListDirectory)
	files.GET("/stats", ctx.Handlers.FileBrowse.GetFileStats)
	files.GET("/download", ctx.Handlers.FileBrowse.DownloadFile)
}
```

修改为：
```go
files := protected.Group("/files")
files.Use(ctx.requirePermission("system", "admin"))
{
	files.GET("/backends", ctx.Handlers.FileBrowse.ListBackends)
	files.GET("/browse", ctx.Handlers.FileBrowse.ListDirectory)
	files.GET("/stats", ctx.Handlers.FileBrowse.GetFileStats)
	files.GET("/download", ctx.Handlers.FileBrowse.DownloadFile)
}
```

- [ ] **步骤 2：编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./cmd/registry/...`
预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add cmd/registry/router.go
git commit -m "feat: add /files/backends route"
```

---

### 任务 7：更新前端 API 层

**文件：**
- 修改：`web/src/api/file.ts`

- [ ] **步骤 1：更新 file.ts，新增 StorageBackendOption 接口和 getBackends 方法，所有方法增加 backend_id 参数**

当前 `file.ts` 完整内容：

```typescript
import request from './request'
import axios from 'axios'

export interface BrowseResponse {
  files: Array<{
    name: string
    path: string
    is_dir: boolean
    size: number
    mod_time: string
  }>
}

export interface DownloadResponse {
  data: Blob
}

export const fileApi = {
  browse(path: string = '/') {
    return request.get<BrowseResponse>('/files/browse', {
      params: { path }
    })
  },

  stats(path: string) {
    return request.get('/files/stats', {
      params: { path }
    })
  },

  download(path: string) {
    return request.get<DownloadResponse>('/files/download', {
      params: { path },
      responseType: 'blob'
    })
  },

  upload(file: File, path?: string, onProgress?: (percent: number) => void) {
    const formData = new FormData()
    formData.append('file', file)
    if (path) {
      formData.append('path', path)
    }
    const token = localStorage.getItem('token')

    return axios.post('/files/upload', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
        ...(token ? { 'Authorization': `Bearer ${token}` } : {})
      },
      onUploadProgress: (progressEvent) => {
        if (onProgress && progressEvent.total) {
          const percent = Math.round((progressEvent.loaded * 100) / progressEvent.total)
          onProgress(percent)
        }
      }
    })
  }
}
```

修改为：

```typescript
import request from './request'
import axios from 'axios'

export interface BrowseResponse {
  files: Array<{
    name: string
    path: string
    is_dir: boolean
    size: number
    mod_time: string
  }>
}

export interface DownloadResponse {
  data: Blob
}

export interface StorageBackendOption {
  id: number
  name: string
  type: string
  is_default: boolean
}

export const fileApi = {
  getBackends() {
    return request.get<StorageBackendOption[]>('/files/backends')
  },

  browse(path: string = '/', backendId: number = 0) {
    return request.get<BrowseResponse>('/files/browse', {
      params: { path, backend_id: backendId || undefined }
    })
  },

  stats(path: string, backendId: number = 0) {
    return request.get('/files/stats', {
      params: { path, backend_id: backendId || undefined }
    })
  },

  download(path: string, backendId: number = 0) {
    return request.get<DownloadResponse>('/files/download', {
      params: { path, backend_id: backendId || undefined },
      responseType: 'blob'
    })
  },

  upload(file: File, path?: string, onProgress?: (percent: number) => void) {
    const formData = new FormData()
    formData.append('file', file)
    if (path) {
      formData.append('path', path)
    }
    const token = localStorage.getItem('token')

    return axios.post('/files/upload', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
        ...(token ? { 'Authorization': `Bearer ${token}` } : {})
      },
      onUploadProgress: (progressEvent) => {
        if (onProgress && progressEvent.total) {
          const percent = Math.round((progressEvent.loaded * 100) / progressEvent.total)
          onProgress(percent)
        }
      }
    })
  }
}
```

关键变更：
- 新增 `StorageBackendOption` 接口
- 新增 `getBackends()` 方法
- `browse/stats/download` 方法新增 `backendId` 参数，`backend_id: backendId || undefined` 确保为 0（默认）时不发送该参数

- [ ] **步骤 2：验证前端编译**

运行：`cd /Users/gracegaoya/work/project/moonlight-box/web && npx vue-tsc --noEmit`
预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add web/src/api/file.ts
git commit -m "feat: add StorageBackendOption and backend_id param to file API"
```

---

### 任务 8：更新 FileBrowser.vue 前端界面

**文件：**
- 修改：`web/src/views/FileBrowser.vue`

- [ ] **步骤 1：在 FileBrowser.vue 中新增存储后端选择器和 backendId 状态**

当前 `FileBrowser.vue` 的 `<script setup>` 部分（第 105-193 行）需要更新。以下为完整的新 `<script setup>` 内容：

```typescript
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { fileApi, StorageBackendOption } from '@/api/file'

interface FileInfo {
  name: string
  path: string
  is_dir: boolean
  size: number
  mod_time: string
}

const loading = ref(false)
const currentPath = ref('/')
const files = ref<FileInfo[]>([])
const currentBackendId = ref(0)
const backends = ref<StorageBackendOption[]>([])

const pathSegments = computed(() => {
  if (currentPath.value === '/' || currentPath.value === '') {
    return []
  }
  return currentPath.value.split('/').filter(Boolean)
})

const loadBackends = async () => {
  try {
    const data = await fileApi.getBackends()
    backends.value = data || []
    const defaultBackend = backends.value.find(b => b.is_default)
    if (defaultBackend) {
      currentBackendId.value = defaultBackend.id
    }
  } catch {
    ElMessage.error('加载存储后端列表失败')
  }
}

const handleBackendChange = () => {
  currentPath.value = '/'
  loadDirectory('/')
}

const loadDirectory = async (path: string) => {
  loading.value = true
  try {
    const response = await fileApi.browse(path, currentBackendId.value)
    files.value = response.files || []
    currentPath.value = path
  } catch (error: any) {
    ElMessage.error(error.message || '加载目录失败')
  } finally {
    loading.value = false
  }
}

const navigateTo = (path: string) => {
  loadDirectory(path)
}

const navigateToSegment = (index: number) => {
  const segments = pathSegments.value.slice(0, index + 1)
  const path = '/' + segments.join('/')
  navigateTo(path)
}

const refresh = () => {
  loadDirectory(currentPath.value)
}

const handleRowClick = (row: FileInfo) => {
  if (row.is_dir) {
    navigateTo(row.path)
  }
}

const getRowClassName = ({ row }: { row: FileInfo }) => {
  return row.is_dir ? 'directory-row' : 'file-row'
}

const formatSize = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

const downloadFile = async (row: FileInfo) => {
  try {
    const blob = await fileApi.download(row.path, currentBackendId.value) as unknown as Blob
    const url = window.URL.createObjectURL(new Blob([blob]))
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', row.name)
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)
    ElMessage.success('下载成功')
  } catch (error: any) {
    ElMessage.error(error.message || '下载失败')
  }
}

onMounted(async () => {
  await loadBackends()
  loadDirectory('/')
})
```

- [ ] **步骤 2：在 template header 区域新增后端选择器**

当前 header 部分（第 1-17 行）：

```html
<header class="list-header">
  <div class="header-content">
    <div class="header-icon">
      <i class="fa-solid fa-folder-open"></i>
    </div>
    <div class="header-text">
      <h2>文件浏览器</h2>
      <p class="header-subtitle">浏览和管理存储中的文件</p>
    </div>
  </div>
  <el-button class="refresh-btn" @click="refresh">
    <i class="fa-solid fa-refresh"></i>
    <span>刷新</span>
  </el-button>
</header>
```

修改为：

```html
<header class="list-header">
  <div class="header-content">
    <div class="header-icon">
      <i class="fa-solid fa-folder-open"></i>
    </div>
    <div class="header-text">
      <h2>文件浏览器</h2>
      <p class="header-subtitle">浏览和管理存储中的文件</p>
    </div>
  </div>
  <div class="header-actions">
    <el-select
      v-model="currentBackendId"
      class="backend-select"
      @change="handleBackendChange"
    >
      <el-option
        v-for="backend in backends"
        :key="backend.id"
        :label="backend.name"
        :value="backend.id"
      >
        <div class="backend-option">
          <i :class="getBackendIcon(backend.type)" class="backend-option-icon"></i>
          <span>{{ backend.name }}</span>
          <el-tag v-if="backend.is_default" size="small" type="warning" class="backend-default-tag">默认</el-tag>
        </div>
      </el-option>
    </el-select>
    <el-button class="refresh-btn" @click="refresh">
      <i class="fa-solid fa-refresh"></i>
      <span>刷新</span>
    </el-button>
  </div>
</header>
```

同时在 `<script setup>` 中添加 `getBackendIcon` 函数：

```typescript
const getBackendIcon = (type: string) => {
  const icons: Record<string, string> = {
    local: 'fa-solid fa-folder',
    s3: 'fa-solid fa-cloud',
    obs: 'fa-solid fa-cloud',
  }
  return icons[type] || 'fa-solid fa-box'
}
```

- [ ] **步骤 3：添加后端选择器的 CSS 样式**

在 `<style scoped>` 部分末尾添加以下样式：

```css
.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.backend-select {
  width: 180px;
}

.backend-option {
  display: flex;
  align-items: center;
  gap: 8px;
}

.backend-option-icon {
  font-size: 14px;
  color: #6b7280;
}

.backend-default-tag {
  margin-left: 4px;
}
```

- [ ] **步骤 4：验证前端编译**

运行：`cd /Users/gracegaoya/work/project/moonlight-box/web && npx vue-tsc --noEmit`
预期：编译通过

- [ ] **步骤 5：Commit**

```bash
git add web/src/views/FileBrowser.vue
git commit -m "feat: add storage backend selector to FileBrowser"
```

---

### 任务 9：验证完整功能

- [ ] **步骤 1：编译验证后端**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./cmd/registry/...`
预期：编译通过，无错误

- [ ] **步骤 2：编译验证前端**

运行：`cd /Users/gracegaoya/work/project/moonlight-box/web && npm run build`
预期：构建成功，无类型错误

- [ ] **步骤 3：Commit（如有 lint/format 修正）**

```bash
git add -A
git commit -m "chore: lint and format fixes"
```