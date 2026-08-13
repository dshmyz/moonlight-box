package model

import "time"

// DownloadDailyStats 每日下载统计聚合表。
// 在 LogBatcher flush 时同步增量更新，供仪表盘查询近 N 天趋势，
// 避免直接 COUNT download_logs（数据量大、DELETE 碎片多）。
type DownloadDailyStats struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Date           time.Time `gorm:"not null;uniqueIndex:idx_daily_stats_unique" json:"date"`
	RepositoryID   uint      `gorm:"not null;uniqueIndex:idx_daily_stats_unique" json:"repository_id"`
	PackageType    string    `gorm:"size:20;not null;uniqueIndex:idx_daily_stats_unique" json:"package_type"`
	PackageName    string    `gorm:"size:200;not null;uniqueIndex:idx_daily_stats_unique" json:"package_name"`
	DownloadCount  int64     `gorm:"default:0" json:"download_count"`
	CachedCount    int64     `gorm:"default:0" json:"cached_count"`
	FailedCount    int64     `gorm:"default:0" json:"failed_count"`
	TotalSizeBytes int64     `gorm:"default:0" json:"total_size_bytes"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (DownloadDailyStats) TableName() string {
	return "download_daily_stats"
}
