package repository

import (
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
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
	
	// SQLite 不支持 ON CONFLICT，使用简单的批量插入
	return r.db.CreateInBatches(items, 100).Error
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
