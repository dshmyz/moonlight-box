# 虚拟仓库路由重构实现计划

> **面向 AI 代理的工作者：** 使用 superpowers:subagent-driven-development 或 superpowers:executing-plans 逐任务实现。

**目标：** 消除 urlBuilder 设计缺陷，让每个仓库用自己的 RemoteURL 构建远程请求 URL

**核心思想：** urlBuilder 不应该绑定到特定的 repo，而是提供"路径构建"能力，由每个仓库自己拼接 RemoteURL

**技术栈：** Go, Gin, GORM

---

## 问题分析

### urlBuilder 的设计缺陷

当前 urlBuilder 签名：
```go
type URLBuilder func(repo *model.Repository, name, version string) string
```

Adapter 创建时：
```go
urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
    baseURL := strings.TrimSuffix(repo.RemoteURL, "/")  // ← 捕获调用方的 repo
    return fmt.Sprintf("%s/%s/%s", baseURL, groupPath, pkgVersion, filename)
}
```

**问题**：urlBuilder 内部使用了创建时的 repo.RemoteURL。当 VirtualRepository 遍历成员仓库时，传入的仍然是虚拟仓库（RemoteURL=""），导致生成相对路径。

### 根本原因

**urlBuilder 混淆了两个职责**：
1. 包类型特定的路径构建（Adapter 的职责）
2. RemoteURL 拼接（Repository 的职责）

### 正确的职责划分

| 组件 | 职责 |
|------|------|
| **Adapter** | 根据包名/版本/文件，构建远程路径（不含 RemoteURL） |
| **Repository** | 用自己的 RemoteURL + Adapter 提供的路径 = 完整 URL |

---

## 目标架构

### Repository.Resolve 去掉 urlBuilder

```go
// 改前
Resolve(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error)

// 改后
Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error)
```

### ProxyRepository 自己构建 URL

```go
func (r *ProxyRepository) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    // 用 r.repo.RemoteURL 自己构建 URL
    return r.router.resolveProxy(ctx, r.repo, pkgType, name, version)
}
```

### resolveProxy 使用 Adapter 的路径构建

```go
func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
    // 1. 获取 Adapter
    adp, ok := r.adapters[pkgType]
    if !ok {
        return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
    }

    // 2. 用 Adapter 构建远程路径
    remotePath := adp.BuildRemotePath(name, version)

    // 3. 拼接完整 URL
    remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), remotePath)

    // 4. 请求远程
    // ...
}
```

### VirtualRepository 遍历成员

```go
func (v *VirtualRepository) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    for _, member := range v.members {
        // 直接调用，不传 urlBuilder
        result, err := member.Resolve(ctx, pkgType, name, version)
        if err == nil && result != nil {
            return result, nil
        }
    }
    return nil, ErrPackageNotFound
}
```

---

## 时序图

### 修复后

```
Client          RepoRouter      Adapter           ProxyRouter      VirtualRepository    ProxyRepository
  |                |                |                  |                  |                  |
  |--GET /repo/-->|                |                  |                  |                  |
  |                |--adp.Handle-->|                  |                  |                  |
  |                |                |--本地缓存查找     |                  |                  |
  |                |                |  (未命中)         |                  |                  |
  |                |                |--Download-------->|                  |                  |
  |                |                |                  |--ResolveForVirtualRepo              |
  |                |                |                  |--buildComposite-->|                  |
  |                |                |                  |                  |--member.Resolve-->|
  |                |                |                  |                  |                  |--adp=adapters[pkgType]
  |                |                |                  |                  |                  |--BuildRemotePath(name, version)
  |                |                |                  |                  |                  |  → remotePath
  |                |                |                  |                  |                  |--remoteURL = repo.RemoteURL + remotePath
  |                |                |                  |                  |                  |--HTTP GET(remoteURL)
  |                |                |                  |                  |                  |<--内容
  |                |                |                  |                  |<--RouteResult    |
  |                |                |                  |<--RouteResult    |                  |
  |                |                |<--缓存+返回       |                  |                  |
  |                |                |--响应内容         |                  |                  |
  |<--200 OK-------|                |                  |                  |                  |
```

---

## Adapter.BuildRemotePath 设计

### 接口

```go
type Adapter interface {
    // ... 现有方法

    // BuildRemotePath 构建远程路径（不含 RemoteURL）
    BuildRemotePath(name, version string) string
}
```

### MavenAdapter 实现

```go
func (a *MavenAdapter) BuildRemotePath(name, version string) string {
    // name 格式: "group/artifact" (存储格式)
    // 转回: "group/artifact" → "group/artifact"
    // 路径: "group/artifact/version/"
    return fmt.Sprintf("%s/%s", name, version)
}
```

### NpmAdapter 实现

```go
func (a *NpmAdapter) BuildRemotePath(name, version string) string {
    if version != "" {
        return fmt.Sprintf("%s/-/%s-%s.tgz", name, name, version)
    }
    return name
}
```

### GoAdapter 实现

```go
func (a *GoAdapter) BuildRemotePath(name, version string) string {
    return fmt.Sprintf("%s/@v/%s", name, version)
}
```

### PyPIAdapter 实现

```go
func (a *PyPIAdapter) BuildRemotePath(name, version string) string {
    return fmt.Sprintf("packages/%s", name)
}
```

---

## 文件清单

| 文件 | 操作 | 改动说明 |
|------|------|----------|
| `internal/proxy/router.go` | 修改 | Repository 接口去掉 urlBuilder，resolveProxy 改用 BuildRemotePath |
| `internal/proxy/repository_component.go` | 修改 | 所有 Repository.Resolve 去掉 urlBuilder 参数 |
| `internal/types/types.go` | 修改 | Adapter 接口添加 BuildRemotePath |
| `internal/adapter/maven_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/npm_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/go_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/pypi_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/yum_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/apt_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/generic_adapter.go` | 实现 | BuildRemotePath |
| `internal/adapter/base_adapter.go` | 修改 | 移除 GetMetadataFromProxy 的 urlBuilder 参数 |
| `internal/service/proxy_download_service.go` | 简化 | 移除 urlBuilder 相关逻辑 |

---

## 任务 1：修改 Repository 接口

**文件：** `internal/proxy/repository_component.go`

- [ ] **步骤 1：去掉 urlBuilder 参数**

```go
type Repository interface {
    GetID() uint
    GetName() string
    GetType() model.RepositoryType
    GetRepo() *model.Repository
    Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error)  // 去掉 urlBuilder
    AddMember(child Repository) error
    RemoveMember(child Repository) error
    GetMembers() []Repository
}
```

- [ ] **步骤 2：修改 LocalRepository.Resolve**

```go
func (r *LocalRepository) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    return r.router.resolveLocal(ctx, r.repo, pkgType, name, version)
}
```

- [ ] **步骤 3：修改 ProxyRepository.Resolve**

```go
func (r *ProxyRepository) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    return r.router.resolveProxy(ctx, r.repo, pkgType, name, version)
}
```

- [ ] **步骤 4：修改 VirtualRepository.Resolve**

```go
func (v *VirtualRepository) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    for _, member := range v.members {
        if !v.router.isMemberTypeMatch(member.GetRepo(), pkgType) {
            continue
        }

        result, err := member.Resolve(ctx, pkgType, name, version)
        if err == nil && result != nil {
            result.Source = member.GetName()
            result.RepoID = member.GetID()
            return result, nil
        }
    }
    return nil, ErrPackageNotFound
}
```

- [ ] **步骤 5：修改 VirtualRepository.ResolveConcurrent**

```go
func (v *VirtualRepository) ResolveConcurrent(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    var tasks []proxyResolveTask
    for _, member := range v.members {
        if member.GetType() == model.RepoTypeProxy {
            tasks = append(tasks, proxyResolveTask{
                member:  model.RepositoryGroup{MemberRepo: *member.GetRepo()},
                pkgType: pkgType,
                name:    name,
                version: version,
            })
        }
    }
    return v.router.resolveConcurrent(ctx, tasks)
}
```

- [ ] **步骤 6：修改 proxyResolveTask 去掉 urlBuilder**

```go
type proxyResolveTask struct {
    member  model.RepositoryGroup
    pkgType string
    name    string
    version string
}
```

---

## 任务 2：修改 ProxyRouter

**文件：** `internal/proxy/router.go`

- [ ] **步骤 1：修改 resolveProxy 使用 BuildRemotePath**

```go
func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
    cacheKey := fmt.Sprintf("proxy:%s:%s:%s", repo.Name, name, version)

    cached, err := r.cache.Get(ctx, cacheKey)
    if err == nil && cached != nil {
        if cached.IsNegative {
            return nil, ErrPackageNotFound
        }
        return &RouteResult{
            SourceType: "proxy",
            Content:    io.NopCloser(bytes.NewReader(cached.Content)),
            Size:       cached.Size,
            FromCache:  true,
            CacheTTL:   repo.CacheTTLSeconds,
        }, nil
    }

    if r.healthCheckSvc != nil && r.healthCheckSvc.ShouldSkipRequest(repo.ID) {
        return nil, fmt.Errorf("circuit breaker open for repo %s", repo.Name)
    }

    // 获取 Adapter 构建远程路径
    adp, ok := r.adapters[pkgType]
    if !ok {
        return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
    }

    remotePath := adp.BuildRemotePath(name, version)
    remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), remotePath)

    // 请求远程...
    authCfg, err := repo.GetAuthConfig()
    if err != nil {
        return nil, err
    }

    readTimeout := r.calcReadTimeout(repo, -1)
    failureRules, _ := ParseFailureCacheRules(repo.FailureCacheRules)

    opts := RequestOptions{
        ReadTimeout:        readTimeout,
        MaxRedirects:       repo.MaxRedirects,
        InsecureSkipVerify: repo.InsecureSkipVerify,
    }

    content, contentType, err := r.client.GetBytes(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
    if err != nil {
        if r.healthCheckSvc != nil {
            r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordFailure()
        }

        if remoteErr, ok := err.(*RemoteError); ok {
            if failureRules.ShouldCache(remoteErr.StatusCode) {
                ttl := failureRules.Match(remoteErr.StatusCode)
                r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
            } else if remoteErr.IsNotFound() {
                r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
            }
        }
        return nil, err
    }

    if r.healthCheckSvc != nil {
        r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordSuccess()
    }

    size := int64(len(content))
    shouldCache := r.largeFileThreshold == 0 || size <= r.largeFileThreshold

    if shouldCache {
        r.cache.Set(ctx, &CacheItem{
            Key:         cacheKey,
            Content:     content,
            ContentType: contentType,
            Size:        size,
        }, time.Duration(repo.CacheTTLSeconds)*time.Second)
    }

    return &RouteResult{
        SourceType: "proxy",
        Content:    io.NopCloser(bytes.NewReader(content)),
        Size:       size,
        FromCache:  false,
        CacheTTL:   repo.CacheTTLSeconds,
    }, nil
}
```

- [ ] **步骤 2：删除 resolveProxyWithURL 方法**

不再需要，统一用 resolveProxy

- [ ] **步骤 3：修改 resolveConcurrent**

```go
func (r *ProxyRouter) resolveConcurrent(ctx context.Context, tasks []proxyResolveTask) (*RouteResult, error) {
    if len(tasks) == 0 {
        return nil, ErrPackageNotFound
    }

    if len(tasks) == 1 {
        task := tasks[0]
        return r.resolveProxy(ctx, &task.member.MemberRepo, task.pkgType, task.name, task.version)
    }

    // 并发解析逻辑...
}
```

- [ ] **步骤 4：修改 ResolveSmart、ResolveForVirtualRepo 等方法**

去掉 urlBuilder 参数

---

## 任务 3：扩展 Adapter 接口

**文件：** `internal/types/types.go`

- [ ] **步骤 1：添加 BuildRemotePath 方法**

```go
type Adapter interface {
    // ... 现有方法

    // BuildRemotePath 构建远程路径（不含 RemoteURL）
    BuildRemotePath(name, version string) string
}
```

---

## 任务 4：各 Adapter 实现 BuildRemotePath

- [ ] **MavenAdapter**
- [ ] **NpmAdapter**
- [ ] **GoAdapter**
- [ ] **PyPIAdapter**
- [ ] **YumAdapter**
- [ ] **AptAdapter**
- [ ] **GenericAdapter**

---

## 任务 5：清理 Adapter 中的 urlBuilder

**文件：** 各 Adapter

- [ ] **步骤 1：移除 urlBuilder 创建**

```go
// 删除
urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
    baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
    // ...
}
```

- [ ] **步骤 2：修改 ProxyDownloadService 调用**

去掉 URLBuilder 参数

- [ ] **步骤 3：修改 base_adapter.go 的 GetMetadataFromProxy**

去掉 urlBuilder 参数

---

## 任务 6：简化 ProxyDownloadService

**文件：** `internal/service/proxy_download_service.go`

- [ ] **步骤 1：移除 URLBuilder 字段**

```go
type DownloadRequest struct {
    Repo      *model.Repository
    PkgType   string
    Name      string
    Version   string
}
```

- [ ] **步骤 2：简化 Download 方法**

```go
func (s *ProxyDownloadService) Download(ctx context.Context, req *DownloadRequest) (*proxy.RouteResult, error) {
    return req.Repo.Resolve(ctx, req.PkgType, req.Name, req.Version)
}
```

**一行搞定**，利用多态，不需要 switch 判断仓库类型。

---

## 任务 7：运行测试验证

- [ ] **步骤 1：编译验证**

```bash
go build ./...
```

- [ ] **步骤 2：单元测试**

```bash
go test ./...
```

- [ ] **步骤 3：集成测试**

```bash
./scripts/run_all_tests.sh
```

- [ ] **步骤 4：验证脚本**

```bash
./scripts/verify_report.sh
```

- [ ] **步骤 5：手动测试**

```bash
# 虚拟仓库
curl -v http://localhost:8080/repo/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom

# 代理仓库
curl -v http://localhost:8080/repo/maven-proxy/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom

# 本地仓库
curl -v http://localhost:8080/repo/maven-local/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom
```

---

## 风险点检查清单

### 1. resolveProxyWithURL 的调用方

**风险**：可能有其他地方直接调用 resolveProxyWithURL

**检查方法**：
```bash
grep -r "resolveProxyWithURL" --include="*.go"
```

### 2. Adapter 的 BuildRemotePath 实现

**风险**：各 Adapter 的路径构建逻辑可能不正确

**检查方法**：
- 对比现有 urlBuilder 中的路径构建逻辑
- 确保 Maven、NPM、Go、PyPI、Yum、Apt 都正确实现

### 3. 缓存 key 变化

**风险**：修改后缓存 key 格式变化，导致缓存失效

**检查方法**：
- 确认缓存 key 仍然唯一
- 清理旧缓存后重新测试

### 4. 并发解析

**风险**：resolveConcurrent 修改后可能影响并发行为

**检查方法**：
- 确认并发请求仍然正确
- 多个代理成员仓库并发请求时，每个都用自己的 RemoteURL

### 5. 健康检查/熔断器

**风险**：修改后可能影响健康检查逻辑

**检查方法**：
- 确认不健康的代理仓库仍然被跳过
- 熔断器仍然正常工作

### 6. 权限检查

**风险**：下载权限检查可能依赖原有逻辑

**检查方法**：
- 确认虚拟仓库下载仍然进行权限检查

### 7. 审计日志

**风险**：审计日志可能记录错误的仓库信息

**检查方法**：
- 下载后检查审计日志
- 确认记录的是实际提供内容的成员仓库

### 8. 错误处理

**风险**：所有成员仓库都未命中时的错误处理

**检查方法**：
- 请求不存在的包
- 确认返回 404，而不是 500

---

## 验证通过标准

- [ ] 编译通过
- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] 验证脚本无错误
- [ ] 虚拟仓库下载返回 200
- [ ] 代理仓库下载返回 200
- [ ] 本地仓库下载返回 200
- [ ] 缓存命中正常
- [ ] 权限检查正常
- [ ] 审计日志正确
- [ ] 错误处理正确（404 返回）
