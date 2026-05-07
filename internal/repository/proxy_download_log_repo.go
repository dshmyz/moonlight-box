package repository

import (
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type ProxyDownloadLogRepository struct {
	db *gorm.DB
}

func NewProxyDownloadLogRepository(db *gorm.DB) *ProxyDownloadLogRepository {
	return &ProxyDownloadLogRepository{db: db}
}

func (r *ProxyDownloadLogRepository) Create(log *model.ProxyDownloadLog) error {
	return r.db.Create(log).Error
}

func (r *ProxyDownloadLogRepository) BatchCreate(logs []*model.ProxyDownloadLog) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.CreateInBatches(logs, 100).Error
}

func (r *ProxyDownloadLogRepository) List(page, pageSize int, repositoryID *uint, packageType, status string, startTime, endTime *time.Time) ([]model.ProxyDownloadLog, int64, error) {
	var logs []model.ProxyDownloadLog
	var total int64

	query := r.db.Model(&model.ProxyDownloadLog{}).Preload("Repository")

	if repositoryID != nil {
		query = query.Where("repository_id = ?", *repositoryID)
	}
	if packageType != "" {
		query = query.Where("package_type = ?", packageType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&logs)

	return logs, total, result.Error
}

func (r *ProxyDownloadLogRepository) GetStats(repositoryID *uint, startTime, endTime *time.Time) (map[string]interface{}, error) {
	query := r.db.Model(&model.ProxyDownloadLog{})

	if repositoryID != nil {
		query = query.Where("repository_id = ?", *repositoryID)
	}
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	var totalCount int64
	var successCount int64
	var failedCount int64
	var cachedCount int64
	var totalBytes int64

	query.Count(&totalCount)
	query.Where("status = ?", model.DownloadStatusSuccess).Count(&successCount)
	query.Where("status = ?", model.DownloadStatusFailed).Count(&failedCount)
	query.Where("status = ?", model.DownloadStatusCached).Count(&cachedCount)
	query.Select("COALESCE(SUM(size_bytes), 0)").Scan(&totalBytes)

	return map[string]interface{}{
		"total_downloads": totalCount,
		"success_count":   successCount,
		"failed_count":    failedCount,
		"cached_count":    cachedCount,
		"total_bytes":     totalBytes,
	}, nil
}

func (r *ProxyDownloadLogRepository) CleanOldLogs(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return r.db.Where("created_at < ?", cutoff).Delete(&model.ProxyDownloadLog{}).Error
}
