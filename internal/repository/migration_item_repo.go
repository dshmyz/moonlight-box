package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type MigrationItemRepository struct {
	db *gorm.DB
}

// ItemStatusUpdate holds the status update for a single migration item
type ItemStatusUpdate struct {
	ItemID   uint
	Status   model.MigrationItemStatus
	ErrorMsg string
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

	// 过滤掉数据库中已存在的 component_id（避免重复插入）
	var existingIDs []string
	r.db.Model(&model.MigrationItem{}).
		Where("task_id = ? AND component_id IN ?", uniqueItems[0].TaskID, uniqueComponentIDs(uniqueItems)).
		Pluck("component_id", &existingIDs)

	existingMap := make(map[string]bool)
	for _, id := range existingIDs {
		existingMap[id] = true
	}

	newItems := make([]model.MigrationItem, 0, len(uniqueItems))
	for _, item := range uniqueItems {
		if !existingMap[item.ComponentID] {
			newItems = append(newItems, item)
		}
	}

	if len(newItems) == 0 {
		return nil
	}

	return r.db.CreateInBatches(newItems, 100).Error
}

func uniqueComponentIDs(items []model.MigrationItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ComponentID
	}
	return ids
}

func (r *MigrationItemRepository) GetPendingItems(taskID uint, maxRetries int, limit int) ([]model.MigrationItem, error) {
	var items []model.MigrationItem
	err := r.db.Where("task_id = ? AND status IN ?", taskID, []model.MigrationItemStatus{
		model.MigrationItemPending,
		model.MigrationItemFailed,
	}).
		Where("retry_count < ?", maxRetries).
		Order("created_at ASC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// GetPendingItemsAfterID 使用游标分页获取待处理项，避免 offset 分页在并发修改状态时跳过记录
func (r *MigrationItemRepository) GetPendingItemsAfterID(taskID uint, maxRetries int, limit int, afterID uint) ([]model.MigrationItem, error) {
	var items []model.MigrationItem
	query := r.db.Where("task_id = ? AND status IN ?", taskID, []model.MigrationItemStatus{
		model.MigrationItemPending,
		model.MigrationItemFailed,
	}).
		Where("retry_count < ?", maxRetries).
		Order("id ASC").
		Limit(limit)

	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	}

	err := query.Find(&items).Error
	return items, err
}

func (r *MigrationItemRepository) BatchUpdateStatus(updates []ItemStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			fields := map[string]interface{}{
				"status":     update.Status,
				"updated_at": time.Now(),
			}
			if update.ErrorMsg != "" {
				fields["error_message"] = update.ErrorMsg
			}
			if update.Status == model.MigrationItemFailed {
				fields["retry_count"] = gorm.Expr("retry_count + 1")
			}
			if update.Status == model.MigrationItemCompleted {
				now := time.Now()
				fields["completed_at"] = &now
			}
			if err := tx.Model(&model.MigrationItem{}).
				Where("id = ?", update.ItemID).
				Updates(fields).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

func (r *MigrationItemRepository) ListByTask(taskID uint, page, pageSize int) ([]model.MigrationItem, int64, error) {
	var items []model.MigrationItem
	var total int64

	query := r.db.Model(&model.MigrationItem{}).Where("task_id = ?", taskID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Order("created_at ASC").Find(&items).Error
	return items, total, err
}

func (r *MigrationItemRepository) CountByTaskID(taskID uint) int64 {
	var count int64
	r.db.Model(&model.MigrationItem{}).Where("task_id = ?", taskID).Count(&count)
	return count
}
