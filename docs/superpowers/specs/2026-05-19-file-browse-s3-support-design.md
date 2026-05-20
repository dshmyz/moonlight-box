# 文件浏览支持多存储后端设计文档

**日期**: 2026-05-19
**版本**: 1.0
**状态**: 待审查

## 概述

当前文件浏览功能只支持本地文件系统，无论系统使用 local 还是 S3 作为存储后端，浏览的都是 `cfg.Storage.Local.BasePath` 目录。需要重构使其支持多存储后端（local/S3/OBS），并允许用户在前端切换后端浏览。

## 目标

1. 文件浏览器能根据存储后端类型自动使用对应的浏览方式（本地文件系统 / S3 API）
2. 前端支持选择不同存储后端进行浏览
3. 下载功能适配所有存储后端类型
4. 保持现有的安全约束（路径不能跳出 basePath）

## 架构设计

### 核心思路：在 Backend 接口新增 Browse 方法

将文件浏览逻辑从 `FileBrowseHandler` 移入 `storage.Backend` 接口。Handler 变为调度层，根据 `backend_id` 参数选择对应后端调用 `Browse()`。

### 数据结构

**BrowseEntry**：文件浏览的统一返回结构

```go
type BrowseEntry struct {
    Name    string `json:"name"`
    Path    string `json:"path"`
    IsDir   bool   `json:"is_dir"`
    Size    int64  `json:"size"`
    ModTime string `json:"mod_time"`
}
```

- LocalStorage：ModTime 来自 `os.Stat` 的 `ModTime`
- S3Storage：文件 ModTime 来自 `LastModified`，目录 ModTime 为 "-"

### Backend 接口扩展

在 `storage.Backend` 接口新增 `Browse` 方法：

```go
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
    Browse(ctx context.Context, path string) ([]BrowseEntry, error)  // 新增
}
```

`List()` 和 `Browse()` 的区别：
- `List()` 用于包管理领域，返回扁平的 `Entry` 列表
- `Browse()` 用于 UI 文件浏览，返回目录树形结构的 `BrowseEntry`，包含 Name、ModTime 等 UI 所需信息

### 各后端实现

**LocalStorage.Browse()**：
- 复用现有 `FileBrowseHandler` 中的 `os.ReadDir` + `os.Stat` 逻辑
- 保留路径安全检查（path traversal 防护）

**S3Storage.Browse()**：
- 使用 `ListObjectsV2` + `Delimiter: "/"` 获取目录结构
- `CommonPrefixes` → 目录条目（IsDir=true, ModTime="-"）
- `Contents` → 文件条目（IsDir=false, ModTime=LastModified 格式化字符串, Size=*obj.Size）
- S3 根目录 path="" 对应 prefix=basePath

**OBS**：使用 S3Storage 实现，无需额外代码

### Handler 重构

`FileBrowseHandler` 从接收 `basePath string` 改为接收 `storageSvc *StorageService`：

```go
type FileBrowseHandler struct {
    storageSvc *service.StorageService
}

func NewFileBrowseHandler(storageSvc *service.StorageService) *FileBrowseHandler {
    return &FileBrowseHandler{storageSvc: storageSvc}
}
```

Handler 方法通过 `backend_id` query 参数选择后端：
- `backend_id=0` 或缺失 → 使用 `GetDefaultBackend()`
- `backend_id=N` → 使用 `GetBackend(N)`

### API 变化

| 接口 | 变化 |
|---|---|
| `GET /files/browse?path=xxx` | 新增 `backend_id` 参数，默认 0 |
| `GET /files/stats?path=xxx` | 新增 `backend_id` 参数 |
| `GET /files/download?path=xxx` | 新增 `backend_id` 参数 |
| `GET /files/backends` | 新增接口，返回可用存储后端列表 |

**`/files/backends` 响应结构**：

```json
{
  "code": 200,
  "data": [
    {
      "id": 0,
      "name": "默认存储",
      "type": "s3",
      "is_default": true
    },
    {
      "id": 1,
      "name": "本地存储",
      "type": "local",
      "is_default": false
    }
  ]
}
```

### 下载实现

统一使用 `Backend.Get(ctx, path)` + `Backend.Size(ctx, path)` 流式下载：
- 对 local 后端，保留 `c.FileAttachment()` 优化路径（性能更好）
- 对 S3/OBS 后端，使用 `Backend.Get()` 获取 `io.ReadCloser`，然后 `io.Copy` 到 `c.Writer`，设置 Content-Disposition/Content-Type/Content-Length

### DirectoryInfo 响应调整

现有 `DirectoryInfo` 结构保留，`IsRoot` 的判断逻辑按后端类型适配：
- Local：`relativePath == "" || relativePath == "/"`
- S3：`relativePath == "" || relativePath == basePath`

## 前端变化

### 存储后端选择器

在 `FileBrowser.vue` header 区域添加下拉选择器：
- 载入时调用 `/files/backends` 获取后端列表
- 默认选中默认后端
- 切换后端时重置 currentPath 到 "/" 并重新加载

### API 层变化

`web/src/api/file.ts` 新增：
```typescript
export interface StorageBackendOption {
  id: number
  name: string
  type: string
  is_default: boolean
}

export const fileApi = {
  // ... 现有方法
  getBackends() {
    return request.get<StorageBackendOption[]>('/files/backends')
  },
}
```

所有 browse/stats/download 方法新增 `backend_id` 参数：
```typescript
browse(path: string = '/', backendId: number = 0) {
  return request.get<BrowseResponse>('/files/browse', {
    params: { path, backend_id: backendId }
  })
}
```

## 文件变更清单

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `internal/storage/backend.go` | 修改 | 新增 BrowseEntry 结构和 Browse 方法 |
| `internal/storage/local_storage.go` | 修改 | 实现 Browse 方法 |
| `internal/storage/s3_storage.go` | 修改 | 实现 Browse 方法 |
| `internal/handler/file_browse_handler.go` | 重构 | 从 basePath 改为注入 StorageService，新增 ListBackends |
| `cmd/registry/main.go` | 修改 | FileBrowseHandler 初始化改为注入 StorageService |
| `cmd/registry/router.go` | 修改 | 新增 /files/backends 路由 |
| `web/src/api/file.ts` | 修改 | 新增 getBackends，所有方法增加 backend_id |
| `web/src/views/FileBrowser.vue` | 修改 | 新增后端选择器，传递 backend_id |

## 测试计划

### 单元测试
- LocalStorage.Browse() 目录列表、路径安全检查
- S3Storage.Browse() 目录列表、S3 前缀处理
- FileBrowseHandler 各 API 端点参数处理

### 集成测试
- local 后端浏览完整流程
- S3 后端浏览完整流程
- 后端切换流程
- 下载功能（local + S3）

### 边界测试
- 无效 backend_id
- 不存在的后端
- 路径越界攻击
- S3 空目录

## 风险和缓解

### 风险1: S3 ListObjectsV2 性能
**缓解**: Browse 方法单层目录扫描（Delimiter="/"），不递归，性能可控

### 风险2: Backend 接口变更影响现有实现
**缓解**: 只有 LocalStorage 和 S3Storage 两个实现，OBS 使用 S3Storage，改动范围可控

### 风险3: S3 下载大文件内存占用
**缓解**: 使用流式传输（io.Copy 到 gin.Writer），不将整个文件加载到内存