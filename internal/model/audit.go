package model

import "time"

type AuditAction string

const (
	ActionLogin           AuditAction = "login"
	ActionLogout          AuditAction = "logout"
	ActionPackageUpload   AuditAction = "package_upload"
	ActionPackageDownload AuditAction = "package_download"
	ActionPackageDelete   AuditAction = "package_delete"
	ActionUserCreate      AuditAction = "user_create"
	ActionUserUpdate      AuditAction = "user_update"
	ActionUserDelete      AuditAction = "user_delete"
	ActionRoleAssign      AuditAction = "role_assign"
	ActionConfigChange    AuditAction = "config_change"
	ActionBlock           AuditAction = "block"
)

type AuditLog struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	UserID         *uint       `json:"user_id,omitempty"`
	Action         AuditAction `gorm:"not null;index" json:"action"`
	ResourceType   string      `gorm:"size:50;index" json:"resource_type,omitempty"`
	ResourceID     *uint       `json:"resource_id,omitempty"`
	ResourceName   string      `gorm:"size:200" json:"resource_name,omitempty"`
	IPAddress      string      `gorm:"size:45" json:"ip_address,omitempty"`
	UserAgent      string      `gorm:"size:500" json:"user_agent,omitempty"`
	RequestID      string      `gorm:"size:36" json:"request_id,omitempty"`
	ResponseStatus int         `json:"response_status,omitempty"`
	Details        string      `gorm:"type:text" json:"details,omitempty"`
	DurationMs     int         `json:"duration_ms,omitempty"`
	CreatedAt      time.Time   `gorm:"autoCreateTime;index" json:"created_at"`
}

type CacheEntry struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	RemoteURL      string     `gorm:"uniqueIndex;not null" json:"remote_url"`
	LocalKey       string     `gorm:"not null" json:"local_key"`
	PackageType    string     `gorm:"not null;index" json:"package_type"`
	ETag           string     `json:"etag,omitempty"`
	LastModified   string     `json:"last_modified,omitempty"`
	ContentType    string     `json:"content_type,omitempty"`
	CachedAt       time.Time  `gorm:"autoCreateTime" json:"cached_at"`
	ExpiresAt      time.Time  `gorm:"index" json:"expires_at"`
	AccessCount    int64      `gorm:"default:0" json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	SizeBytes      int64      `gorm:"default:0" json:"size_bytes"`
	HitCount       int64      `gorm:"default:0" json:"hit_count"`
	MissCount      int64      `gorm:"default:0" json:"miss_count"`
}

type SystemConfig struct {
	Key         string    `gorm:"primaryKey" json:"key"`
	Value       string    `gorm:"not null" json:"value"`
	ValueType   string    `gorm:"default:string" json:"value_type"`
	Category    string    `gorm:"size:50;index" json:"category"`
	Description string    `gorm:"size:500" json:"description,omitempty"`
	IsSensitive bool      `gorm:"default:false" json:"is_sensitive"`
	UpdatedBy   *uint     `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type CASConfig struct {
	Enabled      bool   `json:"enabled"`
	ServerURL    string `json:"server_url"`
	ServiceURL   string `json:"service_url"`
	LoginPath    string `json:"login_path"`
	ValidatePath string `json:"validate_path"`
}
