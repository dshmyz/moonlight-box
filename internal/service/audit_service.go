package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/database"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
)

type AuditService struct {
	logChan  chan *model.AuditLog
	wg       sync.WaitGroup
	shutdown chan struct{}
}

func NewAuditService() *AuditService {
	svc := &AuditService{
		logChan:  make(chan *model.AuditLog, 1000),
		shutdown: make(chan struct{}),
	}
	svc.wg.Add(1)
	util.SafeGo("audit-worker", svc.worker)
	return svc
}

func (s *AuditService) worker() {
	defer s.wg.Done()
	batch := make([]*model.AuditLog, 0, 100)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case log := <-s.logChan:
			batch = append(batch, log)
			if len(batch) >= 100 {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		case <-s.shutdown:
			if len(batch) > 0 {
				s.flushBatch(batch)
			}
			logrus.Info("Audit service worker stopped")
			return
		}
	}
}

func (s *AuditService) flushBatch(batch []*model.AuditLog) {
	if err := database.DB.CreateInBatches(batch, len(batch)).Error; err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "audit",
			"error":  err,
			"count":  len(batch),
		}).Error("Failed to flush audit logs")
	} else {
		logrus.WithFields(logrus.Fields{
			"module": "audit",
			"count":  len(batch),
		}).Debug("Flushed audit logs")
	}
}

func (s *AuditService) Shutdown() {
	close(s.shutdown)
	s.wg.Wait()
}

func (s *AuditService) LogWithRequest(ctx context.Context, userID *uint, action model.AuditAction, resourceType string, resourceID *uint, resourceName string, details string, ipAddress string, userAgent string) error {
	return s.LogWithRequestAndStatus(ctx, userID, action, resourceType, resourceID, resourceName, details, ipAddress, userAgent, 0, 0)
}

func (s *AuditService) LogWithRequestAndStatus(ctx context.Context, userID *uint, action model.AuditAction, resourceType string, resourceID *uint, resourceName string, details string, ipAddress string, userAgent string, responseStatus int, durationMs int) error {
	log := &model.AuditLog{
		UserID:         userID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		ResourceName:   resourceName,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Details:        details,
		ResponseStatus: responseStatus,
		DurationMs:     durationMs,
		CreatedAt:      time.Now().UTC(),
	}
	select {
	case s.logChan <- log:
		return nil
	default:
		logrus.WithFields(logrus.Fields{
			"module": "audit",
			"action": action,
		}).Error("Audit log channel is full, dropping log")
		return fmt.Errorf("audit log channel is full")
	}
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
