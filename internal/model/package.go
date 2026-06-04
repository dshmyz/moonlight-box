package model

import (
	"time"
)

// Package 包聚合表，用于快速查询包列表
// 每个包名在每个仓库中只有一条记录，聚合了版本数量、最新版本等信息
type Package struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	RepositoryID   uint      `gorm:"not null;uniqueIndex:idx_package_repo_format_name,priority:1" json:"repository_id"`
	Format         string    `gorm:"not null;size:64;uniqueIndex:idx_package_repo_format_name,priority:2" json:"format"`
	Name           string    `gorm:"not null;size:512;uniqueIndex:idx_package_repo_format_name,priority:3" json:"name"`
	Namespace      string    `gorm:"size:255;index:idx_package_namespace" json:"namespace,omitempty"`
	DisplayName    string    `gorm:"size:512" json:"display_name,omitempty"`
	Description    string    `gorm:"type:text" json:"description,omitempty"`
	LatestVersion  string    `gorm:"size:255" json:"latest_version,omitempty"`
	VersionCount   int       `gorm:"not null;default:0" json:"version_count"`
	DownloadCount  int64     `gorm:"not null;default:0" json:"download_count"`
	License        string    `gorm:"size:128" json:"license,omitempty"`
	RepositoryName string    `gorm:"-" json:"repository_name,omitempty"` // 不存储，查询时关联
	CreatedAt      time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime;not null" json:"updated_at"`
}

func (Package) TableName() string {
	return "packages"
}
