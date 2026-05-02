package model

import (
	"encoding/json"
	"time"
)

// Repository 仓库模型，支持本地仓、代理仓、虚拟仓三种类型
type Repository struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	Name             string         `json:"name" gorm:"uniqueIndex;size:100"`
	DisplayName      string         `json:"display_name" gorm:"size:200"`
	Description      string         `json:"description"`
	Type             RepositoryType `json:"type" gorm:"size:20"`
	PackageType      string         `json:"package_type" gorm:"size:50"`
	Enabled          bool           `json:"enabled" gorm:"default:true"`
	StorageBackendID *uint          `json:"storage_backend_id,omitempty"`

	RemoteURL     string `json:"remote_url,omitempty"`
	AuthType      string `json:"auth_type" gorm:"default:none"`
	AuthConfig    string `json:"auth_config" gorm:"type:text"`
	ProxyPriority int    `json:"proxy_priority" gorm:"default:0"`

	CacheEnabled     bool    `json:"cache_enabled" gorm:"default:true"`
	CacheTTLSeconds  int     `json:"cache_ttl_seconds" gorm:"default:86400"`
	CacheMaxSizeGB   float64 `json:"cache_max_size_gb" gorm:"default:10"`
	CacheNegativeTTL int     `json:"cache_negative_ttl" gorm:"default:300"`

	TimeoutSeconds     int    `json:"timeout_seconds" gorm:"default:0"`
	MaxRedirects       int    `json:"max_redirects" gorm:"default:0"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" gorm:"default:false"`
	FailureCacheRules  string `json:"failure_cache_rules" gorm:"type:text"`

	AllowOverwrite bool  `json:"allow_overwrite" gorm:"default:false"`
	AllowDelete    bool  `json:"allow_delete" gorm:"default:false"`
	DownloadCount  int64 `json:"download_count" gorm:"default:0"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// PackageTypes JSON 数组字符串，用于虚拟仓库支持多种包类型
	PackageTypes string `json:"package_types" gorm:"type:text"`

	Members []RepositoryGroup `json:"members,omitempty" gorm:"foreignKey:VirtualRepoID"`

	// 元数据同步配置
	MetadataSyncEnabled  bool       `json:"metadata_sync_enabled" gorm:"default:false"`
	MetadataSyncInterval int        `json:"metadata_sync_interval" gorm:"default:3600"`
	SyncMode             string     `json:"sync_mode" gorm:"size:20;default:'metadata_only'"`
	LastMetadataSyncAt   *time.Time `json:"last_metadata_sync_at"`
	LastSyncStatus       string     `json:"last_sync_status" gorm:"size:20;default:''"`
	LastSyncError        string     `json:"last_sync_error" gorm:"type:text"`
}

// GetAuthConfig 解析认证配置JSON字符串为结构体
func (r *Repository) GetAuthConfig() (*ProxyAuthConfig, error) {
	if r.AuthConfig == "" {
		return &ProxyAuthConfig{Type: "none"}, nil
	}
	var cfg ProxyAuthConfig
	err := json.Unmarshal([]byte(r.AuthConfig), &cfg)
	return &cfg, err
}

// RepositoryGroup 虚拟仓与成员仓的关联关系
type RepositoryGroup struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	VirtualRepoID uint      `json:"virtual_repo_id" gorm:"uniqueIndex:idx_virtual_member"`
	MemberRepoID  uint      `json:"member_repo_id" gorm:"uniqueIndex:idx_virtual_member"`
	Priority      int       `json:"priority" gorm:"default:0"`
	CreatedAt     time.Time `json:"created_at"`

	VirtualRepo Repository `json:"virtual_repo,omitempty" gorm:"foreignKey:VirtualRepoID"`
	MemberRepo  Repository `json:"member_repo,omitempty" gorm:"foreignKey:MemberRepoID"`
}

// ProxyAuthConfig 代理仓库认证配置
type ProxyAuthConfig struct {
	Type   string      `json:"type"`
	Basic  *BasicAuth  `json:"basic,omitempty"`
	Bearer *BearerAuth `json:"bearer,omitempty"`
	APIKey *APIKeyAuth `json:"api_key,omitempty"`
}

// BasicAuth Basic 认证配置
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BearerAuth Bearer Token 认证配置
type BearerAuth struct {
	Token string `json:"token"`
}

// APIKeyAuth API Key 认证配置
type APIKeyAuth struct {
	HeaderName string `json:"header_name"`
	KeyValue   string `json:"key_value"`
	QueryParam string `json:"query_param,omitempty"`
}
