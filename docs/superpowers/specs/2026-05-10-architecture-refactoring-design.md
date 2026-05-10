# 架构重构设计：简化 proxy/resolver 和改进依赖注入

## 背景

当前架构存在以下问题：

1. **RepoHandler 职责不清晰**：混合了核心解析逻辑和简单的转发逻辑
2. **依赖注入方式不清晰**：使用 setter 方法和 `c.Get()` 传递依赖
3. **类型安全性差**：`c.Get("repo")` 返回 `interface{}`，需要类型断言

## 目标

1. 简化 proxy/resolver - 移除不必要的转发层
2. 改进依赖注入 - 使用显式参数传递替代上下文传递
3. 明确各层职责边界

## 分层架构

```
Handler 层 (repo_router.go)
├─ 职责：HTTP 请求处理、验证、响应
├─ 仓库验证（存在性、状态）
├─ 权限检查（调用 PermissionService）
├─ 阻断检查（调用 BlockService）
└─ 调用 Service 层

Service 层 (service/)
├─ DownloadService：下载逻辑编排
│  ├─ 解析策略（Local/Proxy/Virtual）
│  ├─ 缓存管理
│  └─ 调用 Adapter
├─ PublishService：发布逻辑编排
│  ├─ 权限验证
│  ├─ 阻断检查
│  └─ 调用 Adapter
├─ BlockService：阻断检查（已存在）
└─ PermissionService：权限检查（已存在）

Adapter 层 (adapter/)
├─ 职责：包类型特定逻辑
├─ URL 解析
├─ 元数据处理
├─ 响应格式化
└─ 调用 Repository 层

Repository 层 (repository/)
└─ 数据持久化
```

## 核心设计

### 1. Context 结构体定义

所有 Context 结构体都不包含 `gin.Context`，保持业务逻辑与 HTTP 框架解耦。

```go
package adapter

// 下载上下文
type DownloadContext struct {
    Repo     *model.Repository
    PkgType  model.PackageType
    Name     string
    Version  string
    Filename string
    UserID   uint
    ClientIP string
}

// 发布上下文
type PublishContext struct {
    Repo     *model.Repository
    PkgType  model.PackageType
    UserID   uint
    ClientIP string
}

// 删除上下文
type DeleteContext struct {
    Repo     *model.Repository
    PkgType  model.PackageType
    Name     string
    Version  string
    UserID   uint
    ClientIP string
}

// 仓库请求上下文（元数据和特殊路径）
type RepoRequestContext struct {
    Repo     *model.Repository
    PkgType  model.PackageType
    Path     string
    UserID   uint
    ClientIP string
}

// 下载结果
type DownloadResult struct {
    Content   io.ReadCloser
    Size      int64
    FromCache bool
    RepoID    uint
    Filename  string
    Name      string
    Version   string
}

// 发布结果
type PublishResult struct {
    PackageName string
    Version     string
    Size        int64
    Filename    string
    Response    interface{}
}
```

### 2. Adapter 接口定义

所有方法统一使用 `(c *gin.Context, ctx *XXXContext)` 的签名模式。

```go
package adapter

import (
    "github.com/gin-gonic/gin"
    "github.com/moonlight-box/registry/internal/model"
)

// Adapter 定义包类型适配器接口
type Adapter interface {
    // 类型标识
    Type() PackageType
    RoutePrefix() string
    
    // 下载相关
    ParsePackagePath(path string) (*PackageIdentity, error)
    HandleDownload(c *gin.Context, ctx *DownloadContext) (*DownloadResult, error)
    FormatDownloadResponse(c *gin.Context, result *DownloadResult)
    
    // 发布相关
    HandlePublish(c *gin.Context, ctx *PublishContext) (*PublishResult, error)
    
    // 删除相关
    HandleDelete(c *gin.Context, ctx *DeleteContext) error
    
    // 元数据和特殊路径请求
    HandleRepoRequest(c *gin.Context, ctx *RepoRequestContext)
    
    // 远程路径构建（用于代理仓库）
    BuildRemotePath(name, version, filename string) string
}
```

### 3. RepoRouter 改进

使用构造函数注入，移除 setter 方法。

```go
package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/moonlight-box/registry/internal/adapter"
    "github.com/moonlight-box/registry/internal/proxy"
    "github.com/moonlight-box/registry/internal/service"
)

type RepoRouter struct {
    // Service 层依赖
    downloadSvc *service.DownloadService
    publishSvc  *service.PublishService
    blockSvc    *service.BlockRuleService
    permSvc     *service.PermissionService
    
    // Adapter 管理
    adapters map[string]adapter.Adapter
    
    // 其他依赖
    repoCache *proxy.RepositoryCache
}

// 构造函数注入 - 移除所有 setter 方法
func NewRepoRouter(
    downloadSvc *service.DownloadService,
    publishSvc *service.PublishService,
    blockSvc *service.BlockRuleService,
    permSvc *service.PermissionService,
    adapters map[string]adapter.Adapter,
    repoCache *proxy.RepositoryCache,
) *RepoRouter {
    return &RepoRouter{
        downloadSvc: downloadSvc,
        publishSvc:  publishSvc,
        blockSvc:    blockSvc,
        permSvc:     permSvc,
        adapters:    adapters,
        repoCache:   repoCache,
    }
}

// HandleRequest 处理下载和元数据请求
func (r *RepoRouter) HandleRequest(c *gin.Context) {
    repoName := c.Param("repoName")
    path := c.Param("path")
    
    // 1. 仓库验证
    repo, err := r.getRepo(repoName)
    if err != nil {
        response.NotFound(c, "仓库不存在")
        return
    }
    
    if !repo.Enabled {
        response.NotFound(c, "仓库已禁用")
        return
    }
    
    // 2. 获取 adapter
    adp, ok := r.adapters[repo.PackageType]
    if !ok {
        response.NotFound(c, "不支持的包类型")
        return
    }
    
    // 3. 尝试解析为下载请求
    pkgIdentity, err := adp.ParsePackagePath(strings.TrimPrefix(path, "/"))
    if err == nil && pkgIdentity != nil {
        // 下载请求处理
        r.handleDownloadRequest(c, repo, adp, pkgIdentity)
        return
    }
    
    // 4. 非下载请求，调用 HandleRepoRequest
    repoReqCtx := &adapter.RepoRequestContext{
        Repo:     repo,
        PkgType:  repo.PackageType,
        Path:     strings.TrimPrefix(path, "/"),
        UserID:   c.GetUint("userID"),
        ClientIP: c.ClientIP(),
    }
    
    adp.HandleRepoRequest(c, repoReqCtx)
}

// handleDownloadRequest 处理下载请求
func (r *RepoRouter) handleDownloadRequest(
    c *gin.Context,
    repo *model.Repository,
    adp adapter.Adapter,
    pkgIdentity *adapter.PackageIdentity,
) {
    // 构建下载上下文
    downloadCtx := &adapter.DownloadContext{
        Repo:     repo,
        PkgType:  repo.PackageType,
        Name:     pkgIdentity.Name,
        Version:  pkgIdentity.Version,
        Filename: pkgIdentity.Filename,
        UserID:   c.GetUint("userID"),
        ClientIP: c.ClientIP(),
    }
    
    // 阻断检查
    if r.checkBlock(c, repo.PackageType, pkgIdentity.Name, pkgIdentity.Version) {
        return
    }
    
    // 权限检查
    if !r.checkPermission(c, repo, "read") {
        return
    }
    
    // 调用 Service 层
    result, err := r.downloadSvc.Download(c.Request.Context(), downloadCtx)
    if err != nil {
        response.NotFound(c, err.Error())
        return
    }
    
    defer result.Content.Close()
    
    // 格式化响应
    adp.FormatDownloadResponse(c, result)
}

// HandlePublish 处理发布请求
func (r *RepoRouter) HandlePublish(c *gin.Context) {
    repoName := c.Param("repoName")
    
    // 1. 仓库验证
    repo, err := r.getRepo(repoName)
    if err != nil {
        response.NotFound(c, "仓库不存在")
        return
    }
    
    if !repo.Enabled {
        response.NotFound(c, "仓库已禁用")
        return
    }
    
    // 2. 仓库类型检查
    switch repo.Type {
    case model.RepoTypeProxy:
        response.Forbidden(c, "代理仓库不支持发布")
        return
    case model.RepoTypeVirtual:
        response.Forbidden(c, "虚拟仓库不支持直接发布")
        return
    case model.RepoTypeLocal:
        break
    default:
        response.BadRequest(c, "未知的仓库类型", "")
        return
    }
    
    // 3. 权限检查
    if !r.checkPermission(c, repo, "write") {
        return
    }
    
    // 4. 构建发布上下文
    publishCtx := &adapter.PublishContext{
        Repo:     repo,
        PkgType:  repo.PackageType,
        UserID:   c.GetUint("userID"),
        ClientIP: c.ClientIP(),
    }
    
    // 5. 调用 Service 层
    result, err := r.publishSvc.Publish(c.Request.Context(), publishCtx)
    if err != nil {
        response.InternalError(c, err.Error())
        return
    }
    
    // 6. 返回响应
    if result.Response != nil {
        response.Success(c, result.Response)
    } else {
        response.Success(c, gin.H{
            "success": true,
            "message": "Package published successfully",
            "package": result.PackageName,
            "version": result.Version,
        })
    }
}
```

### 4. DownloadService 实现

将 RepoHandler 的核心解析逻辑迁移到 DownloadService。

```go
package service

import (
    "context"
    "fmt"
    
    "github.com/moonlight-box/registry/internal/adapter"
    "github.com/moonlight-box/registry/internal/model"
    "github.com/moonlight-box/registry/internal/repository"
)

type DownloadService struct {
    repoRepo    *repository.RepositoryRepository
    groupRepo   *repository.GroupRepository
    storageSvc  *StorageService
    adapters    map[string]adapter.Adapter
}

func NewDownloadService(
    repoRepo *repository.RepositoryRepository,
    groupRepo *repository.GroupRepository,
    storageSvc *StorageService,
    adapters map[string]adapter.Adapter,
) *DownloadService {
    return &DownloadService{
        repoRepo:   repoRepo,
        groupRepo:  groupRepo,
        storageSvc: storageSvc,
        adapters:   adapters,
    }
}

// Download 下载包，根据仓库类型路由到不同的解析策略
func (s *DownloadService) Download(ctx context.Context, downloadCtx *adapter.DownloadContext) (*adapter.DownloadResult, error) {
    switch downloadCtx.Repo.Type {
    case model.RepoTypeLocal:
        return s.downloadFromLocal(ctx, downloadCtx)
    case model.RepoTypeProxy:
        return s.downloadFromProxy(ctx, downloadCtx)
    case model.RepoTypeVirtual:
        return s.downloadFromVirtual(ctx, downloadCtx)
    default:
        return nil, fmt.Errorf("unsupported repository type: %s", downloadCtx.Repo.Type)
    }
}

// downloadFromLocal 从本地仓库下载
func (s *DownloadService) downloadFromLocal(ctx context.Context, downloadCtx *adapter.DownloadContext) (*adapter.DownloadResult, error) {
    adp := s.adapters[string(downloadCtx.PkgType)]
    if adp == nil {
        return nil, fmt.Errorf("unsupported package type: %s", downloadCtx.PkgType)
    }
    
    // 调用 adapter 处理具体逻辑
    return adp.HandleDownload(nil, downloadCtx) // gin.Context 在 adapter 内部不需要
}

// downloadFromProxy 从代理仓库下载
func (s *DownloadService) downloadFromProxy(ctx context.Context, downloadCtx *adapter.DownloadContext) (*adapter.DownloadResult, error) {
    // 代理仓库逻辑：先查缓存，缓存未命中则从远程下载
    // ... 实现细节
    
    adp := s.adapters[string(downloadCtx.PkgType)]
    return adp.HandleDownload(nil, downloadCtx)
}

// downloadFromVirtual 从虚拟仓库下载
func (s *DownloadService) downloadFromVirtual(ctx context.Context, downloadCtx *adapter.DownloadContext) (*adapter.DownloadResult, error) {
    // 获取虚拟仓库成员
    members, err := s.groupRepo.GetMembersByVirtualRepo(downloadCtx.Repo.ID)
    if err != nil {
        return nil, err
    }
    
    // 遍历成员仓库
    for _, member := range members {
        if member.MemberRepo.PackageType != downloadCtx.PkgType {
            continue
        }
        
        // 构建新的上下文
        memberCtx := *downloadCtx
        memberCtx.Repo = &member.MemberRepo
        
        result, err := s.Download(ctx, &memberCtx)
        if err == nil {
            result.RepoID = member.MemberRepo.ID
            return result, nil
        }
    }
    
    return nil, fmt.Errorf("package not found in virtual repository")
}
```

### 5. 移除 RepoHandler

将 RepoHandler 的有价值逻辑迁移到 DownloadService 后，完全移除 RepoHandler。

**迁移内容**：
- `Resolve` → `DownloadService.Download`
- `resolveLocal` → `DownloadService.downloadFromLocal`
- `resolveProxy` → `DownloadService.downloadFromProxy`
- `resolveVirtual` → `DownloadService.downloadFromVirtual`

**移除内容**：
- `HandleRepoRequest` → RepoRouter 直接调用 Adapter
- `HandleRepoPublish` → RepoRouter 直接调用 Adapter
- `HandleRepoDelete` → RepoRouter 直接调用 Adapter
- `FormatDownloadResponse` → RepoRouter 直接调用 Adapter

## 实施步骤

### 阶段一：创建 Context 结构体和接口
1. 在 `internal/adapter/types.go` 中定义所有 Context 结构体
2. 更新 Adapter 接口定义
3. 更新所有 Adapter 实现

### 阶段二：创建 DownloadService
1. 创建 `internal/service/download_service.go`
2. 迁移 RepoHandler 的解析逻辑
3. 编写单元测试

### 阶段三：创建 PublishService
1. 创建 `internal/service/publish_service.go`
2. 迁移发布相关逻辑
3. 编写单元测试

### 阶段四：重构 RepoRouter
1. 更新 RepoRouter 构造函数
2. 移除所有 setter 方法
3. 更新请求处理方法
4. 编写集成测试

### 阶段五：移除 RepoHandler
1. 确保所有逻辑已迁移
2. 删除 `internal/proxy/resolver.go`
3. 更新所有引用

### 阶段六：测试和验证
1. 运行所有单元测试
2. 运行集成测试
3. 手动测试关键功能

## 优点

1. **职责清晰**：每层职责明确，符合单一职责原则
2. **类型安全**：使用显式上下文传递，避免 `c.Get()` 的类型不安全
3. **可测试性强**：所有依赖都是显式的，易于 mock
4. **依赖注入清晰**：构造函数注入，移除 setter 方法
5. **扩展性好**：新增包类型只需实现 Adapter 接口
6. **代码简洁**：移除了不必要的转发层

## 风险和注意事项

1. **改动范围大**：涉及多个模块的重构，需要谨慎测试
2. **向后兼容**：确保 API 接口不变
3. **性能影响**：虽然增加了 Service 层，但影响应该很小
4. **学习曲线**：团队需要理解新的架构

## 测试策略

1. **单元测试**：为每个 Service 和 Adapter 编写单元测试
2. **集成测试**：测试完整的请求处理流程
3. **回归测试**：运行现有的测试脚本确保功能正常
4. **性能测试**：对比重构前后的性能指标
