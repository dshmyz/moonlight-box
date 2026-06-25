package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/sirupsen/logrus"
)

type SchedulerService struct {
	backupSvc  *BackupService
	configSvc  *SystemConfigService
	webhookSvc *WebhookService
	tickers    map[string]*time.Ticker
	cancelChs  map[string]chan struct{} // 每个任务独立的取消 channel
	mu         sync.RWMutex
	stopChan   chan struct{}
}

func NewSchedulerService(
	backupSvc *BackupService,
	configSvc *SystemConfigService,
	webhookSvc *WebhookService,
) *SchedulerService {
	return &SchedulerService{
		backupSvc:  backupSvc,
		configSvc:  configSvc,
		webhookSvc: webhookSvc,
		tickers:    make(map[string]*time.Ticker),
		cancelChs:  make(map[string]chan struct{}),
		stopChan:   make(chan struct{}),
	}
}

func (s *SchedulerService) Start() error {
	logrus.Info("Starting scheduler service")

	if err := s.ScheduleBackupFromConfig(); err != nil {
		logrus.WithError(err).Error("Failed to schedule backup from config")
	}

	s.ScheduleConfigSync()

	logrus.Info("Scheduler service started")
	return nil
}

func (s *SchedulerService) Stop() {
	logrus.Info("Stopping scheduler service")
	close(s.stopChan)

	s.mu.Lock()
	defer s.mu.Unlock()

	for name, ticker := range s.tickers {
		ticker.Stop()
		delete(s.tickers, name)
		if cancelCh, exists := s.cancelChs[name]; exists {
			close(cancelCh)
			delete(s.cancelChs, name)
		}
		logrus.WithField("task", name).Info("Stopped scheduled task")
	}
}

// ScheduleBackupFromConfig 根据系统配置调度备份任务
func (s *SchedulerService) ScheduleBackupFromConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 移除旧的备份任务
	if ticker, exists := s.tickers["daily_backup"]; exists {
		ticker.Stop()
		delete(s.tickers, "daily_backup")
	}
	if cancelCh, exists := s.cancelChs["daily_backup"]; exists {
		close(cancelCh)
		delete(s.cancelChs, "daily_backup")
	}

	// 读取备份配置
	enabled, _ := s.getBoolConfig("backup.enabled", true)
	if !enabled {
		logrus.Info("Scheduled backup is disabled")
		return nil
	}

	intervalStr, _ := s.getConfigValue("backup.interval", "24h")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		logrus.WithError(err).Warn("Invalid backup interval, using default 24h")
		interval = 24 * time.Hour
	}

	timeStr, _ := s.getConfigValue("backup.time", "02:00")
	hour, minute := s.parseTimeStr(timeStr)

	// 计算下次备份时间
	now := time.Now()
	nextBackup := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if nextBackup.Before(now) || nextBackup.Equal(now) {
		nextBackup = nextBackup.Add(interval)
	}

	// 如果间隔小于24小时，使用固定间隔而非固定时间
	if interval < 24*time.Hour {
		return s.scheduleBackupWithInterval(interval)
	}

	initialDelay := nextBackup.Sub(now)
	logrus.WithFields(logrus.Fields{
		"initial_delay": initialDelay,
		"next_backup":   nextBackup,
		"interval":      interval,
	}).Info("Scheduling backup from config")

	// 提前创建并注册 ticker，保证 RemoveTask/Stop 随时能找到并清理，
	// 避免 goroutine 内延迟注册导致的 ticker 泄漏和热更新窗口竞态。
	ticker := time.NewTicker(interval)
	s.tickers["daily_backup"] = ticker

	cancelCh := make(chan struct{})
	s.cancelChs["daily_backup"] = cancelCh

	go func() {
		// 先等待到首次备份时间，再开始按 ticker 周期执行
		select {
		case <-time.After(initialDelay):
			s.performBackup()
		case <-cancelCh:
			ticker.Stop()
			return
		case <-s.stopChan:
			ticker.Stop()
			return
		}

		for {
			select {
			case <-ticker.C:
				s.performBackup()
			case <-cancelCh:
				ticker.Stop()
				return
			case <-s.stopChan:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

// scheduleBackupWithInterval 使用固定间隔调度备份。
// 调用约束：调用方必须持有 s.mu 写锁（与 ScheduleBackupFromConfig 一致），
// 因为它直接读写 s.tickers 和 s.cancelChs。Go 的 sync.RWMutex 不可重入，
// 所以本方法不再自行加锁，避免死锁。
func (s *SchedulerService) scheduleBackupWithInterval(interval time.Duration) error {
	if _, exists := s.tickers["daily_backup"]; exists {
		return fmt.Errorf("backup already scheduled")
	}

	ticker := time.NewTicker(interval)
	s.tickers["daily_backup"] = ticker

	cancelCh := make(chan struct{})
	s.cancelChs["daily_backup"] = cancelCh

	go func() {
		// 立即执行一次
		s.performBackup()

		for {
			select {
			case <-ticker.C:
				s.performBackup()
			case <-cancelCh:
				ticker.Stop()
				return
			case <-s.stopChan:
				ticker.Stop()
				return
			}
		}
	}()

	logrus.WithField("interval", interval).Info("Scheduled backup with interval")
	return nil
}

// UpdateBackupSchedule 更新备份计划（热更新）
func (s *SchedulerService) UpdateBackupSchedule() error {
	// 移除旧任务
	s.RemoveTask("daily_backup")
	// 重新调度
	return s.ScheduleBackupFromConfig()
}

// getBoolConfig 获取布尔配置值
func (s *SchedulerService) getBoolConfig(key string, defaultVal bool) (bool, error) {
	if s.configSvc == nil {
		return defaultVal, nil
	}
	config, err := s.configSvc.Get(key)
	if err != nil {
		return defaultVal, err
	}
	return config.Value == "true" || config.Value == "1", nil
}

// getConfigValue 获取字符串配置值
func (s *SchedulerService) getConfigValue(key, defaultVal string) (string, error) {
	if s.configSvc == nil {
		return defaultVal, nil
	}
	config, err := s.configSvc.Get(key)
	if err != nil {
		return defaultVal, err
	}
	return config.Value, nil
}

// parseTimeStr 解析时间字符串 "HH:MM"
func (s *SchedulerService) parseTimeStr(timeStr string) (hour, minute int) {
	fmt.Sscanf(timeStr, "%d:%d", &hour, &minute)
	if hour < 0 || hour > 23 {
		hour = 2
	}
	if minute < 0 || minute > 59 {
		minute = 0
	}
	return
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

	cancelCh := make(chan struct{})
	s.cancelChs["config_sync"] = cancelCh

	go func() {
		for {
			select {
			case <-ticker.C:
				s.syncConfigs()
			case <-cancelCh:
				ticker.Stop()
				return
			case <-s.stopChan:
				ticker.Stop()
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

	cancelCh := make(chan struct{})
	s.cancelChs[name] = cancelCh

	go func() {
		for {
			select {
			case <-ticker.C:
				task()
			case <-cancelCh:
				ticker.Stop()
				return
			case <-s.stopChan:
				ticker.Stop()
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

	if cancelCh, exists := s.cancelChs[name]; exists {
		close(cancelCh)
		delete(s.cancelChs, name)
	}

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
