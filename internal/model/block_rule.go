package model

import "time"

type BlockMatchType string

const (
	BlockMatchExact    BlockMatchType = "exact"
	BlockMatchWildcard BlockMatchType = "wildcard"
)

type BlockRule struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	PackageName string         `gorm:"size:200;not null;index" json:"package_name"`
	Version     string         `gorm:"size:100;not null" json:"version"`
	MatchType   BlockMatchType `gorm:"size:20;not null;default:exact" json:"match_type"`
	PackageType string         `gorm:"size:20;not null" json:"package_type"`
	Reason      string         `gorm:"size:500" json:"reason,omitempty"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	CreatedBy   *uint          `json:"created_by,omitempty"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}
