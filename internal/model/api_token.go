package model

import "time"

// APIToken 用户签发的 API 访问令牌（用于 MCP 等场景）
type APIToken struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index:idx_api_tokens_user" json:"user_id"`
	Name      string     `gorm:"size:100;not null" json:"name"`
	TokenHash []byte     `gorm:"size:32;not null;uniqueIndex" json:"-"`
	Prefix    string     `gorm:"size:12;not null;index" json:"prefix"`
	LastUsed  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (APIToken) TableName() string {
	return "api_tokens"
}
