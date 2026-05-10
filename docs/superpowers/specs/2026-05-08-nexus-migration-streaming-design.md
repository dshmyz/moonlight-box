# Nexus 迁移流式处理设计文档

**日期**: 2026-05-08  
**作者**: AI Assistant  
**状态**: 待审查

## 1. 背景和目标

### 1.1 当前问题

现有的 Nexus 迁移实现存在以下问题：

1. **内存占用高**：一次性加载所有组件到内存，对于大规模仓库（>10000组件）可能导致 OOM
2. **不支持断点续传**：任务中断后需要从头开始，浪费已完成的进度
3. **无法精确控制进度**：用户无法看到详细的迁移进度和失败原因

### 1.2 目标

- 支持大规模仓库迁移（>10000组件）
- 支持断点续传，任务中断后能够从中断点继续
- 优化内存使用，避免 OOM
- 提供详细的迁移进度和错误信息
- 保持高性能和可扩展性

## 2. 设计方案

### 2.1 核心架构

采用**生产者-消费者模式**，实现流式分批处理：

```
┌─────────────────────────────────────────────────────────┐
│                  MigrationWorker                        │
│                                                         │
│  ┌──────────────┐         ┌──────────────────────┐    │
│  │  Producer    │────────>│  Component Queue     │    │
│  │  (获取组件)   │         │  (有界队列, size=100) │    │
│  └──────────────┘         └──────────────────────┘    │
│                                    │                    │
│                                    ▼                    │
│                         ┌──────────────────┐           │
│                         │  Consumer Pool   │           │
│                         │  (并发处理组件)   │           │
│                         └──────────────────┘           │
│                                    │                    │
│                                    ▼                    │
│                         ┌──────────────────┐           │
│                         │  Progress Tracker│           │
│                         │  (持久化进度)     │           │
│                         └──────────────────┘           │
└─────────────────────────────────────────────────────────┘
```

### 2.2 数据库设计

#### 2.2.1 新增 MigrationItem 表

```go
type MigrationItem struct {
    ID             uint       `json:"id" gorm:"primaryKey"`
    TaskID         uint       `json:"task_id" gorm:"index"`
    Repository     string     `json:"repository" gorm:"size:200"`
    ComponentID    string     `json:"component_id" gorm:"size:200;uniqueIndex:idx_task_component"`
    ComponentName  string     `json:"component_name" gorm:"size:500"`
    ComponentGroup string     `json:"component_group" gorm:"size:500"`
    Version        string     `json:"version" gorm:"size:100"`
    Format         string     `json:"format" gorm:"size:50"`
    Status         string     `json:"status" gorm:"size:20;index"`
    ErrorMessage   string     `json:"error_message" gorm:"type:text"`
    RetryCount     int        `json:"retry_count" gorm:"default:0"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
    CompletedAt    *time.Time `json:"completed_at"`
}
```

**索引设计**：
- `task_id` + `status` 组合索引：加速查询待处理项
- `task_id` + `component_id` 唯一索引：避免重复迁移

#### 2.2.2 MigrationTask 表扩展

```go
type MigrationTask struct {
    // ... 现有字段 ...
    
    // 新增配置字段
    WorkerCount  int `json:"worker_count" gorm:"default:10"`
    MaxRetries   int `json:"max_retries" gorm:"default:3"`
    BatchSize    int `json:"batch_size" gorm:"default:50"`
}
```

### 2.3 核心组件设计

#### 2.3.1 MigrationItemRepository

```go
type MigrationItemRepository struct {
    db *gorm.DB
}

// 批量创建迁移项
func (r *MigrationItemRepository) BatchCreate(items []MigrationItem) error

// 获取待处理的项（支持断点续传）
func (r *MigrationItemRepository) GetPendingItems(taskID uint, limit int) ([]MigrationItem, error)

// 批量更新状态
func (r *MigrationItemRepository) BatchUpdateStatus(ids []uint, status string, errMsg string) error

// 更新单个项状态
func (r *MigrationItemRepository) UpdateStatus(id uint, status string, errMsg string) error

// 获取统计信息
func (r *MigrationItemRepository) GetStats(taskID uint) (total, pending, processing, completed, failed int, err error)

// 清理已完成的项（任务完成后）
func (r *MigrationItemRepository) CleanCompletedItems(taskID uint) error
```

#### 2.3.2 ComponentQueue

```go
type ComponentQueue struct {
    queue chan MigrationItem
    size  int
}

func NewComponentQueue(size int) *ComponentQueue

// 放入组件（非阻塞，队列满时返回false）
func (q *ComponentQueue) TryPush(item MigrationItem) bool

// 取出组件（阻塞）
func (q *ComponentQueue) Pop() (MigrationItem, bool)

// 关闭队列
func (q *ComponentQueue) Close()
```

#### 2.3.3 ProgressUpdater

```go
type ProgressUpdater struct {
    taskID       uint
    db           *gorm.DB
    updateTicker *time.Ticker
    buffer       struct {
        processed int
        failed    int
        mu        sync.Mutex
    }
}

// 增量计数（内存缓冲）
func (p *ProgressUpdater) IncrementProcessed()

// 定期批量更新数据库（每5秒或每100个组件）
func (p *ProgressUpdater) Start(ctx context.Context)

// 刷新缓冲区
func (p *ProgressUpdater) flush()
```

### 2.4 核心流程

#### 2.4.1 Producer（生产者）

```go
func (w *MigrationWorker) producer(ctx context.Context, task *model.MigrationTask, queue *ComponentQueue) {
    client := w.service.GetNexusClient(task.ID)
    
    for _, repoName := range selectedRepos {
        token := ""
        for {
            // 1. 按页获取组件
            components, nextToken, err := client.ListComponentsPage(ctx, repoName, token)
            if err != nil {
                log.Error("Failed to fetch components", err)
                break
            }
            
            // 2. 批量写入数据库（状态为 pending）
            items := convertToMigrationItems(task.ID, repoName, components)
            if err := w.itemRepo.BatchCreate(items); err != nil {
                log.Error("Failed to save items", err)
                break
            }
            
            // 3. 放入队列（控制内存）
            for _, item := range items {
                select {
                case <-ctx.Done():
                    return
                default:
                    if !queue.TryPush(item) {
                        // 队列满，等待
                        time.Sleep(100 * time.Millisecond)
                    }
                }
            }
            
            // 4. 检查是否还有下一页
            if nextToken == "" {
                break
            }
            token = nextToken
        }
    }
    
    queue.Close()
}
```

#### 2.4.2 Consumer（消费者）

```go
func (w *MigrationWorker) consumer(ctx context.Context, task *model.MigrationTask, queue *ComponentQueue) {
    for {
        select {
        case <-ctx.Done():
            return
        case item, ok := <-queue.Pop():
            if !ok {
                return
            }
            
            // 1. 更新状态为 processing
            w.itemRepo.UpdateStatus(item.ID, "processing", "")
            
            // 2. 处理组件（带重试）
            err := w.processWithRetry(ctx, task, item)
            
            // 3. 更新状态
            if err != nil {
                w.itemRepo.UpdateStatus(item.ID, "failed", err.Error())
                w.progressUpdater.IncrementFailed()
            } else {
                w.itemRepo.UpdateStatus(item.ID, "completed", "")
                w.progressUpdater.IncrementProcessed()
            }
        }
    }
}
```

#### 2.4.3 断点续传

```go
func (w *MigrationWorker) Execute(ctx context.Context, task *model.MigrationTask) error {
    // 1. 检查是否有未完成的项
    pendingItems, err := w.itemRepo.GetPendingItems(task.ID, 1000)
    if err != nil {
        return err
    }
    
    // 2. 如果有待处理项，说明是断点续传
    if len(pendingItems) > 0 {
        log.Info("Resuming from checkpoint", "items", len(pendingItems))
        // 直接加载待处理项到队列
        go w.loadPendingItems(ctx, task, pendingItems, queue)
    } else {
        // 启动生产者获取新组件
        go w.producer(ctx, task, queue)
    }
    
    // 3. 启动消费者池
    w.consumerPool(ctx, task, queue)
    
    // 4. 清理
    w.cleanup(task.ID)
}
```

### 2.5 错误处理和重试

#### 2.5.1 错误分类

```go
type MigrationError struct {
    Type    string // network/download/store/parse
    Message string
    Retry   bool
}

const (
    ErrorTypeNetwork  = "network"   // 可重试
    ErrorTypeDownload = "download"  // 可重试
    ErrorTypeStore    = "store"     // 可重试
    ErrorTypeParse    = "parse"     // 不可重试
)
```

#### 2.5.2 重试策略

- **指数退避**：初始延迟 1 秒，每次失败后延迟翻倍，最大 30 秒
- **最大重试次数**：默认 3 次，用户可配置
- **失败处理**：
  - 可重试错误：更新 `retry_count`，状态保持 `pending`
  - 不可重试错误或超过最大重试次数：状态更新为 `failed`

### 2.6 性能优化

#### 2.6.1 内存优化

- **有界队列**：队列大小默认 100，控制内存使用
- **流式处理**：生产者和消费者并行，不需要等所有组件加载完
- **批量操作**：数据库批量插入/更新，减少连接开销

#### 2.6.2 进度更新优化

- **内存缓冲**：在内存中累计进度，定期批量更新数据库
- **更新频率**：每 5 秒或每 100 个组件更新一次
- **最终刷新**：任务完成前最后一次刷新缓冲区

#### 2.6.3 清理策略

- **任务完成后**：删除所有成功的 `MigrationItem` 记录
- **失败记录保留**：保留失败的记录用于分析和重试
- **定期清理**：可配置清理超过一定时间的记录

### 2.7 用户界面配置

在迁移界面添加"高级配置"选项：

```
┌─────────────────────────────────────────┐
│  创建迁移任务                             │
├─────────────────────────────────────────┤
│  Nexus URL: [___________________]       │
│  Username:  [___________________]       │
│  Password:  [___________________]       │
│  选择仓库:   [多选框列表]                │
│  目标仓库:   [下拉选择]                  │
│                                         │
│  ▼ 高级配置（可选）                      │
│  ┌─────────────────────────────────┐   │
│  │ 并发数: [10] (1-50)              │   │
│  │ 最大重试次数: [3] (0-10)          │   │
│  │ 批量大小: [50] (10-200)          │   │
│  │                                 │   │
│  │ 💡 提示：                        │   │
│  │ - 并发数越高，迁移越快，但占用更多资源 │   │
│  │ - 批量大小影响数据库写入频率       │   │
│  └─────────────────────────────────┘   │
│                                         │
│  [开始迁移]                             │
└─────────────────────────────────────────┘
```

**配置参数**：
- `worker_count`：并发数（1-50，默认 10）
- `max_retries`：最大重试次数（0-10，默认 3）
- `batch_size`：批量大小（10-200，默认 50）

## 3. 实现计划

### 3.1 数据库迁移

1. 创建 `migration_items` 表
2. 为 `migration_tasks` 表添加配置字段
3. 编写数据迁移脚本（兼容现有数据）

### 3.2 后端实现

1. 实现 `MigrationItemRepository`
2. 实现 `ComponentQueue`
3. 实现 `ProgressUpdater`
4. 重构 `MigrationWorker`（生产者-消费者模式）
5. 实现断点续传逻辑
6. 实现错误处理和重试机制
7. 更新 API 接口（支持配置参数）

### 3.3 前端实现

1. 更新迁移界面（添加高级配置）
2. 更新 API 调用（传递配置参数）
3. 优化进度显示（显示详细统计）

### 3.4 测试

1. 单元测试（生产者、消费者、重试逻辑）
2. 集成测试（完整迁移流程）
3. 性能测试（大规模数据）
4. 断点续传测试

## 4. 兼容性和迁移

### 4.1 向后兼容

- 保留现有 `MigrationWorker` 代码路径
- 通过配置开关控制使用新旧实现
- 数据库表独立，不影响现有数据

### 4.2 数据迁移

```go
func MigrateMigrationItems(db *gorm.DB) error {
    // 1. 创建 migration_items 表
    // 2. 为 migration_tasks 添加配置字段
    // 3. 对于正在运行的任务，标记为需要重新开始
}
```

### 4.3 回滚方案

如果新方案出现问题：
- 通过配置切换回旧版本
- 数据库表独立，不影响现有数据
- 可快速回滚代码

## 5. 监控和运维

### 5.1 监控指标

- 总组件数、已处理数、失败数
- 当前队列长度
- 活跃工作协程数
- 吞吐量（个/秒）
- 预计剩余时间
- 内存使用

### 5.2 日志记录

- 每个组件的处理结果
- 错误详情和堆栈
- 性能指标（处理时间、下载速度）
- 任务生命周期事件

### 5.3 告警规则

- 失败率超过阈值
- 处理速度异常下降
- 内存使用过高
- 数据库连接池耗尽

## 6. 风险和限制

### 6.1 已知限制

- 数据库存储开销：每个组件一条记录，大规模仓库会增加数据库负担
- 断点续传精度：基于组件级别，可能重复处理少量组件
- 并发控制：需要合理配置并发数，避免对 Nexus 和存储系统造成过大压力

### 6.2 风险缓解

- 定期清理已完成的记录
- 提供配置参数让用户根据实际情况调整
- 监控系统资源使用，动态调整并发数

## 7. 未来优化方向

1. **动态并发调整**：根据系统负载自动调整并发数
2. **分布式处理**：支持多节点并行处理大规模迁移
3. **增量迁移**：支持只迁移新增或变更的组件
4. **迁移预览**：在迁移前预览将要迁移的组件列表
5. **性能报告**：生成详细的迁移性能报告

## 8. 总结

本设计通过生产者-消费者模式和批量存储策略，实现了：
- ✅ 支持大规模仓库迁移（>10000组件）
- ✅ 支持断点续传
- ✅ 优化内存使用
- ✅ 提供详细进度和错误信息
- ✅ 保持高性能和可扩展性
- ✅ 用户可配置关键参数

该方案在实现复杂度和性能之间取得了良好的平衡，适合当前需求。
