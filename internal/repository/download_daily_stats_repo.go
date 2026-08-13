package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DownloadDailyStatsRepository struct {
	db *gorm.DB
}

func NewDownloadDailyStatsRepository(db *gorm.DB) *DownloadDailyStatsRepository {
	return &DownloadDailyStatsRepository{db: db}
}

// IncrByLog 增量更新某天某包的统计计数。
// 使用 INSERT ON CONFLICT UPDATE 实现 upsert，避免并发 flush 时重复插入。
func (r *DownloadDailyStatsRepository) IncrByLog(log *model.DownloadLog) error {
	date := log.CreatedAt.Truncate(24 * time.Hour)
	onlyCache := log.Status == model.DownloadStatusCached
	onlyFailed := log.Status == model.DownloadStatusFailed

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if !onlyCache && !onlyFailed {
		// success
		updates["download_count"] = gorm.Expr("download_count + 1")
		updates["total_size_bytes"] = gorm.Expr("total_size_bytes + ?", log.SizeBytes)
	}
	if onlyCache {
		updates["cached_count"] = gorm.Expr("cached_count + 1")
	}
	if onlyFailed {
		updates["failed_count"] = gorm.Expr("failed_count + 1")
	}

	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "repository_id"}, {Name: "package_type"}, {Name: "package_name"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&model.DownloadDailyStats{
		Date:          date,
		RepositoryID:  log.RepositoryID,
		PackageType:   log.PackageType,
		PackageName:   log.PackageName,
		DownloadCount: 0,
		CachedCount:   0,
		FailedCount:   0,
	}).Error
}

// BatchIncrByLogs 批量增量更新，供 LogBatcher flush 时调用。
func (r *DownloadDailyStatsRepository) BatchIncrByLogs(logs []*model.DownloadLog) error {
	if len(logs) == 0 {
		return nil
	}
	for _, log := range logs {
		if err := r.IncrByLog(log); err != nil {
			return err
		}
	}
	return nil
}

// GetDailyTotals 查询指定天数内每天的下载总量（success + cached）。
func (r *DownloadDailyStatsRepository) GetDailyTotals(days int) ([]struct {
	Date  time.Time
	Count int64
}, error) {
	since := time.Now().Truncate(24 * time.Hour).AddDate(0, 0, -(days - 1))
	var rows []struct {
		Date  time.Time
		Count int64
	}
	err := r.db.Model(&model.DownloadDailyStats{}).
		Select("date, SUM(download_count + cached_count) as count").
		Where("date >= ?", since).
		Group("date").
		Order("date ASC").
		Scan(&rows).Error
	return rows, err
}

// GetTopPackages 查询指定天数内下载量最多的包。
func (r *DownloadDailyStatsRepository) GetTopPackages(days, limit int) ([]struct {
	PackageName   string `gorm:"column:package_name"`
	PackageType   string `gorm:"column:package_type"`
	DownloadCount int64  `gorm:"column:download_count"`
}, error) {
	since := time.Now().Truncate(24 * time.Hour).AddDate(0, 0, -(days - 1))
	var rows []struct {
		PackageName   string `gorm:"column:package_name"`
		PackageType   string `gorm:"column:package_type"`
		DownloadCount int64  `gorm:"column:download_count"`
	}
	err := r.db.Model(&model.DownloadDailyStats{}).
		Select("package_name, package_type, SUM(download_count) as download_count").
		Where("date >= ?", since).
		Group("package_name, package_type").
		Order("download_count DESC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

// GetCacheHitRate 查询指定天数内的缓存命中率。
func (r *DownloadDailyStatsRepository) GetCacheHitRate(days int) (float64, error) {
	since := time.Now().Truncate(24 * time.Hour).AddDate(0, 0, -(days - 1))
	var result struct {
		TotalDownloads int64
		TotalCached    int64
	}
	err := r.db.Model(&model.DownloadDailyStats{}).
		Select("SUM(download_count) as total_downloads, SUM(cached_count) as total_cached").
		Where("date >= ?", since).
		Scan(&result).Error
	if err != nil {
		return 0, err
	}
	total := result.TotalDownloads + result.TotalCached
	if total == 0 {
		return 0, nil
	}
	hitRate := float64(result.TotalCached) / float64(total) * 100
	return float64(int(hitRate*10)) / 10, nil
}

// GetTodayDownloadsByRepo 查询今天各仓库的下载量（success）。
func (r *DownloadDailyStatsRepository) GetTodayDownloadsByRepo() (map[uint]int64, error) {
	today := time.Now().Truncate(24 * time.Hour)
	var rows []struct {
		RepositoryID uint
		Count        int64
	}
	err := r.db.Model(&model.DownloadDailyStats{}).
		Select("repository_id, SUM(download_count) as count").
		Where("date = ?", today).
		Group("repository_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[uint]int64, len(rows))
	for _, row := range rows {
		result[row.RepositoryID] = row.Count
	}
	return result, nil
}

// CleanOldStats 清理过期聚合数据。
func (r *DownloadDailyStatsRepository) CleanOldStats(maxAge time.Duration) error {
	cutoff := time.Now().Truncate(24 * time.Hour).Add(-maxAge)
	return r.db.Where("date < ?", cutoff).Delete(&model.DownloadDailyStats{}).Error
}
