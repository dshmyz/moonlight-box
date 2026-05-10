package model

import (
	"time"

	"github.com/moonlight-box/registry/internal/util"
)

type MigrationStatus string

const (
	MigrationQueued    MigrationStatus = "queued" // 在队列中等待
	MigrationPending   MigrationStatus = "pending"
	MigrationRunning   MigrationStatus = "running"
	MigrationCompleted MigrationStatus = "completed"
	MigrationFailed    MigrationStatus = "failed"
	MigrationCancelled MigrationStatus = "cancelled"
)

var encryptionKey = []byte("moonlight-box-registry-32bytes!!")

// 任务类型常量
const (
	MigrationTaskFull       = "full"
	MigrationTaskSyncConfig = "sync_config_only"
)

type MigrationTask struct {
	ID                 uint            `json:"id" gorm:"primaryKey"`
	SourceType         string          `json:"source_type" gorm:"size:50"`
	SourceURL          string          `json:"source_url" gorm:"size:500"`
	Username           string          `json:"username" gorm:"size:100"`
	PasswordEncrypted  string          `json:"-" gorm:"column:password_encrypted;type:text"`
	Status             MigrationStatus `json:"status" gorm:"size:20"`
	TaskType           string          `json:"task_type" gorm:"size:50;default:full"` // "full" 或 "sync_config_only"
	TotalItems         int             `json:"total_items" gorm:"default:0"`
	ProcessedItems     int             `json:"processed_items" gorm:"default:0"`
	FailedItems        int             `json:"failed_items" gorm:"default:0"`
	SelectedRepos      string          `json:"selected_repos" gorm:"type:text"`
	ErrorMessage       string          `json:"error_message" gorm:"type:text"`
	TargetRepositoryID uint            `json:"target_repository_id" gorm:"default:0"`
	TargetRepository   string          `json:"target_repository" gorm:"size:200"`
	WorkerCount        int             `json:"worker_count" gorm:"default:10"`
	MaxRetries         int             `json:"max_retries" gorm:"default:3"`
	BatchSize          int             `json:"batch_size" gorm:"default:50"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	StartedAt          *time.Time      `json:"started_at"`
	CompletedAt        *time.Time      `json:"completed_at"`
}

func (MigrationTask) TableName() string {
	return "migration_tasks"
}

func (t *MigrationTask) SetPassword(password string) error {
	if password == "" {
		t.PasswordEncrypted = ""
		return nil
	}
	encrypted, err := util.EncryptString(password, encryptionKey)
	if err != nil {
		return err
	}
	t.PasswordEncrypted = encrypted
	return nil
}

func (t *MigrationTask) GetPassword() (string, error) {
	if t.PasswordEncrypted == "" {
		return "", nil
	}
	password, err := util.DecryptString(t.PasswordEncrypted, encryptionKey)
	if err != nil {
		return t.PasswordEncrypted, nil
	}
	return password, nil
}
