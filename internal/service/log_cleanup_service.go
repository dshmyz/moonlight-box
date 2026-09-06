package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
)

// LogCleanupService 实现 ScheduledTask，定期清理过期的下载日志和聚合数据。
// 由 TaskScheduler 统一调度，自身不维护 ticker。
type LogCleanupService struct {
	logRepo        *repository.DownloadLogRepository
	dailyStatsRepo *repository.DownloadDailyStatsRepository
	configSvc      *SystemConfigService

	// 静态默认值（YAML 配置），作为系统配置缺失时的回退
	defaultRetentionDays int

	mu            sync.RWMutex
	retentionDays int
	enabled       bool
}

func NewLogCleanupService(
	logRepo *repository.DownloadLogRepository,
	dailyStatsRepo *repository.DownloadDailyStatsRepository,
	retentionDays int,
) *LogCleanupService {
	if retentionDays <= 0 {
		retentionDays = 30
	}

	return &LogCleanupService{
		logRepo:              logRepo,
		dailyStatsRepo:       dailyStatsRepo,
		defaultRetentionDays: retentionDays,
		retentionDays:        retentionDays,
		enabled:              true,
	}
}

// SetConfigService 注入系统配置服务，启用热更新能力。必须在 Register 之前调用。
func (s *LogCleanupService) SetConfigService(configSvc *SystemConfigService) {
	s.configSvc = configSvc
}

// LoadConfig 启动时调用一次，从 SystemConfigService 加载持久化配置。
// 否则重启后会以 YAML 默认值运行（enabled=true/默认保留期），忽略管理员配置。
func (s *LogCleanupService) LoadConfig() {
	s.loadConfigFromSystem()
}

func (s *LogCleanupService) Name() string { return "log_cleanup" }

// ConfigFields 实现 ConfigurableTask，声明可配置参数。
func (s *LogCleanupService) ConfigFields() []TaskConfigField {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return []TaskConfigField{
		{Key: "enabled", Label: "启用自动清理", Kind: ConfigKindBool, Value: s.enabled},
		{Key: "retention_days", Label: "下载日志保留天数", Kind: ConfigKindInt, Value: s.retentionDays},
	}
}

// UpdateConfig 实现 ConfigurableTask，校验并持久化配置。
func (s *LogCleanupService) UpdateConfig(values map[string]any, updatedBy uint) error {
	if s.configSvc == nil {
		return fmt.Errorf("log cleanup: config service unavailable")
	}
	enabled, err := configBoolField(values, "enabled")
	if err != nil {
		return err
	}
	retentionDays, err := configIntField(values, "retention_days")
	if err != nil {
		return err
	}
	if err := s.configSvc.Set("log_cleanup.enabled", strconv.FormatBool(enabled), "boolean", "logging", "启用下载日志自动清理", false, updatedBy); err != nil {
		return fmt.Errorf("set log_cleanup.enabled: %w", err)
	}
	if err := s.configSvc.Set("log_cleanup.retention_days", strconv.Itoa(retentionDays), "int", "logging", "下载日志保留天数", false, updatedBy); err != nil {
		return fmt.Errorf("set log_cleanup.retention_days: %w", err)
	}
	s.loadConfigFromSystem()
	return nil
}

// loadConfigFromSystem 从 SystemConfigService 读取配置，失败时回退到默认值。
func (s *LogCleanupService) loadConfigFromSystem() {
	if s.configSvc == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if enabled, err := s.configSvc.Get("log_cleanup.enabled"); err == nil {
		s.enabled = enabled.Value == "true" || enabled.Value == "1"
	}
	if days, err := s.configSvc.Get("log_cleanup.retention_days"); err == nil {
		if d, err := strconv.Atoi(days.Value); err == nil && d > 0 {
			s.retentionDays = d
		}
	}
}

// Reload 实现 ScheduledTask.Reload，热更新配置。
func (s *LogCleanupService) Reload() {
	s.loadConfigFromSystem()
	s.mu.RLock()
	defer s.mu.RUnlock()
	logrus.WithFields(logrus.Fields{
		"module":         "log_cleanup",
		"enabled":        s.enabled,
		"retention_days": s.retentionDays,
	}).Info("Log cleanup config reloaded")
}

// Run 实现 ScheduledTask.Run，执行一次日志清理。
func (s *LogCleanupService) Run(ctx context.Context) (int, error) {
	s.mu.RLock()
	enabled := s.enabled
	retentionDays := s.retentionDays
	s.mu.RUnlock()

	if !enabled {
		return 0, nil
	}

	maxAge := time.Duration(retentionDays) * 24 * time.Hour

	logrus.WithFields(logrus.Fields{
		"module":         "log_cleanup",
		"retention":      maxAge,
		"retention_days": retentionDays,
	}).Info("Starting log cleanup")

	startTime := time.Now()
	err := s.logRepo.CleanOldLogs(maxAge)
	duration := time.Since(startTime)

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":      "log_cleanup",
			"error":       err,
			"duration_ms": duration,
		}).Error("Failed to cleanup old logs")
		return 0, fmt.Errorf("clean old logs: %w", err)
	}

	// 聚合表保留 90 天（远大于 raw logs 的保留期）
	if s.dailyStatsRepo != nil {
		if err := s.dailyStatsRepo.CleanOldStats(90 * 24 * time.Hour); err != nil {
			logrus.WithFields(logrus.Fields{
				"module": "log_cleanup",
				"error":  err,
			}).Error("Failed to cleanup old daily stats")
		}
	}

	logrus.WithFields(logrus.Fields{
		"module":      "log_cleanup",
		"duration_ms": duration,
	}).Info("Log cleanup completed successfully")
	return 0, nil
}

func (s *LogCleanupService) Stop() {}
