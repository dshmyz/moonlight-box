package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"

	"github.com/dshmyz/moonlight-box/internal/util"
)

// ScheduledTask 可注入的定时任务接口。
// 各功能模块实现此接口，由 TaskScheduler 统一调度（cron/间隔 + 可配 + 热更新）。
type ScheduledTask interface {
	// Name 返回任务标识（如 "maven_snapshot"、"proxy_metadata_cache_gc"、"log_cleanup"）。
	Name() string
	// Run 执行一次任务，返回本次处理数量（清理类任务为删除数，其余为 0）。
	Run(ctx context.Context) (processed int, err error)
	// Reload 重新加载配置（热更新）。
	Reload()
	// Stop 停止任务（释放资源）。
	Stop()
}

// ErrTaskNotFound 表示 RunNow / UpdateSchedule 指定的任务名未注册。
var ErrTaskNotFound = errors.New("scheduled task not found")

// TaskConfigKind 可配置参数的类型（UI 据此渲染控件）。
type TaskConfigKind string

const (
	ConfigKindBool   TaskConfigKind = "bool"
	ConfigKindInt    TaskConfigKind = "int"
	ConfigKindString TaskConfigKind = "string"
)

// TaskConfigField 描述任务的一个可配置参数（供统一管理页渲染/保存）。
type TaskConfigField struct {
	Key   string         `json:"key"`
	Label string         `json:"label"`
	Kind  TaskConfigKind `json:"kind"`
	Value any            `json:"value"`
}

// ConfigurableTask 可选接口：任务声明可配置参数并接收更新。
// 实现此接口的任务会在统一调度页展示配置表单；未实现则只管理调度。
type ConfigurableTask interface {
	// ConfigFields 返回任务当前的可配置参数（含当前值）。
	ConfigFields() []TaskConfigField
	// UpdateConfig 接收 JSON 反序列化后的 typed 值（bool/float64/string），
	// 由任务校验、持久化到 system_configs 并重载。
	UpdateConfig(values map[string]any, updatedBy uint) error
}

// ScheduleConfigKey 返回某任务的调度配置 key（存于 system_configs）。
func ScheduleConfigKey(name string) string {
	return "scheduler.cron." + name
}

// ValidateSchedule 校验调度表达式：必须是合法的 cron/@every，且 @every 间隔必须为正。
// robfig/cron 对 "@every 0s" / 负间隔不报错（clamp 到 1s 每秒触发），必须显式拦截。
func ValidateSchedule(spec string) error {
	if _, err := cron.ParseStandard(spec); err != nil {
		return err
	}
	if strings.HasPrefix(spec, "@every ") {
		d, err := time.ParseDuration(strings.TrimPrefix(spec, "@every "))
		if err != nil || d <= 0 {
			return fmt.Errorf("invalid @every interval %q: must be a positive duration", spec)
		}
	}
	return nil
}

// taskRunTimeout 单次任务执行上限，防止上游/DB 卡死时任务无界挂起。
// 清理类任务（如 maven_snapshot 全仓扫描）按此上限取消，各批次删除是原子的，中断无损坏。
const taskRunTimeout = 1 * time.Hour

// TaskScheduler 通用定时任务编排器，基于 robfig/cron 单引擎。
// 每个任务一条 cron entry：优先用任务的专属调度（scheduler.cron.<name>，
// 支持标准 cron 表达式与 @every 间隔），否则回退到全局间隔。
type TaskScheduler struct {
	configSvc *SystemConfigService
	tasks     []ScheduledTask
	mu        sync.RWMutex
	interval  time.Duration

	cron       *cron.Cron
	entryIDs   map[string]cron.EntryID
	entrySpecs map[string]string
	// schedMu 串行化 rebuild/Start 对 entryIDs/entrySpecs 的读写：
	// ReloadAll 由多个 HTTP handler 并发调用，并发 map 读写会直接 panic 崩溃进程。
	schedMu sync.Mutex

	// running 每任务运行锁：上一次执行未结束时不启动新实例，避免 @every 短于任务时长时并发重叠。
	running map[string]bool
	runMu   sync.Mutex
}

func NewTaskScheduler(configSvc *SystemConfigService) *TaskScheduler {
	return &TaskScheduler{
		configSvc: configSvc,
		interval:  24 * time.Hour,
		running:   make(map[string]bool),
	}
}

// Register 注册一个定时任务。必须在 Start 之前调用。
func (s *TaskScheduler) Register(task ScheduledTask) {
	s.tasks = append(s.tasks, task)
}

// Start 启动调度：为每个任务按当前调度建立 cron entry 并启动引擎。
func (s *TaskScheduler) Start() {
	s.loadInterval()
	s.cron = cron.New()
	s.entryIDs = make(map[string]cron.EntryID)
	s.entrySpecs = make(map[string]string)
	s.rebuild()

	names := make([]string, len(s.tasks))
	for i, t := range s.tasks {
		names[i] = t.Name()
	}
	logrus.WithFields(logrus.Fields{
		"module":     "scheduler",
		"tasks":      names,
		"task_count": len(s.tasks),
		"interval":   s.interval,
	}).Info("Task scheduler started")

	s.cron.Start()
}

// loadInterval 从 SystemConfigService 读取全局调度间隔（作为任务的默认间隔）。
// 优先读 scheduler.interval，兼容旧的 cleanup.interval（老部署的全局清理间隔）。
// 注意：log_cleanup.interval 曾是 log 清理专属间隔，现已映射为 scheduler.cron.log_cleanup，
// 不再作为全局回退，避免与 cleanup.interval 冲突导致失效。
func (s *TaskScheduler) loadInterval() {
	if s.configSvc == nil {
		return
	}
	for _, key := range []string{"scheduler.interval", "cleanup.interval"} {
		if v, err := s.configSvc.Get(key); err == nil {
			if d, err := time.ParseDuration(v.Value); err == nil && d > 0 {
				s.mu.Lock()
				s.interval = d
				s.mu.Unlock()
				return
			}
		}
	}
}

// GetInterval 返回全局调度间隔，供 API 查询。
func (s *TaskScheduler) GetInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interval
}

// IntervalString 返回全局间隔的简洁字符串（如 "24h"）。
func (s *TaskScheduler) IntervalString() string {
	return util.CompactDuration(s.GetInterval())
}

// ScheduleFor 返回任务当前生效的调度表达式：任务有专属配置则用之，否则 @every <全局间隔>。
func (s *TaskScheduler) ScheduleFor(task ScheduledTask) string {
	spec, _ := s.ScheduleInfo(task)
	return spec
}

// ScheduleInfo 返回任务的生效调度表达式与是否来自专属配置（单次 configSvc.Get）。
func (s *TaskScheduler) ScheduleInfo(task ScheduledTask) (spec string, custom bool) {
	if s.configSvc != nil {
		if v, err := s.configSvc.Get(ScheduleConfigKey(task.Name())); err == nil && v.Value != "" {
			return v.Value, true
		}
	}
	return "@every " + util.CompactDuration(s.GetInterval()), false
}

// configIntField 把 JSON 反序列化后的字段值（float64）强转为正整数。
func configIntField(values map[string]any, key string) (int, error) {
	v, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing config field %q", key)
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("invalid config field %q: %v (want number)", key, v)
	}
	n := int(f)
	if float64(n) != f || n <= 0 {
		return 0, fmt.Errorf("invalid config field %q: %v (must be positive integer)", key, v)
	}
	return n, nil
}

// configBoolField 把 JSON 反序列化后的字段值强转为 bool。
func configBoolField(values map[string]any, key string) (bool, error) {
	v, ok := values[key]
	if !ok {
		return false, fmt.Errorf("missing config field %q", key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("invalid config field %q: %v (want bool)", key, v)
	}
	return b, nil
}

// ScheduleForName 返回指定任务的生效调度表达式；未知任务返回空串。
func (s *TaskScheduler) ScheduleForName(name string) string {
	for _, task := range s.tasks {
		if task.Name() == name {
			return s.ScheduleFor(task)
		}
	}
	return ""
}

// rebuild 按每个任务的当前调度重建 cron entry（Start 与 ReloadAll 共用）。
// 仅对 spec 变化的任务 Remove+AddFunc，未变化的任务保留原 entry 与其下次触发时刻，
// 避免编辑任一任务把其它任务的重建全部重置。
func (s *TaskScheduler) rebuild() {
	if s.cron == nil {
		return
	}
	s.schedMu.Lock()
	defer s.schedMu.Unlock()
	for _, task := range s.tasks {
		spec := s.ScheduleFor(task)
		if err := ValidateSchedule(spec); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"module": "scheduler",
				"task":   task.Name(),
				"spec":   spec,
			}).Warn("Scheduler: invalid schedule, task disabled")
			if id, ok := s.entryIDs[task.Name()]; ok {
				s.cron.Remove(id)
				delete(s.entryIDs, task.Name())
			}
			delete(s.entrySpecs, task.Name())
			continue
		}
		if oldSpec, ok := s.entrySpecs[task.Name()]; ok && oldSpec == spec {
			continue
		}
		if id, ok := s.entryIDs[task.Name()]; ok {
			s.cron.Remove(id)
		}
		id, err := s.cron.AddFunc(spec, func() { s.runTask(task) })
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"module": "scheduler",
				"task":   task.Name(),
				"spec":   spec,
			}).Warn("Scheduler: invalid schedule, task disabled")
			continue
		}
		s.entryIDs[task.Name()] = id
		s.entrySpecs[task.Name()] = spec
	}
}

func (s *TaskScheduler) runTask(task ScheduledTask) {
	if !s.tryLock(task.Name()) {
		logrus.WithField("task", task.Name()).Warn("Scheduler: task already running, skip overlapping run")
		return
	}
	defer s.unlock(task.Name())

	ctx, cancel := context.WithTimeout(context.Background(), taskRunTimeout)
	defer cancel()
	processed, err := task.Run(ctx)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"module": "scheduler",
			"task":   task.Name(),
		}).Error("Scheduled task failed")
		return
	}
	if processed > 0 {
		logrus.WithFields(logrus.Fields{
			"module":    "scheduler",
			"task":      task.Name(),
			"processed": processed,
		}).Info("Scheduled task completed")
	}
}

// tryLock 尝试占用任务的运行锁；已被占用返回 false（跳过本次触发）。
func (s *TaskScheduler) tryLock(name string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.running[name] {
		return false
	}
	s.running[name] = true
	return true
}

func (s *TaskScheduler) unlock(name string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.running, name)
}

// RunNow 立即执行任务。name 为 "all" 或空时执行全部任务，否则执行指定任务。
// 返回本次总处理数；任务执行失败时返回第一个错误（已处理数仍累计）。
func (s *TaskScheduler) RunNow(name string) (int, error) {
	totalProcessed := 0
	var firstErr error
	for _, task := range s.tasks {
		if name != "" && name != "all" && task.Name() != name {
			continue
		}
		if !s.tryLock(task.Name()) {
			logrus.WithField("task", task.Name()).Warn("Scheduled task already running, skip")
			continue
		}
		// defer 释放锁：task.Run panic 时锁不能泄漏，否则该任务在重启前被永久跳过
		func() {
			defer s.unlock(task.Name())
			ctx, cancel := context.WithTimeout(context.Background(), taskRunTimeout)
			defer cancel()
			processed, err := task.Run(ctx)
			if err != nil {
				logrus.WithError(err).WithField("task", task.Name()).Warn("Scheduled task failed")
				if firstErr == nil {
					firstErr = fmt.Errorf("task %s: %w", task.Name(), err)
				}
				return
			}
			totalProcessed += processed
		}()
	}
	if name != "" && name != "all" && !s.hasTask(name) {
		return 0, ErrTaskNotFound
	}
	return totalProcessed, firstErr
}

func (s *TaskScheduler) hasTask(name string) bool {
	for _, task := range s.tasks {
		if task.Name() == name {
			return true
		}
	}
	return false
}

// ReloadAll 重新加载编排器和所有任务的配置，并按新调度重建 cron entry。
func (s *TaskScheduler) ReloadAll() {
	s.loadInterval()
	for _, task := range s.tasks {
		task.Reload()
	}
	s.rebuild()
}

// GetTasks 返回已注册的任务列表（供 Handler 查询配置）。
func (s *TaskScheduler) GetTasks() []ScheduledTask {
	return s.tasks
}

// Stop 停止调度器和所有任务。
func (s *TaskScheduler) Stop() {
	logrus.WithField("module", "scheduler").Info("Stopping task scheduler")
	for _, task := range s.tasks {
		task.Stop()
	}
	if s.cron != nil {
		s.cron.Stop()
	}
}
