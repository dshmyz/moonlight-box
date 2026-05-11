package repository

import (
	"testing"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/stretchr/testify/assert"
	_ "github.com/mattn/go-sqlite3"
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
	for _, item := range items {
		db.Create(&item)
	}

	pending, err := repo.GetPendingItems(1, 10, 10)
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

func TestGetStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMigrationItemRepository(db)

	items := []model.MigrationItem{
		{TaskID: 1, ComponentID: "comp1", Status: model.MigrationItemPending},
		{TaskID: 1, ComponentID: "comp2", Status: model.MigrationItemCompleted},
		{TaskID: 1, ComponentID: "comp3", Status: model.MigrationItemFailed},
		{TaskID: 1, ComponentID: "comp4", Status: model.MigrationItemProcessing},
	}
	for _, item := range items {
		db.Create(&item)
	}

	total, pending, processing, completed, failed, err := repo.GetStats(1)
	assert.NoError(t, err)
	assert.Equal(t, 4, total)
	assert.Equal(t, 1, pending)
	assert.Equal(t, 1, processing)
	assert.Equal(t, 1, completed)
	assert.Equal(t, 1, failed)
}

func TestCleanCompletedItems(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMigrationItemRepository(db)

	items := []model.MigrationItem{
		{TaskID: 1, ComponentID: "comp1", Status: model.MigrationItemCompleted},
		{TaskID: 1, ComponentID: "comp2", Status: model.MigrationItemPending},
	}
	for _, item := range items {
		db.Create(&item)
	}

	err := repo.CleanCompletedItems(1)
	assert.NoError(t, err)

	var count int64
	db.Model(&model.MigrationItem{}).Where("task_id = ?", 1).Count(&count)
	assert.Equal(t, int64(1), count)
}
