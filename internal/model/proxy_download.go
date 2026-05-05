package model

import "time"

type ProxyDownloadLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RepositoryID uint      `gorm:"not null;index" json:"repository_id"`
	Repository   *Repository `gorm:"foreignKey:RepositoryID" json:"repository,omitempty"`
	PackageType  string    `gorm:"size:20;not null;index" json:"package_type"`
	PackageName  string    `gorm:"size:200;not null;index" json:"package_name"`
	Version      string    `gorm:"size:50;index" json:"version,omitempty"`
	Filename     string    `gorm:"size:255" json:"filename,omitempty"`
	RemoteURL    string    `gorm:"size:500" json:"remote_url,omitempty"`
	Status       string    `gorm:"size:20;not null;index" json:"status"`
	StatusCode   int       `json:"status_code,omitempty"`
	SizeBytes    int64     `json:"size_bytes,omitempty"`
	DurationMs   int       `json:"duration_ms,omitempty"`
	FromCache    bool      `gorm:"default:false" json:"from_cache"`
	IPAddress    string    `gorm:"size:45" json:"ip_address,omitempty"`
	UserAgent    string    `gorm:"size:500" json:"user_agent,omitempty"`
	UserID       *uint     `json:"user_id,omitempty"`
	ErrorMessage string    `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

const (
	DownloadStatusSuccess = "success"
	DownloadStatusFailed  = "failed"
	DownloadStatusCached  = "cached"
)
