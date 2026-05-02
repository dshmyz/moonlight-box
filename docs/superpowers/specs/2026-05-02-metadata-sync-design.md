# 元数据预同步功能设计文档

## 1. 功能概述

为代理仓库（Proxy Repository）添加元数据预同步功能，类似 Nexus2 的预缓存能力。用户可以定时或手动从远程仓库同步包的元数据（名称、版本列表、描述等），实现：

- 用户浏览仓库时能看到所有可用的包和版本
- 实际文件仍按需从远程下载
- 支持配置为完整同步（包含文件）

## 2. 需求确认

### 2.1 触发方式
- ✅ 定时自动同步（可配置同步间隔）
- ✅ 手动触发同步（管理员操作）

### 2.2 同步范围
- ✅ 可配置策略：
  - `metadata_only`：仅同步元数据
  - `full`：同步元数据 + 下载所有文件

### 2.3 支持的包类型（第一版）
- ✅ Maven
- ✅ NPM
- ✅ PyPI

### 2.4 失败处理
- ✅ 记录错误继续同步其他包
- ✅ 管理员可查看失败记录

## 3. 数据模型设计

### 3.1 Repository 模型扩展

在现有 `Repository` 模型中添加字段：

```go
type Repository struct {
    // ... 现有字段 ...
    
    // 元数据同步配置
    MetadataSyncEnabled  bool       `json:"metadata_sync_enabled" gorm:"default:false"`
    MetadataSyncInterval int        `json:"metadata_sync_interval" gorm:"default:3600"` // 秒，默认1小时
    SyncMode            string     `json:"sync_mode" gorm:"default:'metadata_only'"` // metadata_only, full
    LastMetadataSyncAt  *time.Time `json:"last_metadata_sync_at"`
    LastSyncStatus      string     `json:"last_sync_status" gorm:"size:20"` // success, failed, partial
    LastSyncError       string     `json:"last_sync_error" gorm:"type:text"`
}
```

### 3.2 Package 模型扩展

```go
type Package struct {
    // ... 现有字段 ...
    
    // 元数据同步标记
    MetadataSynced   bool       `json:"metadata_synced" gorm:"default:false"` // 是否从远程同步
    MetadataSyncAt   *time.Time `json:"metadata_sync_at"` // 上次同步时间
}
```

### 3.3 PackageVersion 模型扩展

```go
type PackageVersion struct {
    // ... 现有字段 ...
    
    // 文件下载状态
    FilesDownloaded  bool `json:"files_downloaded" gorm:"default:false"` // 实际文件是否已下载
}
```

### 3.4 新增 MetadataSyncTask 模型

用于记录同步任务历史：

```go
type MetadataSyncTask struct {
    ID              uint       `json:"id" gorm:"primaryKey"`
    RepositoryID    uint       `json:"repository_id" gorm:"index"`
    Repository      Repository `json:"repository" gorm:"foreignKey:RepositoryID"`
    
    Status          string     `json:"status"` // pending, running, completed, failed, cancelled
    StartedAt       *time.Time `json:"started_at"`
    CompletedAt     *time.Time `json:"completed_at"`
    
    TotalPackages   int        `json:"total_packages"`
    SyncedPackages  int        `json:"synced_packages"`
    FailedPackages  int        `json:"failed_packages"`
    SkippedPackages int        `json:"skipped_packages"`
    
    ErrorMessage    string     `json:"error_message" gorm:"type:text"`
    SyncLog         string     `json:"sync_log" gorm:"type:text"` // JSON格式的详细日志
    
    TriggerType     string     `json:"trigger_type"` // manual, scheduled
    TriggeredBy     *uint      `json:"triggered_by"` // 用户ID
    
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
}
```

## 4. 服务层设计

### 4.1 MetadataSyncService（新增）

创建 `internal/service/metadata_sync_service.go`：

**职责**：
- 管理元数据同步任务
- 协调 Adapter 执行同步
- 记录同步状态和日志

**核心方法**：
```go
type MetadataSyncService struct {
    db           *gorm.DB
    repoRepo     *repository.RepositoryRepository
    pkgRepo      *repository.PackageRepository
    proxyRouter  *proxy.ProxyRouter
    adapters     map[string]adapter.PackageAdapter
}

// 同步仓库元数据
func (s *MetadataSyncService) SyncRepositoryMetadata(repoID uint) error

// 手动触发同步
func (s *MetadataSyncService) TriggerManualSync(repoID uint, userID uint) (*model.MetadataSyncTask, error)

// 获取任务状态
func (s *MetadataSyncService) GetTaskStatus(taskID uint) (*model.MetadataSyncTask, error)

// 获取仓库同步历史
func (s *MetadataSyncService) GetRepositorySyncHistory(repoID uint) ([]model.MetadataSyncTask, error)
```

### 4.2 SchedulerService 扩展

在现有 `SchedulerService` 中添加：

```go
type SchedulerService struct {
    // ... 现有字段 ...
    metadataSyncSvc *MetadataSyncService
}

// 调度元数据同步任务
func (s *SchedulerService) ScheduleMetadataSync(repoID uint, interval time.Duration) error

// 启动时恢复所有定时任务
func (s *SchedulerService) Start() error {
    // ... 现有代码 ...
    
    // 恢复所有启用了元数据同步的仓库的定时任务
    var repos []model.Repository
    s.db.Where("metadata_sync_enabled = ?", true).Find(&repos)
    
    for _, repo := range repos {
        interval := time.Duration(repo.MetadataSyncInterval) * time.Second
        s.ScheduleMetadataSync(repo.ID, interval)
    }
}
```

### 4.3 Adapter 扩展

为每个 Adapter 添加元数据同步方法：

**接口定义**：
```go
type MetadataSyncer interface {
    SyncMetadata(ctx context.Context, repo *model.Repository) (*SyncResult, error)
}

type SyncResult struct {
    Total   int
    Synced  int
    Failed  int
    Skipped int
}
```

**NPM Adapter** (`internal/adapter/npm_adapter.go`)：
```go
func (a *NpmAdapter) SyncMetadata(ctx context.Context, repo *model.Repository) (*SyncResult, error) {
    // 1. 调用远程仓库的 /-/all 接口
    // 2. 解析所有包的元数据
    // 3. 存入 Package 和 PackageVersion 表
    // 4. 标记 MetadataSynced=true, FilesDownloaded=false
}
```

**Maven Adapter** (`internal/adapter/maven_adapter.go`)：
```go
func (a *MavenAdapter) SyncMetadata(ctx context.Context, repo *model.Repository) (*SyncResult, error) {
    // 1. 遍历远程仓库目录结构
    // 2. 查找所有 maven-metadata.xml 文件
    // 3. 解析并存储包和版本信息
}
```

**PyPI Adapter** (`internal/adapter/pypi_adapter.go`)：
```go
func (a *PyPIAdapter) SyncMetadata(ctx context.Context, repo *model.Repository) (*SyncResult, error) {
    // 1. 拉取 Simple Index 页面
    // 2. 解析所有包名
    // 3. 拉取每个包的元数据
}
```

## 5. API 接口设计

### 5.1 接口列表

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/repositories/:name/sync` | 手动触发元数据同步 | repository:write |
| GET | `/api/repositories/:name/sync/history` | 获取同步历史记录 | repository:read |
| PUT | `/api/repositories/:name/sync-config` | 更新同步配置 | repository:write |
| GET | `/api/sync-tasks/:taskID` | 获取同步任务状态 | repository:read |
| POST | `/api/sync-tasks/:taskID/cancel` | 取消正在运行的任务 | repository:write |

### 5.2 接口详情

**触发同步**：
```json
POST /api/repositories/npm-proxy/sync

Response:
{
  "id": 123,
  "repository_id": 1,
  "status": "running",
  "started_at": "2026-05-02T10:00:00Z",
  "trigger_type": "manual",
  "triggered_by": 1
}
```

**获取任务状态**：
```json
GET /api/sync-tasks/123

Response:
{
  "id": 123,
  "status": "running",
  "total_packages": 1000,
  "synced_packages": 500,
  "failed_packages": 2,
  "started_at": "2026-05-02T10:00:00Z"
}
```

**更新同步配置**：
```json
PUT /api/repositories/npm-proxy/sync-config

Request:
{
  "metadata_sync_enabled": true,
  "metadata_sync_interval": 3600,
  "sync_mode": "metadata_only"
}

Response:
{
  "ok": true
}
```

## 6. 前端界面设计

### 6.1 仓库表单扩展

在 `web/src/components/repository/RepositoryFormDialog.vue` 中添加：

**元数据同步配置区域**（仅代理仓库显示）：
- 启用元数据同步（开关）
- 同步间隔（下拉选择：30分钟、1小时、6小时、12小时、每天）
- 同步模式（单选：仅元数据、完整同步）

### 6.2 仓库列表扩展

在 `web/src/views/RepositoryList.vue` 中添加：

**操作列**：
- 新增"同步元数据"按钮（仅代理仓库显示）
- 显示同步状态图标

**同步状态列**：
- 显示上次同步时间
- 显示同步状态标签（成功/失败/部分成功）

### 6.3 同步历史页面

创建 `web/src/views/RepositorySyncHistory.vue`：

**功能**：
- 显示同步任务列表
- 实时显示任务进度
- 查看详细日志
- 取消正在运行的任务

### 6.4 前端 API 扩展

在 `web/src/api/repository.ts` 中添加：

```typescript
export const repositoryAPI = {
  // ... 现有方法 ...
  
  triggerSync: (name: string) => 
    request.post(`/api/repositories/${name}/sync`),
  
  getSyncHistory: (name: string) => 
    request.get(`/api/repositories/${name}/sync/history`),
  
  getSyncTaskStatus: (taskId: string) => 
    request.get(`/api/sync-tasks/${taskId}`),
  
  updateSyncConfig: (name: string, config: SyncConfig) => 
    request.put(`/api/repositories/${name}/sync-config`, config),
  
  cancelSyncTask: (taskId: string) => 
    request.post(`/api/sync-tasks/${taskId}/cancel`),
}
```

## 7. 实现细节

### 7.1 同步流程

```
1. 触发同步（定时/手动）
   ↓
2. 创建 MetadataSyncTask 记录（status=running）
   ↓
3. 根据 PackageType 选择对应的 Adapter
   ↓
4. Adapter.SyncMetadata()
   - 从远程仓库拉取元数据
   - 解析包名和版本列表
   - 存入 Package 表（MetadataSynced=true）
   - 存入 PackageVersion 表（FilesDownloaded=false）
   - 如果 sync_mode=full，同时下载文件
   ↓
5. 更新任务状态（completed/failed）
   ↓
6. 更新 Repository.LastMetadataSyncAt 和 LastSyncStatus
```

### 7.2 错误处理

- 单个包同步失败：记录错误日志，继续同步其他包
- 整体任务失败：更新任务状态为 failed，记录错误信息
- 网络超时：使用仓库配置的超时时间
- 认证失败：记录错误，停止同步

### 7.3 性能优化

**NPM**：
- 使用 `/-/all` 接口一次性获取所有包
- 避免逐个包请求

**Maven**：
- 并行遍历目录结构
- 缓存已访问的目录

**PyPI**：
- 使用 Simple Index API
- 批量请求包元数据

### 7.4 数据一致性

- 使用 `MetadataSynced` 字段区分同步的包和本地上传的包
- 使用 `FilesDownloaded` 字段标记文件是否已下载
- 定期清理过期的元数据（可配置保留时间）

## 8. 数据库迁移

### 8.1 迁移文件

创建 `internal/database/migrations/YYYYMMDDHHMMSS_add_metadata_sync_fields.go`：

```go
func Up(db *gorm.DB) error {
    // Repository 表添加字段
    db.Exec("ALTER TABLE repositories ADD COLUMN metadata_sync_enabled BOOLEAN DEFAULT FALSE")
    db.Exec("ALTER TABLE repositories ADD COLUMN metadata_sync_interval INT DEFAULT 3600")
    db.Exec("ALTER TABLE repositories ADD COLUMN sync_mode VARCHAR(20) DEFAULT 'metadata_only'")
    db.Exec("ALTER TABLE repositories ADD COLUMN last_metadata_sync_at TIMESTAMP NULL")
    db.Exec("ALTER TABLE repositories ADD COLUMN last_sync_status VARCHAR(20) DEFAULT ''")
    db.Exec("ALTER TABLE repositories ADD COLUMN last_sync_error TEXT")
    
    // Package 表添加字段
    db.Exec("ALTER TABLE packages ADD COLUMN metadata_synced BOOLEAN DEFAULT FALSE")
    db.Exec("ALTER TABLE packages ADD COLUMN metadata_sync_at TIMESTAMP NULL")
    
    // PackageVersion 表添加字段
    db.Exec("ALTER TABLE package_versions ADD COLUMN files_downloaded BOOLEAN DEFAULT FALSE")
    
    // 创建 MetadataSyncTask 表
    db.AutoMigrate(&model.MetadataSyncTask{})
    
    return nil
}
```

## 9. 测试计划

### 9.1 单元测试

- MetadataSyncService 的核心方法
- 各 Adapter 的 SyncMetadata 方法
- 错误处理逻辑

### 9.2 集成测试

- 完整的同步流程
- 定时任务调度
- 手动触发和取消

### 9.3 性能测试

- 大规模仓库同步（10000+ 包）
- 并发同步多个仓库
- 网络异常情况处理

## 10. 实现优先级

### Phase 1（核心功能）
1. 数据模型扩展和迁移
2. MetadataSyncService 核心逻辑
3. NPM Adapter 元数据同步
4. API 接口实现
5. 基础前端界面

### Phase 2（扩展功能）
1. Maven Adapter 元数据同步
2. PyPI Adapter 元数据同步
3. 完整同步模式（下载文件）
4. 同步历史和日志查看

### Phase 3（优化功能）
1. 性能优化
2. 增量同步
3. 同步失败重试
4. 高级配置选项

## 11. 风险和限制

### 11.1 风险
- 远程仓库结构变化导致同步失败
- 大规模仓库同步耗时较长
- 网络不稳定导致同步中断

### 11.2 限制
- 第一版仅支持 Maven、NPM、PyPI
- 不支持增量同步（每次全量同步）
- 不支持同步失败自动重试

## 12. 后续优化方向

1. **增量同步**：只同步新增和变更的包
2. **智能调度**：根据仓库活跃度动态调整同步频率
3. **分布式同步**：支持多节点并行同步
4. **同步策略模板**：预定义常用配置模板
5. **监控告警**：同步失败时发送通知
