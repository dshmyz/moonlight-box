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
	
	// 去重：按 ComponentID 去重，保留第一个
	seen := make(map[string]bool)
	uniqueItems := make([]model.MigrationItem, 0, len(items))
	for _, item := range items {
		if !seen[item.ComponentID] {
			seen[item.ComponentID] = true
			// 截断过长的字段
			if len(item.ComponentID) > 200 {
				item.ComponentID = item.ComponentID[:200]
			}
			if len(item.ComponentName) > 500 {
				item.ComponentName = item.ComponentName[:500]
			}
			if len(item.ComponentGroup) > 500 {
				item.ComponentGroup = item.ComponentGroup[:500]
			}
			uniqueItems = append(uniqueItems, item)
		}
	}
	
	if len(uniqueItems) == 0 {
		return nil
	}
	
	return r.db.CreateInBatches(uniqueItems, 100).Error
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
