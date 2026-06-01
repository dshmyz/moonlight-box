package service

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
)

type LogCleanupService struct {
	logRepo         *repository.DownloadLogRepository
	retentionDays   int
	cleanupInterval time.Duration
	stopCh          chan struct{}
}

func NewLogCleanupService(
	logRepo *repository.DownloadLogRepository,
	retentionDays int,
	cleanupInterval time.Duration,
) *LogCleanupService {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if cleanupInterval <= 0 {
		cleanupInterval = 24 * time.Hour
	}

	return &LogCleanupService{
		logRepo:         logRepo,
		retentionDays:   retentionDays,
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}
}

func (s *LogCleanupService) Start() {
	logrus.WithFields(logrus.Fields{
		"module":         "log_cleanup",
		"retention_days": s.retentionDays,
		"interval":       s.cleanupInterval,
	}).Info("Log cleanup service started")

	util.SafeGo("log-cleanup", s.cleanupLoop)
}

func (s *LogCleanupService) cleanupLoop() {
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			s.cleanup()
			return
		}
	}
}

func (s *LogCleanupService) cleanup() {
	maxAge := time.Duration(s.retentionDays) * 24 * time.Hour

	logrus.WithFields(logrus.Fields{
		"module":         "log_cleanup",
		"retention":      maxAge,
		"retention_days": s.retentionDays,
	}).Info("Starting log cleanup")

	startTime := time.Now()
	err := s.logRepo.CleanOldLogs(maxAge)
	duration := time.Since(startTime)

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":   "log_cleanup",
			"error":    err,
			"duration": duration,
		}).Error("Failed to cleanup old logs")
		return
	}

	logrus.WithFields(logrus.Fields{
		"module":   "log_cleanup",
		"duration": duration,
	}).Info("Log cleanup completed successfully")
}

func (s *LogCleanupService) Stop() {
	logrus.WithField("module", "log_cleanup").Info("Stopping log cleanup service")
	close(s.stopCh)
}

func (s *LogCleanupService) CleanupNow() error {
	maxAge := time.Duration(s.retentionDays) * 24 * time.Hour
	return s.logRepo.CleanOldLogs(maxAge)
}
