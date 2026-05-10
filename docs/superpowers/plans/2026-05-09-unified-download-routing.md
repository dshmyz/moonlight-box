# 统一下载路由重构实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将"先查本地存储，未命中走代理下载"的路由逻辑从 7 个 Adapter 中抽出来，统一放到 RepoRouter 层处理，Adapter 只负责包格式解析和响应渲染。

**架构：** RepoRouter.HandleRequest 获取 repo 后，调用统一的 PackageDownloadService 完成"本地查找→代理下载→返回内容流"的完整流程，然后用 Adapter 做 Content-Type/headers 等格式处理。Adapter 不再持有 resolver 和 proxyDownloadSvc 的下载逻辑。

**技术栈：** Go, Gin, GORM

---

## 文件结构

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/service/package_download.go` | 创建 | 统一的包下载服务，封装"本地存储查找→代理下载→审计日志"完整流程 |
| `internal/handler/repo_router.go` | 修改 | HandleRequest 改为调用统一下载服务，不再直接委托给 Adapter.HandleRepoRequest |
| `internal/adapter/adapter.go` | 修改 | RepoAwareAdapter 接口增加 FormatDownloadResponse 方法 |
| `internal/adapter/base_adapter.go` | 修改 | 移除 resolver/proxyDownloadSvc 相关下载逻辑，保留 FormatDownloadResponse |
| `internal/adapter/maven_adapter.go` | 修改 | 移除 handleDownloadArtifact 中的代理下载逻辑，保留 checksum 和格式处理 |
| `internal/adapter/npm_adapter.go` | 修改 | 移除 downloadTarballForRepo/downloadFromProxy 中的代理下载逻辑 |
| `internal/adapter/pypi_adapter.go` | 修改 | 移除 handleDownloadFile 中的代理下载逻辑，保留 PyPI JSON API 特殊处理 |
| `internal/adapter/go_adapter.go` | 修改 | 移除 handleDownloadMod/handleDownloadZip 中的代理下载逻辑 |
| `internal/adapter/yum_adapter.go` | 修改 | 移除 HandleRepoRequest 中的代理下载逻辑 |
| `internal/adapter/apt_adapter.go` | 修改 | 移除 HandleRepoRequest 中的代理下载逻辑 |
| `internal/adapter/generic_adapter.go` | 修改 | 移除 HandleRepoRequest 中的代理下载逻辑 |
| `cmd/registry/main.go` | 修改 | 创建 PackageDownloadService 并注入 |

---

## 任务 1：创建 PackageDownloadService

**文件：**
- 创建：`internal/service/package_download.go`
- 测试：`internal/service/package_download_test.go`

- [ ] **步骤 1：创建 PackageDownloadService 结构体**

```go
package service

import (
	"context"
	"fmt"
	"io"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
)

type PackageDownloadService struct {
	storageSvc   *StorageService
	resolver     *proxy.RepositoryResolver
	proxyDownSvc *ProxyDownloadService
	pkgRepo      *repository.PackageRepository
	logRepo      *repository.ProxyDownloadLogRepository
}

func NewPackageDownloadService(
	storageSvc *StorageService,
	resolver *proxy.RepositoryResolver,
	proxyDownSvc *ProxyDownloadService,
	pkgRepo *repository.PackageRepository,
	logRepo *repository.ProxyDownloadLogRepository,
) *PackageDownloadService {
	return &PackageDownloadService{
		storageSvc:   storageSvc,
		resolver:     resolver,
		proxyDownSvc: proxyDownSvc,
		pkgRepo:      pkgRepo,
		logRepo:      logRepo,
	}
}
```

- [ ] **步骤 2：实现 DownloadPackage 核心方法**

```go
type DownloadResult struct {
	Content    io.ReadCloser
	Size       int64
	Source     string // "local" 或 proxy repo name
	RepoID     uint
	FromCache  bool
}

func (s *PackageDownloadService) DownloadPackage(
	ctx context.Context,
	repo *model.Repository,
	pkgType string,
	name string,
	version string,
	filename string,
) (*DownloadResult, error) {
	// 1. 先查本地存储
	content, size, err := s.storageSvc.GetPackage(ctx, pkgType, name, version)
	if err == nil {
		return &DownloadResult{
			Content: content,
			Size:    size,
			Source:  "local",
		}, nil
	}

	// 2. 本地未命中，走代理下载
	if s.resolver == nil || s.proxyDownSvc == nil {
		return nil, fmt.Errorf("package not found in local storage and proxy download not configured")
	}

	result, downloadErr := s.proxyDownSvc.Download(ctx, &ProxyDownloadRequest{
		PkgType:        pkgType,
		Name:           name,
		Version:        version,
		Filename:       filename,
		Repo:           repo,
		PackageType:    model.PackageType(pkgType),
		RepositoryType: repo.Type,
		FileType:       model.FileTypePrimary,
	})

	if downloadErr != nil {
		return nil, downloadErr
	}

	return &DownloadResult{
		Content:   result.Content,
		Size:      result.Size,
		Source:    result.Source,
		RepoID:    result.RepoID,
		FromCache: result.FromCache,
	}, nil
}
```

- [ ] **步骤 3：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 2：修改 RepoRouter 统一路由

**文件：**
- 修改：`internal/handler/repo_router.go`
- 修改：`internal/adapter/adapter.go`

- [ ] **步骤 1：在 RepoRouter 中添加 PackageDownloadService**

```go
type RepoRouter struct {
	repoSvc        *service.RepositoryService
	repoCache      *proxy.RepositoryCache
	auditSvc       *service.AuditService
	adapters       map[string]adapter.RepoAwareAdapter
	typeDetector   *proxy.TypeDetector
	downloadPlugin *adapter.DownloadPluginChain
	downloadSvc    *service.PackageDownloadService  // 新增
}

func (r *RepoRouter) SetDownloadService(svc *service.PackageDownloadService) {
	r.downloadSvc = svc
}
```

- [ ] **步骤 2：修改 HandleRequest 统一路由逻辑**

```go
func (r *RepoRouter) HandleRequest(c *gin.Context) {
	repoName := c.Param("repoName")
	path := c.Param("path")

	repo, err := r.getRepo(repoName)
	if err != nil {
		response.NotFound(c, "仓库不存在")
		return
	}

	if !repo.Enabled {
		response.NotFound(c, "仓库已禁用")
		return
	}

	var pkgType string

	if repo.Type == model.RepoTypeVirtual {
		trimmedPath := strings.TrimPrefix(path, "/")
		pkgType = r.typeDetector.Detect(trimmedPath)

		if pkgType == "" {
			response.BadRequest(c, "无法从请求路径识别包类型",
				"请确保 URL 包含包类型前缀，如 /npm/ 或 /maven/")
			return
		}

		if pkgType != repo.PackageType {
			response.NotFound(c, fmt.Sprintf("此虚拟仓库不支持 %s 类型的包", pkgType))
			return
		}
	} else {
		pkgType = repo.PackageType
	}

	adp, ok := r.adapters[pkgType]
	if !ok {
		response.NotFound(c, fmt.Sprintf("不支持的包类型: %s", pkgType))
		return
	}

	if r.downloadPlugin != nil {
		c.Set("downloadPlugin", r.downloadPlugin)
	}

	c.Set("repo", repo)

	// 统一下载路由：先查本地，未命中走代理
	if r.downloadSvc != nil {
		pkgInfo, err := adp.ParseDownloadPath(strings.TrimPrefix(path, "/"))
		if err == nil && pkgInfo != nil {
			// 权限检查
			decision := r.CheckDownloadPermission(c, repo, model.PackageType(pkgType), pkgInfo.Name, pkgInfo.Version, pkgInfo.Filename)
			if !decision.Allow {
				c.JSON(decision.Code, gin.H{"error": decision.Message})
				return
			}

			result, downloadErr := r.downloadSvc.DownloadPackage(
				c.Request.Context(),
				repo,
				pkgType,
				pkgInfo.Name,
				pkgInfo.Version,
				pkgInfo.Filename,
			)

			if downloadErr == nil && result != nil {
				defer result.Content.Close()
				adp.FormatDownloadResponse(c, result, pkgInfo)
				return
			}
		}
	}

	// 降级到 Adapter 自己的处理（元数据请求、checksum 等特殊请求）
	adp.HandleRepoRequest(c, repo, strings.TrimPrefix(path, "/"))
}
```

- [ ] **步骤 3：在 RepoAwareAdapter 接口中新增方法**

```go
type DownloadPathInfo struct {
	Name     string
	Version  string
	Filename string
}

type RepoAwareAdapter interface {
	types.Adapter
	HandleRepoRequest(c *gin.Context, repo *model.Repository, path string)
	HandleRepoPublish(c *gin.Context, repo *model.Repository)
	HandleRepoDelete(c *gin.Context, repo *model.Repository)
	SetPackageCache(pkgCache *cache.PackageCache)
	ParseDownloadPath(path string) (*DownloadPathInfo, error)  // 新增
	FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *DownloadPathInfo)  // 新增
}
```

- [ ] **步骤 4：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：编译失败（Adapter 还没实现新方法），这是预期的

---

## 任务 3：实现 Maven Adapter 的新方法

**文件：**
- 修改：`internal/adapter/maven_adapter.go`

- [ ] **步骤 1：实现 ParseDownloadPath**

```go
func (a *MavenAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid maven path")
	}

	version := parts[len(parts)-2]
	filename := parts[len(parts)-1]
	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	pkgName := groupArtifactToName(groupArtifact)

	return &adapter.DownloadPathInfo{
		Name:     pkgName,
		Version:  version,
		Filename: filename,
	}, nil
}
```

- [ ] **步骤 2：实现 FormatDownloadResponse**

```go
func (a *MavenAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := a.storageSvc.GetContentType(info.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 3：简化 handleDownloadArtifact，移除代理下载逻辑**

将 `handleDownloadArtifact` 中 `if a.resolver != nil && a.proxyDownloadSvc != nil` 整个代理下载分支删除，只保留本地存储查找和 checksum 处理。

- [ ] **步骤 4：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS（Maven 部分编译通过）

---

## 任务 4：实现 NPM Adapter 的新方法

**文件：**
- 修改：`internal/adapter/npm_adapter.go`

- [ ] **步骤 1：实现 ParseDownloadPath**

```go
func (a *NpmAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")

	// tarball 路径: @scope/name/-/name-version.tgz 或 name/-/name-version.tgz
	if strings.Contains(path, "/-/") && strings.HasSuffix(path, ".tgz") {
		parts := strings.SplitN(path, "/-/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid npm tarball path")
		}

		pkgName := parts[0]
		filename := parts[1]
		filenameWithoutExt := strings.TrimSuffix(filename, ".tgz")
		version := strings.TrimPrefix(filenameWithoutExt, pkgName+"-")

		if version == filenameWithoutExt {
			if strings.HasPrefix(pkgName, "@") {
				scopeParts := strings.SplitN(pkgName, "/", 2)
				if len(scopeParts) == 2 {
					pkgName = scopeParts[0] + "/" + scopeParts[1]
					version = strings.TrimPrefix(filenameWithoutExt, scopeParts[1]+"-")
				}
			}
		}

		return &adapter.DownloadPathInfo{
			Name:     pkgName,
			Version:  version,
			Filename: filename,
		}, nil
	}

	return nil, fmt.Errorf("not a tarball path")
}
```

- [ ] **步骤 2：实现 FormatDownloadResponse**

```go
func (a *NpmAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := a.storageSvc.GetContentType(info.Filename)
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 3：简化 downloadTarballForRepo，移除代理下载逻辑**

将 `downloadTarballForRepo` 中 `case model.RepoTypeProxy` 和 `case model.RepoTypeVirtual` 分支中调用 `a.proxyDownloadSvc.Download` 的逻辑删除。

- [ ] **步骤 4：简化 downloadFromProxy 和 downloadFromVirtual**

这两个方法可以删除，因为代理下载逻辑已经统一到了 PackageDownloadService。

- [ ] **步骤 5：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 5：实现 PyPI Adapter 的新方法

**文件：**
- 修改：`internal/adapter/pypi_adapter.go`

- [ ] **步骤 1：实现 ParseDownloadPath**

```go
func (a *PyPIAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid pypi path")
	}

	filename := parts[len(parts)-1]
	version := parts[len(parts)-2]
	name := parts[0]

	return &adapter.DownloadPathInfo{
		Name:     name,
		Version:  version,
		Filename: filename,
	}, nil
}
```

- [ ] **步骤 2：实现 FormatDownloadResponse**

```go
func (a *PyPIAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := a.storageSvc.GetContentType(info.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 3：简化 handleDownloadFile，移除代理下载逻辑**

将 `handleDownloadFile` 中调用 `a.proxyDownloadSvc.Download` 的整个分支删除。保留 PyPI JSON API 获取真实 URL 的逻辑（如果需要），但改为在 ParseDownloadPath 或单独的方法中处理。

注意：PyPI 的特殊逻辑（从 pypi.org JSON API 获取文件真实下载 URL）需要在 PackageDownloadService 或 Adapter 中保留。这个逻辑应该在 ParseDownloadPath 之后、调用 DownloadPackage 之前，由 Adapter 提供一个 `PreDownloadHook` 来处理。

实际上，PyPI 的 JSON API 逻辑是为了构建正确的远程 URL。这个应该在 `BuildRemotePath` 中处理，而不是在下载流程中。检查当前 `BuildRemotePath` 实现，如果已经处理了则不需要额外逻辑。

- [ ] **步骤 4：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 6：实现 Go Adapter 的新方法

**文件：**
- 修改：`internal/adapter/go_adapter.go`

- [ ] **步骤 1：实现 ParseDownloadPath**

```go
func (a *GoAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")

	// /@v/list → 版本列表
	// /@v/<version>.mod → go.mod
	// /@v/<version>.zip → zip
	// /@v/<version>.info → info

	if !strings.HasPrefix(path, "@v/") {
		return nil, fmt.Errorf("invalid go path")
	}

	subPath := strings.TrimPrefix(path, "@v/")
	parts := strings.SplitN(subPath, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid go path")
	}

	module := parts[0]
	filePart := parts[1]

	version := strings.TrimSuffix(filePart, ".mod")
	version = strings.TrimSuffix(version, ".zip")
	version = strings.TrimSuffix(version, ".info")

	return &adapter.DownloadPathInfo{
		Name:     module,
		Version:  version,
		Filename: filePart,
	}, nil
}
```

- [ ] **步骤 2：实现 FormatDownloadResponse**

```go
func (a *GoAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := "application/octet-stream"
	if strings.HasSuffix(info.Filename, ".mod") {
		contentType = "text/plain"
	} else if strings.HasSuffix(info.Filename, ".zip") {
		contentType = "application/zip"
	} else if strings.HasSuffix(info.Filename, ".info") {
		contentType = "application/json"
	}
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 3：简化 handleDownloadMod 和 handleDownloadZip，移除代理下载逻辑**

将两个方法中调用 `a.proxyDownloadSvc.Download` 的分支删除。

- [ ] **步骤 4：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 7：实现 Yum/Apt/Generic Adapter 的新方法

**文件：**
- 修改：`internal/adapter/yum_adapter.go`
- 修改：`internal/adapter/apt_adapter.go`
- 修改：`internal/adapter/generic_adapter.go`

- [ ] **步骤 1：Yum Adapter 实现 ParseDownloadPath 和 FormatDownloadResponse**

```go
func (a *YumAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")
	if strings.HasPrefix(path, "repodata/") {
		filePath := strings.TrimPrefix(path, "repodata/")
		return &adapter.DownloadPathInfo{
			Name:     "repodata",
			Version:  "",
			Filename: filepath.Base(filePath),
		}, nil
	}
	if strings.HasPrefix(path, "Packages/") {
		filePath := strings.TrimPrefix(path, "Packages/")
		return &adapter.DownloadPathInfo{
			Name:     "packages",
			Version:  "",
			Filename: filepath.Base(filePath),
		}, nil
	}
	return nil, fmt.Errorf("invalid yum path")
}

func (a *YumAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := "application/octet-stream"
	if strings.HasSuffix(info.Filename, ".xml") {
		contentType = "application/xml"
	} else if strings.HasSuffix(info.Filename, ".gz") {
		contentType = "application/gzip"
	} else if strings.HasSuffix(info.Filename, ".rpm") {
		contentType = "application/x-rpm"
	}
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 2：简化 Yum HandleRepoRequest，移除代理下载逻辑**

- [ ] **步骤 3：Apt Adapter 实现 ParseDownloadPath 和 FormatDownloadResponse**

```go
func (a *AptAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")
	return &adapter.DownloadPathInfo{
		Name:     "apt",
		Version:  "",
		Filename: filepath.Base(path),
	}, nil
}

func (a *AptAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := a.storageSvc.GetContentType(info.Filename)
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 4：Generic Adapter 实现 ParseDownloadPath 和 FormatDownloadResponse**

```go
func (a *GenericAdapter) ParseDownloadPath(path string) (*adapter.DownloadPathInfo, error) {
	path = strings.TrimPrefix(path, "/")
	return &adapter.DownloadPathInfo{
		Name:     "generic",
		Version:  "",
		Filename: filepath.Base(path),
	}, nil
}

func (a *GenericAdapter) FormatDownloadResponse(c *gin.Context, result *service.DownloadResult, info *adapter.DownloadPathInfo) {
	contentType := a.storageSvc.GetContentType(info.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}
```

- [ ] **步骤 5：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 8：更新依赖注入

**文件：**
- 修改：`cmd/registry/main.go`
- 修改：`cmd/registry/router.go`

- [ ] **步骤 1：在 main.go 中创建 PackageDownloadService**

```go
// 创建 PackageDownloadService（统一包下载服务）
pkgDownloadSvc := service.NewPackageDownloadService(storageSvc, repoResolver, proxyDownloadSvc, pkgRepo, proxyDownloadLogRepo)

// 注入到 RepoRouter
repoRouter := handler.NewRepoRouter(repoSvc, auditSvc)
repoRouter.SetRepoCache(repoCache)
repoRouter.SetDownloadService(pkgDownloadSvc)
```

- [ ] **步骤 2：从 Adapter 中移除 resolver 和 proxyDownloadSvc 的注入**

```go
// 删除这些行：
// npmAdapter.SetResolver(repoResolver)
// mavenAdapter.SetResolver(repoResolver)
// ... 所有 adapter 的 SetResolver 调用

// 保留必要的注入（如 logRepo、packageCache 等）
```

- [ ] **步骤 3：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 9：清理 BaseAdapter 和 Adapter 接口

**文件：**
- 修改：`internal/adapter/base_adapter.go`
- 修改：`internal/adapter/adapter.go`

- [ ] **步骤 1：从 BaseAdapter 移除 resolver 和 proxyDownloadSvc 字段**

```go
// 删除：
// resolver       *proxy.RepositoryResolver
// 以及 SetResolver 方法
```

- [ ] **步骤 2：从 BaseAdapter 移除 GetMetadataFromProxy 方法**

这个方法应该移到需要的 Adapter 中，或者通过新的机制处理。

- [ ] **步骤 3：运行编译验证**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

---

## 任务 10：修复测试

**文件：**
- 修改：`internal/adapter/*_test.go`
- 修改：`tests/e2e/*_test.go`

- [ ] **步骤 1：修复 adapter 测试中的 resolver 注入**

所有测试中调用 `adapter.SetResolver(...)` 的地方需要删除或改为注入 PackageDownloadService。

- [ ] **步骤 2：修复 e2e 测试**

更新 e2e 测试中的服务创建逻辑，使用新的 PackageDownloadService。

- [ ] **步骤 3：运行所有测试**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go test ./internal/... -count=1 -timeout 120s`
预期：PASS（已知 maven_adapter_test.go 有预先存在的 storage backend 问题，不在此次修复范围内）

---

## 任务 11：验证完整流程

- [ ] **步骤 1：运行完整编译**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go build ./...`
预期：PASS

- [ ] **步骤 2：运行 proxy 层测试**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go test ./internal/proxy/... -count=1 -timeout 60s`
预期：PASS

- [ ] **步骤 3：运行 service 层测试**

运行：`cd /Users/gracegaoya/work/project/moonlight-box && go test ./internal/service/... -count=1 -timeout 60s`
预期：PASS
