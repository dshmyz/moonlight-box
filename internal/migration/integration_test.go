package migration

import (
	"context"
	"testing"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/stretchr/testify/assert"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
)

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(gormlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.MigrationTask{}, &model.MigrationItem{})
	assert.NoError(t, err)
	return db
}

func TestComponentQueue_Integration(t *testing.T) {
	queue := NewComponentQueue(10)
	defer queue.Close()

	go func() {
		for i := 0; i < 5; i++ {
			item := model.MigrationItem{
				TaskID:        1,
				ComponentID:   string(rune('A' + i)),
				ComponentName: "test-component",
				Status:        model.MigrationItemPending,
			}
			assert.True(t, queue.Push(item))
		}
	}()

	count := 0
	for i := 0; i < 5; i++ {
		item, ok := queue.Pop()
		if ok {
			count++
			assert.Equal(t, "test-component", item.ComponentName)
		}
	}
	assert.Equal(t, 5, count)
}

func TestProgressUpdater_Integration(t *testing.T) {
	db := setupIntegrationTestDB(t)
	task := &model.MigrationTask{Status: model.MigrationRunning}
	db.Create(task)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	itemRepo := repository.NewMigrationItemRepository(db)
	updater := NewProgressUpdater(task.ID, db, itemRepo, 100*time.Millisecond)
	go updater.Start(ctx)

	for i := 0; i < 10; i++ {
		updater.IncrementProcessed()
	}

	time.Sleep(200 * time.Millisecond)

	updater.Stop()

	var updated model.MigrationTask
	db.First(&updated, task.ID)
	assert.Equal(t, 10, updated.ProcessedItems)
}

func TestMigrationItemRepository_Integration(t *testing.T) {
	db := setupIntegrationTestDB(t)
	repo := repository.NewMigrationItemRepository(db)

	items := []model.MigrationItem{
		{TaskID: 1, ComponentID: "comp1", ComponentName: "test1", Status: model.MigrationItemPending},
		{TaskID: 1, ComponentID: "comp2", ComponentName: "test2", Status: model.MigrationItemPending},
		{TaskID: 1, ComponentID: "comp3", ComponentName: "test3", Status: model.MigrationItemPending},
	}

	err := repo.BatchCreate(items)
	assert.NoError(t, err)

	pending, err := repo.GetPendingItems(1, 10, 10)
	assert.NoError(t, err)
	assert.Len(t, pending, 3)

	err = repo.UpdateStatus(pending[0].ID, model.MigrationItemCompleted, "")
	assert.NoError(t, err)

	err = repo.UpdateStatus(pending[1].ID, model.MigrationItemFailed, "test error")
	assert.NoError(t, err)

	total, pendingCount, processing, completed, failed, err := repo.GetStats(1)
	assert.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, 1, pendingCount)
	assert.Equal(t, 0, processing)
	assert.Equal(t, 1, completed)
	assert.Equal(t, 1, failed)
}

func TestProducerConsumer_Integration(t *testing.T) {
	db := setupIntegrationTestDB(t)
	repo := repository.NewMigrationItemRepository(db)

	task := &model.MigrationTask{
		Status:      model.MigrationRunning,
		WorkerCount: 2,
		MaxRetries:  3,
		BatchSize:   10,
	}
	db.Create(task)

	queue := NewComponentQueue(20)

	produceDone := make(chan struct{})
	go func() {
		defer close(produceDone)
		items := []model.MigrationItem{
			{TaskID: task.ID, ComponentID: "comp1", ComponentName: "test1", Status: model.MigrationItemPending},
			{TaskID: task.ID, ComponentID: "comp2", ComponentName: "test2", Status: model.MigrationItemPending},
			{TaskID: task.ID, ComponentID: "comp3", ComponentName: "test3", Status: model.MigrationItemPending},
		}
		repo.BatchCreate(items)

		pendingItems, _ := repo.GetPendingItems(task.ID, 10, 100)
		for _, item := range pendingItems {
			queue.Push(item)
		}
	}()

	<-produceDone

	processedCount := 0
	for i := 0; i < 3; i++ {
		item, ok := queue.Pop()
		if ok {
			repo.UpdateStatus(item.ID, model.MigrationItemCompleted, "")
			processedCount++
		}
	}

	queue.Close()

	assert.Equal(t, 3, processedCount)

	total, _, _, completed, _, err := repo.GetStats(task.ID)
	assert.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, 3, completed)
}

func TestRetryMechanism_Integration(t *testing.T) {
	db := setupIntegrationTestDB(t)
	repo := repository.NewMigrationItemRepository(db)

	item := model.MigrationItem{
		TaskID:        1,
		ComponentID:   "comp1",
		ComponentName: "test",
		Status:        model.MigrationItemPending,
		RetryCount:    0,
	}
	db.Create(&item)

	for i := 0; i < 3; i++ {
		err := repo.UpdateStatus(item.ID, model.MigrationItemFailed, "test error")
		assert.NoError(t, err)

		var updated model.MigrationItem
		db.First(&updated, item.ID)
		assert.Equal(t, i+1, updated.RetryCount)
	}

	pending, err := repo.GetPendingItems(1, 3, 10)
	assert.NoError(t, err)
	assert.Len(t, pending, 0)

	pendingWithHighLimit, err := repo.GetPendingItems(1, 10, 10)
	assert.NoError(t, err)
	assert.Len(t, pendingWithHighLimit, 1)
}

func TestProgressBuffer_Integration(t *testing.T) {
	db := setupIntegrationTestDB(t)
	task := &model.MigrationTask{Status: model.MigrationRunning}
	db.Create(task)

	itemRepo := repository.NewMigrationItemRepository(db)
	updater := NewProgressUpdater(task.ID, db, itemRepo, 1*time.Second)

	for i := 0; i < 100; i++ {
		updater.IncrementProcessed()
		if i%2 == 0 {
			updater.IncrementFailed()
		}
	}

	updater.Stop()

	var updated model.MigrationTask
	db.First(&updated, task.ID)
	assert.Equal(t, 100, updated.ProcessedItems)
	assert.Equal(t, 50, updated.FailedItems)
}
