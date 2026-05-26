# 删除权限控制 + 审计日志实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为包删除功能添加基于 RBAC 的权限控制和操作审计日志

**架构：** 利用现有的 RBAC 中间件和 AuditService，在路由层添加权限检查，在 HandleDelete 中记录审计日志

**技术栈：** Gin middleware, GORM, existing RBAC/Audit infrastructure

---

## 文件清单

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/database/migration.go` | 修改 | 添加 `package:delete` 和 `package:delete_own` 权限，添加 maintainer 角色 |
| `internal/handler/repo_router.go` | 修改 | HandleDelete 增加 userID 传递和审计日志记录 |
| `internal/adapter/base_adapter.go` | 修改 | 添加 auditSvc 依赖和 TriggerDeleteAudit 方法 |
| `internal/adapter/maven_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/npm_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/pypi_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/go_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/nuget_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/apt_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/yum_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `internal/adapter/generic_adapter.go` | 修改 | HandleRepoDelete 传递 userID 和 request context |
| `cmd/registry/main.go` | 修改 | 为其他包类型删除路由添加权限中间件 |
| `internal/adapter/maven_adapter_test.go` | 修改 | 更新测试适配新签名 |
| `internal/adapter/npm_adapter_test.go` | 修改 | 更新测试适配新签名 |
| `internal/adapter/pypi_adapter_test.go` | 修改 | 更新测试适配新签名 |

---

### 任务 1：添加默认权限和角色

**文件：**
- 修改：`internal/database/migration.go`

- [ ] **步骤 1：在 permissions 切片中添加 package 权限**

在 `SeedData()` 函数的 permissions 切片中（约第 60 行，webhooks 权限之后）添加：

```go
// 包管理（通用）
{Resource: "package", Action: "read"},
{Resource: "package", Action: "write"},
{Resource: "package", Action: "delete"},
{Resource: "package", Action: "delete_own"},
```

- [ ] **步骤 2：添加 maintainer 角色**

在 roles 切片中添加：

```go
{
    Name:         "maintainer",
    Description:  "维护者，可删除包",
    IsSystemRole: true,
},
```

- [ ] **步骤 3：为 maintainer 角色分配 package:delete 权限**

在 admin 角色权限分配之后（约第 120 行），添加：

```go
// 为 maintainer 角色分配 package:delete 权限
var maintainerRole model.Role
if err := DB.Where("name = ?", "maintainer").First(&maintainerRole).Error; err == nil {
    var pkgDeletePerm model.Permission
    if err := DB.Where("resource = ? AND action = ?", "package", "delete").First(&pkgDeletePerm).Error; err == nil {
        rp := model.RolePermission{
            RoleID:       maintainerRole.ID,
            PermissionID: pkgDeletePerm.ID,
        }
        DB.Where(rp).FirstOrCreate(&rp)
    }
}
```

- [ ] **步骤 4：验证编译**

运行：`go build ./cmd/registry/`
预期：编译成功

---

### 任务 2：BaseAdapter 添加审计支持

**文件：**
- 修改：`internal/adapter/base_adapter.go`

- [ ] **步骤 1：添加 auditSvc 字段到 BaseAdapter**

```go
type BaseAdapter struct {
    pkgRepo    *repository.PackageRepository
    storageSvc *service.StorageService
    auditSvc   *service.AuditService
}
```

- [ ] **步骤 2：修改 NewBaseAdapter 构造函数**

```go
func NewBaseAdapter(pkgRepo *repository.PackageRepository, storageSvc *service.StorageService, auditSvc *service.AuditService) *BaseAdapter {
    return &BaseAdapter{
        pkgRepo:    pkgRepo,
        storageSvc: storageSvc,
        auditSvc:   auditSvc,
    }
}
```

- [ ] **步骤 3：添加 LogDeleteAudit 方法**

```go
func (a *BaseAdapter) LogDeleteAudit(c *gin.Context, repoName, pkgName, version string, pkgID *uint) {
    userID := c.GetUint("userID")
    var uid *uint
    if userID > 0 {
        uid = &userID
    }

    details := fmt.Sprintf(`{"repo":"%s","name":"%s","version":"%s"}`, repoName, pkgName, version)

    a.auditSvc.LogWithRequest(
        c.Request.Context(),
        uid,
        model.ActionPackageDelete,
        "package",
        pkgID,
        pkgName,
        details,
        c.ClientIP(),
        c.Request.UserAgent(),
    )
}
```

- [ ] **步骤 4：添加必要的 import**

确保 import 中包含 `"fmt"` 和 `"github.com/dshmyz/moonlight-box/internal/model"`

- [ ] **步骤 5：验证编译**

运行：`go build ./cmd/registry/`
预期：编译失败（因为子 adapter 构造函数还没更新）- 这是预期的

---

### 任务 3：更新所有 Adapter 构造函数

**文件：**
- 修改：`internal/adapter/maven_adapter.go`
- 修改：`internal/adapter/npm_adapter.go`
- 修改：`internal/adapter/pypi_adapter.go`
- 修改：`internal/adapter/go_adapter.go`
- 修改：`internal/adapter/nuget_adapter.go`
- 修改：`internal/adapter/apt_adapter.go`
- 修改：`internal/adapter/yum_adapter.go`
- 修改：`internal/adapter/generic_adapter.go`

每个 adapter 的 `NewBaseAdapter` 调用都需要添加 `auditSvc` 参数。

- [ ] **步骤 1：更新 maven_adapter.go**

在 `NewMavenAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 2：更新 npm_adapter.go**

在 `NewNpmAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 3：更新 pypi_adapter.go**

在 `NewPyPIAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 4：更新 go_adapter.go**

在 `NewGoAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 5：更新 nuget_adapter.go**

在 `NewNuGetAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 6：更新 apt_adapter.go**

在 `NewAptAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 7：更新 yum_adapter.go**

在 `NewYumAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 8：更新 generic_adapter.go**

在 `NewGenericAdapter` 函数中，将：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc),
```
改为：
```go
BaseAdapter: NewBaseAdapter(pkgRepo, storageSvc, auditSvc),
```

- [ ] **步骤 9：验证编译**

运行：`go build ./cmd/registry/`
预期：编译成功

---

### 任务 4：HandleDelete 添加审计日志

**文件：**
- 修改：`internal/handler/repo_router.go`

- [ ] **步骤 1：修改 HandleDelete 签名，接收 auditService**

修改 RepoRouter 结构体：

```go
type RepoRouter struct {
    repoSvc      *service.RepositoryService
    auditSvc     *service.AuditService
    adapters     map[string]adapter.RepoAwareAdapter
    typeDetector *proxy.TypeDetector
}
```

修改构造函数：

```go
func NewRepoRouter(repoSvc *service.RepositoryService, auditSvc *service.AuditService) *RepoRouter {
    return &RepoRouter{
        repoSvc:      repoSvc,
        auditSvc:     auditSvc,
        adapters:     make(map[string]adapter.RepoAwareAdapter),
        typeDetector: proxy.NewTypeDetector(),
    }
}
```

- [ ] **步骤 2：修改 HandleDelete 在删除后记录审计日志**

在 `adp.HandleRepoDelete(c, repo)` 调用之前，我们需要获取包信息。修改 HandleRepoDelete 的调用方式，传递 audit service：

实际上更简洁的方式是：让 HandleRepoDelete 内部调用 BaseAdapter 的审计方法。修改 HandleDelete 为：

```go
func (r *RepoRouter) HandleDelete(c *gin.Context) {
    // ... 现有的仓库类型检查逻辑保持不变 ...

    // 传递 audit service 到 context
    c.Set("auditSvc", r.auditSvc)

    adp.HandleRepoDelete(c, repo)
}
```

- [ ] **步骤 3：验证编译**

运行：`go build ./cmd/registry/`
预期：编译失败（因为 main.go 中 NewRepoRouter 调用还没更新）- 这是预期的

---

### 任务 5：更新各 Adapter 的 HandleRepoDelete 添加审计

**文件：**
- 修改：`internal/adapter/maven_adapter.go`
- 修改：`internal/adapter/npm_adapter.go`
- 修改：`internal/adapter/pypi_adapter.go`
- 修改：`internal/adapter/go_adapter.go`
- 修改：`internal/adapter/nuget_adapter.go`
- 修改：`internal/adapter/apt_adapter.go`
- 修改：`internal/adapter/yum_adapter.go`
- 修改：`internal/adapter/generic_adapter.go`

每个 adapter 的 HandleRepoDelete 方法需要在删除成功后调用审计日志。

- [ ] **步骤 1：更新 maven_adapter.go 的 HandleRepoDelete**

在 `a.Delete(c.Request.Context(), identity)` 成功之后，`a.TriggerWebhook` 之前添加：

```go
// 获取包ID用于审计
pkg, _ := a.pkgRepo.GetByName(identity.Name, model.PackageTypeMaven)
var pkgID *uint
if pkg != nil {
    pkgID = &pkg.ID
}

a.LogDeleteAudit(c, repo.Name, identity.Name, identity.Version, pkgID)
```

需要添加 import `"github.com/dshmyz/moonlight-box/internal/model"`（如果还没有）。

- [ ] **步骤 2：更新 npm_adapter.go 的 HandleRepoDelete**

在删除成功后添加类似的审计调用：

```go
pkg, _ := a.pkgRepo.GetByName(identity.Name, model.PackageTypeNPM)
var pkgID *uint
if pkg != nil {
    pkgID = &pkg.ID
}
a.LogDeleteAudit(c, repo.Name, identity.Name, identity.Version, pkgID)
```

- [ ] **步骤 3：更新 pypi_adapter.go 的 HandleRepoDelete**

同上模式。

- [ ] **步骤 4：更新 go_adapter.go 的 HandleRepoDelete**

同上模式。

- [ ] **步骤 5：更新 nuget_adapter.go 的 HandleRepoDelete**

同上模式。

- [ ] **步骤 6：更新 apt_adapter.go 的 HandleRepoDelete**

同上模式。

- [ ] **步骤 7：更新 yum_adapter.go 的 HandleRepoDelete**

同上模式。

- [ ] **步骤 8：更新 generic_adapter.go 的 HandleRepoDelete**

同上模式。

- [ ] **步骤 9：验证编译**

运行：`go build ./cmd/registry/`
预期：编译成功

---

### 任务 6：更新 main.go 路由配置

**文件：**
- 修改：`cmd/registry/main.go`

- [ ] **步骤 1：更新 NewRepoRouter 调用**

找到 `repoRouter := handler.NewRepoRouter(repoSvc)` 改为：

```go
repoRouter := handler.NewRepoRouter(repoSvc, auditSvc)
```

- [ ] **步骤 2：为 Maven 删除路由添加权限中间件**

找到 Maven 路由组（搜索 `mavenGroup`），在 deleteGroup 添加权限中间件：

```go
mavenDeleteGroup := mavenGroup.Group("")
mavenDeleteGroup.Use(authMw, permMw("maven", "delete"))
{
    mavenDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 3：为 PyPI 删除路由添加权限中间件**

```go
pypiDeleteGroup := pypiGroup.Group("")
pypiDeleteGroup.Use(authMw, permMw("pypi", "delete"))
{
    pypiDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 4：为 Go 删除路由添加权限中间件**

```go
goDeleteGroup := goGroup.Group("")
goDeleteGroup.Use(authMw, permMw("go", "delete"))
{
    goDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 5：为 NuGet 删除路由添加权限中间件**

```go
nugetDeleteGroup := nugetGroup.Group("")
nugetDeleteGroup.Use(authMw, permMw("nuget", "delete"))
{
    nugetDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 6：为 Generic 删除路由添加权限中间件**

```go
genericDeleteGroup := genericGroup.Group("")
genericDeleteGroup.Use(authMw, permMw("generic", "delete"))
{
    genericDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 7：为 Apt 删除路由添加权限中间件（如果有）**

```go
aptDeleteGroup := aptGroup.Group("")
aptDeleteGroup.Use(authMw, permMw("apt", "delete"))
{
    aptDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 8：为 Yum 删除路由添加权限中间件（如果有）**

```go
yumDeleteGroup := yumGroup.Group("")
yumDeleteGroup.Use(authMw, permMw("yum", "delete"))
{
    yumDeleteGroup.DELETE("/*path", repoRouter.HandleDelete)
}
```

- [ ] **步骤 9：验证编译**

运行：`go build ./cmd/registry/`
预期：编译成功

---

### 任务 7：添加各包类型删除权限到 seed 数据

**文件：**
- 修改：`internal/database/migration.go`

- [ ] **步骤 1：添加缺失的包类型删除权限**

在 permissions 切片中添加（如果还没有）：

```go
// PyPI 包管理
{Resource: "pypi", Action: "read"},
{Resource: "pypi", Action: "write"},
{Resource: "pypi", Action: "delete"},

// Go 包管理
{Resource: "go", Action: "read"},
{Resource: "go", Action: "write"},
{Resource: "go", Action: "delete"},

// NuGet 包管理
{Resource: "nuget", Action: "read"},
{Resource: "nuget", Action: "write"},
{Resource: "nuget", Action: "delete"},

// Generic 包管理
{Resource: "generic", Action: "read"},
{Resource: "generic", Action: "write"},
{Resource: "generic", Action: "delete"},
```

- [ ] **步骤 2：验证编译**

运行：`go build ./cmd/registry/`
预期：编译成功

---

### 任务 8：更新测试文件

**文件：**
- 修改：`internal/adapter/maven_adapter_test.go`
- 修改：`internal/adapter/npm_adapter_test.go`
- 修改：`internal/adapter/pypi_adapter_test.go`
- 修改：`internal/adapter/yum_adapter_full_test.go`

- [ ] **步骤 1：更新 maven_adapter_test.go**

在 `setupMavenAdapter` 函数中，`NewBaseAdapter` 调用添加 `auditSvc` 参数：

```go
adapter := NewMavenAdapter(pkgRepo, storageSvc, auditSvc, nil, logRepo, proxyDownloadSvc)
```

（auditSvc 已经在 setup 中创建了，应该不需要额外修改）

- [ ] **步骤 2：更新 npm_adapter_test.go**

同上，确保 `NewBaseAdapter` 调用包含 `auditSvc`。

- [ ] **步骤 3：更新 pypi_adapter_test.go**

同上。

- [ ] **步骤 4：更新 yum_adapter_full_test.go**

同上。

- [ ] **步骤 5：运行测试**

运行：`go test ./internal/adapter/... -count=1`
预期：所有测试通过

---

### 任务 9：运行集成测试验证

- [ ] **步骤 1：构建并启动服务**

```bash
go build -o bin/moonlight-registry ./cmd/registry/
pkill -f "moonlight-registry" 2>/dev/null
sleep 2
nohup ./bin/moonlight-registry -config configs/config.yaml > /tmp/registry.log 2>&1 &
sleep 3
```

- [ ] **步骤 2：运行 HTTP 测试脚本**

```bash
bash scripts/test_basic_http.sh http://localhost:9081
```

预期：所有测试通过（11/11）

- [ ] **步骤 3：验证审计日志**

```bash
sqlite3 data/registry.db "SELECT id, user_id, action, resource_name, details FROM audit_logs WHERE action = 'package_delete' ORDER BY id DESC LIMIT 5;"
```

预期：能看到删除操作的审计记录

---

## 自检

1. **规格覆盖度：** 所有设计需求都有对应任务 ✅
2. **占位符扫描：** 无 TODO/待定 ✅
3. **类型一致性：** 所有 adapter 使用相同的审计调用模式 ✅
4. **权限一致性：** 各包类型权限命名与现有 npm 权限一致 ✅
