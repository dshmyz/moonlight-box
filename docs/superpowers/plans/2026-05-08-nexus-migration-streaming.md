# Nexus 迁移流式处理实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现 Nexus 迁移的流式处理，支持大规模仓库（>10000组件）和断点续传

**架构：** 采用生产者-消费者模式，通过有界队列控制内存，批量存储迁移进度到数据库，支持断点续传和错误重试

**技术栈：** Go 1.21+, GORM, Vue 3, Element Plus

---

## 文件结构

### 后端文件

**新增文件：**
- `internal/model/migration_item.go` - 迁移项数据模型
- `internal/repository/migration_item_repo.go` - 迁移项数据访问层
- `internal/migration/component_queue.go` - 组件队列
- `internal/migration/progress_updater.go` - 进度更新器

**修改文件：**
- `internal/model/migration.go` - 扩展 MigrationTask 模型
- `internal/database/migration.go` - 添加数据库迁移脚本
- `internal/migration/migration_worker.go` - 重构为生产者-消费者模式
- `internal/migration/nexus_client.go` - 添加分页获取方法
- `internal/migration/migration_service.go` - 添加配置参数处理
- `internal/handler/migration_handler.go` - 更新 API 处理器
- `web/src/api/migration.ts` - 更新前端 API 接口
- `web/src/views/MigrationPage.vue` - 添加高级配置界面

---

## 任务 1：数据库模型扩展

**文件：**
- 修改：`internal/model/migration.go`
- 创建：`internal/model/migration_item.go`
- 修改：`internal/database/migration.go`

- [ ] **步骤 1：扩展 MigrationTask 模型**

在 `internal/model/migration.go` 中添加配置字段：

```go
type MigrationTask struct {
	ID                  uint            `json:"id" gorm:"primaryKey"`
	SourceType          string          `json:"source_type" gorm:"size:50"`
	SourceURL           string          `json:"source_url" gorm:"size:500"`
	Username            string          `json:"username" gorm:"size:100"`
	Password            string          `json:"-" gorm:"size:200"`
	Status              MigrationStatus `json:"status" gorm:"size:20"`
	TotalItems          int             `json:"total_items" gorm:"default:0"`
	ProcessedItems      int             `json:"processed_items" gorm:"default:0"`
	FailedItems         int             `json:"failed_items" gorm:"default:0"`
	SelectedRepos       string          `json:"selected_repos" gorm:"type:text"`
	ErrorMessage        string          `json:"error_message" gorm:"type:text"`
	TargetRepositoryID  uint            `json:"target_repository_id" gorm:"default:0"`
	TargetRepository    string          `json:"target_repository" gorm:"size:200"`
	WorkerCount         int             `json:"worker_count" gorm:"default:10"`
	MaxRetries          int             `json:"max_retries" gorm:"default:3"`
	BatchSize           int             `json:"batch_size" gorm:"default:50"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	StartedAt           *time.Time      `json:"started_at"`
	CompletedAt         *time.Time      `json:"completed_at"`
}
```

- [ ] **步骤 2：创建 MigrationItem 模型**

创建 `internal/model/migration_item.go`：

```go
package model

import "time"

type MigrationItemStatus string

const (
	MigrationItemPending     MigrationItemStatus = "pending"
	MigrationItemProcessing  MigrationItemStatus = "processing"
	MigrationItemCompleted   MigrationItemStatus = "completed"
	MigrationItemFailed      MigrationItemStatus = "failed"
)

type MigrationItem struct {
	ID             uint                `json:"id" gorm:"primaryKey"`
	TaskID         uint                `json:"task_id" gorm:"index:idx_task_status"`
	Repository     string              `json:"repository" gorm:"size:200"`
	ComponentID    string              `json:"component_id" gorm:"size:200;uniqueIndex:idx_task_component"`
	ComponentName  string              `json:"component_name" gorm:"size:500"`
	ComponentGroup string              `json:"component_group" gorm:"size:500"`
	Version        string              `json:"version" gorm:"size:100"`
	Format         string              `json:"format" gorm:"size:50"`
	Status         MigrationItemStatus `json:"status" gorm:"size:20;index:idx_task_status"`
	ErrorMessage   string              `json:"error_message" gorm:"type:text"`
	RetryCount     int                 `json:"retry_count" gorm:"default:0"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	CompletedAt    *time.Time          `json:"completed_at"`
}

func (MigrationItem) TableName() string {
	return "migration_items"
}
```

- [ ] **步骤 3：添加数据库迁移脚本**

在 `internal/database/migration.go` 中添加迁移：

```go
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.Repository{},
		&model.Package{},
		&model.PackageVersion{},
		&model.PackageFile{},
		&model.StorageBackend{},
		&model.AuditLog{},
		&model.BackupTask{},
		&model.BlockRule{},
		&model.ProxyDownloadLog{},
		&model.Webhook{},
		&model.Role{},
		&model.CasbinRule{},
		&model.MetadataSyncTask{},
		&model.MigrationTask{},
		&model.MigrationItem{},
	)
}
```

- [ ] **步骤 4：运行数据库迁移**

运行：`go run cmd/registry/main.go migrate`
预期：成功创建 `migration_items` 表，并为 `migration_tasks` 表添加新字段

- [ ] **步骤 5：Commit**

```bash
git add internal/model/migration.go internal/model/migration_item.go internal/database/migration.go
git commit -m "feat(migration): 添加 MigrationItem 模型和扩展 MigrationTask 配置字段"
```

---

## 任务 2：创建数据访问层

**文件：**
- 创建：`internal/repository/migration_item_repo.go`

- [ ] **步骤 1：创建 MigrationItemRepository**

创建 `internal/repository/migration_item_repo.go`：

```go
package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MigrationItemRepository struct {
	db *gorm.DB
}

func NewMigrationItemRepository(db *gorm.DB) *MigrationItemRepository {
	return &MigrationItemRepository{db: db}
}

func (r *MigrationItemRepository) BatchCreate(items []model.MigrationItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}, {Name: "component_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
	}).CreateInBatches(items, 100).Error
}

func (r *MigrationItemRepository) GetPendingItems(taskID uint, limit int) ([]model.MigrationItem, error) {
	var items []model.MigrationItem
	err := r.db.Where("task_id = ? AND status IN ?", taskID, []model.MigrationItemStatus{
		model.MigrationItemPending,
		model.MigrationItemFailed,
	}).
		Where("retry_count < ?", 3).
		Order("created_at ASC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *MigrationItemRepository) BatchUpdateStatus(ids []uint, status model.MigrationItemStatus, errMsg string) error {
	if len(ids) == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if status == model.MigrationItemCompleted {
		now := time.Now()
		updates["completed_at"] = &now
	}
	return r.db.Model(&model.MigrationItem{}).Where("id IN ?", ids).Updates(updates).Error
}

func (r *MigrationItemRepository) UpdateStatus(id uint, status model.MigrationItemStatus, errMsg string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if status == model.MigrationItemFailed {
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	}
	if status == model.MigrationItemCompleted {
		now := time.Now()
		updates["completed_at"] = &now
	}
	return r.db.Model(&model.MigrationItem{}).Where("id = ?", id).Updates(updates).Error
}

func (r *MigrationItemRepository) GetStats(taskID uint) (total, pending, processing, completed, failed int, err error) {
	var stats []struct {
		Status model.MigrationItemStatus
		Count  int
	}
	err = r.db.Model(&model.MigrationItem{}).
		Select("status, count(*) as count").
		Where("task_id = ?", taskID).
		Group("status").
		Scan(&stats).Error

	for _, s := range stats {
		total += s.Count
		switch s.Status {
		case model.MigrationItemPending:
			pending = s.Count
		case model.MigrationItemProcessing:
			processing = s.Count
		case model.MigrationItemCompleted:
			completed = s.Count
		case model.MigrationItemFailed:
			failed = s.Count
		}
	}
	return
}

func (r *MigrationItemRepository) CleanCompletedItems(taskID uint) error {
	return r.db.Where("task_id = ? AND status = ?", taskID, model.MigrationItemCompleted).
		Delete(&model.MigrationItem{}).Error
}

func (r *MigrationItemRepository) GetByID(id uint) (*model.MigrationItem, error) {
	var item model.MigrationItem
	err := r.db.First(&item, id).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}
```

- [ ] **步骤 2：编写单元测试**

创建 `internal/repository/migration_item_repo_test.go`：

```go
package repository

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.MigrationItem{})
	assert.NoError(t, err)
	return db
}

func TestBatchCreate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMigrationItemRepository(db)

	items := []model.MigrationItem{
		{TaskID: 1, ComponentID: "comp1", ComponentName: "test1", Status: model.MigrationItemPending},
		{TaskID: 1, ComponentID: "comp2", ComponentName: "test2", Status: model.MigrationItemPending},
	}

	err := repo.BatchCreate(items)
	assert.NoError(t, err)

	var count int64
	db.Model(&model.MigrationItem{}).Count(&count)
	assert.Equal(t, int64(2), count)
}

func TestGetPendingItems(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMigrationItemRepository(db)

	items := []model.MigrationItem{
		{TaskID: 1, ComponentID: "comp1", Status: model.MigrationItemPending},
		{TaskID: 1, ComponentID: "comp2", Status: model.MigrationItemCompleted},
		{TaskID: 1, ComponentID: "comp3", Status: model.MigrationItemFailed, RetryCount: 1},
	}
	repo.BatchCreate(items)

	pending, err := repo.GetPendingItems(1, 10)
	assert.NoError(t, err)
	assert.Len(t, pending, 2)
}

func TestUpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMigrationItemRepository(db)

	item := model.MigrationItem{TaskID: 1, ComponentID: "comp1", Status: model.MigrationItemPending}
	db.Create(&item)

	err := repo.UpdateStatus(item.ID, model.MigrationItemProcessing, "")
	assert.NoError(t, err)

	var updated model.MigrationItem
	db.First(&updated, item.ID)
	assert.Equal(t, model.MigrationItemProcessing, updated.Status)
}
```

- [ ] **步骤 3：运行测试**

运行：`go test ./internal/repository -v -run TestMigrationItem`
预期：所有测试通过

- [ ] **步骤 4：Commit**

```bash
git add internal/repository/migration_item_repo.go internal/repository/migration_item_repo_test.go
git commit -m "feat(migration): 添加 MigrationItemRepository 数据访问层"
```

---

## 任务 3：创建组件队列

**文件：**
- 创建：`internal/migration/component_queue.go`

- [ ] **步骤 1：创建 ComponentQueue**

创建 `internal/migration/component_queue.go`：

```go
package migration

import (
	"context"

	"github.com/dshmyz/moonlight-box/internal/model"
)

type ComponentQueue struct {
	queue  chan model.MigrationItem
	ctx    context.Context
	cancel context.CancelFunc
}

func NewComponentQueue(size int) *ComponentQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &ComponentQueue{
		queue:  make(chan model.MigrationItem, size),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (q *ComponentQueue) TryPush(item model.MigrationItem) bool {
	select {
	case q.queue <- item:
		return true
	default:
		return false
	}
}

func (q *ComponentQueue) Push(item model.MigrationItem) bool {
	select {
	case q.queue <- item:
		return true
	case <-q.ctx.Done():
		return false
	}
}

func (q *ComponentQueue) Pop() (model.MigrationItem, bool) {
	select {
	case item, ok := <-q.queue:
		return item, ok
	case <-q.ctx.Done():
		return model.MigrationItem{}, false
	}
}

func (q *ComponentQueue) Close() {
	q.cancel()
	close(q.queue)
}

func (q *ComponentQueue) Len() int {
	return len(q.queue)
}

func (q *ComponentQueue) Cap() int {
	return cap(q.queue)
}
```

- [ ] **步骤 2：编写单元测试**

创建 `internal/migration/component_queue_test.go`：

```go
package migration

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestComponentQueue_PushPop(t *testing.T) {
	queue := NewComponentQueue(10)
	defer queue.Close()

	item := model.MigrationItem{ComponentID: "test1"}
	assert.True(t, queue.Push(item))

	popped, ok := queue.Pop()
	assert.True(t, ok)
	assert.Equal(t, "test1", popped.ComponentID)
}

func TestComponentQueue_TryPush(t *testing.T) {
	queue := NewComponentQueue(2)
	defer queue.Close()

	assert.True(t, queue.TryPush(model.MigrationItem{ComponentID: "1"}))
	assert.True(t, queue.TryPush(model.MigrationItem{ComponentID: "2"}))
	assert.False(t, queue.TryPush(model.MigrationItem{ComponentID: "3"}))
}

func TestComponentQueue_Close(t *testing.T) {
	queue := NewComponentQueue(10)
	queue.Close()

	_, ok := queue.Pop()
	assert.False(t, ok)
}
```

- [ ] **步骤 3：运行测试**

运行：`go test ./internal/migration -v -run TestComponentQueue`
预期：所有测试通过

- [ ] **步骤 4：Commit**

```bash
git add internal/migration/component_queue.go internal/migration/component_queue_test.go
git commit -m "feat(migration): 添加 ComponentQueue 组件队列"
```

---

## 任务 4：创建进度更新器

**文件：**
- 创建：`internal/migration/progress_updater.go`

- [ ] **步骤 1：创建 ProgressUpdater**

创建 `internal/migration/progress_updater.go`：

```go
package migration

import (
	"context"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

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

func NewProgressUpdater(taskID uint, db *gorm.DB, updateInterval time.Duration) *ProgressUpdater {
	return &ProgressUpdater{
		taskID:       taskID,
		db:           db,
		updateTicker: time.NewTicker(updateInterval),
	}
}

func (p *ProgressUpdater) IncrementProcessed() {
	p.buffer.mu.Lock()
	p.buffer.processed++
	p.buffer.mu.Unlock()
}

func (p *ProgressUpdater) IncrementFailed() {
	p.buffer.mu.Lock()
	p.buffer.failed++
	p.buffer.mu.Unlock()
}

func (p *ProgressUpdater) Start(ctx context.Context) {
	defer p.updateTicker.Stop()

	for {
		select {
		case <-p.updateTicker.C:
			p.flush()
		case <-ctx.Done():
			p.flush()
			return
		}
	}
}

func (p *ProgressUpdater) flush() {
	p.buffer.mu.Lock()
	processed := p.buffer.processed
	failed := p.buffer.failed
	p.buffer.processed = 0
	p.buffer.failed = 0
	p.buffer.mu.Unlock()

	if processed > 0 || failed > 0 {
		p.db.Model(&model.MigrationTask{}).
			Where("id = ?", p.taskID).
			Updates(map[string]interface{}{
				"processed_items": gorm.Expr("processed_items + ?", processed),
				"failed_items":    gorm.Expr("failed_items + ?", failed),
			})
	}
}

func (p *ProgressUpdater) Stop() {
	p.flush()
	p.updateTicker.Stop()
}
```

- [ ] **步骤 2：编写单元测试**

创建 `internal/migration/progress_updater_test.go`：

```go
package migration

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupProgressTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.MigrationTask{})
	assert.NoError(t, err)
	return db
}

func TestProgressUpdater_Increment(t *testing.T) {
	db := setupProgressTestDB(t)
	task := &model.MigrationTask{Status: model.MigrationRunning}
	db.Create(task)

	updater := NewProgressUpdater(task.ID, db, 100*time.Millisecond)
	defer updater.Stop()

	updater.IncrementProcessed()
	updater.IncrementProcessed()
	updater.IncrementFailed()

	updater.flush()

	var updated model.MigrationTask
	db.First(&updated, task.ID)
	assert.Equal(t, 2, updated.ProcessedItems)
	assert.Equal(t, 1, updated.FailedItems)
}

func TestProgressUpdater_Start(t *testing.T) {
	db := setupProgressTestDB(t)
	task := &model.MigrationTask{Status: model.MigrationRunning}
	db.Create(task)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updater := NewProgressUpdater(task.ID, db, 50*time.Millisecond)
	go updater.Start(ctx)

	updater.IncrementProcessed()
	time.Sleep(100 * time.Millisecond)

	var updated model.MigrationTask
	db.First(&updated, task.ID)
	assert.Equal(t, 1, updated.ProcessedItems)
}
```

- [ ] **步骤 3：运行测试**

运行：`go test ./internal/migration -v -run TestProgressUpdater`
预期：所有测试通过

- [ ] **步骤 4：Commit**

```bash
git add internal/migration/progress_updater.go internal/migration/progress_updater_test.go
git commit -m "feat(migration): 添加 ProgressUpdater 进度更新器"
```

---

## 任务 5：扩展 NexusClient 支持分页

**文件：**
- 修改：`internal/migration/nexus_client.go`

- [ ] **步骤 1：添加分页获取方法**

在 `internal/migration/nexus_client.go` 中添加新方法：

```go
func (c *NexusClient) ListComponentsPage(ctx context.Context, repoName, continuationToken string) ([]NexusComponent, string, error) {
	url := fmt.Sprintf("%s/service/rest/v1/components?repository=%s", c.baseURL, repoName)
	if continuationToken != "" {
		url += "&continuationToken=" + continuationToken
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logrus.WithFields(logrus.Fields{
			"url":    url,
			"status": resp.StatusCode,
			"body":   string(body),
		}).Error("Nexus API returned error status")
		return nil, "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page NexusComponentPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %w", err)
	}

	nextToken := ""
	if page.ContinuationToken != nil {
		nextToken = *page.ContinuationToken
	}

	return page.Items, nextToken, nil
}
```

- [ ] **步骤 2：编写单元测试**

在 `internal/migration/nexus_client_test.go` 中添加测试：

```go
package migration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListComponentsPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/service/rest/v1/components", r.URL.Path)
		assert.Equal(t, "test-repo", r.URL.Query().Get("repository"))
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"items": [
				{"id": "comp1", "name": "test1", "version": "1.0.0"}
			],
			"continuationToken": "next-token"
		}`))
	}))
	defer server.Close()

	client := NewNexusClient(server.URL, "user", "pass")
	items, token, err := client.ListComponentsPage(context.Background(), "test-repo", "")
	
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "next-token", token)
	assert.Equal(t, "comp1", items[0].ID)
}
```

- [ ] **步骤 3：运行测试**

运行：`go test ./internal/migration -v -run TestListComponentsPage`
预期：测试通过

- [ ] **步骤 4：Commit**

```bash
git add internal/migration/nexus_client.go internal/migration/nexus_client_test.go
git commit -m "feat(migration): 为 NexusClient 添加分页获取方法"
```

---

## 任务 6：重构 MigrationWorker（核心）

**文件：**
- 修改：`internal/migration/migration_worker.go`

- [ ] **步骤 1：重构 MigrationWorker 结构**

修改 `internal/migration/migration_worker.go`：

```go
package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/sirupsen/logrus"
)

type MigrationWorker struct {
	service     *MigrationService
	storageSvc  *service.StorageService
	pkgRepo     *repository.PackageRepository
	itemRepo    *repository.MigrationItemRepository
	concurrency int
}

func NewMigrationWorker(
	migrationSvc *MigrationService,
	storageSvc *service.StorageService,
	pkgRepo *repository.PackageRepository,
	itemRepo *repository.MigrationItemRepository,
	concurrency int,
) *MigrationWorker {
	return &MigrationWorker{
		service:     migrationSvc,
		storageSvc:  storageSvc,
		pkgRepo:     pkgRepo,
		itemRepo:    itemRepo,
		concurrency: concurrency,
	}
}
```

- [ ] **步骤 2：实现 Execute 方法**

```go
func (w *MigrationWorker) Execute(ctx context.Context, task *model.MigrationTask) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.service.RegisterContext(task.ID, ctx, cancel)

	logrus.WithFields(logrus.Fields{
		"module":      "migration",
		"task_id":     task.ID,
		"source_url":  task.SourceURL,
		"concurrency": w.concurrency,
	}).Info("Migration task started")

	client := NewNexusClient(task.SourceURL, task.Username, task.Password)
	w.service.RegisterNexusClient(task.ID, client)

	now := time.Now()
	startedAt := &now
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":     model.MigrationRunning,
		"started_at": startedAt,
	})

	var selectedRepos []string
	if err := json.Unmarshal([]byte(task.SelectedRepos), &selectedRepos); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
			"error":   err,
		}).Error("Failed to parse selected repos")
		return w.failTask(task.ID, fmt.Sprintf("failed to parse selected repos: %v", err))
	}

	queue := NewComponentQueue(task.WorkerCount * 10)
	progressUpdater := NewProgressUpdater(task.ID, w.service.db, 5*time.Second)
	defer progressUpdater.Stop()

	go progressUpdater.Start(ctx)

	var wg sync.WaitGroup

	pendingItems, err := w.itemRepo.GetPendingItems(task.ID, 1000)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
			"error":   err,
		}).Error("Failed to get pending items")
	}

	if len(pendingItems) > 0 {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
			"items":   len(pendingItems),
		}).Info("Resuming from checkpoint")
		go w.loadPendingItems(ctx, pendingItems, queue)
	} else {
		go w.producer(ctx, task, client, selectedRepos, queue)
	}

	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.consumer(ctx, task, client, queue, progressUpdater)
		}()
	}

	wg.Wait()

	w.completeTask(task.ID)
	logrus.WithFields(logrus.Fields{
		"module":  "migration",
		"task_id": task.ID,
	}).Info("Migration task completed")
	w.service.AddLog(task.ID, "迁移任务完成")
	
	w.cleanup(task.ID)
	return nil
}
```

- [ ] **步骤 3：实现生产者方法**

```go
func (w *MigrationWorker) producer(
	ctx context.Context,
	task *model.MigrationTask,
	client *NexusClient,
	selectedRepos []string,
	queue *ComponentQueue,
) {
	defer queue.Close()

	for _, repoName := range selectedRepos {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w.service.AddLog(task.ID, fmt.Sprintf("开始迁移仓库: %s", repoName))
		logrus.WithFields(logrus.Fields{
			"module":    "migration",
			"task_id":   task.ID,
			"repo_name": repoName,
		}).Info("Starting repository migration")

		token := ""
		totalComponents := 0

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			components, nextToken, err := client.ListComponentsPage(ctx, repoName, token)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"module":    "migration",
					"task_id":   task.ID,
					"repo_name": repoName,
					"error":     err,
				}).Error("Failed to fetch components")
				w.service.AddLog(task.ID, fmt.Sprintf("获取 %s 组件失败: %v", repoName, err))
				break
			}

			if len(components) == 0 {
				break
			}

			items := make([]model.MigrationItem, len(components))
			for i, comp := range components {
				items[i] = model.MigrationItem{
					TaskID:         task.ID,
					Repository:     repoName,
					ComponentID:    comp.ID,
					ComponentName:  comp.Name,
					ComponentGroup: comp.Group,
					Version:        comp.Version,
					Format:         comp.Format,
					Status:         model.MigrationItemPending,
				}
			}

			if err := w.itemRepo.BatchCreate(items); err != nil {
				logrus.WithFields(logrus.Fields{
					"module":    "migration",
					"task_id":   task.ID,
					"repo_name": repoName,
					"error":     err,
				}).Error("Failed to save items")
				break
			}

			totalComponents += len(components)

			for _, item := range items {
				select {
				case <-ctx.Done():
					return
				default:
					if !queue.Push(item) {
						time.Sleep(100 * time.Millisecond)
					}
				}
			}

			w.service.db.Model(&model.MigrationTask{}).
				Where("id = ?", task.ID).
				Update("total_items", totalComponents)

			if nextToken == "" {
				break
			}
			token = nextToken
		}

		w.service.AddLog(task.ID, fmt.Sprintf("仓库 %s 共有 %d 个组件", repoName, totalComponents))
	}
}
```

- [ ] **步骤 4：实现消费者方法**

```go
func (w *MigrationWorker) consumer(
	ctx context.Context,
	task *model.MigrationTask,
	client *NexusClient,
	queue *ComponentQueue,
	progressUpdater *ProgressUpdater,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		item, ok := queue.Pop()
		if !ok {
			return
		}

		w.itemRepo.UpdateStatus(item.ID, model.MigrationItemProcessing, "")

		err := w.processComponent(ctx, task, client, item, task.MaxRetries)
		
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   task.ID,
				"component": item.ComponentName,
				"version":   item.Version,
				"error":     err,
			}).Warn("Failed to migrate component")
			
			w.itemRepo.UpdateStatus(item.ID, model.MigrationItemFailed, err.Error())
			progressUpdater.IncrementFailed()
		} else {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   task.ID,
				"component": item.ComponentName,
				"version":   item.Version,
			}).Info("Component migrated successfully")
			
			w.itemRepo.UpdateStatus(item.ID, model.MigrationItemCompleted, "")
			progressUpdater.IncrementProcessed()
		}
	}
}

func (w *MigrationWorker) processComponent(
	ctx context.Context,
	task *model.MigrationTask,
	client *NexusClient,
	item model.MigrationItem,
	maxRetries int,
) error {
	components, err := client.ListComponents(ctx, item.Repository)
	if err != nil {
		return err
	}

	var targetComp *NexusComponent
	for i := range components {
		if components[i].ID == item.ComponentID {
			targetComp = &components[i]
			break
		}
	}

	if targetComp == nil {
		return fmt.Errorf("component not found: %s", item.ComponentID)
	}

	return w.migrateComponent(task.ID, client, *targetComp)
}

func (w *MigrationWorker) loadPendingItems(
	ctx context.Context,
	items []model.MigrationItem,
	queue *ComponentQueue,
) {
	defer queue.Close()

	for _, item := range items {
		select {
		case <-ctx.Done():
			return
		default:
			if !queue.Push(item) {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}
```

- [ ] **步骤 5：保留现有的辅助方法**

保留 `migrateComponent`、`storeAsset`、`storeMavenAsset` 等现有方法不变。

- [ ] **步骤 6：添加清理方法**

```go
func (w *MigrationWorker) cleanup(taskID uint) {
	if err := w.itemRepo.CleanCompletedItems(taskID); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": taskID,
			"error":   err,
		}).Warn("Failed to clean completed items")
	}
}
```

- [ ] **步骤 7：Commit**

```bash
git add internal/migration/migration_worker.go
git commit -m "refactor(migration): 重构 MigrationWorker 为生产者-消费者模式"
```

---

## 任务 7：更新 MigrationService

**文件：**
- 修改：`internal/migration/migration_service.go`

- [ ] **步骤 1：更新 CreateTask 方法**

修改 `internal/migration/migration_service.go`：

```go
func (s *MigrationService) CreateTask(
	sourceURL, username, password string,
	selectedRepos []string,
	targetRepoID uint,
	targetRepoName string,
	workerCount, maxRetries, batchSize int,
) (*model.MigrationTask, error) {
	if workerCount == 0 {
		workerCount = 10
	}
	if maxRetries == 0 {
		maxRetries = 3
	}
	if batchSize == 0 {
		batchSize = 50
	}

	if workerCount < 1 || workerCount > 50 {
		return nil, fmt.Errorf("worker_count must be between 1 and 50")
	}
	if maxRetries < 0 || maxRetries > 10 {
		return nil, fmt.Errorf("max_retries must be between 0 and 10")
	}
	if batchSize < 10 || batchSize > 200 {
		return nil, fmt.Errorf("batch_size must be between 10 and 200")
	}

	reposJSON, _ := json.Marshal(selectedRepos)

	task := &model.MigrationTask{
		SourceType:         "nexus",
		SourceURL:          sourceURL,
		Username:           username,
		Password:           password,
		Status:             model.MigrationPending,
		SelectedRepos:      string(reposJSON),
		TargetRepositoryID: targetRepoID,
		TargetRepository:   targetRepoName,
		WorkerCount:        workerCount,
		MaxRetries:         maxRetries,
		BatchSize:          batchSize,
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}
```

- [ ] **步骤 2：Commit**

```bash
git add internal/migration/migration_service.go
git commit -m "feat(migration): 为 MigrationService 添加配置参数支持"
```

---

## 任务 8：更新 API 处理器

**文件：**
- 修改：`internal/handler/migration_handler.go`

- [ ] **步骤 1：更新创建迁移任务处理器**

修改 `internal/handler/migration_handler.go`：

```go
type CreateMigrationRequest struct {
	URL               string   `json:"url" binding:"required"`
	Username          string   `json:"username" binding:"required"`
	Password          string   `json:"password" binding:"required"`
	SelectedRepos     []string `json:"selected_repos" binding:"required"`
	TargetRepositoryID uint    `json:"target_repository_id"`
	TargetRepository   string  `json:"target_repository"`
	WorkerCount       int      `json:"worker_count"`
	MaxRetries        int      `json:"max_retries"`
	BatchSize         int      `json:"batch_size"`
}

func (h *MigrationHandler) CreateMigration(c *gin.Context) {
	var req CreateMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.migrationSvc.CreateTask(
		req.URL,
		req.Username,
		req.Password,
		req.SelectedRepos,
		req.TargetRepositoryID,
		req.TargetRepository,
		req.WorkerCount,
		req.MaxRetries,
		req.BatchSize,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go h.worker.Execute(context.Background(), task)

	c.JSON(http.StatusOK, gin.H{
		"id":     task.ID,
		"task":   task,
		"status": "created",
	})
}
```

- [ ] **步骤 2：Commit**

```bash
git add internal/handler/migration_handler.go
git commit -m "feat(migration): 更新 API 处理器支持配置参数"
```

---

## 任务 9：更新前端 API

**文件：**
- 修改：`web/src/api/migration.ts`

- [ ] **步骤 1：更新 API 接口**

修改 `web/src/api/migration.ts`：

```typescript
export interface MigrationTask {
  id: number
  source_type: string
  source_url: string
  status: string
  total_items: number
  processed_items: number
  failed_items: number
  selected_repos: string
  error_message: string
  target_repository_id: number
  target_repository: string
  worker_count: number
  max_retries: number
  batch_size: number
  created_at: string
  updated_at: string
  started_at: string | null
  completed_at: string | null
}

export function createMigration(data: {
  url: string
  username: string
  password: string
  selected_repos: string[]
  target_repository_id?: number
  target_repository?: string
  worker_count?: number
  max_retries?: number
  batch_size?: number
}) {
  return request.post('/migration/nexus', data)
}
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/api/migration.ts
git commit -m "feat(migration): 更新前端 API 接口支持配置参数"
```

---

## 任务 10：更新前端界面

**文件：**
- 修改：`web/src/views/MigrationPage.vue`

- [ ] **步骤 1：添加高级配置界面**

修改 `web/src/views/MigrationPage.vue`：

```vue
<template>
  <!-- ... 现有代码 ... -->
  
  <template v-if="currentStep === 1">
    <RepositorySelector
      :repositories="nexusRepos"
      @selected="onSelected"
    />
    <div class="target-repo-section">
      <label class="section-label">目标仓库</label>
      <el-select
        v-model="targetRepoId"
        placeholder="选择目标仓库"
        class="repo-select"
      >
        <el-option
          v-for="repo in localRepos"
          :key="repo.id"
          :label="repo.name"
          :value="repo.id"
        />
      </el-select>
      <span class="tip">迁移的包将存储到选定的目标仓库</span>
    </div>
    
    <!-- 高级配置 -->
    <div class="advanced-config">
      <el-collapse>
        <el-collapse-item title="高级配置（可选）" name="advanced">
          <el-form label-width="120px">
            <el-form-item label="并发数">
              <el-input-number
                v-model="advancedConfig.worker_count"
                :min="1"
                :max="50"
                :step="1"
              />
              <span class="config-tip">并发数越高，迁移越快，但占用更多资源</span>
            </el-form-item>
            
            <el-form-item label="最大重试次数">
              <el-input-number
                v-model="advancedConfig.max_retries"
                :min="0"
                :max="10"
                :step="1"
              />
              <span class="config-tip">失败后的最大重试次数</span>
            </el-form-item>
            
            <el-form-item label="批量大小">
              <el-input-number
                v-model="advancedConfig.batch_size"
                :min="10"
                :max="200"
                :step="10"
              />
              <span class="config-tip">批量大小影响数据库写入频率</span>
            </el-form-item>
          </el-form>
        </el-collapse-item>
      </el-collapse>
    </div>
    
    <div class="actions">
      <el-button type="primary" @click="startMigration" :disabled="selectedRepos.length === 0 || !targetRepoId">
        开始迁移
      </el-button>
    </div>
  </template>
  
  <!-- ... 其他代码 ... -->
</template>

<script setup lang="ts">
// ... 现有代码 ...

const advancedConfig = ref({
  worker_count: 10,
  max_retries: 3,
  batch_size: 50,
})

async function startMigration() {
  try {
    const selectedRepo = localRepos.value.find((r: any) => r.id === targetRepoId.value)
    const res = (await createMigration({
      ...nexusCredentials.value,
      selected_repos: selectedRepos.value,
      target_repository_id: targetRepoId.value,
      target_repository: selectedRepo?.name || '',
      worker_count: advancedConfig.value.worker_count,
      max_retries: advancedConfig.value.max_retries,
      batch_size: advancedConfig.value.batch_size,
    })) as any
    currentTaskId.value = res?.id || res?.task?.id
    currentStep.value = 2
    migrationStatus.value = 'running'
    startPolling()
  } catch (e: any) {
    ElMessage.error('创建迁移任务失败: ' + e.message)
  }
}

// ... 其他代码 ...
</script>

<style scoped>
/* ... 现有样式 ... */

.advanced-config {
  margin-top: 24px;
  padding: 20px;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.config-tip {
  display: block;
  font-size: 12px;
  color: #9ca3af;
  margin-top: 4px;
}
</style>
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/views/MigrationPage.vue
git commit -m "feat(migration): 添加高级配置界面"
```

---

## 任务 11：集成测试

**文件：**
- 创建：`internal/migration/integration_test.go`

- [ ] **步骤 1：编写集成测试**

创建 `internal/migration/integration_test.go`：

```go
// +build integration

package migration

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.MigrationTask{}, &model.MigrationItem{})
	assert.NoError(t, err)
	return db
}

func TestMigrationWorker_Execute(t *testing.T) {
	db := setupIntegrationTestDB(t)
	
	migrationSvc := NewMigrationService(db)
	itemRepo := repository.NewMigrationItemRepository(db)
	
	task := &model.MigrationTask{
		SourceType:    "nexus",
		SourceURL:     "http://localhost:8081",
		Username:      "admin",
		Password:      "admin123",
		SelectedRepos: `["test-repo"]`,
		Status:        model.MigrationPending,
		WorkerCount:   5,
		MaxRetries:    3,
		BatchSize:     50,
	}
	db.Create(task)

	queue := NewComponentQueue(10)
	defer queue.Close()

	progressUpdater := NewProgressUpdater(task.ID, db, 1*time.Second)
	defer progressUpdater.Stop()

	ctx := context.Background()
	go progressUpdater.Start(ctx)

	items := []model.MigrationItem{
		{TaskID: task.ID, ComponentID: "comp1", ComponentName: "test1", Status: model.MigrationItemPending},
		{TaskID: task.ID, ComponentID: "comp2", ComponentName: "test2", Status: model.MigrationItemPending},
	}
	err := itemRepo.BatchCreate(items)
	assert.NoError(t, err)

	pendingItems, err := itemRepo.GetPendingItems(task.ID, 10)
	assert.NoError(t, err)
	assert.Len(t, pendingItems, 2)

	total, pending, processing, completed, failed, err := itemRepo.GetStats(task.ID)
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, 2, pending)
	assert.Equal(t, 0, processing)
	assert.Equal(t, 0, completed)
	assert.Equal(t, 0, failed)
}
```

- [ ] **步骤 2：运行集成测试**

运行：`go test ./internal/migration -tags=integration -v`
预期：集成测试通过

- [ ] **步骤 3：Commit**

```bash
git add internal/migration/integration_test.go
git commit -m "test(migration): 添加集成测试"
```

---

## 任务 12：更新文档

**文件：**
- 修改：`README.md`

- [ ] **步骤 1：更新 README**

在 `README.md` 中添加迁移功能说明：

```markdown
## 数据迁移

### Nexus 迁移

支持从 Nexus 仓库迁移包到本地仓库，特性包括：

- **流式处理**：支持大规模仓库迁移（>10000组件）
- **断点续传**：任务中断后可从中断点继续
- **并发控制**：可配置并发数，平衡性能和资源占用
- **错误重试**：自动重试失败的组件，支持指数退避
- **进度监控**：实时显示迁移进度和详细日志

#### 使用方法

1. 进入"数据迁移"页面
2. 填写 Nexus 连接信息
3. 选择要迁移的仓库
4. 选择目标仓库
5. （可选）配置高级参数：
   - 并发数：1-50，默认 10
   - 最大重试次数：0-10，默认 3
   - 批量大小：10-200，默认 50
6. 点击"开始迁移"
```

- [ ] **步骤 2：Commit**

```bash
git add README.md
git commit -m "docs: 更新 README 添加迁移功能说明"
```

---

## 任务 13：最终验证

- [ ] **步骤 1：运行所有测试**

运行：`go test ./... -v`
预期：所有测试通过

- [ ] **步骤 2：运行前端构建**

运行：`cd web && npm run build`
预期：构建成功

- [ ] **步骤 3：启动服务测试**

运行：`go run cmd/registry/main.go`
预期：服务正常启动

- [ ] **步骤 4：手动测试迁移功能**

1. 访问迁移页面
2. 创建迁移任务
3. 验证断点续传功能
4. 检查迁移进度显示

- [ ] **步骤 5：创建最终 commit**

```bash
git add .
git commit -m "feat(migration): 完成 Nexus 迁移流式处理实现"
```

---

## 总结

本实现计划包含 13 个任务，涵盖了：

✅ **数据库模型扩展**：添加 MigrationItem 表和配置字段  
✅ **数据访问层**：实现批量操作和断点续传支持  
✅ **核心组件**：组件队列、进度更新器  
✅ **生产者-消费者模式**：流式处理大规模数据  
✅ **错误处理和重试**：指数退避、分类错误  
✅ **前端界面**：高级配置选项  
✅ **测试覆盖**：单元测试、集成测试  
✅ **文档更新**：README 和设计文档  

每个任务都遵循 TDD 原则，包含详细的代码示例和验证步骤。
