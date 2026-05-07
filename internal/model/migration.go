package model

import "time"

type MigrationStatus string

const (
	MigrationPending   MigrationStatus = "pending"
	MigrationRunning   MigrationStatus = "running"
	MigrationCompleted MigrationStatus = "completed"
	MigrationFailed    MigrationStatus = "failed"
	MigrationCancelled MigrationStatus = "cancelled"
)

type MigrationTask struct {
	ID                  uint            `json:"id" gorm:"primaryKey"`
	SourceType          string          `json:"source_type" gorm:"size:50"`
	SourceURL           string          `json:"source_url" gorm:"size:500"`
	Username            string          `json:"username" gorm:"size:100"`
	Password            string          `json:"-" gorm:"size:200"`
	Status              MigrationStatus `json:"status" gorm:"size:20"`
	TotalItems          int             `json:"total_items" gorm:"default:0"`
	ProcessedItems      int             `json:"processed_items" gorm:"default:0"`
	FailedItems         int             `json:"failed_items" gorm:"default:0"`
	SelectedRepos       string          `json:"selected_repos" gorm:"type:text"`
	ErrorMessage        string          `json:"error_message" gorm:"type:text"`
	TargetRepositoryID  uint            `json:"target_repository_id" gorm:"default:0"`
	TargetRepository    string          `json:"target_repository" gorm:"size:200"`
	WorkerCount         int             `json:"worker_count" gorm:"default:10"`
	MaxRetries          int             `json:"max_retries" gorm:"default:3"`
	BatchSize           int             `json:"batch_size" gorm:"default:50"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	StartedAt           *time.Time      `json:"started_at"`
	CompletedAt         *time.Time      `json:"completed_at"`
}

func (MigrationTask) TableName() string {
	return "migration_tasks"
}
