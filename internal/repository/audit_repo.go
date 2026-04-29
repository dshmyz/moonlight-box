package repository

import (
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *AuditRepository) List(page, pageSize int, ipAddress, resourceType string, action *model.AuditAction) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	query := r.db.Model(&model.AuditLog{})

	if ipAddress != "" {
		query = query.Where("ip_address = ?", ipAddress)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if action != nil {
		query = query.Where("action = ?", *action)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&logs)

	return logs, total, result.Error
}

func (r *AuditRepository) GetByID(id uint) (*model.AuditLog, error) {
	var log model.AuditLog
	err := r.db.First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *AuditRepository) CleanOldLogs(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return r.db.Where("created_at < ?", cutoff).Delete(&model.AuditLog{}).Error
}
