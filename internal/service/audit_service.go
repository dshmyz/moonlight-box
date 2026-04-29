package service

import (
	"context"
	"time"

	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/model"
)

type AuditService struct{}

func NewAuditService() *AuditService {
	return &AuditService{}
}

func (s *AuditService) Log(ctx context.Context, userID *uint, action model.AuditAction, resourceType string, resourceID *uint, resourceName string, details string) error {
	log := model.AuditLog{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		CreatedAt:    time.Now().UTC(),
	}
	return database.DB.Create(&log).Error
}

func (s *AuditService) LogWithRequest(ctx context.Context, userID *uint, action model.AuditAction, resourceType string, resourceID *uint, resourceName string, details string, ipAddress string, userAgent string) error {
	log := model.AuditLog{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Details:      details,
		CreatedAt:    time.Now().UTC(),
	}
	return database.DB.Create(&log).Error
}

func (s *AuditService) List(page, pageSize int, userID *uint, action string) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	query := database.DB.Model(&model.AuditLog{})

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	if action != "" {
		query = query.Where("action = ?", action)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&logs)

	return logs, total, result.Error
}
