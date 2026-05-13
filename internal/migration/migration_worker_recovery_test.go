package migration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWorkerRecoveryTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.MigrationTask{}, &model.MigrationItem{})
	assert.NoError(t, err)
	return db
}

func TestExecute_SkipRescanWhenItemsExist(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationRunning,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     `["npm-proxy"]`,
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	for i := 0; i < 5; i++ {
		item := &model.MigrationItem{
			TaskID:        task.ID,
			Repository:    "npm-proxy",
			ComponentID:   "comp-existing-" + string(rune('0'+i)),
			ComponentName: "existing-pkg",
			Version:       "1.0.0",
			Format:        "npm",
			Status:        model.MigrationItemPending,
		}
		err := db.Create(item).Error
		assert.NoError(t, err)
	}

	itemRepo := repository.NewMigrationItemRepository(db)
	count := itemRepo.CountByTaskID(task.ID)
	assert.Equal(t, int64(5), count)

	var pendingItems []model.MigrationItem
	err = db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemPending).Find(&pendingItems).Error
	assert.NoError(t, err)
	assert.Len(t, pendingItems, 5)
}

func TestBatchCreateDeduplication(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	itemRepo := repository.NewMigrationItemRepository(db)

	items := []model.MigrationItem{
		{TaskID: 1, Repository: "npm-proxy", ComponentID: "comp-001", ComponentName: "pkg-a", Version: "1.0.0", Format: "npm", Status: model.MigrationItemPending},
		{TaskID: 1, Repository: "npm-proxy", ComponentID: "comp-002", ComponentName: "pkg-b", Version: "1.0.0", Format: "npm", Status: model.MigrationItemPending},
		{TaskID: 1, Repository: "npm-proxy", ComponentID: "comp-003", ComponentName: "pkg-c", Version: "1.0.0", Format: "npm", Status: model.MigrationItemPending},
	}

	err := itemRepo.BatchCreate(items)
	assert.NoError(t, err)

	count := itemRepo.CountByTaskID(1)
	assert.Equal(t, int64(3), count)

	duplicateItems := []model.MigrationItem{
		{TaskID: 1, Repository: "npm-proxy", ComponentID: "comp-001", ComponentName: "pkg-a", Version: "1.0.0", Format: "npm", Status: model.MigrationItemPending},
		{TaskID: 1, Repository: "npm-proxy", ComponentID: "comp-002", ComponentName: "pkg-b", Version: "1.0.0", Format: "npm", Status: model.MigrationItemPending},
		{TaskID: 1, Repository: "npm-proxy", ComponentID: "comp-004", ComponentName: "pkg-d", Version: "1.0.0", Format: "npm", Status: model.MigrationItemPending},
	}

	err = itemRepo.BatchCreate(duplicateItems)
	assert.NoError(t, err)

	count = itemRepo.CountByTaskID(1)
	assert.Equal(t, int64(4), count)

	var comp4 model.MigrationItem
	err = db.Where("component_id = ?", "comp-004").First(&comp4).Error
	assert.NoError(t, err)
	assert.Equal(t, "pkg-d", comp4.ComponentName)
}

func TestCountByTaskID(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task := &model.MigrationTask{
		SourceType:    "nexus",
		SourceURL:     "http://nexus.example.com",
		Username:      "admin",
		Status:        model.MigrationQueued,
		TaskType:      model.MigrationTaskFull,
		SelectedRepos: `["npm-proxy"]`,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		item := &model.MigrationItem{
			TaskID:        task.ID,
			Repository:    "npm-proxy",
			ComponentID:   "comp-count-" + string(rune('0'+i)),
			ComponentName: "count-pkg",
			Version:       "1.0.0",
			Format:        "npm",
			Status:        model.MigrationItemPending,
		}
		db.Create(item)
	}

	itemRepo := repository.NewMigrationItemRepository(db)
	count := itemRepo.CountByTaskID(task.ID)
	assert.Equal(t, int64(10), count)

	var completedItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemCompleted).Find(&completedItems)
	assert.Len(t, completedItems, 0)
}

func TestRecoveryWithPartialScan(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationRunning,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     `["npm-proxy", "npm-local"]`,
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		item := &model.MigrationItem{
			TaskID:        task.ID,
			Repository:    "npm-proxy",
			ComponentID:   "comp-partial-" + string(rune('0'+i)),
			ComponentName: "partial-pkg",
			Version:       "1.0.0",
			Format:        "npm",
			Status:        model.MigrationItemPending,
		}
		db.Create(item)
	}

	itemRepo := repository.NewMigrationItemRepository(db)
	count := itemRepo.CountByTaskID(task.ID)
	assert.Equal(t, int64(3), count)

	var pendingItems []model.MigrationItem
	err = db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemPending).Find(&pendingItems).Error
	assert.NoError(t, err)
	assert.Len(t, pendingItems, 3)
}

func TestRecoveryWithAllStatuses(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationRunning,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     `["npm-proxy"]`,
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	statuses := []model.MigrationItemStatus{
		model.MigrationItemPending,
		model.MigrationItemProcessing,
		model.MigrationItemCompleted,
		model.MigrationItemFailed,
	}

	for i, status := range statuses {
		item := &model.MigrationItem{
			TaskID:        task.ID,
			Repository:    "npm-proxy",
			ComponentID:   "comp-status-" + string(rune('0'+i)),
			ComponentName: "status-pkg",
			Version:       "1.0.0",
			Format:        "npm",
			Status:        status,
		}
		db.Create(item)
	}

	itemRepo := repository.NewMigrationItemRepository(db)
	total := itemRepo.CountByTaskID(task.ID)
	assert.Equal(t, int64(4), total)

	var pendingItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemPending).Find(&pendingItems)
	assert.Len(t, pendingItems, 1)

	var processingItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemProcessing).Find(&processingItems)
	assert.Len(t, processingItems, 1)

	var completedItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemCompleted).Find(&completedItems)
	assert.Len(t, completedItems, 1)

	var failedItems []model.MigrationItem
	db.Where("task_id = ? AND status = ?", task.ID, model.MigrationItemFailed).Find(&failedItems)
	assert.Len(t, failedItems, 1)
}

func TestRecoveryWithSelectedReposJSON(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	selectedRepos := []string{"npm-proxy", "npm-local", "maven-central"}
	reposJSON, err := json.Marshal(selectedRepos)
	assert.NoError(t, err)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationQueued,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     string(reposJSON),
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err = db.Create(task).Error
	assert.NoError(t, err)

	var parsedRepos []string
	err = json.Unmarshal([]byte(task.SelectedRepos), &parsedRepos)
	assert.NoError(t, err)
	assert.Len(t, parsedRepos, 3)
	assert.Equal(t, "npm-proxy", parsedRepos[0])
	assert.Equal(t, "npm-local", parsedRepos[1])
	assert.Equal(t, "maven-central", parsedRepos[2])
}

func TestRecoveryWithEmptyItems(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationQueued,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     `["npm-proxy"]`,
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	itemRepo := repository.NewMigrationItemRepository(db)
	count := itemRepo.CountByTaskID(task.ID)
	assert.Equal(t, int64(0), count)
}

func TestRecoveryProcessingItemsReset(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationRunning,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     `["npm-proxy"]`,
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	processingItem := &model.MigrationItem{
		TaskID:        task.ID,
		Repository:    "npm-proxy",
		ComponentID:   "comp-processing-reset",
		ComponentName: "processing-pkg",
		Version:       "1.0.0",
		Format:        "npm",
		Status:        model.MigrationItemProcessing,
		RetryCount:    2,
		ErrorMessage:  "timeout",
	}
	err = db.Create(processingItem).Error
	assert.NoError(t, err)

	resetResult := db.Model(&model.MigrationItem{}).
		Where("task_id = ? AND status = ?", task.ID, model.MigrationItemProcessing).
		Updates(map[string]interface{}{
			"status":        model.MigrationItemPending,
			"error_message": "",
			"retry_count":   0,
		})
	assert.NoError(t, resetResult.Error)
	assert.Equal(t, int64(1), resetResult.RowsAffected)

	var resetItem model.MigrationItem
	db.Where("id = ?", processingItem.ID).First(&resetItem)
	assert.Equal(t, model.MigrationItemPending, resetItem.Status)
	assert.Equal(t, 0, resetItem.RetryCount)
	assert.Empty(t, resetItem.ErrorMessage)
}

func TestCountByTaskIDWithMultipleTasks(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	task1 := &model.MigrationTask{
		SourceType:    "nexus",
		SourceURL:     "http://nexus1.example.com",
		Username:      "admin",
		Status:        model.MigrationQueued,
		TaskType:      model.MigrationTaskFull,
		SelectedRepos: `["npm-proxy"]`,
	}
	db.Create(task1)

	task2 := &model.MigrationTask{
		SourceType:    "nexus",
		SourceURL:     "http://nexus2.example.com",
		Username:      "admin",
		Status:        model.MigrationQueued,
		TaskType:      model.MigrationTaskFull,
		SelectedRepos: `["maven-central"]`,
	}
	db.Create(task2)

	for i := 0; i < 5; i++ {
		item := &model.MigrationItem{
			TaskID:        task1.ID,
			Repository:    "npm-proxy",
			ComponentID:   "comp-task1-" + string(rune('0'+i)),
			ComponentName: "task1-pkg",
			Version:       "1.0.0",
			Format:        "npm",
			Status:        model.MigrationItemPending,
		}
		db.Create(item)
	}

	for i := 0; i < 3; i++ {
		item := &model.MigrationItem{
			TaskID:        task2.ID,
			Repository:    "maven-central",
			ComponentID:   "comp-task2-" + string(rune('0'+i)),
			ComponentName: "task2-pkg",
			Version:       "1.0.0",
			Format:        "maven2",
			Status:        model.MigrationItemPending,
		}
		db.Create(item)
	}

	itemRepo := repository.NewMigrationItemRepository(db)
	count1 := itemRepo.CountByTaskID(task1.ID)
	count2 := itemRepo.CountByTaskID(task2.ID)

	assert.Equal(t, int64(5), count1)
	assert.Equal(t, int64(3), count2)
}

type mockMigrationWorkerForRecovery struct{}

func (w *mockMigrationWorkerForRecovery) Execute(ctx context.Context, task *model.MigrationTask) error {
	return nil
}

func (w *mockMigrationWorkerForRecovery) RetryFailed(ctx context.Context, task *model.MigrationTask) error {
	return nil
}

type mockRepositoryCreatorForRecovery struct{}

func (r *mockRepositoryCreatorForRecovery) CreateRepo(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint) error {
	return nil
}

func (r *mockRepositoryCreatorForRecovery) RepoExists(name string) bool {
	return false
}

func (r *mockRepositoryCreatorForRecovery) FindDefaultStorageBackendID() (*uint, error) {
	return nil, nil
}

func TestIntegration_RecoverySkipRescan(t *testing.T) {
	db := setupWorkerRecoveryTestDB(t)

	mockWorker := &mockMigrationWorkerForRecovery{}
	mockRepoCreator := &mockRepositoryCreatorForRecovery{}

	svc := NewMigrationService(db, mockWorker, mockRepoCreator, 1)

	task := &model.MigrationTask{
		SourceType:        "nexus",
		SourceURL:         "http://nexus.example.com",
		Username:          "admin",
		PasswordEncrypted: "encrypted",
		Status:            model.MigrationRunning,
		TaskType:          model.MigrationTaskFull,
		SelectedRepos:     `["npm-proxy"]`,
		WorkerCount:       3,
		MaxRetries:        2,
		BatchSize:         50,
	}
	err := db.Create(task).Error
	assert.NoError(t, err)

	for i := 0; i < 5; i++ {
		item := &model.MigrationItem{
			TaskID:        task.ID,
			Repository:    "npm-proxy",
			ComponentID:   "comp-integration-" + string(rune('0'+i)),
			ComponentName: "integration-pkg",
			Version:       "1.0.0",
			Format:        "npm",
			Status:        model.MigrationItemPending,
		}
		db.Create(item)
	}

	svc.StartQueueWithRecovery()

	time.Sleep(100 * time.Millisecond)

	var updatedTask model.MigrationTask
	err = db.First(&updatedTask, task.ID).Error
	assert.NoError(t, err)
	assert.Contains(t, []model.MigrationStatus{model.MigrationQueued, model.MigrationRunning, model.MigrationCompleted}, updatedTask.Status)

	itemRepo := repository.NewMigrationItemRepository(db)
	count := itemRepo.CountByTaskID(task.ID)
	assert.Equal(t, int64(5), count)
}
