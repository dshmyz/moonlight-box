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

// PackageVersion 版本级聚合索引表，用于快速查询包详情页的版本列表。
// Source of truth 仍然是 artifacts；本表可从 artifacts 重建。
type PackageVersion struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	RepositoryID     uint       `gorm:"not null;uniqueIndex:idx_pkg_ver_repo_format_name_version,priority:1;index:idx_pkg_ver_repo_format_name_updated,priority:1;index:idx_pkg_ver_repo_format_name_published,priority:1" json:"repository_id"`
	Format           string     `gorm:"not null;size:64;uniqueIndex:idx_pkg_ver_repo_format_name_version,priority:2;index:idx_pkg_ver_repo_format_name_updated,priority:2;index:idx_pkg_ver_repo_format_name_published,priority:2" json:"format"`
	PackageName      string     `gorm:"not null;size:512;uniqueIndex:idx_pkg_ver_repo_format_name_version,priority:3;index:idx_pkg_ver_repo_format_name_updated,priority:3;index:idx_pkg_ver_repo_format_name_published,priority:3" json:"package_name"`
	Namespace        string     `gorm:"size:512;index:idx_package_version_namespace" json:"namespace,omitempty"`
	Version          string     `gorm:"not null;size:255;uniqueIndex:idx_pkg_ver_repo_format_name_version,priority:4;index:idx_package_version_version" json:"version"`
	Status           string     `gorm:"not null;size:32;default:published;index:idx_package_version_status" json:"status"`
	PublishedAt      *time.Time `gorm:"index:idx_pkg_ver_repo_format_name_published,priority:4" json:"published_at,omitempty"`
	LatestArtifactAt time.Time  `gorm:"not null;index:idx_pkg_ver_repo_format_name_updated,priority:4" json:"latest_artifact_at"`
	FileCount        int        `gorm:"not null;default:0" json:"file_count"`
	FilesDownloaded  bool       `gorm:"not null;default:false" json:"files_downloaded"`
	SizeBytes        int64      `gorm:"not null;default:0" json:"size_bytes"`
	DownloadCount    int64      `gorm:"not null;default:0" json:"download_count"`
	License          string     `gorm:"size:128" json:"license,omitempty"`
	ChecksumSHA256   string     `gorm:"size:128" json:"checksum_sha256,omitempty"`
	CreatedAt        time.Time  `gorm:"autoCreateTime;not null" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime;not null" json:"updated_at"`
}

func (PackageVersion) TableName() string {
	return "package_versions"
}
