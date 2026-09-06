package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type countingTask struct {
	mu   sync.Mutex
	runs int
	name string
}

func (t *countingTask) Name() string { return t.name }
func (t *countingTask) Run(ctx context.Context) (int, error) {
	t.mu.Lock()
	t.runs++
	t.mu.Unlock()
	return 0, nil
}
func (t *countingTask) Reload() {}
func (t *countingTask) Stop()   {}

func (t *countingTask) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.runs
}

func newTestConfigSvc(t *testing.T) *SystemConfigService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewSystemConfigService(repository.NewSystemConfigRepository(db))
}

// TestTaskSchedulerScheduleForDefault 验证无专属配置时使用全局间隔 @every。
func TestTaskSchedulerScheduleForDefault(t *testing.T) {
	s := NewTaskScheduler(nil)
	s.Register(&countingTask{name: "t1"})
	if got := s.ScheduleFor(s.GetTasks()[0]); got != "@every 24h" {
		t.Fatalf("ScheduleFor = %q, want @every 24h", got)
	}
}

// TestTaskSchedulerCronScheduleRuns 验证配置专属调度后，cron 引擎真实按表达式触发任务。
func TestTaskSchedulerCronScheduleRuns(t *testing.T) {
	configSvc := newTestConfigSvc(t)
	s := NewTaskScheduler(configSvc)
	task := &countingTask{name: "t1"}
	s.Register(task)
	if err := configSvc.Set(ScheduleConfigKey("t1"), "@every 100ms", "string", "scheduler", "", false, 0); err != nil {
		t.Fatalf("set schedule: %v", err)
	}

	s.Start()
	defer s.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if task.count() >= 2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("task did not run via cron schedule within 2s (runs=%d)", task.count())
}

// TestTaskSchedulerInvalidScheduleSkipsTask 验证非法调度不会导致 panic，任务被安全跳过。
func TestTaskSchedulerInvalidScheduleSkipsTask(t *testing.T) {
	configSvc := newTestConfigSvc(t)
	s := NewTaskScheduler(configSvc)
	task := &countingTask{name: "t1"}
	s.Register(task)
	if err := configSvc.Set(ScheduleConfigKey("t1"), "not-a-valid-schedule", "string", "scheduler", "", false, 0); err != nil {
		t.Fatalf("set schedule: %v", err)
	}

	s.Start()
	defer s.Stop()

	time.Sleep(300 * time.Millisecond)
	if got := s.ScheduleFor(task); got != "not-a-valid-schedule" {
		t.Fatalf("ScheduleFor = %q, want config value preserved", got)
	}
	if task.count() != 0 {
		t.Fatalf("invalid schedule task ran %d times, want 0", task.count())
	}
}

// TestTaskSchedulerRunNow 验证 RunNow 按名执行，未知任务返回 ErrTaskNotFound。
func TestTaskSchedulerRunNow(t *testing.T) {	s := NewTaskScheduler(nil)
	a := &countingTask{name: "a"}
	b := &countingTask{name: "b"}
	s.Register(a)
	s.Register(b)

	if _, err := s.RunNow("a"); err != nil {
		t.Fatalf("RunNow(a): %v", err)
	}
	if a.count() != 1 || b.count() != 0 {
		t.Fatalf("RunNow(a): a=%d b=%d, want 1/0", a.count(), b.count())
	}

	if _, err := s.RunNow("missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("RunNow(missing) err = %v, want ErrTaskNotFound", err)
	}
}

// TestTaskSchedulerPerTaskScheduleIsolation 回归测试：全局 cleanup.interval 存在时，
// 设置某任务专属调度（scheduler.cron.<task>）只影响该任务，其余任务回退全局间隔。
// 防止把 log_cleanup.interval 等旧专属配置误当全局回退导致失效。
func TestTaskSchedulerPerTaskScheduleIsolation(t *testing.T) {
	configSvc := newTestConfigSvc(t)
	s := NewTaskScheduler(configSvc)
	s.Register(&countingTask{name: "a"})
	s.Register(&countingTask{name: "b"})

	// 模拟老部署的全局清理间隔
	if err := configSvc.Set("cleanup.interval", "12h", "string", "maven", "", false, 0); err != nil {
		t.Fatalf("set global interval: %v", err)
	}
	if err := configSvc.Set(ScheduleConfigKey("a"), "@every 6h", "string", "scheduler", "", false, 0); err != nil {
		t.Fatalf("set task schedule: %v", err)
	}
	s.loadInterval() // Start 时也会调用；测试里手动加载全局间隔

	tasks := s.GetTasks()
	var taskA, taskB ScheduledTask
	for _, task := range tasks {
		switch task.Name() {
		case "a":
			taskA = task
		case "b":
			taskB = task
		}
	}
	if got := s.ScheduleFor(taskA); got != "@every 6h" {
		t.Fatalf("task a schedule = %q, want @every 6h", got)
	}
	if got := s.ScheduleFor(taskB); got != "@every 12h" {
		t.Fatalf("task b schedule = %q, want global @every 12h (isolation broken)", got)
	}
}

// TestValidateSchedule 验证调度校验：cron/@every 合法，@every 0s 与负间隔被拒绝。
func TestValidateSchedule(t *testing.T) {
	valid := []string{"0 3 * * *", "@every 24h", "@every 90m", "@daily"}
	for _, spec := range valid {
		if err := ValidateSchedule(spec); err != nil {
			t.Fatalf("ValidateSchedule(%q) = %v, want nil", spec, err)
		}
	}
	invalid := []string{"@every 0s", "@every -5s", "@every 0h", "not-a-schedule", "24h"}
	for _, spec := range invalid {
		if err := ValidateSchedule(spec); err == nil {
			t.Fatalf("ValidateSchedule(%q) = nil, want error", spec)
		}
	}
}

// TestTaskSchedulerRunNowReturnsError 验证 RunNow 返回任务失败错误（不再吞掉）。
func TestTaskSchedulerRunNowReturnsError(t *testing.T) {
	s := NewTaskScheduler(nil)
	s.Register(&countingTask{name: "ok"})
	s.Register(&failingTask{})

	_, err := s.RunNow("failing")
	if err == nil {
		t.Fatalf("RunNow(failing) = nil error, want task failure surfaced")
	}
	// 成功任务仍累计处理数
	_, err = s.RunNow("ok")
	if err != nil {
		t.Fatalf("RunNow(ok) err = %v, want nil", err)
	}
}

type failingTask struct{}

func (t *failingTask) Name() string                        { return "failing" }
func (t *failingTask) Run(ctx context.Context) (int, error) { return 0, errors.New("boom") }
func (t *failingTask) Reload()                              {}
func (t *failingTask) Stop()                                {}

// gatedTask 在 release 前阻塞，用于确定性测试运行锁。
type gatedTask struct {
	started chan struct{}
	release chan struct{}
}

func (t *gatedTask) Name() string { return "gated" }
func (t *gatedTask) Run(ctx context.Context) (int, error) {
	close(t.started)
	<-t.release
	return 1, nil
}
func (t *gatedTask) Reload() {}
func (t *gatedTask) Stop()   {}

// TestTaskSchedulerRunLockSkipsOverlap 验证运行锁：任务正在执行时再次 RunNow 被跳过。
func TestTaskSchedulerRunLockSkipsOverlap(t *testing.T) {
	s := NewTaskScheduler(nil)
	task := &gatedTask{started: make(chan struct{}), release: make(chan struct{})}
	s.Register(task)

	go func() { _, _ = s.RunNow("gated") }()
	<-task.started // 第一个实例已持锁并进入 Run

	processed, err := s.RunNow("gated")
	if err != nil {
		t.Fatalf("overlapping RunNow err = %v, want nil", err)
	}
	if processed != 0 {
		t.Fatalf("overlapping RunNow processed = %d, want 0 (skipped)", processed)
	}
	close(task.release)
}

// TestTaskSchedulerRebuildSkipsUnchanged 验证 rebuild 只更新 spec 变化的任务，
// 未变化任务的 entry spec 保持不变（避免重置其下次触发时刻）。
func TestTaskSchedulerRebuildSkipsUnchanged(t *testing.T) {
	configSvc := newTestConfigSvc(t)
	s := NewTaskScheduler(configSvc)
	s.Register(&countingTask{name: "a"})
	s.Register(&countingTask{name: "b"})
	if err := configSvc.Set(ScheduleConfigKey("a"), "@every 6h", "string", "scheduler", "", false, 0); err != nil {
		t.Fatal(err)
	}
	if err := configSvc.Set(ScheduleConfigKey("b"), "@every 12h", "string", "scheduler", "", false, 0); err != nil {
		t.Fatal(err)
	}

	s.Start()
	defer s.Stop()
	if s.entrySpecs["a"] != "@every 6h" || s.entrySpecs["b"] != "@every 12h" {
		t.Fatalf("initial entrySpecs = %v, want a=@every 6h b=@every 12h", s.entrySpecs)
	}

	// 只改任务 a 的调度，rebuild 后 b 的 entry spec 应保持不变
	if err := configSvc.Set(ScheduleConfigKey("a"), "@every 3h", "string", "scheduler", "", false, 0); err != nil {
		t.Fatal(err)
	}
	s.ReloadAll()
	if s.entrySpecs["a"] != "@every 3h" {
		t.Fatalf("a entrySpec after reload = %q, want @every 3h", s.entrySpecs["a"])
	}
	if s.entrySpecs["b"] != "@every 12h" {
		t.Fatalf("b entrySpec changed after reload = %q, want unchanged @every 12h", s.entrySpecs["b"])
	}
}

// TestConfigurableTaskUpdatePersists 验证实现 ConfigurableTask 的任务
// 能通过 UpdateConfig 持久化 typed 配置、校验非法值，且 ConfigFields 反映当前值。
func TestConfigurableTaskUpdatePersists(t *testing.T) {
	configSvc := newTestConfigSvc(t)
	gc := NewProxyMetadataCacheGC(nil, nil, configSvc)

	if err := gc.UpdateConfig(map[string]any{"enabled": false, "max_age_days": 45.0}, 1); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	enabled, maxAge := gc.getConfig()
	if enabled || maxAge != 45 {
		t.Fatalf("after update: enabled=%v maxAge=%d, want false/45", enabled, maxAge)
	}

	// 非法 int（负数）应被拒绝
	if err := gc.UpdateConfig(map[string]any{"enabled": true, "max_age_days": -3.0}, 1); err == nil {
		t.Fatalf("negative max_age_days should be rejected")
	}
	// 非法类型应被拒绝
	if err := gc.UpdateConfig(map[string]any{"enabled": "yes", "max_age_days": 30.0}, 1); err == nil {
		t.Fatalf("non-bool enabled should be rejected")
	}

	// ConfigFields 反映当前持久化的值
	var foundEnabled, foundDays bool
	for _, f := range gc.ConfigFields() {
		switch f.Key {
		case "enabled":
			if v, ok := f.Value.(bool); !ok || v {
				t.Fatalf("ConfigFields enabled = %v, want false", f.Value)
			}
			foundEnabled = true
		case "max_age_days":
			if v, ok := f.Value.(int); !ok || v != 45 {
				t.Fatalf("ConfigFields max_age_days = %v, want 45", f.Value)
			}
			foundDays = true
		}
	}
	if !foundEnabled || !foundDays {
		t.Fatalf("ConfigFields missing fields: enabled=%v days=%v", foundEnabled, foundDays)
	}
}
