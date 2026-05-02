package service

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type SchedulerService struct {
	backupSvc       *BackupService
	configSvc       *SystemConfigService
	webhookSvc      *WebhookService
	metadataSyncSvc *MetadataSyncService
	repoRepo        *repository.RepositoryRepository
	tickers         map[string]*time.Ticker
	mu              sync.RWMutex
	stopChan        chan struct{}
}

func NewSchedulerService(
	backupSvc *BackupService,
	configSvc *SystemConfigService,
	webhookSvc *WebhookService,
	metadataSyncSvc *MetadataSyncService,
	repoRepo *repository.RepositoryRepository,
) *SchedulerService {
	return &SchedulerService{
		backupSvc:       backupSvc,
		configSvc:       configSvc,
		webhookSvc:      webhookSvc,
		metadataSyncSvc: metadataSyncSvc,
		repoRepo:        repoRepo,
		tickers:         make(map[string]*time.Ticker),
		stopChan:        make(chan struct{}),
	}
}

func (s *SchedulerService) Start() error {
	logrus.Info("Starting scheduler service")

	if err := s.ScheduleDailyBackup(); err != nil {
		logrus.WithError(err).Error("Failed to schedule daily backup")
	}

	s.ScheduleConfigSync()

	// 恢复所有启用了元数据同步的仓库的定时任务
	if s.metadataSyncSvc != nil && s.repoRepo != nil {
		s.restoreMetadataSyncTasks()
	}

	logrus.Info("Scheduler service started")
	return nil
}

// restoreMetadataSyncTasks 恢复元数据同步定时任务
func (s *SchedulerService) restoreMetadataSyncTasks() {
	repos, err := s.repoRepo.FindMetadataSyncEnabled()
	if err != nil {
		logrus.WithError(err).Error("Failed to get repositories with metadata sync enabled")
		return
	}

	for _, repo := range repos {
		taskName := s.getMetadataSyncTaskName(repo.ID)
		interval := time.Duration(repo.MetadataSyncInterval) * time.Second
		if interval <= 0 {
			interval = time.Hour
		}

		repoID := repo.ID
		if err := s.ScheduleCustomTask(taskName, interval, func() {
			if err := s.metadataSyncSvc.SyncRepositoryMetadata(repoID); err != nil {
				logrus.WithError(err).WithField("repo_id", repoID).Error("Failed to sync repository metadata")
			}
		}); err != nil {
			logrus.WithError(err).WithField("repo_id", repoID).Error("Failed to restore metadata sync task")
		} else {
			logrus.WithFields(logrus.Fields{
				"repo_id":  repoID,
				"interval": interval,
			}).Info("Restored metadata sync task")
		}
	}
}

// getMetadataSyncTaskName 获取元数据同步任务名称
func (s *SchedulerService) getMetadataSyncTaskName(repoID uint) string {
	return "metadata_sync_" + strconv.FormatUint(uint64(repoID), 10)
}

// ScheduleMetadataSync 调度元数据同步任务
func (s *SchedulerService) ScheduleMetadataSync(repoID uint, interval time.Duration) error {
	if s.metadataSyncSvc == nil {
		return fmt.Errorf("metadata sync service not configured")
	}

	taskName := s.getMetadataSyncTaskName(repoID)

	// 先移除已存在的任务
	s.RemoveTask(taskName)

	repoIDCopy := repoID
	return s.ScheduleCustomTask(taskName, interval, func() {
		if err := s.metadataSyncSvc.SyncRepositoryMetadata(repoIDCopy); err != nil {
			logrus.WithError(err).WithField("repo_id", repoIDCopy).Error("Failed to sync repository metadata")
		}
	})
}

// RemoveMetadataSync 移除元数据同步任务
func (s *SchedulerService) RemoveMetadataSync(repoID uint) error {
	taskName := s.getMetadataSyncTaskName(repoID)
	return s.RemoveTask(taskName)
}

func (s *SchedulerService) Stop() {
	logrus.Info("Stopping scheduler service")
	close(s.stopChan)

	s.mu.Lock()
	defer s.mu.Unlock()

	for name, ticker := range s.tickers {
		ticker.Stop()
		logrus.WithField("task", name).Info("Stopped scheduled task")
	}
}

func (s *SchedulerService) ScheduleDailyBackup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tickers["daily_backup"]; exists {
		return fmt.Errorf("daily backup already scheduled")
	}

	now := time.Now()
	nextBackup := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	if nextBackup.Before(now) {
		nextBackup = nextBackup.Add(24 * time.Hour)
	}

	initialDelay := nextBackup.Sub(now)
	logrus.WithFields(logrus.Fields{
		"initial_delay": initialDelay,
		"next_backup":   nextBackup,
	}).Info("Scheduling daily backup")

	go func() {
		select {
		case <-time.After(initialDelay):
			s.performBackup()
		case <-s.stopChan:
			return
		}

		ticker := time.NewTicker(24 * time.Hour)
		s.mu.Lock()
		s.tickers["daily_backup"] = ticker
		s.mu.Unlock()

		for {
			select {
			case <-ticker.C:
				s.performBackup()
			case <-s.stopChan:
				return
			}
		}
	}()

	return nil
}

func (s *SchedulerService) performBackup() {
	backupName := fmt.Sprintf("auto-backup-%s", time.Now().Format("20060102-150405"))

	logrus.WithField("backup_name", backupName).Info("Starting scheduled backup")

	backup, err := s.backupSvc.CreateBackup(backupName, model.BackupTypeFull, "Automated daily backup", 0)
	if err != nil {
		logrus.WithError(err).Error("Failed to create scheduled backup")
		return
	}

	logrus.WithFields(logrus.Fields{
		"backup_id":   backup.ID,
		"backup_name": backupName,
	}).Info("Scheduled backup created successfully")

	if s.webhookSvc != nil {
		payload := &WebhookPayload{
			Event:       string(model.WebhookEventPackageUploaded),
			Timestamp:   time.Now().Format(time.RFC3339),
			PackageName: "system",
			Version:     "backup",
			Data: map[string]interface{}{
				"backup_id":   backup.ID,
				"backup_name": backupName,
				"type":        "scheduled",
			},
		}
		s.webhookSvc.TriggerEvent("system.backup.completed", payload)
	}
}

func (s *SchedulerService) ScheduleConfigSync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tickers["config_sync"]; exists {
		return
	}

	ticker := time.NewTicker(1 * time.Hour)
	s.tickers["config_sync"] = ticker

	go func() {
		for {
			select {
			case <-ticker.C:
				s.syncConfigs()
			case <-s.stopChan:
				return
			}
		}
	}()

	logrus.Info("Scheduled config sync task")
}

func (s *SchedulerService) syncConfigs() {
	logrus.Debug("Syncing system configurations")
}

func (s *SchedulerService) ScheduleCustomTask(name string, interval time.Duration, task func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tickers[name]; exists {
		return fmt.Errorf("task %s already scheduled", name)
	}

	ticker := time.NewTicker(interval)
	s.tickers[name] = ticker

	go func() {
		for {
			select {
			case <-ticker.C:
				task()
			case <-s.stopChan:
				return
			}
		}
	}()

	logrus.WithFields(logrus.Fields{
		"task":     name,
		"interval": interval,
	}).Info("Scheduled custom task")

	return nil
}

func (s *SchedulerService) RemoveTask(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticker, exists := s.tickers[name]
	if !exists {
		return fmt.Errorf("task %s not found", name)
	}

	ticker.Stop()
	delete(s.tickers, name)

	logrus.WithField("task", name).Info("Removed scheduled task")
	return nil
}

func (s *SchedulerService) ListTasks() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]string, 0, len(s.tickers))
	for name := range s.tickers {
		tasks = append(tasks, name)
	}
	return tasks
}
