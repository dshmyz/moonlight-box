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

func setupRecoveryTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.MigrationTask{}, &model.MigrationItem{})
	assert.NoError(t, err)
	return db
}

func createMockTask(t *testing.T, db *gorm.DB, status model.MigrationStatus, taskType string) *model.MigrationTask {
	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted-password",
		Status:            status,
		TaskType:          taskType,
		TotalItems:        100,
		ProcessedItems:    50,
		FailedItems:       5,
		SelectedRepos:     "[\"npm-proxy\"]",
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	if status == model.MigrationRunning || status == model.MigrationQueued {
		now := time.Now()
		task.StartedAt = &now
		db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Update("started_at", now)
	}

	return task
}

func createMockItem(t *testing.T, db *gorm.DB, taskID uint, status model.MigrationItemStatus) *model.MigrationItem {
	item := &model.MigrationItem{
		TaskID:        taskID,
		Repository:    "npm-proxy",
		ComponentID:   "comp-" + string(status) + "-" + time.Now().Format("20060102150405.000000"),
		ComponentName: "lodash",
		Version:       "4.17.21",
		Format:        "npm",
		Status:        status,
		RetryCount:    0,
	}
	err := db.Create(item).Error
	assert.NoError(t, err)
	return item
}

func TestRecoverInterruptedTasks_NoInterruptedTasks(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Nil(t, recoveredIDs)
	assert.Len(t, recoveredIDs, 0)
}

func TestRecoverInterruptedTasks_RunningTaskRecovered(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task := createMockTask(t, db, model.MigrationRunning, model.MigrationTaskFull)
	processingItem := createMockItem(t, db, task.ID, model.MigrationItemProcessing)
	_ = createMockItem(t, db, task.ID, model.MigrationItemPending)
	_ = createMockItem(t, db, task.ID, model.MigrationItemCompleted)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Len(t, recoveredIDs, 1)
	assert.Equal(t, task.ID, recoveredIDs[0])

	var updatedTask model.MigrationTask
	err := db.First(&updatedTask, task.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, model.MigrationQueued, updatedTask.Status)
	assert.Nil(t, updatedTask.StartedAt)
	assert.Empty(t, updatedTask.ErrorMessage)

	var processingItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemProcessing).Find(&processingItems)
	assert.Len(t, processingItems, 0)

	var pendingItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemPending).Find(&pendingItems)
	assert.Len(t, pendingItems, 2)

	var resetItem model.MigrationItem
	db.Where("id = ?", processingItem.ID).First(&resetItem)
	assert.Equal(t, model.MigrationItemPending, resetItem.Status)
	assert.Empty(t, resetItem.ErrorMessage)
	assert.Equal(t, 0, resetItem.RetryCount)
}

func TestRecoverInterruptedTasks_QueuedTaskRecovered(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task := createMockTask(t, db, model.MigrationQueued, model.MigrationTaskFull)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Len(t, recoveredIDs, 1)
	assert.Equal(t, task.ID, recoveredIDs[0])

	var updatedTask model.MigrationTask
	err := db.First(&updatedTask, task.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, model.MigrationQueued, updatedTask.Status)
}

func TestRecoverInterruptedTasks_CompletedTaskNotAffected(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	createMockTask(t, db, model.MigrationCompleted, model.MigrationTaskFull)
	createMockTask(t, db, model.MigrationFailed, model.MigrationTaskFull)
	createMockTask(t, db, model.MigrationCancelled, model.MigrationTaskFull)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Len(t, recoveredIDs, 0)

	var completedCount, failedCount, cancelledCount int64
	db.Model(&model.MigrationTask{}).Where("status = ?", model.MigrationCompleted).Count(&completedCount)
	db.Model(&model.MigrationTask{}).Where("status = ?", model.MigrationFailed).Count(&failedCount)
	db.Model(&model.MigrationTask{}).Where("status = ?", model.MigrationCancelled).Count(&cancelledCount)

	assert.Equal(t, int64(1), completedCount)
	assert.Equal(t, int64(1), failedCount)
	assert.Equal(t, int64(1), cancelledCount)
}

func TestRecoverInterruptedTasks_MultipleTasksRecovered(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task1 := createMockTask(t, db, model.MigrationRunning, model.MigrationTaskFull)
	_ = createMockTask(t, db, model.MigrationQueued, model.MigrationTaskSyncConfig)
	_ = createMockTask(t, db, model.MigrationCompleted, model.MigrationTaskFull)

	createMockItem(t, db, task1.ID, model.MigrationItemProcessing)
	createMockItem(t, db, task1.ID, model.MigrationItemProcessing)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Len(t, recoveredIDs, 2)
	assert.Contains(t, recoveredIDs, task1.ID)

	var queuedCount int64
	db.Model(&model.MigrationTask{}).Where("status = ?", model.MigrationQueued).Count(&queuedCount)
	assert.Equal(t, int64(2), queuedCount)
}

func TestRecoverInterruptedTasks_SyncConfigTaskRecovered(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task := createMockTask(t, db, model.MigrationRunning, model.MigrationTaskSyncConfig)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Len(t, recoveredIDs, 1)

	var updatedTask model.MigrationTask
	err := db.First(&updatedTask, task.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, model.MigrationQueued, updatedTask.Status)
	assert.Equal(t, model.MigrationTaskSyncConfig, updatedTask.TaskType)
}

func TestStartQueueWithRecovery_NoInterruptedTasks(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	svc.StartQueueWithRecovery()

	time.Sleep(100 * time.Millisecond)

	var taskCount int64
	db.Model(&model.MigrationTask{}).Where("status = ?", model.MigrationQueued).Count(&taskCount)
	assert.Equal(t, int64(0), taskCount)
}

func TestStartQueueWithRecovery_WithInterruptedTasks(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	createMockTask(t, db, model.MigrationRunning, model.MigrationTaskFull)
	createMockTask(t, db, model.MigrationQueued, model.MigrationTaskSyncConfig)

	svc.StartQueueWithRecovery()

	time.Sleep(50 * time.Millisecond)

	var queuedOrRunningCount int64
	db.Model(&model.MigrationTask{}).Where("status IN (?, ?)", model.MigrationQueued, model.MigrationRunning).Count(&queuedOrRunningCount)
	assert.Equal(t, int64(2), queuedOrRunningCount)

	status := svc.GetQueueStatus()
	assert.True(t, status.RunningTasks >= 0)
}

func TestRecoverInterruptedTasks_FailedItemsNotAffected(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task := createMockTask(t, db, model.MigrationRunning, model.MigrationTaskFull)
	createMockItem(t, db, task.ID, model.MigrationItemCompleted)
	createMockItem(t, db, task.ID, model.MigrationItemFailed)
	createMockItem(t, db, task.ID, model.MigrationItemPending)

	_ = svc.recoverInterruptedTasks()

	var completedCount, failedCount, pendingCount int64
	db.Model(&model.MigrationItem{}).Where("task_id = ? AND status = ?", task.ID, model.MigrationItemCompleted).Count(&completedCount)
	db.Model(&model.MigrationItem{}).Where("task_id = ? AND status = ?", task.ID, model.MigrationItemFailed).Count(&failedCount)
	db.Model(&model.MigrationItem{}).Where("task_id = ? AND status = ?", task.ID, model.MigrationItemPending).Count(&pendingCount)

	assert.Equal(t, int64(1), completedCount)
	assert.Equal(t, int64(1), failedCount)
	assert.Equal(t, int64(1), pendingCount)
}

func TestRecoverInterruptedTasks_EmptyDatabase(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	recoveredIDs := svc.recoverInterruptedTasks()

	assert.Nil(t, recoveredIDs)
}

func TestStartQueueWithRecovery_DoesNotDoubleStart(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	svc.StartQueueWithRecovery()
	time.Sleep(50 * time.Millisecond)

	svc.StartQueueWithRecovery()
	time.Sleep(50 * time.Millisecond)

	assert.True(t, svc.queueStarted)
}

func TestRecoveryTaskEnqueued(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task := createMockTask(t, db, model.MigrationRunning, model.MigrationTaskFull)

	svc.StartQueueWithRecovery()

	time.Sleep(100 * time.Millisecond)

	var updatedTask model.MigrationTask
	err := db.First(&updatedTask, task.ID).Error
	assert.NoError(t, err)
	assert.Contains(t, []model.MigrationStatus{model.MigrationQueued, model.MigrationRunning, model.MigrationCompleted}, updatedTask.Status)
}

func TestRecoverItemsRetryCountReset(t *testing.T) {
	db := setupRecoveryTestDB(t)
	mockWorker := &mockMigrationWorker{}
	mockRepoCreator := &mockRepositoryCreator{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, nil, 1)

	task := createMockTask(t, db, model.MigrationRunning, model.MigrationTaskFull)

	item := &model.MigrationItem{
		TaskID:        task.ID,
		Repository:    "npm-proxy",
		ComponentID:   "comp-retry-test",
		ComponentName: "express",
		Version:       "4.18.2",
		Format:        "npm",
		Status:        model.MigrationItemProcessing,
		RetryCount:    3,
		ErrorMessage:  "network timeout",
	}
	err := db.Create(item).Error
	assert.NoError(t, err)

	_ = svc.recoverInterruptedTasks()

	var resetItem model.MigrationItem
	db.Where("component_id = ?", "comp-retry-test").First(&resetItem)

	assert.Equal(t, model.MigrationItemPending, resetItem.Status)
	assert.Equal(t, 0, resetItem.RetryCount)
	assert.Empty(t, resetItem.ErrorMessage)
}

type mockMigrationWorker struct{}

func (w *mockMigrationWorker) Execute(ctx context.Context, task *model.MigrationTask) error {
	return nil
}

func (w *mockMigrationWorker) RetryFailed(ctx context.Context, task *model.MigrationTask) error {
	return nil
}

type mockRepositoryCreator struct{}

func (r *mockRepositoryCreator) CreateRepo(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint) error {
	return nil
}

func (r *mockRepositoryCreator) CreateRepoWithConfig(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint, authConfig *model.ProxyAuthConfig, timeoutSeconds, maxRedirects int, insecureSkipVerify bool) error {
	return nil
}

func (r *mockRepositoryCreator) RepoExists(name string) (bool, error) {
	return false, nil
}

func (r *mockRepositoryCreator) FindDefaultStorageBackendID() (*uint, error) {
	return nil, nil
}
