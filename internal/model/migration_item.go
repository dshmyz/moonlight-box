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
