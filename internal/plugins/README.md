# Plugin 开发指南

本文档描述 Moonlight Box 插件架构的核心概念、接口定义和开发规范。

---

## 目录

1. [架构概览](#架构概览)
2. [核心接口](#核心接口)
3. [数据结构](#数据结构)
4. [请求处理流程](#请求处理流程)
5. [开发规范](#开发规范)
6. [最佳实践](#最佳实践)
7. [测试指南](#测试指南)

---

## 架构概览

### 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                      HTTP Handler                            │
│                   (router.go)                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    ProtocolPlugin                            │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐           │
│  │  Maven  │ │   NPM   │ │  PyPI   │ │   Go    │ ...       │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘           │
│                                                              │
│  职责: 协议语法、路径解析、请求路由、响应渲染               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   RepositoryRuntime                          │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐           │
│  │ HostedRuntime│ │ ProxyRuntime│ │ GroupRuntime│           │
│  └─────────────┘ └─────────────┘ └─────────────┘           │
│                                                              │
│  职责: 仓库行为、缓存策略、回源时机、merge 策略             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Storage Layer                             │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐           │
│  │ MetadataStore│ │  BlobStore  │ │ RemoteClient│           │
│  └─────────────┘ └─────────────┘ └─────────────┘           │
└─────────────────────────────────────────────────────────────┘
```

### 职责边界

| 层 | 职责 | 禁止 |
|----|------|------|
| **ProtocolPlugin** | 协议语法、路径解析、响应渲染 | 直接调用 HTTP 访问上游、判断仓库类型 |
| **RepositoryRuntime** | 仓库行为、缓存策略、回源时机 | 感知协议格式（XML/JSON/HTML） |
| **RemoteClient** | HTTP 请求、响应读取 | 业务逻辑 |

---

## 核心接口

### ProtocolPlugin（必须实现）

```go
type ProtocolPlugin interface {
    Name() string
    Handle(ctx *RequestContext, runtime RepositoryRuntime) error
}
```

- `Name()`: 返回插件名称（如 "maven"、"npm"、"pypi"）
- `Handle()`: 处理 HTTP 请求，返回 error 表示内部错误，nil 表示请求已处理

### RemoteFetcher（可选实现）

```go
type RemoteFetcher interface {
    FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error)
}
```

- 当 ProxyRuntime 需要回源时，会回调此方法
- `remoteURL`: 上游仓库基础 URL
- `path`: 相对路径（由 QueryArtifacts 的 RemotePath 字段传入）
- 返回从远端解析出的 artifacts 列表

### RepositoryRuntime（插件调用）

```go
type RepositoryRuntime interface {
    GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
    QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
    RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
    BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
    DeleteArtifact(ctx context.Context, key ArtifactKey) error
}
```

| 方法 | 用途 |
|------|------|
| `GetArtifact` | 获取单个 artifact（下载文件） |
| `QueryArtifacts` | 查询多个 artifacts（列表、元数据） |
| `RenderProjection` | 渲染投影（目录列表、索引文件） |
| `BeginUpload` | 开始上传会话 |
| `DeleteArtifact` | 删除 artifact |

---

## 数据结构

### ArtifactKey

唯一标识一个 artifact：

```go
type ArtifactKey struct {
    RepositoryID string            // 仓库 ID
    Format       string            // 格式（"maven"、"npm" 等）
    Kind         string            // "file"、"version"、"metadata" 等
    Name         string            // 包名 / artifactId / module path
    Namespace    string            // groupId / scope / organization
    Version      string            // 版本
    Path         string            // 目录路径，不含文件名
    Filename     string            // 文件名
    RemotePath   string            // 协议内相对路径
    Qualifiers   map[string]string // classifier、extension、arch 等身份限定
}
```

**重要**: `RemotePath` 是文件 artifact 的首选身份字段；`Name`/`Version`/`Qualifiers` 用于协议语义查询，必须与存储时保持一致。

### ArtifactQuery

查询条件：

```go
type ArtifactQuery struct {
    RepositoryID string
    Format       string
    Kind         string
    Name         string
    Namespace    string
    Version      string
    RemotePath   string            // 协议内相对路径，触发 RemoteFetcher 回调
    Qualifiers   map[string]string // 精确限定条件
    Limit        int
    Offset       int
}
```

**关键**: `RemotePath` 非空时，ProxyRuntime 会回调 `RemoteFetcher.FetchRemote`。

### Artifact

artifact 数据：

```go
type Artifact struct {
    ID           string
    RepositoryID string
    Format       string
    Kind         string            // "artifact"、"version"、"metadata" 等
    Name         string
    Namespace    string
    Version      string
    Path         string
    Filename     string
    RemotePath   string
    DownloadPath string
    DownloadURL  string            // 上游绝对 URL，仅用于回源
    Qualifiers   map[string]string // 身份限定
    Attributes   map[string]string // license、description、published_at 等语义字段
    Metadata     map[string]any    // 协议原始补充信息
    BlobRefs     []BlobRef         // 关联的 blob 引用
    Content      io.ReadCloser     // 文件内容（GetArtifact 时填充）
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### RequestContext

请求上下文：

```go
type RequestContext struct {
    Writer         http.ResponseWriter
    Request        *http.Request
    Repository     *Repository
    Runtime        RepositoryRuntime
    RepositoryPath string            // 仓库内路径
    RouteStyle     RouteStyle
    Blocker        PackageBlocker
    
    // 请求统计（由 Runtime 设置）
    StatusCode     int
    FromCache      bool
    RemoteURL      string
    SizeBytes      int64
    
    // 协议解析结果（由 Plugin 填充）
    PackageName    string
    Version        string
    Filename       string
}
```

---

## 请求处理流程

### 下载流程

```
Plugin.Handle()
  │
  ├─ 解析路径，识别请求语义
  │
  ├─ 构造 ArtifactQuery（带 RemotePath）
  │
  ▼
runtime.QueryArtifacts(query)
  │
  ├─ 查询本地缓存
  │   └─ 命中 → 返回 artifacts
  │
  ├─ 未命中 → 回调 plugin.FetchRemote()
  │   └─ Plugin 拉取远端、解析、返回 []*Artifact
  │
  ├─ 缓存到 MetadataStore
  │
  ▼
返回 artifacts
  │
  ▼
Plugin 渲染响应（JSON/XML/HTML）
```

### 单文件下载流程

```
Plugin.Handle()
  │
  ├─ 解析路径，识别文件请求
  │
  ├─ 构造 ArtifactKey
  │
  ▼
runtime.GetArtifact(key)
  │
  ├─ 查询 MetadataStore
  │
  ├─ 未命中 → 回调 RemoteClient.FetchBlob()
  │
  ├─ 缓存到 BlobStore
  │
  ▼
返回 artifact（Content 已填充）
  │
  ▼
Plugin 流式写入响应
```

### 上传流程

```
Plugin.Handle()
  │
  ├─ 解析路径，识别上传请求
  │
  ├─ 调用 runtime.BeginUpload()
  │   └─ 返回 UploadSession
  │
  ├─ session.PutBlob() 上传文件内容
  │
  ├─ session.PutArtifact() 存储元数据
  │
  ├─ session.Commit() 提交事务
  │   或 session.Abort() 回滚
  │
  ▼
返回成功响应
```

---

## 开发规范

### 必须遵守

1. **不在 Handle 中直接访问上游**
   ```go
   // ❌ 错误
   resp, _ := http.Get("https://proxy.golang.org/" + path)
   
   // ✅ 正确
   artifacts, _ := runtime.QueryArtifacts(ctx, query) // Runtime 负责回源
   ```

2. **不判断仓库类型**
   ```go
   // ❌ 错误
   if ctx.Repository.Type == "proxy" {
       // ...
   }
   
   // ✅ 正确
   // Runtime 层根据仓库类型自动选择策略
   artifacts, _ := runtime.QueryArtifacts(ctx, query)
   ```

3. **QueryArtifacts 必须带 RemotePath**
   ```go
   // ✅ 正确
   query := ArtifactQuery{
       RepositoryID: ctx.Repository.ID,
       Format:       "maven",
       Kind:         runtime.KindMetadata,
       Name:         artifact,
       Namespace:    group,
       RemotePath:   group + "/" + artifact + "/maven-metadata.xml", // 触发回源
   }
   ```

4. **GetArtifact 返回的 Content 必须检查 nil**
   ```go
   artifact, err := runtime.GetArtifact(ctx, key)
   if err != nil { /* 处理错误 */ }
   if artifact.Content == nil {
       http.Error(ctx.Writer, "Not found", http.StatusNotFound)
       return nil
   }
   defer artifact.Content.Close()
   ```

5. **io.Copy 写入 ResponseWriter 后不返回 error**
   ```go
   // ✅ 正确：响应头已发送，返回 error 会导致上层重复 WriteHeader
   if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
       logrus.WithError(err).Warn("failed to write to client")
       return nil // 不返回 error
   }
   return nil
   ```

### 推荐做法

1. **区分 Qualifiers / Attributes / Metadata**
   ```go
   artifact.Qualifiers = map[string]string{
       "classifier": "sources",
       "extension":  "jar",
   }
   artifact.Attributes = map[string]string{
       "license":      license,
       "description":  description,
       "published_at": publishedAt,
   }
   ```

2. **使用强字段表达关键标识**
   ```go
   Name:       artifact,
   Namespace:  group,
   Version:    version,
   RemotePath: path,
   Filename:   filename,
   Qualifiers: map[string]string{
       "classifier": classifier,
       "extension":  extension,
   },
   ```

3. **填充 RequestContext 的协议字段**
   ```go
   ctx.PackageName = packageName
   ctx.Version = version
   ctx.Filename = filename
   ```

---

## 最佳实践

### 错误处理

```go
artifact, err := runtime.GetArtifact(ctx, key)
if err != nil {
    if errors.Is(err, runtime.ErrNotFound) {
        http.Error(ctx.Writer, "Not found", http.StatusNotFound)
    } else {
        logrus.WithError(err).Error("get artifact failed")
        http.Error(ctx.Writer, "Internal error", http.StatusInternalServerError)
    }
    return nil
}
```

### 并发控制

```go
const maxConcurrent = 3
sem := make(chan struct{}, maxConcurrent)
var wg sync.WaitGroup

for _, item := range items {
    wg.Add(1)
    go func(i Item) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        // 处理 i
    }(item)
}
wg.Wait()
```

### 流式响应

```go
ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
ctx.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
ctx.Writer.WriteHeader(http.StatusOK)

if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
    logrus.WithError(err).Warn("stream to client failed")
}
```

---

## 测试指南

### 使用 MockRuntime

```go
import "github.com/dshmyz/moonlight-box/internal/plugins/testhelper"

func TestMyPlugin(t *testing.T) {
    p := NewMyPlugin()
    
    // 创建 mock artifact
    art := testhelper.NewArtifact("myformat", "mykind", map[string]string{
        "name":    "test-pkg",
        "version": "1.0.0",
    }, "file-content")
    
    rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}
    
    // 创建请求上下文
    ctx, w := testhelper.NewContext("GET", "/test-pkg/1.0.0/file.txt", nil)
    
    // 执行
    if err := p.Handle(ctx, rt); err != nil {
        t.Fatalf("Handle failed: %v", err)
    }
    
    // 验证
    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}
```

### MockRuntime 方法

| 方法 | 说明 |
|------|------|
| `NewArtifact(format, kind, coords, content)` | 创建测试 artifact |
| `NewContext(method, path, body)` | 创建测试请求上下文 |

---

## 附录：现有插件列表

| 插件 | 格式 | 说明 |
|------|------|------|
| Maven | `maven` | Maven 仓库协议 |
| NPM | `npm` | npm Registry 协议 |
| PyPI | `pypi` | PyPI Simple API (PEP 503) |
| Go | `go` | Go Module Proxy 协议 |
| YUM | `yum` | YUM/DNF 仓库协议 |
| APT | `apt` | APT/deb 仓库协议 |
| Raw | `generic` | 通用文件存储 |

---

## 参考文档

- [架构红线](../../docs/new3.md) - 分层边界和请求流程
- [接口定义](../core/runtime/interface.go) - 核心接口定义
- [类型定义](../core/runtime/types.go) - 数据结构定义
