package model

import "time"

type MetadataSyncTask struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	RepositoryID uint       `json:"repository_id" gorm:"index"`
	Repository   Repository `json:"repository" gorm:"foreignKey:RepositoryID"`

	Status          string     `json:"status" gorm:"size:20;default:'pending'"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`

	TotalPackages   int `json:"total_packages"`
	SyncedPackages  int `json:"synced_packages"`
	FailedPackages  int `json:"failed_packages"`
	SkippedPackages int `json:"skipped_packages"`

	ErrorMessage string `json:"error_message" gorm:"type:text"`
	SyncLog      string `json:"sync_log" gorm:"type:text"`

	TriggerType string `json:"trigger_type" gorm:"size:20"`
	TriggeredBy *uint  `json:"triggered_by"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MetadataSyncTask) TableName() string {
	return "metadata_sync_tasks"
}
