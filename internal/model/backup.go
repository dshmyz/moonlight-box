package model

import (
	"time"

	"gorm.io/gorm"
)

type BackupStatus string

const (
	BackupStatusPending   BackupStatus = "pending"
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"
)

type BackupType string

const (
	BackupTypeFull        BackupType = "full"
	BackupTypeIncremental BackupType = "incremental"
)

type Backup struct {
	gorm.Model
	Name        string       `json:"name" gorm:"not null;unique"`
	Type        BackupType   `json:"type" gorm:"not null;default:'full'"`
	Status      BackupStatus `json:"status" gorm:"not null;default:'pending'"`
	SizeBytes   int64        `json:"size_bytes"`
	FilePath    string       `json:"file_path" gorm:"not null"`
	Description string       `json:"description"`
	StartedAt   *time.Time   `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	Error       string       `json:"error"`
	CreatedBy   uint         `json:"created_by"`
}

func (Backup) TableName() string {
	return "backups"
}
