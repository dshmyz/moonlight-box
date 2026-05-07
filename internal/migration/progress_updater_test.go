package migration

import (
	"context"
	"testing"
	"time"

	"github.com/moonlight-box/registry/internal/model"
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

func TestProgressUpdater_Stop(t *testing.T) {
	db := setupProgressTestDB(t)
	task := &model.MigrationTask{Status: model.MigrationRunning}
	db.Create(task)

	updater := NewProgressUpdater(task.ID, db, 1*time.Second)

	updater.IncrementProcessed()
	updater.IncrementFailed()

	updater.Stop()

	var updated model.MigrationTask
	db.First(&updated, task.ID)
	assert.Equal(t, 1, updated.ProcessedItems)
	assert.Equal(t, 1, updated.FailedItems)
}
