package model

import (
	"time"
)

// Repository 仓库模型，支持本地仓、代理仓、虚拟仓三种类型
type Repository struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	Name             string         `json:"name" gorm:"uniqueIndex;size:100"`
	DisplayName      string         `json:"display_name" gorm:"size:200"`
	Description      string         `json:"description"`
	Type             RepositoryType `json:"type" gorm:"size:20;index:idx_repo_type_pkg"`
	PackageType      string         `json:"package_type" gorm:"size:50;index:idx_repo_type_pkg"`
	Enabled          bool           `json:"enabled" gorm:"default:true;index"`
	PublicVisible    bool           `json:"public_visible" gorm:"default:true;index"`
	StorageBackendID *uint          `json:"storage_backend_id,omitempty"`

	// Config JSONB 字段存储代理仓库相关配置（RemoteURL、认证、缓存策略等）
	Config *RepositoryConfig `json:"config,omitempty" gorm:"serializer:json;type:text"`

	AllowOverwrite bool  `json:"allow_overwrite" gorm:"default:false"`
	AllowDelete    bool  `json:"allow_delete" gorm:"default:false"`
	DownloadCount  int64 `json:"download_count" gorm:"default:0"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Members []RepositoryMember `json:"members,omitempty" gorm:"foreignKey:RepositoryID"`

	// URL 计算字段，不存储到数据库
	URL string `json:"url" gorm:"-"`
}

func (Repository) TableName() string {
	return "repositories"
}

// GetAuthConfig 解析认证配置
func (r *Repository) GetAuthConfig() (*ProxyAuthConfig, error) {
	if r.Config == nil || r.Config.Auth == nil {
		return &ProxyAuthConfig{Type: "none"}, nil
	}
	return r.Config.Auth, nil
}

// SanitizedAuthConfig 返回脱敏后的认证配置
func (r *Repository) SanitizedAuthConfig() (*ProxyAuthConfig, error) {
	cfg, err := r.GetAuthConfig()
	if err != nil {
		return nil, err
	}

	sanitized := *cfg
	if sanitized.Basic != nil {
		b := *sanitized.Basic
		if b.Password != "" {
			b.Password = "******"
		}
		sanitized.Basic = &b
	}
	if sanitized.Bearer != nil {
		b := *sanitized.Bearer
		if b.Token != "" {
			b.Token = "******"
		}
		sanitized.Bearer = &b
	}
	if sanitized.APIKey != nil {
		k := *sanitized.APIKey
		if k.KeyValue != "" {
			k.KeyValue = "******"
		}
		sanitized.APIKey = &k
	}

	return &sanitized, nil
}

// SanitizedConfig 返回脱敏后的深拷贝 Config（API 响应使用）
func (r *Repository) SanitizedConfig() *RepositoryConfig {
	if r.Config == nil {
		return nil
	}
	cfg := *r.Config
	sanitizedAuth, _ := r.SanitizedAuthConfig()
	cfg.Auth = sanitizedAuth
	return &cfg
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

// RepositoryConfig 仓库配置 JSONB，替代原有的独立代理字段
type RepositoryConfig struct {
	RemoteURL         string           `json:"remote_url,omitempty"`
	AuthType          string           `json:"auth_type,omitempty"`
	Auth              *ProxyAuthConfig `json:"auth,omitempty"`
	ProxyPriority     int              `json:"proxy_priority,omitempty"`
	CacheEnabled      bool             `json:"cache_enabled,omitempty"`
	CacheTTLSeconds   int              `json:"cache_ttl_seconds,omitempty"`
	CacheMaxSizeGB    float64          `json:"cache_max_size_gb,omitempty"`
	CacheNegativeTTL  int              `json:"cache_negative_ttl,omitempty"`
	TimeoutSeconds    int              `json:"timeout_seconds,omitempty"`
	MaxRedirects      int              `json:"max_redirects,omitempty"`
	InsecureSkipVerify bool            `json:"insecure_skip_verify,omitempty"`
	FailureCacheRules string           `json:"failure_cache_rules,omitempty"`
}
