# 请求处理流程文档

> 本文档是所有请求处理的权威描述，任何代码修改不得破坏此处定义的流程和旁路逻辑。

## 术语定义

| 术语 | 说明 |
|------|------|
| **RepoRouter** | 位于 `internal/handler/repo_router.go`，所有仓库请求的单一入口 |
| **RepoHandler** | 位于 `internal/proxy/resolver.go`，负责下载解析策略分发（Local/Proxy/Virtual） |
| **DownloadService** | 位于 `internal/service/download_service.go`，负责存储读取、代理下载、日志、计数 |
| **ParseIntent** | Adapter 方法，解析请求路径为 `RequestIntent`（含 Type、Name、Version、Filename、PkgPathInfo） |
| **FetchContent** | Adapter 方法，非下载请求（元数据/列表/校验和）的内容获取 |
| **DownloadPluginChain** | 下载前插件链，按优先级顺序执行检查 |
| **RequestType** | 请求意图类型：Download / Metadata / List / Checksum / Unknown |

---

## 1. 全局中间件

所有请求（包括 `/repo/:repoName` 和非仓库路由）都经过以下全局中间件：

```
Request → Recovery → RequestID → CORS → PrometheusMiddleware → Logger → Handler
```

**PrometheusMiddleware** (`internal/middleware/prometheus.go`):

```
c.Next() 后记录:
- metrics.RecordHTTPRequest(method, path, status, duration)
  → moonlight_http_requests_total{method, path, status}
  → moonlight_http_request_duration_seconds{method, path}
```

---

## 2. 路由定义

```
GET    /repo/:repoName/*path   → repoRouter.HandleRequest         # 下载 + 元数据（无认证）
PUT    /repo/:repoName/*path   → authMw → repoRouter.HandlePublish # 发布（需认证）
DELETE /repo/:repoName/*path   → authMw + permMw("package","delete") → repoRouter.HandleDelete # 删除（需认证+权限）
```

---

## 3. 下载流程（GET）

```
请求: GET /repo/:repoName/*path
入口: repoRouter.HandleRequest (repo_router.go:118)
```

### 3.1 Repo 解析

```
getRepo(repoName):
  1. repoCache 中有 → 返回缓存
  2. repoSvc.Get(name) → DB 查询

检查:
  - repo == nil → 404 "仓库不存在"
  - !repo.Enabled → 404 "仓库已禁用"
```

### 3.2 Adapter 查找

```
resolver.GetAdapter(pkgType):
  - 从 adapters map 查找对应类型的 adapter（如 npm/maven/pypi/go/yum/apt/generic）
  - 没找到 → 404 "不支持的包类型"
```

### 3.3 ParseIntent（请求意图解析）

```
intent = adp.ParseIntent(requestPath, c.Request.Method):
  - 解析路径，确定请求类型 (Download/Metadata/List/Checksum/Unknown)
  - 提取 Name、Version、Filename、PkgPathInfo
  - intent.Type == Unknown → 404 "无法识别的请求路径"
```

### 3.4 阻断检查（Block Check）

```
执行条件: blockSvc != nil && intent.Name != ""

流程:
  result = blockSvc.IsBlocked(pkgType, intent.Name, intent.Version):
    1. 检查缓存是否过期（TTL = 1 分钟）
    2. 过期则 refreshCache():
       - 从 DB 加载所有启用的精确匹配规则 exactRules
       - 从 DB 加载所有启用的通配符规则 wildcardRules（编译 regex）
       - 构建 exactRulesCache: key = "pkgType:pkgName:version"
       - 构建 wildcardRulesCache: key = pkgType
    3. 先查 exactRulesCache[key]
    4. 再查 wildcardRulesCache[pkgType]，逐个匹配 regex

  如果 result.Blocked:
    1. blockSvc.LogBlock() → auditSvc.LogWithRequest() → 异步写入审计日志
    2. response.Forbidden(c, "包 xxx@yyy 已被阻断: reason") → 403

  未阻断 → 继续
```

### 3.5 下载权限检查（Download Permission Check）

```
执行条件: intent.Type == RequestDownload

流程:
  decision = r.CheckDownloadPermission(c, repo, pkgType, name, version, filename):
    1. downloadPlugin == nil → return AllowDownload()
    2. 构建 DownloadContext（含 UserID、ClientIP 等）
    3. downloadPlugin.Execute(ctx):
       - 按 Priority 排序的插件链
       - 依次执行每个插件的 BeforeDownload()
       - 任一插件返回 BlockDownload(code, message) → 立即返回阻断决策
       - 全部通过 → AllowDownload()

  如果 !decision.Allow:
    c.JSON(decision.Code, {error: decision.Message})

  允许 → 继续
```

### 3.6 内容获取

#### 3.6.1 下载请求 (intent.Type == RequestDownload)

```
构建 DownloadContext:
  Repo, PkgType, Name, Version, Filename
  UserID = c.GetUint("userID")
  ClientIP = c.ClientIP()
  ResolvedPath = intent.PkgPathInfo

routeResult = resolver.Resolve(ctx, downloadCtx):
  根据 downloadCtx.Repo.Type 分发:
```

##### Local 仓库

```
resolveLocal():
  → downloadSvc.Download(ctx, downloadCtx)
```

##### Proxy 仓库

```
resolveProxy():
  → downloadSvc.Download(ctx, downloadCtx)
```

##### Virtual 仓库

```
resolveVirtual():
  1. getMembers(virtualRepoID):
     - repoCache → GetMembers() 或
     - groupRepo.GetMembersByVirtualRepo()
  2. 过滤匹配包类型的成员仓库
  3. 无匹配成员 → ErrPackageNotFound
  4. 并发探测所有匹配成员:
     for each member:
       go func() {
         memberCtx = *downloadCtx, memberCtx.Repo = &member.MemberRepo
         res, err = r.Resolve(ctx, &memberCtx)  // 递归，走 Local/Proxy 分支
         resultCh <- memberResult{res, err, sourceName, repoID}
       }()
  5. 第一个成功结果 → cancel() → 返回
  6. 全部失败:
     - 有非 ErrPackageNotFound 错误 → 返回第一个非 package-not-found 错误
     - 全部 ErrPackageNotFound → 返回 ErrPackageNotFound
```

#### DownloadService.Download 详解

```
downloadSvc.Download(ctx, downloadCtx):
  pathInfo = downloadCtx.ResolvedPath
  startTime = time.Now()

  第一步: 尝试本地存储
  content, size, err = storageSvc.GetPackage(ctx, pkgType, pathInfo.StorageName, pathInfo.StorageVersion)
  if err == nil:
    // 存储命中
    recordLog(downloadCtx, size, duration, nil)        // 旁路逻辑: 下载日志
    incrementDownloadCount(downloadCtx)                  // 旁路逻辑: 下载计数
    return DownloadResult{Content: content, Size: size, FromCache: true, ...}

  第二步: 处理未命中
  if fetcher == nil OR repo.Type == RepoTypeLocal:
    // 没有代理能力 或 本地仓库（禁止回源）
    recordLog(downloadCtx, 0, duration, error)           // 旁路逻辑: 失败的下载日志
    return error "package not found"

  第三步: 从代理下载
  return downloadFromProxy(ctx, downloadCtx, pathInfo, startTime)
```

#### downloadFromProxy 详解

```
downloadFromProxy(ctx, downloadCtx, pathInfo, startTime):

  1. 构造远程 URL
     remoteURL = fmt.Sprintf("%s/%s", Repo.RemoteURL, pathInfo.RemotePath)

  2. fetcher.FetchFromRemote(ctx, repo, remoteURL) (ProxyDownloader.FetchFromRemote):
     a. 检查代理缓存:
        cacheKey = "proxy:{repoName}:{remoteURL}"
        cached, err = cache.Get(ctx, cacheKey)
        if cached != nil:
          if cached.IsNegative → ErrPackageNotFound
          else → return RouteResult{Content: cached.Content, FromCache: true}

     b. 检查熔断器:
        if healthCheckSvc.ShouldSkipRequest(repo.ID):
          return error "circuit breaker open"

     c. 配置认证:
        authCfg = repo.GetAuthConfig() (Basic/Bearer/API Key)

     d. 配置超时:
        readTimeout = calcReadTimeout(repo, -1)  // 基于 repo 配置或全局默认

     e. 执行 HTTP GET:
        resp, err = client.GetStream(ctx, remoteURL, opts, authCfg)
        if err:
          // 错误处理
          if healthCheckSvc: RecordFailure()           // 旁路逻辑: 熔断器失败计数
          if RemoteError:
            if failureRules.ShouldCache(statusCode):
              cache.SetNegative(ctx, cacheKey, ttl)     // 旁路逻辑: 失败缓存
            elif IsNotFound:
              cache.SetNegative(ctx, cacheKey, negativeTTL)
          return error

        if healthCheckSvc: RecordSuccess()              // 旁路逻辑: 熔断器成功恢复

     f. 大文件处理:
        if isLargeFile:
          return RouteResult{Content: resp.Body, IsLarge: true}  // 流式返回

     g. 小文件处理:
        body = io.ReadAll(resp.Body)
        cache.Set(ctx, cacheItem, ttl)                  // 旁路逻辑: 代理缓存写入
        return RouteResult{Content: body, FromCache: false}

  3. 存储到本地
     storageVersion = pathInfo.StorageVersion
     backendID = repo.StorageBackendID 或 0

     大文件分支:
       storageKey = storageSvc.StorePackageWithBackend(..., result.Content, result.Size, backendID)
       // 流式直接写入存储
       result.Content.Close()
       if storageKey != "":
         storePackageFileRecord(ctx, downloadCtx, result.RepoID, storageKey, result.Size)
       storedContent, storedSize = storageSvc.GetPackage(...)  // 重新读取
       recordLog(...)
       return DownloadResult{FromCache: true, ...}

     小文件分支:
       contentBytes = io.ReadAll(result.Content)
       size = len(contentBytes)
       storageKey = storageSvc.StorePackageWithBackend(..., contentBytes, size, backendID)
       if storeErr:
         logrus.Warn("failed to store proxy package")
         recordLog(downloadCtx, size, duration, nil)  // 存储失败但下载成功
         // 不阻塞继续返回
       else if storageKey != "":
         storePackageFileRecord(...)

       recordLog(...)
       return DownloadResult{FromCache: true, Content: contentBytes, ...}
```

#### 3.6.2 非下载请求（intent.Type == Metadata/List/Checksum）

```
contentResult, err = adp.FetchContent(ctx, repo, intent):
  - 各 adapter 自行实现，通过 pkgCache 读取 DB 中的版本信息/文件列表等
  - 返回 ContentResult（JSON 数据或文件流）
  - 错误 → 404
```

### 3.7 响应格式化

```
formatContentResponse(c, result):
  if result == nil → 404

  写入 Headers:
    for key, value = range result.Headers:
      c.Header(key, value)

  如果 result.Content != nil:
    c.DataFromReader(result.StatusCode, result.Size, result.ContentType, result.Content, nil)
    return

  如果 result.ExtraData != nil:
    c.JSON(result.StatusCode, result.ExtraData)
    return

  否则:
    c.Status(result.StatusCode)

下载请求的固定响应:
  ContentType = "application/octet-stream"
  Header["Content-Disposition"] = 'attachment; filename="{intent.Filename}"'
```

### 3.8 审计日志

```
执行条件: intent.Type == RequestDownload && auditSvc != nil

auditSvc.LogWithStatus(ctx, nil, ActionPackageDownload, pkgType, nil, Name, Version, 0, 0):
  → 构建 AuditLog → 发送到 logChan (buffer=1000)
  → worker 协程:
      - 积累到 100 条 或 每 1 秒 → flushBatch()
      - Batch INSERT 到 audit_logs 表
      - logChan 满则 drop（不阻塞）
```

### 3.9 下载旁路逻辑汇总

| 旁路 | 触发位置 | 机制 | 数据目标 |
|------|----------|------|----------|
| **阻断检查** | HandleRequest (3.4) | BlockRuleService.IsBlocked → 1min 缓存 → DB | 阻断后写审计日志 |
| **阻断审计日志** | HandleRequest (3.4) checkBlock() | BlockRuleService.LogBlock → AuditService | audit_logs 表 |
| **下载权限插件** | HandleRequest (3.5) | DownloadPluginChain.Execute → 按优先级 | 直接阻断 HTTP 响应 |
| **代理缓存** | FetchFromRemote (3.6) | CacheService.Get/Set → 内存 | 内存缓存 |
| **熔断器** | FetchFromRemote (3.6) | HealthCheckService → CircuitBreaker | 内存状态 |
| **失败缓存** | FetchFromRemote (3.6) | CacheService.SetNegative | 内存缓存 |
| **远程请求认证** | FetchFromRemote (3.6) | ProxyAuthConfig (Basic/Bearer/API Key) | HTTP Header |
| **大文件流式处理** | FetchFromRemote (3.6) | IsLarge 标记 → 不缓存直接存储 | Storage |
| **下载日志** | DownloadService.recordLog() | LogBatcher → 100条/5s → BatchCreate | proxy_download_logs 表 |
| **下载计数** | DownloadService.incrementDownloadCount() / storePackageFileRecord() | DownloadCountBatcher → 10s → SQL UPDATE | 4 张表 |
| **Prometheus 下载指标** | DownloadService.recordLog() | metrics.RecordDownload() | Prometheus |
| **审计日志** | HandleRequest (3.8) | AuditService.LogWithStatus → 100条/1s → DB | audit_logs 表 |

---

## 4. 上传流程（PUT）

```
请求: PUT /repo/:repoName/*path
入口: repoRouter.HandlePublish (repo_router.go:245)
中间件: authMw (JWT 认证)
```

### 4.1 Repo 解析

```
getRepo(repoName) → repo 检查:
  - 不存在 → 404
  - 已禁用 → 404
```

### 4.2 仓库类型检查

```
switch repo.Type:
  case Proxy:    → 403 "代理仓库不支持发布"
  case Virtual:  → 403 "虚拟仓库不支持直接发布"
  case Local:    → OK（继续）
  default:       → 400 "未知的仓库类型"
```

### 4.3 权限检查

```
执行条件: permCache != nil

1. userID = c.GetUint("userID")
   if userID == 0 → 401 "missing user information"

2. permissions = permCache.GetUserPermissions(userID)
   if err → 500 "failed to load user permissions"

3. 逐个检查 permissions:
   - resource = strings.ToLower(string(repo.PackageType)) + action = "write" → 有权限
   - resource = "system" + action = "admin" → 有权限
   - 否则 → 403 "insufficient permissions"

权限缓存: PermissionCacheService, TTL = 5 分钟
```

### 4.4 Adapter 发布处理

```
publishResult = resolver.HandleRepoPublish(c, repo):
  → adp.HandlePublish(c, ctx)  // 各 adapter 处理上传请求体

返回 PublishResult:
  PackageName, Version, Filename, Content (io.Reader), Size, StorageVersion, FileType, Metadata, Response
```

### 4.5 UploadService 上传

```
uploadCtx = UploadContext{
  PkgType, Name, Version, StorageVersion, Filename, Content, Size,
  PackageType, RepositoryType, RepositoryID, UploadedBy, Metadata, FileType
}

uploadResult = uploadSvc.Upload(ctx, uploadCtx):

  1. checksumReader = NewChecksumReader(content)     // 旁路: 同时计算 SHA256 + MD5
  2. storageKey = storageSvc.StorePackage(...)         // 旁路: 存储到文件系统/后端
     if err → return error

  3. checksum = checksumReader.GetResult()
     if checksum == nil → return error

  4. pkg, ver, _, err = pkgRepo.StorePackageFile(..., model.Package{...}, model.PackageVersion{...}, model.PackageFile{...})
     // DB 事务: 创建 Package → PackageVersion → PackageFile
     if err:
       storageSvc.DeletePackage(...)                   // 旁路: 存储回滚
       return error

  5. return UploadResult{PackageID, VersionID, StorageKey, Size, Checksums}
```

### 4.6 审计日志

```
auditSvc.LogWithStatus(ctx, nil, ActionPackageUpload, packageType, &packageID, packageName, "", 0, 0):
  → 异步写入 audit_logs 表（同下载审计机制）
```

### 4.7 Metrics

```
metrics.RecordUpload(packageType, packageName, version):
  → moonlight_uploads_total{package_type, package_name, version}++
```

### 4.8 响应 + Webhook

```
构建 RepoOperationResult{PackageName, Version, Size, Filename, Response}

if result != nil:
  metrics.RecordUpload(packageType, packageName, version)

  if result.Response != nil:
    response.Success(c, result.Response)
  else:
    response.Success(c, PublishResponse{Success: true, ...})

  if webhookSvc != nil:
    webhookSvc.TriggerEvent(WebhookEventPackageUploaded, payload):
      → 从 DB 加载所有配置了该事件的 webhook
      → 对每个 webhook 异步发送 HTTP POST:
          Headers: Content-Type, X-Webhook-Event, X-Webhook-Timestamp
          Body: {event, timestamp, package_name, version, repository, data}
          Signature: 如果 webhook.Secret 不为空，计算 HMAC-SHA256
      → 记录 Delivery 结果到 webhook_deliveries 表
      → 连续失败 5 次 → 自动禁用 webhook
```

### 4.9 上传旁路逻辑汇总

| 旁路 | 触发位置 | 机制 | 数据目标 |
|------|----------|------|----------|
| **JWT 认证** | authMw（路由中间件） | Token 解析 → userID | Gin Context |
| **仓库类型限制** | HandlePublish (4.2) | 仅 Local 仓库允许 | HTTP 响应 |
| **RBAC 权限** | HandlePublish (4.3) | PermissionCache → 5min 缓存 | DB → 缓存 |
| **Checksum 计算** | UploadService.Upload | ChecksumReader → SHA256 + MD5 | DB（PackageFile） |
| **存储** | UploadService.Upload | StorePackage | 文件系统/存储后端 |
| **DB 记录** | UploadService.Upload | StorePackageFile（事务） | 3 张表 |
| **存储回滚** | UploadService.Upload | DB 写入失败 → DeletePackage | Storage |
| **审计日志** | HandlePublish (4.6) | AuditService.LogWithStatus | audit_logs 表 |
| **Prometheus 上传指标** | HandlePublish (4.7) | metrics.RecordUpload() | Prometheus |
| **Webhook 通知** | HandlePublish (4.8) | WebhookService.TriggerEvent → 异步 HTTP | 外部系统 |
| **Webhook 失败自禁** | WebhookService.sendWebhook | FailureCount ≥ 5 → Disabled | webhooks 表 |

---

## 5. 删除流程（DELETE）

```
请求: DELETE /repo/:repoName/*path
入口: repoRouter.HandleDelete (repo_router.go:391)
中间件: authMw + permMw("package", "delete")
```

### 5.1 Repo 解析

```
getRepo(repoName) → repo 检查:
  - 不存在 → 404
  - 已禁用 → 404
```

### 5.2 仓库类型 + 删除权限检查

```
switch repo.Type:
  case Proxy:   → 403 "代理仓库不支持删除"
  case Virtual: → 403 "虚拟仓库不支持直接删除"
  case Local:
    if !repo.AllowDelete → 403 "此仓库不允许删除"
    else → OK
  default: → 400 "未知的仓库类型"
```

### 5.3 Adapter 删除处理

```
result = resolver.HandleRepoDelete(c, repo):
  → adp.HandleDelete(c, ctx)
  → 各 adapter 解析路径、从存储和 DB 删除对应数据
  → 返回 RepoOperationResult（含 PackageName, Version 等）
```

### 5.4 审计日志（统一在 HandleDelete 中处理）

```
if auditSvc != nil && result != nil:
  auditSvc.LogWithRequestAndStatus(..., ActionPackageDelete, ...)
  → 异步写入 audit_logs 表（同下载审计机制）
```

### 5.5 Webhook

```
if result != nil && webhookSvc != nil:
  webhookSvc.TriggerEvent(WebhookEventPackageDeleted, payload):
    → 异步 HTTP POST 到所有配置了删除事件的 webhook（同上传）
```

## 5. 删除旁路逻辑汇总

| 旁路 | 触发位置 | 机制 | 数据目标 |
|------|----------|------|----------|
| **JWT 认证** | authMw（路由中间件） | Token 解析 | Gin Context |
| **RBAC 权限** | permMw("package", "delete")（路由中间件） | PermissionCache | HTTP 响应 |
| **仓库类型限制** | HandleDelete (5.2) | 仅 Local + AllowDelete 仓库允许 | HTTP 响应 |
| **审计日志** | HandleDelete (5.4) | AuditService.LogWithRequestAndStatus → 异步写入 | audit_logs 表 |
| **Webhook 通知** | HandleDelete (5.5) | WebhookService.TriggerEvent | 外部系统 |

---

## 6. 下载计数更新详细机制

```
调用路径:
  DownloadService.Download() 中存储命中:
    → incrementDownloadCount(downloadCtx)

  downloadFromProxy() 中代理下载成功:
    → storePackageFileRecord() → countBatcher.Increment()
```

### incrementDownloadCount 流程

```
incrementDownloadCount(req):
  if countBatcher == nil → return

  1. pkg, err = pkgRepo.FindByNameAndType(req.Name, req.PkgType)
     if err → return

  2. ver, err = pkgRepo.FindVersionByPackageAndVersion(pkg.ID, req.Version)
     if err → countBatcher.Increment(pkg.ID, 0, 0, req.Repo.ID)  // 仅包级计数

  3. file, err = pkgRepo.FindFileByVersionAndFilename(ver.ID, req.Filename)
     if err → countBatcher.Increment(pkg.ID, ver.ID, 0, req.Repo.ID)  // 包+版本级计数

  4. countBatcher.Increment(pkg.ID, ver.ID, file.ID, req.Repo.ID)  // 包+版本+文件级计数
```

### storePackageFileRecord 流程

```
storePackageFileRecord(...):
  1. pkgRepo.StorePackageFile(...):
     - 创建/更新 Package, PackageVersion, PackageFile 记录

  2. if countBatcher != nil:
     countBatcher.Increment(pkg.ID, versionID, fileID, repoID)
```

### DownloadCountBatcher 刷出机制

```
flushLoop():
  ticker = 10 秒
  flush():
    - 锁定 map，复制后清空
    - batchUpdateCounts(ctx, counts):
      - 按 RepoID / PackageID / VersionID / FileID 分组汇总
      - UPDATE repositories SET download_count = download_count + ? WHERE id = ?
      - UPDATE packages SET download_count = download_count + ? WHERE id = ?
      - UPDATE package_versions SET download_count = download_count + ? WHERE id = ?
      - UPDATE package_files SET download_count = download_count + ? WHERE id = ?
```

---

## 7. 下载日志更新详细机制

```
recordLog(req, sizeBytes, duration, err):
  执行条件: logRepo != nil || logBatcher != nil

  构建 ProxyDownloadLog:
    RepositoryID, PackageType, PackageName, Version, Filename
    Status = DownloadStatusSuccess (err==nil) / DownloadStatusFailed
    StatusCode = 200 / 404
    SizeBytes, DurationMs, FromCache = false
    IPAddress = req.ClientIP
    UserID = req.UserID（如果 > 0）
    ErrorMessage = err.Error()（如果有）

  写入:
    if logBatcher != nil:
      logBatcher.Record(log)  // 积累到 100 条或每 5 秒 → BatchCreate
    elif logRepo != nil:
      logRepo.Create(log)     // 直接写入

  同时:
    metrics.RecordDownload(pkgType, name, version)
    → moonlight_downloads_total{pkg_type, pkg_name, version}++
```

---

## 8. 代理缓存管理详细机制

```
FetchFromRemote 中使用的 CacheService:
  - 内存缓存，可在配置中设置最大大小
  - 缓存 key = "proxy:{repoName}:{remoteURL}"

缓存写入:
  - 成功响应: cache.Set(ctx, &CacheItem{Key, Content, Size}, TTL)
    TTL = repo.CacheTTLSeconds

  - 失败响应（根据 FailureCacheRules）:
    cache.SetNegative(ctx, key, failureTTL)

  - 404: cache.SetNegative(ctx, key, repo.CacheNegativeTTL)

缓存读取:
  若缓存命中且非 Negative → 直接返回缓存内容（FromCache=true）
  若缓存命中且是 Negative → ErrPackageNotFound（不请求远程）
  若缓存未命中 → 发起远程请求

大文件（超过 largeFileThreshold）:
  - 不缓存到 CacheService
  - 直接流式返回
```

---

## 9. 熔断器详细机制

```
位置: ProxyDownloader.FetchFromRemote

每次远程请求失败 → RecordFailure():
  - 连续失败达到 FailureThreshold → 熔断器打开
  - 熔断器打开期间 → ShouldSkipRequest() 返回 true
  - 请求直接被拒绝，不发起远程请求
  - 熔断器在 RetryTimeout 后尝试半开

每次远程请求成功 → RecordSuccess():
  - 熔断器关闭
  - 失败计数重置
```

---

## 10. 不变约束（修改时不得破坏）

1. **下载请求必须先 ParseIntent，再走 BlockCheck → PermissionCheck → ContentFetch 的顺序**
2. **所有请求（下载/元数据/列表/校验和）都必须经过阻断检查（block check），只要 intent.Name 不为空**
3. **阻断检查必须在所有其他业务逻辑之前执行（ParseIntent 之后立即执行）**
4. **下载权限检查（DownloadPluginChain）仅在 RequestDownload 时执行**
5. **Local 仓库的下载请求永远不能回源到代理（proxy fetcher）**
6. **Virtual 仓库的解析必须并发探测所有匹配成员，取第一个成功结果**
7. **下载请求必须记录审计日志（ActionPackageDownload）**
8. **DownloadService.Download 中只要存储命中就必须记录下载日志和下载计数**
9. **下载日志必须在下载计数之前或同时记录，不能先计数后日志**
10. **上传请求仅允许 Local 仓库，Proxy 和 Virtual 必须拒绝**
11. **删除请求仅允许 AllowDelete=true 的 Local 仓库，Proxy 和 Virtual 必须拒绝**
12. **上传请求必须包含认证（authMw），删除请求必须同时包含认证和 RBAC 权限**
13. **UploadService.Upload 中 DB 写入失败必须回滚存储（DeletePackage）**
14. **Webhook 发送失败时，连续失败 5 次必须自动禁用 webhook**
15. **所有异步写入（审计日志、下载日志、下载计数）必须在 server shutdown 时完成刷出**
16. **下载响应必须始终为 application/octet-stream + Content-Disposition（对 Download 类型）**