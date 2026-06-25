package service

import (
	"strconv"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
)

// LogCleanupService 负责定期清理过期的下载日志。
// 支持通过 SystemConfigService 进行热更新，修改配置后调用 UpdateSchedule 即可生效。
type LogCleanupService struct {
	logRepo *repository.DownloadLogRepository
	// configSvc 为 nil 时，回退到构造时传入的静态配置
	configSvc *SystemConfigService

	// 静态默认值（YAML 配置），作为系统配置缺失时的回退
	defaultRetentionDays int
	defaultInterval      time.Duration

	mu              sync.RWMutex
	retentionDays   int
	cleanupInterval time.Duration
	enabled         bool
	stopCh          chan struct{}
	reloadCh        chan struct{} // 触发 ticker 重建
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
		logRepo:              logRepo,
		defaultRetentionDays: retentionDays,
		defaultInterval:      cleanupInterval,
		retentionDays:        retentionDays,
		cleanupInterval:      cleanupInterval,
		enabled:              true,
		stopCh:               make(chan struct{}),
		reloadCh:             make(chan struct{}, 1),
	}
}

// SetConfigService 注入系统配置服务，启用热更新能力。
// 必须在 Start 之前调用。
func (s *LogCleanupService) SetConfigService(configSvc *SystemConfigService) {
	s.configSvc = configSvc
}

// Start 启动清理循环。会先尝试从系统配置加载最新参数。
func (s *LogCleanupService) Start() {
	s.loadConfigFromSystem()

	s.mu.RLock()
	retention := s.retentionDays
	interval := s.cleanupInterval
	enabled := s.enabled
	s.mu.RUnlock()

	logrus.WithFields(logrus.Fields{
		"module":         "log_cleanup",
		"retention_days": retention,
		"interval":       interval,
		"enabled":        enabled,
	}).Info("Log cleanup service started")

	util.SafeGo("log-cleanup", s.cleanupLoop)
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
	if interval, err := s.configSvc.Get("log_cleanup.interval"); err == nil {
		if d, err := time.ParseDuration(interval.Value); err == nil && d > 0 {
			s.cleanupInterval = d
		}
	}
}

// UpdateSchedule 热更新清理计划。重新从系统配置加载参数并重建 ticker。
func (s *LogCleanupService) UpdateSchedule() {
	s.loadConfigFromSystem()

	s.mu.RLock()
	retention := s.retentionDays
	interval := s.cleanupInterval
	enabled := s.enabled
	s.mu.RUnlock()

	logrus.WithFields(logrus.Fields{
		"module":         "log_cleanup",
		"retention_days": retention,
		"interval":       interval,
		"enabled":        enabled,
	}).Info("Log cleanup schedule updated")

	// 非阻塞发送 reload 信号
	select {
	case s.reloadCh <- struct{}{}:
	default:
	}
}

func (s *LogCleanupService) cleanupLoop() {
	s.runCleanupCycle()

	for {
		s.mu.RLock()
		interval := s.cleanupInterval
		enabled := s.enabled
		s.mu.RUnlock()

		if !enabled {
			// 已禁用：等待 reload 或 stop
			select {
			case <-s.reloadCh:
				continue
			case <-s.stopCh:
				return
			}
		}

		ticker := time.NewTicker(interval)
		reloaded := false

		for !reloaded {
			select {
			case <-ticker.C:
				s.cleanup()
			case <-s.reloadCh:
				reloaded = true
			case <-s.stopCh:
				ticker.Stop()
				return
			}
		}

		ticker.Stop()
	}
}

// runCleanupCycle 在启动时立即执行一次清理（仅在启用时）。
func (s *LogCleanupService) runCleanupCycle() {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if enabled {
		s.cleanup()
	}
}

func (s *LogCleanupService) cleanup() {
	s.mu.RLock()
	retentionDays := s.retentionDays
	s.mu.RUnlock()

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

// CleanupNow 立即执行一次清理，使用当前配置的保留天数。
func (s *LogCleanupService) CleanupNow() error {
	s.mu.RLock()
	retentionDays := s.retentionDays
	s.mu.RUnlock()

	maxAge := time.Duration(retentionDays) * 24 * time.Hour
	return s.logRepo.CleanOldLogs(maxAge)
}

// GetConfig 返回当前清理配置快照，供 API 查询使用。
func (s *LogCleanupService) GetConfig() (enabled bool, retentionDays int, interval time.Duration) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled, s.retentionDays, s.cleanupInterval
}
