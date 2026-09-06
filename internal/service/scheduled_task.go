package service

import (
	"context"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
)

// CleanupTask 可注入的清理任务接口。
// 各协议/功能模块实现此接口，由 CleanupService 统一调度。
type CleanupTask interface {
	// Name 返回任务标识（如 "maven_snapshot"）。
	Name() string
	// Cleanup 执行一次清理，返回本次删除数量。
	Cleanup(ctx context.Context) (deleted int, err error)
	// Reload 重新加载配置（热更新）。
	Reload()
	// Stop 停止任务（释放资源）。
	Stop()
}

// CleanupService 通用清理任务编排器。
// 管理多个 CleanupTask 的注册、定时调度和热更新。
type CleanupService struct {
	configSvc *SystemConfigService
	tasks     []CleanupTask
	mu        sync.RWMutex
	interval  time.Duration
	stopCh    chan struct{}
	reloadCh  chan struct{}
}

func NewCleanupService(configSvc *SystemConfigService) *CleanupService {
	return &CleanupService{
		configSvc: configSvc,
		interval:  24 * time.Hour,
		stopCh:    make(chan struct{}),
		reloadCh:  make(chan struct{}, 1),
	}
}

// Register 注册一个清理任务。必须在 Start 之前调用。
func (s *CleanupService) Register(task CleanupTask) {
	s.tasks = append(s.tasks, task)
}

// Start 启动清理调度循环。
func (s *CleanupService) Start() {
	s.loadInterval()

	names := make([]string, len(s.tasks))
	for i, t := range s.tasks {
		names[i] = t.Name()
	}
	logrus.WithFields(logrus.Fields{
		"module":     "cleanup",
		"tasks":      names,
		"task_count": len(s.tasks),
		"interval":   s.interval,
	}).Info("Cleanup service started")

	util.SafeGo("cleanup-orchestrator", s.loop)
}

// loadInterval 从 SystemConfigService 读取调度间隔。
func (s *CleanupService) loadInterval() {
	if s.configSvc == nil {
		return
	}
	if v, err := s.configSvc.Get("cleanup.interval"); err == nil {
		if d, err := time.ParseDuration(v.Value); err == nil && d > 0 {
			s.mu.Lock()
			s.interval = d
			s.mu.Unlock()
		}
	}
}

// GetInterval 返回当前调度间隔，供 API 查询。
func (s *CleanupService) GetInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interval
}

func (s *CleanupService) loop() {
	// 不在启动时立即执行全量清理（runAll）。正常启动路径不应触发全量修复/重建，
	// 需要时由管理员手动调用 POST /snapshot-cleanup/now，或由定时器在 interval 后触发。

	for {
		s.mu.RLock()
		interval := s.interval
		s.mu.RUnlock()

		ticker := time.NewTicker(interval)
		reloaded := false

		for !reloaded {
			select {
			case <-ticker.C:
				s.runAll()
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

func (s *CleanupService) runAll() {
	ctx := context.Background()
	for _, task := range s.tasks {
		deleted, err := task.Cleanup(ctx)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"module": "cleanup",
				"task":   task.Name(),
			}).Error("Cleanup task failed")
			continue
		}
		if deleted > 0 {
			logrus.WithFields(logrus.Fields{
				"module":  "cleanup",
				"task":    task.Name(),
				"deleted": deleted,
			}).Info("Cleanup task completed")
		}
	}
}

// CleanupNow 立即执行所有清理任务，返回总删除数。
func (s *CleanupService) CleanupNow() (int, error) {
	ctx := context.Background()
	totalDeleted := 0
	for _, task := range s.tasks {
		deleted, err := task.Cleanup(ctx)
		if err != nil {
			logrus.WithError(err).WithField("task", task.Name()).Warn("Cleanup task failed")
			continue
		}
		totalDeleted += deleted
	}
	return totalDeleted, nil
}

// ReloadAll 重新加载编排器和所有任务的配置。
func (s *CleanupService) ReloadAll() {
	s.loadInterval()
	for _, task := range s.tasks {
		task.Reload()
	}
	select {
	case s.reloadCh <- struct{}{}:
	default:
	}
}

// GetTasks 返回已注册的任务列表（供 Handler 查询配置）。
func (s *CleanupService) GetTasks() []CleanupTask {
	return s.tasks
}

// Stop 停止调度器和所有任务。
func (s *CleanupService) Stop() {
	logrus.WithField("module", "cleanup").Info("Stopping cleanup service")
	for _, task := range s.tasks {
		task.Stop()
	}
	close(s.stopCh)
}
