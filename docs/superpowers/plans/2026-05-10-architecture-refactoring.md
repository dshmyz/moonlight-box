# 架构重构实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 简化 proxy/resolver 层，移除不必要的转发层，改进依赖注入方式，明确各层职责边界

**架构：** 采用分层架构：Handler 层处理 HTTP 请求，Service 层编排业务逻辑，Adapter 层处理包类型特定逻辑，Repository 层负责数据持久化。使用显式 Context 结构体传递参数，移除 `c.Get()` 方式。

**技术栈：** Go 1.26, Gin Web Framework, GORM

---

## 文件结构

### 新增文件
- `internal/types/context.go` - Context 结构体定义
- `internal/service/download_service.go` - 下载服务
- `internal/service/publish_service.go` - 发布服务

### 修改文件
- `internal/types/types.go` - 更新 Adapter 接口定义
- `internal/adapter/base_adapter.go` - 更新 BaseAdapter 实现
- `internal/adapter/maven_adapter.go` - 更新方法签名
- `internal/adapter/npm_adapter.go` - 更新方法签名
- `internal/adapter/pypi_adapter.go` - 更新方法签名
- `internal/adapter/go_adapter.go` - 更新方法签名
- `internal/adapter/apt_adapter.go` - 更新方法签名
- `internal/adapter/yum_adapter.go` - 更新方法签名
- `internal/adapter/generic_adapter.go` - 更新方法签名
- `internal/handler/repo_router.go` - 重构请求处理逻辑
- `cmd/registry/router.go` - 更新依赖注入方式

### 删除文件
- `internal/proxy/resolver.go` - 移除转发层

---

## 阶段一：创建 Context 结构体和接口

### 任务 1：创建 Context 结构体定义

**文件：**
- 创建：`internal/types/context.go`

- [ ] **步骤 1：创建 context.go 文件**

```go
package types

import (
	"io"
	
	"github.com/moonlight-box/registry/internal/model"
)

// DownloadContext 下载上下文
type DownloadContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	Name     string
	Version  string
	Filename string
	UserID   uint
	ClientIP string
}

// PublishContext 发布上下文
type PublishContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	UserID   uint
	ClientIP string
}

// DeleteContext 删除上下文
type DeleteContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	Name     string
	Version  string
	UserID   uint
	ClientIP string
}

// RepoRequestContext 仓库请求上下文（元数据和特殊路径）
type RepoRequestContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	Path     string
	UserID   uint
	ClientIP string
}

// DownloadResult 下载结果
type DownloadResult struct {
	Content   io.ReadCloser
	Size      int64
	FromCache bool
	RepoID    uint
	Filename  string
	Name      string
	Version   string
}

// PublishResult 发布结果
type PublishResult struct {
	PackageName string
	Version     string
	Size        int64
	Filename    string
	Response    interface{}
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./internal/types`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/types/context.go
git commit -m "feat: add context types for explicit parameter passing"
```

### 任务 2：更新 Adapter 接口定义

**文件：**
- 修改：`internal/types/types.go`

- [ ] **步骤 1：更新 Adapter 接口**

在 `internal/types/types.go` 中更新 Adapter 接口：

```go
package types

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

// RepoAwareAdapter 仓库感知的适配器接口
type RepoAwareAdapter interface {
	Adapter
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./internal/types`
预期：编译错误，因为现有实现未更新

- [ ] **步骤 3：Commit**

```bash
git add internal/types/types.go
git commit -m "refactor: update Adapter interface with explicit context parameters"
```

---

## 阶段二：更新所有 Adapter 实现

### 任务 3：更新 BaseAdapter

**文件：**
- 修改：`internal/adapter/base_adapter.go`

- [ ] **步骤 1：添加辅助方法**

在 `internal/adapter/base_adapter.go` 中添加：

```go
// CheckDownloadPermission 检查下载权限
func (a *BaseAdapter) CheckDownloadPermission(
	c *gin.Context,
	repo *model.Repository,
	pkgType model.PackageType,
	name, version, filename string,
) *DownloadDecision {
	if a.downloadPlugin == nil {
		return AllowDownload()
	}

	userID := c.GetUint("userID")
	downloadCtx := &types.DownloadContext{
		Repo:     repo,
		PkgType:  pkgType,
		Name:     name,
		Version:  version,
		Filename: filename,
		UserID:   userID,
		ClientIP: c.ClientIP(),
	}

	return a.downloadPlugin.Execute(downloadCtx)
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./internal/adapter`
预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/base_adapter.go
git commit -m "refactor: add CheckDownloadPermission helper method"
```

### 任务 4：更新 MavenAdapter

**文件：**
- 修改：`internal/adapter/maven_adapter.go`

- [ ] **步骤 1：更新 HandleRepoRequest 方法签名**

找到 `HandleRepoRequest` 方法，修改签名：

```go
func (a *MavenAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	repo := ctx.Repo
	path := ctx.Path
	
	c.Set("repo", repo)
	
	if strings.HasSuffix(path, "/index") || path == "index" {
		a.handleIndexRequest(c)
		return
	}
	
	if strings.HasSuffix(path, "maven-metadata.xml") {
		a.handleMetadataXML(c, path)
		return
	}
	
	a.handleDownloadArtifact(c, path)
}
```

- [ ] **步骤 2：添加 HandleDownload 方法**

注意：此方法需要从现有的下载逻辑中迁移，主要变更：
1. 从 `c.Get("repo")` 改为使用 `ctx.Repo`
2. 使用 `ctx.Name`, `ctx.Version`, `ctx.Filename` 等显式参数
3. 返回 `*DownloadResult` 而不是直接写入响应

```go
func (a *MavenAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有 handleDownloadArtifact 方法迁移逻辑
	// 使用 ctx.Repo, ctx.Name, ctx.Version, ctx.Filename
	// 返回 DownloadResult{Content, Size, Filename, Name, Version}
	return nil, fmt.Errorf("not implemented")
}
```

- [ ] **步骤 3：添加 HandlePublish 方法**

注意：此方法需要从现有的发布逻辑中迁移，主要变更：
1. 从 `c.Get("repo")` 改为使用 `ctx.Repo`
2. 返回 `*PublishResult` 包含响应数据

```go
func (a *MavenAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有 HandleRepoPublish 方法迁移逻辑
	// 使用 ctx.Repo, ctx.UserID
	// 返回 PublishResult{PackageName, Version, Size, Filename, Response}
	return nil, fmt.Errorf("not implemented")
}
```

- [ ] **步骤 4：添加 HandleDelete 方法**

```go
func (a *MavenAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有 HandleRepoDelete 方法迁移逻辑
	// 使用 ctx.Repo, ctx.Name, ctx.Version
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 5：验证编译**

运行：`go build ./internal/adapter`
预期：编译成功

- [ ] **步骤 6：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "refactor: update MavenAdapter with explicit context parameters"
```

### 任务 5：更新 NpmAdapter

**文件：**
- 修改：`internal/adapter/npm_adapter.go`

- [ ] **步骤 1：更新 HandleRepoRequest 方法签名**

```go
func (a *NpmAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	repo := ctx.Repo
	path := ctx.Path
	
	c.Set("repo", repo)
	a.getPackageForRepo(c, repo, path)
}
```

- [ ] **步骤 2：添加 HandleDownload 方法**

注意：此方法需要从现有的 tarball 下载逻辑中迁移

```go
func (a *NpmAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有 tarball 下载逻辑迁移
	// 使用 ctx.Repo, ctx.Name, ctx.Version, ctx.Filename
	return nil, fmt.Errorf("not implemented")
}
```

- [ ] **步骤 3：添加 HandlePublish 和 HandleDelete 方法**

```go
func (a *NpmAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有 HandleRepoPublish 方法迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *NpmAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有 HandleRepoDelete 方法迁移
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 4：验证编译**

运行：`go build ./internal/adapter`
预期：编译成功

- [ ] **步骤 5：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "refactor: update NpmAdapter with explicit context parameters"
```

### 任务 6：更新 PyPIAdapter

**文件：**
- 修改：`internal/adapter/pypi_adapter.go`

- [ ] **步骤 1：更新 HandleRepoRequest 方法签名**

```go
func (a *PyPIAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	repo := ctx.Repo
	path := ctx.Path
	
	c.Set("repo", repo)
	if strings.HasPrefix(path, "simple/") {
		pkgPath := strings.TrimPrefix(path, "simple/")
		if pkgPath == "" || pkgPath == "/" {
			a.ListPackages(c)
		} else {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: strings.Trim(pkgPath, "/")})
			a.PackageFiles(c)
		}
	} else if strings.HasPrefix(path, "packages/") {
		filename := strings.TrimPrefix(path, "packages/")
		c.Params = append(c.Params, gin.Param{Key: "filename", Value: filename})
		a.DownloadPackage(c)
	} else if strings.Contains(path, "/json") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: parts[0]})
			c.Params = append(c.Params, gin.Param{Key: "version", Value: parts[1]})
			a.JSONAPI(c)
		}
	} else {
		if a.fetcher != nil {
			result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "pypi", path, "")
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					contentType := a.storageSvc.GetContentType(path)
					c.Data(200, contentType, body)
					return
				}
			}
		}
		
		response.NotFound(c, "path not found")
	}
}
```

- [ ] **步骤 2：添加 HandleDownload, HandlePublish, HandleDelete 方法**

```go
func (a *PyPIAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有下载逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *PyPIAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有 HandleRepoPublish 方法迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *PyPIAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有 HandleRepoDelete 方法迁移
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 3：验证编译**

运行：`go build ./internal/adapter`
预期：编译成功

- [ ] **步骤 4：Commit**

```bash
git add internal/adapter/pypi_adapter.go
git commit -m "refactor: update PyPIAdapter with explicit context parameters"
```

### 任务 7：更新其他 Adapter（Go, Apt, Yum, Generic）

**文件：**
- 修改：`internal/adapter/go_adapter.go`
- 修改：`internal/adapter/apt_adapter.go`
- 修改：`internal/adapter/yum_adapter.go`
- 修改：`internal/adapter/generic_adapter.go`

- [ ] **步骤 1：更新 GoAdapter**

在 `internal/adapter/go_adapter.go` 中：

```go
func (a *GoAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	c.Params = append(c.Params, gin.Param{Key: "path", Value: "/" + ctx.Path})
	a.goProxyHandler(c)
}

func (a *GoAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *GoAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *GoAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有逻辑迁移
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 2：更新 AptAdapter**

在 `internal/adapter/apt_adapter.go` 中：

```go
func (a *AptAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	// 从现有逻辑迁移
}

func (a *AptAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *AptAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *AptAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有逻辑迁移
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 3：更新 YumAdapter**

在 `internal/adapter/yum_adapter.go` 中：

```go
func (a *YumAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	// 从现有逻辑迁移
}

func (a *YumAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *YumAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *YumAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有逻辑迁移
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 4：更新 GenericAdapter**

在 `internal/adapter/generic_adapter.go` 中：

```go
func (a *GenericAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	a.DownloadOrBrowse(c)
}

func (a *GenericAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *GenericAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	// 从现有逻辑迁移
	return nil, fmt.Errorf("not implemented")
}

func (a *GenericAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	// 从现有逻辑迁移
	return fmt.Errorf("not implemented")
}
```

- [ ] **步骤 5：验证编译**

运行：`go build ./internal/adapter`
预期：编译成功

- [ ] **步骤 6：Commit**

```bash
git add internal/adapter/go_adapter.go internal/adapter/apt_adapter.go internal/adapter/yum_adapter.go internal/adapter/generic_adapter.go
git commit -m "refactor: update remaining adapters with explicit context parameters"
```

---

## 阶段三：创建 Service 层

### 任务 8：创建 DownloadService

**文件：**
- 创建：`internal/service/download_service.go`

- [ ] **步骤 1：创建 DownloadService**

```go
package service

import (
	"context"
	"fmt"
	
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
)

type DownloadService struct {
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	adapters  map[string]types.Adapter
}

func NewDownloadService(
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	adapters map[string]types.Adapter,
) *DownloadService {
	return &DownloadService{
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		adapters:  adapters,
	}
}

// Download 下载包，根据仓库类型路由到不同的解析策略
func (s *DownloadService) Download(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
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

func (s *DownloadService) downloadFromLocal(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	adp := s.adapters[string(downloadCtx.PkgType)]
	if adp == nil {
		return nil, fmt.Errorf("unsupported package type: %s", downloadCtx.PkgType)
	}
	
	return adp.HandleDownload(nil, downloadCtx)
}

func (s *DownloadService) downloadFromProxy(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	adp := s.adapters[string(downloadCtx.PkgType)]
	if adp == nil {
		return nil, fmt.Errorf("unsupported package type: %s", downloadCtx.PkgType)
	}
	
	return adp.HandleDownload(nil, downloadCtx)
}

func (s *DownloadService) downloadFromVirtual(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	members, err := s.groupRepo.GetMembersByVirtualRepo(downloadCtx.Repo.ID)
	if err != nil {
		return nil, err
	}
	
	for _, member := range members {
		if member.MemberRepo.PackageType != downloadCtx.PkgType {
			continue
		}
		
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

- [ ] **步骤 2：验证编译**

运行：`go build ./internal/service`
预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/service/download_service.go
git commit -m "feat: add DownloadService for download logic orchestration"
```

### 任务 9：创建 PublishService

**文件：**
- 创建：`internal/service/publish_service.go`

- [ ] **步骤 1：创建 PublishService**

```go
package service

import (
	"context"
	"fmt"
	
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
)

type PublishService struct {
	repoRepo  *repository.RepositoryRepository
	adapters  map[string]types.Adapter
}

func NewPublishService(
	repoRepo *repository.RepositoryRepository,
	adapters map[string]types.Adapter,
) *PublishService {
	return &PublishService{
		repoRepo: repoRepo,
		adapters: adapters,
	}
}

// Publish 发布包
func (s *PublishService) Publish(ctx context.Context, publishCtx *types.PublishContext) (*types.PublishResult, error) {
	if publishCtx.Repo.Type != model.RepoTypeLocal {
		return nil, fmt.Errorf("only local repository supports publishing")
	}
	
	adp := s.adapters[string(publishCtx.PkgType)]
	if adp == nil {
		return nil, fmt.Errorf("unsupported package type: %s", publishCtx.PkgType)
	}
	
	return adp.HandlePublish(nil, publishCtx)
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./internal/service`
预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/service/publish_service.go
git commit -m "feat: add PublishService for publish logic orchestration"
```

---

## 阶段四：重构 RepoRouter

### 任务 10：更新 RepoRouter 构造函数

**文件：**
- 修改：`internal/handler/repo_router.go`

- [ ] **步骤 1：更新 RepoRouter 结构体和构造函数**

```go
package handler

import (
	"strings"
	
	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
)

type RepoRouter struct {
	// Service 层依赖
	downloadSvc *service.DownloadService
	publishSvc  *service.PublishService
	blockSvc    *service.BlockRuleService
	permSvc     *service.PermissionCacheService
	
	// Adapter 管理
	adapters map[string]types.Adapter
	
	// 其他依赖
	repoCache       *proxy.RepositoryCache
	downloadPlugin  *types.DownloadPluginChain
	webhookSvc      *service.WebhookService
}

// 构造函数注入 - 移除所有 setter 方法
func NewRepoRouter(
	downloadSvc *service.DownloadService,
	publishSvc *service.PublishService,
	blockSvc *service.BlockRuleService,
	permSvc *service.PermissionCacheService,
	adapters map[string]types.Adapter,
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
```

- [ ] **步骤 2：移除所有 setter 方法**

删除以下方法：
- `SetRepoCache`
- `SetResolver`
- `SetWebhookService`
- `SetPermCache`
- `SetBlockService`

- [ ] **步骤 3：验证编译**

运行：`go build ./internal/handler`
预期：编译错误，因为 cmd/registry/router.go 还在调用 setter 方法

- [ ] **步骤 4：Commit**

```bash
git add internal/handler/repo_router.go
git commit -m "refactor: update RepoRouter constructor with dependency injection"
```

### 任务 11：更新 RepoRouter 请求处理方法

**文件：**
- 修改：`internal/handler/repo_router.go`

- [ ] **步骤 1：更新 HandleRequest 方法**

```go
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
	repoReqCtx := &types.RepoRequestContext{
		Repo:     repo,
		PkgType:  repo.PackageType,
		Path:     strings.TrimPrefix(path, "/"),
		UserID:   c.GetUint("userID"),
		ClientIP: c.ClientIP(),
	}
	
	adp.HandleRepoRequest(c, repoReqCtx)
}

func (r *RepoRouter) handleDownloadRequest(
	c *gin.Context,
	repo *model.Repository,
	adp types.Adapter,
	pkgIdentity *types.PackageIdentity,
) {
	// 构建下载上下文
	downloadCtx := &types.DownloadContext{
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
```

- [ ] **步骤 2：更新 HandlePublish 方法**

```go
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
	publishCtx := &types.PublishContext{
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
	
	// 6. 记录指标
	metrics.RecordUpload(string(repo.PackageType), result.PackageName, result.Version)
	
	// 7. 返回响应
	if result.Response != nil {
		response.Success(c, result.Response)
	} else {
		response.Success(c, &types.PublishResponse{
			Success:  true,
			Message:  "Package published successfully",
			Package:  result.PackageName,
			Version:  result.Version,
			Filename: result.Filename,
			Size:     result.Size,
		})
	}
}
```

- [ ] **步骤 3：添加辅助方法**

```go
func (r *RepoRouter) checkPermission(c *gin.Context, repo *model.Repository, action string) bool {
	if r.permSvc == nil {
		return true
	}
	
	userID := c.GetUint("userID")
	if userID == 0 {
		response.Unauthorized(c, "missing user information")
		return false
	}
	
	permissions, err := r.permSvc.GetUserPermissions(userID)
	if err != nil {
		response.InternalError(c, "failed to load user permissions")
		return false
	}
	
	hasPermission := false
	packageType := strings.ToLower(string(repo.PackageType))
	for _, p := range permissions {
		if p.Resource == packageType && p.Action == action {
			hasPermission = true
			break
		}
		if p.Resource == "system" && p.Action == "admin" {
			hasPermission = true
			break
		}
	}
	
	if !hasPermission {
		response.Forbidden(c, "insufficient permissions for "+packageType+" repository")
		return false
	}
	
	return true
}

func (r *RepoRouter) getRepo(name string) (*model.Repository, error) {
	if r.repoCache != nil {
		return r.repoCache.GetByName(name)
	}
	return nil, fmt.Errorf("repository cache not initialized")
}
```

- [ ] **步骤 4：验证编译**

运行：`go build ./internal/handler`
预期：编译成功

- [ ] **步骤 5：Commit**

```bash
git add internal/handler/repo_router.go
git commit -m "refactor: update RepoRouter request handling methods"
```

---

## 阶段五：更新依赖注入

### 任务 12：更新 cmd/registry/router.go

**文件：**
- 修改：`cmd/registry/router.go`

- [ ] **步骤 1：更新 RouterContext 结构体**

```go
type RouterContext struct {
	Config       *config.Config
	AuthSvc      *service.AuthService
	AuditSvc     *service.AuditService
	PermCache    *service.PermissionCacheService
	BlockRule    *service.BlockRuleService
	RepoSvc      *service.RepositoryService
	RepoCache    *proxy.RepositoryCache
	WebhookSvc   *service.WebhookService
	
	// 新增 Service 层
	DownloadSvc  *service.DownloadService
	PublishSvc   *service.PublishService
	
	Handlers struct {
		// ... 现有字段
	}
	
	Adapters map[string]types.Adapter
}
```

- [ ] **步骤 2：更新 NewRouterContext**

```go
func NewRouterContext(
	cfg *config.Config,
	authSvc *service.AuthService,
	auditSvc *service.AuditService,
	permCache *service.PermissionCacheService,
	blockRule *service.BlockRuleService,
	repoSvc *service.RepositoryService,
	adapters map[string]types.Adapter,
	webhookSvc *service.WebhookService,
	repoCache *proxy.RepositoryCache,
	groupRepo *repository.GroupRepository,
) *RouterContext {
	ctx := &RouterContext{
		Config:     cfg,
		AuthSvc:    authSvc,
		AuditSvc:   auditSvc,
		PermCache:  permCache,
		BlockRule:  blockRule,
		RepoSvc:    repoSvc,
		RepoCache:  repoCache,
		WebhookSvc: webhookSvc,
		Adapters:   adapters,
	}
	
	// 创建 Service 层
	ctx.DownloadSvc = service.NewDownloadService(
		repoRepo,
		groupRepo,
		adapters,
	)
	
	ctx.PublishSvc = service.NewPublishService(
		repoRepo,
		adapters,
	)
	
	ctx.Handlers.Auth = handler.NewAuthHandler(authSvc, auditSvc)
	
	return ctx
}
```

- [ ] **步骤 3：更新 setupRepoRoutes**

```go
func (ctx *RouterContext) setupRepoRoutes(r *gin.Engine, repoCache *proxy.RepositoryCache) {
	// 创建 RepoRouter，使用构造函数注入
	repoRouter := handler.NewRepoRouter(
		ctx.DownloadSvc,
		ctx.PublishSvc,
		ctx.BlockRule,
		ctx.PermCache,
		ctx.Adapters,
		repoCache,
	)
	
	authMw := middleware.Auth(ctx.AuthSvc)
	permMw := ctx.requirePermission
	
	repoGroup := r.Group("/repo/:repoName")
	{
		repoGroup.GET("/*path", repoRouter.HandleRequest)
		
		publishGroup := repoGroup.Group("")
		publishGroup.Use(authMw)
		{
			publishGroup.PUT("/*path", repoRouter.HandlePublish)
		}
		
		deleteGroup := repoGroup.Group("")
		deleteGroup.Use(authMw, permMw("package", "delete"))
		{
			deleteGroup.DELETE("/*path", repoRouter.HandleDelete)
		}
	}
}
```

- [ ] **步骤 4：验证编译**

运行：`go build ./cmd/registry`
预期：编译成功

- [ ] **步骤 5：Commit**

```bash
git add cmd/registry/router.go
git commit -m "refactor: update dependency injection in router setup"
```

---

## 阶段六：移除 RepoHandler

### 任务 13：移除 proxy/resolver.go

**文件：**
- 删除：`internal/proxy/resolver.go`

- [ ] **步骤 1：删除文件**

```bash
git rm internal/proxy/resolver.go
```

- [ ] **步骤 2：验证编译**

运行：`go build ./...`
预期：编译成功，没有引用 resolver.go 的错误

- [ ] **步骤 3：Commit**

```bash
git commit -m "refactor: remove unnecessary resolver layer"
```

---

## 阶段七：测试和验证

### 任务 14：运行单元测试

- [ ] **步骤 1：运行所有测试**

运行：`go test ./... -v`
预期：所有测试通过

- [ ] **步骤 2：运行集成测试**

运行：`bash scripts/run_all_tests.sh http://localhost:9081`
预期：所有关键测试通过

### 任务 15：手动验证

- [ ] **步骤 1：启动服务**

运行：`./cmd/registry/registry`
预期：服务正常启动

- [ ] **步骤 2：测试 Maven 上传下载**

```bash
# 上传
curl -X PUT http://localhost:9081/repo/maven-local/com/test/lib/1.0.0/lib-1.0.0.jar \
  -H "Authorization: Bearer $TOKEN" \
  --data-binary @test.jar

# 下载
curl http://localhost:9081/repo/maven-local/com/test/lib/1.0.0/lib-1.0.0.jar
```

预期：上传和下载成功

- [ ] **步骤 3：测试 NPM 上传下载**

```bash
# 下载包
curl http://localhost:9081/repo/npm-proxy/lodash
```

预期：下载成功

- [ ] **步骤 4：Commit**

```bash
git add .
git commit -m "test: verify architecture refactoring"
```

---

## 最终验证

- [ ] **步骤 1：运行完整测试套件**

运行：`bash scripts/run_all_tests.sh http://localhost:9081`
预期：所有测试通过

- [ ] **步骤 2：检查代码质量**

运行：`go vet ./...`
预期：无警告

- [ ] **步骤 3：最终 Commit**

```bash
git add .
git commit -m "refactor: complete architecture refactoring

- Add explicit context types for parameter passing
- Create DownloadService and PublishService
- Update all adapters with new method signatures
- Refactor RepoRouter to use constructor injection
- Remove unnecessary resolver layer
- Improve type safety and testability"
```
