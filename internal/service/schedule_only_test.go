package service

import (
	"context"
	"testing"
)

// scheduleOnlyTaskStub 同时实现 ConfigurableTask 与 ScheduleOnlyTask。
type scheduleOnlyTaskStub struct{}

func (s *scheduleOnlyTaskStub) Name() string                         { return "stub_schedule_only" }
func (s *scheduleOnlyTaskStub) Run(ctx context.Context) (int, error) { return 0, nil }
func (s *scheduleOnlyTaskStub) Reload()                              {}
func (s *scheduleOnlyTaskStub) Stop()                                {}
func (s *scheduleOnlyTaskStub) ConfigFields() []TaskConfigField      { return nil }
func (s *scheduleOnlyTaskStub) UpdateConfig(map[string]any, uint) error {
	return nil
}
func (s *scheduleOnlyTaskStub) ScheduleOnly() bool { return true }

// plainConfigurableTaskStub 只实现 ConfigurableTask，参数应在调度页展示。
type plainConfigurableTaskStub struct{}

func (s *plainConfigurableTaskStub) Name() string                         { return "stub_plain" }
func (s *plainConfigurableTaskStub) Run(ctx context.Context) (int, error) { return 0, nil }
func (s *plainConfigurableTaskStub) Reload()                              {}
func (s *plainConfigurableTaskStub) Stop()                                {}
func (s *plainConfigurableTaskStub) ConfigFields() []TaskConfigField      { return nil }
func (s *plainConfigurableTaskStub) UpdateConfig(map[string]any, uint) error {
	return nil
}

type nonConfigurableTaskStub struct{}

func (s *nonConfigurableTaskStub) Name() string                         { return "stub_plain_task" }
func (s *nonConfigurableTaskStub) Run(ctx context.Context) (int, error) { return 0, nil }
func (s *nonConfigurableTaskStub) Reload()                              {}
func (s *nonConfigurableTaskStub) Stop()                                {}

func TestIsScheduleOnly(t *testing.T) {
	if !IsScheduleOnly(&scheduleOnlyTaskStub{}) {
		t.Error("task declaring ScheduleOnly should report true")
	}
	if IsScheduleOnly(&plainConfigurableTaskStub{}) {
		t.Error("configurable task without ScheduleOnly should report false")
	}
	if IsScheduleOnly(&nonConfigurableTaskStub{}) {
		t.Error("non-configurable task should report false")
	}
	// 下载日志清理的参数由下载日志页维护，调度页不应再渲染表单
	if !IsScheduleOnly(&LogCleanupService{}) {
		t.Error("LogCleanupService should declare ScheduleOnly")
	}
}
