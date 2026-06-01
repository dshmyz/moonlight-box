package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type DownloadLogRepository struct {
	db *gorm.DB
}

func NewDownloadLogRepository(db *gorm.DB) *DownloadLogRepository {
	return &DownloadLogRepository{db: db}
}

func (r *DownloadLogRepository) Create(log *model.DownloadLog) error {
	return r.db.Create(log).Error
}

func (r *DownloadLogRepository) BatchCreate(logs []*model.DownloadLog) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.CreateInBatches(logs, 500).Error
}

func (r *DownloadLogRepository) List(page, pageSize int, repositoryID *uint, packageType, status string, startTime, endTime *time.Time) ([]model.DownloadLog, int64, error) {
	var logs []model.DownloadLog
	var total int64

	query := r.db.Model(&model.DownloadLog{}).Preload("Repository")

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

func (r *DownloadLogRepository) GetStats(repositoryID *uint, startTime, endTime *time.Time) (map[string]interface{}, error) {
	query := r.db.Model(&model.DownloadLog{})

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

func (r *DownloadLogRepository) CleanOldLogs(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return r.db.Where("created_at < ?", cutoff).Delete(&model.DownloadLog{}).Error
}
