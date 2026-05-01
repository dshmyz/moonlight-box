# 设计文档：跨类型智能路由 + Nexus 迁移工具

**日期**: 2026-05-01  
**状态**: 待审查  

---

## 一、目标

### 功能 1：跨类型智能路由

当前虚拟仓库只能绑定单一 `PackageType`（如 npm），用户需要为每种包类型创建不同的虚拟仓库。目标是让虚拟仓库支持多种包类型，一个虚拟仓库即可代理 npm + maven + pypi 等，系统根据请求自动选择对应的适配器和成员仓库。

### 功能 2：Nexus 一键迁移工具

提供从 Sonatype Nexus 仓库一键迁移包数据到 moonlight-box 的能力，包括仓库配置迁移和包数据迁移。

---

## 二、现状分析

### 虚拟仓库当前架构

```
Repository 模型:
  - Type: "virtual" / "proxy" / "local"
  - PackageType: "npm" (单一类型)
  - Members: []RepositoryGroup (成员列表，每个成员有自己的类型)

ProxyRouter.ResolveForVirtualRepo:
  - 按优先级遍历成员
  - 每个成员按自己的 PackageType 处理
  - 但虚拟仓库本身只有一个 PackageType
```

### 路由当前流程

```
GET /repo/{repoName}/{path}
  → RepoRouter.HandleRequest
    → 根据 repo.PackageType 找适配器
    → adapter.HandleRepoRequest(c, repo, path)
```

**问题**: 虚拟仓库的 `PackageType` 是单一的，无法根据请求自动分发到不同适配器。

---

## 三、功能 1 设计：跨类型智能路由

### 3.1 数据模型改造

**改动 `Repository` 模型**:

```go
type Repository struct {
    // ... 保留所有现有字段
    
    // 新增: 支持的包类型列表
    // 对虚拟仓库: ["npm", "maven", "pypi"]
    // 对代理/本地仓库: 保持 PackageType 单值不变
    PackageTypes string `json:"package_types" gorm:"type:text"` // JSON 数组序列化
    
    // PackageType 保留向后兼容
    // 新增仓库时如果 PackageTypes 非空，用第一个值填充 PackageType
    PackageType string `json:"package_type" gorm:"size:50"`
}
```

**改造理由**:
- 不破坏现有单一类型的仓库
- 虚拟仓库可通过 `PackageTypes` 支持多类型
- 序列化用 `string` 类型存储 JSON 数组，符合现有模式

### 3.2 路由层改造

**改造 `RepoRouter.HandleRequest`**:

```
请求进入
  ↓
判断仓库类型
  ↓
如果是虚拟仓库且 PackageTypes 包含多类型:
  → 调用 detectPackageType(path, headers) 检测请求的包类型
  → 选择对应适配器
  → 调用 adapter.HandleRepoRequest
否则:
  → 走现有逻辑 (按 PackageType 选适配器)
```

### 3.3 包类型检测器

新增 `TypeDetector` 组件，职责是根据请求特征判断包类型。

**检测策略** (按优先级):

| 优先级 | 策略 | 示例 |
|--------|------|------|
| 1 | URL 路径特征匹配 | `/npm/`, `/maven/`, `/pypi/simple/` |
| 2 | 包类型注册表匹配 | 路径前缀 `/npm/` → npm |
| 3 | Content-Type 推断 | `application/json` + 特定路径 → npm |
| 4 | 虚拟仓库配置类型遍历 | 尝试所有支持的类型 |

**实现**:

```go
type TypeDetector struct {
    // URL 路径前缀到包类型的映射
    prefixMap map[string]string
}

func (d *TypeDetector) Detect(path string, headers http.Header) string {
    // 1. 路径前缀匹配
    for prefix, pkgType := range d.prefixMap {
        if strings.HasPrefix(path, prefix) {
            return pkgType
        }
    }
    
    // 2. 包类型特有的 URL 模式匹配
    return d.matchPatterns(path)
}
```

**URL 模式匹配规则**:

```
npm:    包含 /-/ 或路径是包名格式 (@scope/name)
maven:  包含 / 分隔的 groupId/artifactId/version 格式
pypi:   包含 /simple/ 或 /packages/ 路径
go:     包含 /@v/ 或 /mod/ 路径
nuget:  包含 /odata/ 或 /package/ 路径
yum:    包含 /repodata/ 路径
apt:    包含 /dists/ 或 /pool/ 路径
```

### 3.4 虚拟仓库请求处理改造

改造各适配器中的 `handleVirtualMetadata` 和 `downloadFromVirtual` 方法。

**现有问题**: 当前虚拟仓库遍历成员时，没有过滤成员的 `PackageType` 是否匹配当前请求。

**改造方案**:

```go
func (r *ProxyRouter) ResolveForVirtualRepo(ctx context.Context, virtualRepo *model.Repository, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
    members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
    if err != nil {
        return nil, err
    }

    for _, member := range members {
        repo := member.MemberRepo
        
        // 新增: 过滤不匹配类型的成员
        if !r.isMemberTypeMatch(&repo, pkgType) {
            continue
        }

        // ... 现有逻辑
    }
    
    return nil, ErrPackageNotFound
}

func (r *ProxyRouter) isMemberTypeMatch(repo *model.Repository, pkgType string) bool {
    // 支持 PackageTypes 多类型的成员
    if repo.PackageTypes != "" {
        var types []string
        json.Unmarshal([]byte(repo.PackageTypes), &types)
        for _, t := range types {
            if t == pkgType {
                return true
            }
        }
        return false
    }
    
    // 回退到单一 PackageType
    return repo.PackageType == pkgType
}
```

### 3.5 配置页面改造

**前端改动**:

1. `RepositoryFormDialog.vue` - 仓库创建/编辑表单
   - 虚拟仓库类型选择改为多选（支持多选包类型）
   - 展示当前已支持的包类型列表

2. `RepositoryList.vue` - 仓库列表
   - 包类型列显示多个类型标签（如 `npm`, `maven`）

### 3.6 向后兼容

- 现有单类型虚拟仓库不受影响
- `PackageType` 字段保留，新建仓库时如果 `PackageTypes` 非空，自动取第一个值填充
- 查询时优先用 `PackageTypes`，为空时回退到 `PackageType`

---

## 四、功能 2 设计：Nexus 一键迁移工具

### 4.1 架构设计

```
┌───────────────────────────────────────────┐
│              前端迁移页面                    │
│  - Nexus 连接配置表单                       │
│  - 仓库选择列表                             │
│  - 迁移进度展示                             │
│  - 迁移日志                                │
└──────────────┬────────────────────────────┘
               │
┌──────────────▼────────────────────────────┐
│           MigrationHandler                 │
│  - POST /api/migration/nexus               │
│  - GET  /api/migration/{id}/status         │
│  - POST /api/migration/{id}/cancel         │
└──────────────┬────────────────────────────┘
               │
┌──────────────▼────────────────────────────┐
│           MigrationService                 │
│  - 创建迁移任务                             │
│  - 异步执行迁移                             │
│  - 更新进度                                │
│  - 记录日志                                │
└──────────────┬────────────────────────────┘
               │
┌──────────────▼────────────────────────────┐
│           NexusClient                      │
│  - 连接 Nexus 实例                         │
│  - 获取仓库列表                             │
│  - 获取包列表                               │
│  - 下载包数据                               │
└──────────────┬────────────────────────────┘
               │
┌──────────────▼────────────────────────────┐
│           目标仓库                          │
│  - 创建对应的本地/代理仓库                   │
│  - 写入包数据                               │
│  - 重建元数据                               │
└────────────────────────────────────────────┘
```

### 4.2 数据模型

**新增 `MigrationTask` 模型**:

```go
type MigrationTask struct {
    ID           uint              `json:"id" gorm:"primaryKey"`
    SourceType   string            `json:"source_type" gorm:"size:50"` // "nexus"
    SourceURL    string            `json:"source_url" gorm:"size:500"`
    Username     string            `json:"username" gorm:"size:100"`
    Password     string            `json:"-" gorm:"size:200"` // 不序列化
    Status       MigrationStatus   `json:"status" gorm:"size:20"`
    TotalItems   int               `json:"total_items" gorm:"default:0"`
    ProcessedItems int             `json:"processed_items" gorm:"default:0"`
    FailedItems  int               `json:"failed_items" gorm:"default:0"`
    SelectedRepos string           `json:"selected_repos" gorm:"type:text"` // JSON 数组
    ErrorMessage string            `json:"error_message" gorm:"type:text"`
    CreatedAt    time.Time         `json:"created_at"`
    UpdatedAt    time.Time         `json:"updated_at"`
    StartedAt    *time.Time        `json:"started_at"`
    CompletedAt  *time.Time        `json:"completed_at"`
}

type MigrationStatus string

const (
    MigrationPending    MigrationStatus = "pending"
    MigrationRunning    MigrationStatus = "running"
    MigrationCompleted  MigrationStatus = "completed"
    MigrationFailed     MigrationStatus = "failed"
    MigrationCancelled  MigrationStatus = "cancelled"
)
```

### 4.3 Nexus 客户端

**新增 `internal/migration/nexus_client.go`**:

```go
type NexusClient struct {
    baseURL  string
    username string
    password string
    client   *http.Client
}

// 能力:
// 1. TestConnection() error           - 测试连接
// 2. ListRepositories() ([]NexusRepo, error)  - 获取仓库列表
// 3. ListComponents(repoName string) ([]NexusComponent, error) - 获取包列表
// 4. DownloadAsset(assetURL string) (io.ReadCloser, error) - 下载包
```

**Nexus REST API 使用**:

```
GET /service/rest/v1/repositories           - 获取仓库列表
GET /service/rest/v1/components?repository={name} - 获取组件列表
GET /service/rest/v1/assets?repository={name}     - 获取资源列表
```

### 4.4 迁移流程

```
1. 用户提交迁移请求
   - Nexus URL + 认证信息
   - 选择要迁移的仓库
   ↓
2. MigrationService 创建迁移任务 (status=pending)
   ↓
3. 启动异步协程执行迁移
   ↓
4. 预扫描阶段:
   - 连接 Nexus，获取选中仓库的包清单
   - 更新 TotalItems
   - status = running
   ↓
5. 逐个迁移包:
   for each component:
     - 检测包类型 (npm/maven/pypi)
     - 在 moonlight-box 中创建对应的 proxy/local 仓库（如果不存在）
     - 下载包数据
     - 写入存储 + 更新元数据
     - 更新 ProcessedItems
   ↓
6. 完成或失败
   - status = completed / failed
   - 记录 ErrorMessage（如果失败）
```

### 4.5 并发控制

```go
type MigrationWorker struct {
    concurrency int           // 并发数，默认 5
    semaphore   chan struct{} // 并发控制信号量
}

func (w *MigrationWorker) Run(ctx context.Context, task *MigrationTask) {
    w.semaphore = make(chan struct{}, w.concurrency)
    
    for _, repo := range task.SelectedRepos {
        components := w.fetchComponents(repo)
        for _, comp := range components {
            w.semaphore <- struct{}{}
            go func(c NexusComponent) {
                defer func() { <-w.semaphore }()
                w.migrateComponent(c, task)
            }(comp)
        }
    }
}
```

### 4.6 前端页面

**新增路由**:

```typescript
{
  path: 'migration',
  name: 'Migration',
  component: () => import('@/views/MigrationPage.vue'),
  meta: { title: '数据迁移' },
}
```

**页面组件结构**:

```
MigrationPage.vue
├── NexusConnectionForm.vue      - Nexus 连接配置
├── RepositorySelector.vue       - 仓库选择列表
├── MigrationProgress.vue        - 迁移进度条 + 统计
├── MigrationLog.vue             - 迁移日志
└── MigrationHistory.vue         - 历史迁移任务列表
```

**NexusConnectionForm 功能**:
- URL 输入框
- 用户名/密码输入
- "测试连接" 按钮 → 调用 `POST /api/migration/nexus/test`
- 连接成功后显示仓库列表

**RepositorySelector 功能**:
- 多选仓库
- 显示每个仓库的包数量
- 全选/取消全选

**MigrationProgress 功能**:
- 进度条 (ProcessedItems / TotalItems)
- 成功数、失败数统计
- 开始/暂停/取消按钮
- 实时状态刷新（每 3 秒轮询）

### 4.7 API 设计

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/admin/migration/nexus/test` | 测试 Nexus 连接 |
| `GET` | `/api/admin/migration/nexus/repositories` | 获取 Nexus 仓库列表 |
| `POST` | `/api/admin/migration/nexus` | 创建迁移任务 |
| `GET` | `/api/admin/migration/:id/status` | 获取迁移进度 |
| `POST` | `/api/admin/migration/:id/cancel` | 取消迁移任务 |
| `GET` | `/api/admin/migration` | 获取迁移历史 |

---

## 五、改动文件清单

### 后端

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/model/repository.go` | 修改 | 新增 `PackageTypes` 字段 |
| `internal/handler/repo_router.go` | 修改 | 改造 HandleRequest 支持多类型检测 |
| `internal/proxy/router.go` | 修改 | 新增 `isMemberTypeMatch` 过滤逻辑 |
| `internal/migration/nexus_client.go` | 新增 | Nexus REST API 客户端 |
| `internal/migration/migration_service.go` | 新增 | 迁移任务管理 |
| `internal/migration/migration_worker.go` | 新增 | 并发迁移执行器 |
| `internal/handler/migration_handler.go` | 新增 | 迁移 API 处理器 |
| `internal/model/migration.go` | 新增 | 迁移任务数据模型 |
| `internal/service/repository_service.go` | 修改 | 支持 PackageTypes 的 CRUD |
| `internal/adapter/npm_adapter.go` | 修改 | 虚拟仓库类型过滤 |
| `internal/adapter/maven_adapter.go` | 修改 | 虚拟仓库类型过滤 |
| `internal/adapter/pypi_adapter.go` | 修改 | 虚拟仓库类型过滤 |
| `internal/router/router.go` | 修改 | 注册迁移 API 路由 |

### 前端

| 文件 | 操作 | 说明 |
|------|------|------|
| `web/src/router/index.ts` | 修改 | 新增 `/admin/migration` 路由 |
| `web/src/api/migration.ts` | 新增 | 迁移 API 请求函数 |
| `web/src/views/MigrationPage.vue` | 新增 | 数据迁移页面 |
| `web/src/components/migration/NexusConnectionForm.vue` | 新增 | Nexus 连接表单 |
| `web/src/components/migration/RepositorySelector.vue` | 新增 | 仓库选择组件 |
| `web/src/components/migration/MigrationProgress.vue` | 新增 | 迁移进度组件 |
| `web/src/components/migration/MigrationLog.vue` | 新增 | 迁移日志组件 |
| `web/src/components/migration/MigrationHistory.vue` | 新增 | 迁移历史组件 |
| `web/src/components/repository/RepositoryFormDialog.vue` | 修改 | 支持多包类型选择 |
| `web/src/components/repository/RepositoryTable.vue` | 修改 | 显示多包类型标签 |

### 数据库迁移

```sql
-- 新增 PackageTypes 字段
ALTER TABLE repositories ADD COLUMN package_types TEXT DEFAULT '';

-- 新增迁移任务表
CREATE TABLE migration_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type VARCHAR(50) NOT NULL,
    source_url VARCHAR(500) NOT NULL,
    username VARCHAR(100),
    password VARCHAR(200),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    total_items INTEGER DEFAULT 0,
    processed_items INTEGER DEFAULT 0,
    failed_items INTEGER DEFAULT 0,
    selected_repos TEXT,
    error_message TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME
);
```

---

## 六、测试策略

### 单元测试

| 模块 | 测试内容 |
|------|----------|
| `TypeDetector` | URL 路径匹配、Content-Type 推断、边界情况 |
| `isMemberTypeMatch` | 单类型/多类型匹配、回退逻辑 |
| `NexusClient` | 连接测试、API 调用 mock、错误处理 |
| `MigrationWorker` | 并发控制、进度更新、取消机制 |

### 集成测试

| 测试场景 | 验证内容 |
|----------|----------|
| 跨类型虚拟仓库请求 | npm/maven 请求正确路由到对应适配器 |
| Nexus 连接测试 | 正确连接和认证失败的情况 |
| 完整迁移流程 | 从 Nexus 迁移到 moonlight-box 端到端 |

---

## 七、风险和注意事项

1. **向后兼容**: 确保所有现有单类型虚拟仓库继续正常工作
2. **性能**: 类型检测器应该缓存匹配结果，避免每次请求都遍历
3. **迁移安全性**: Nexus 密码存储应加密，迁移完成后从内存清除
4. **迁移中断恢复**: 支持断点续传，记录已迁移的组件 ID
5. **错误隔离**: 单个包迁移失败不影响整个任务，只记录 FailedItems
