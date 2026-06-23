package service

import (
	"runtime"
	"testing"
	"time"
)

// TestSchedulerServiceRemoveTaskDoesNotLeakGoroutine 验证 RemoveTask 后 goroutine 正确退出，
// 不存在热更新重复 goroutine 泄漏。
func TestSchedulerServiceRemoveTaskDoesNotLeakGoroutine(t *testing.T) {
	s := NewSchedulerService(nil, nil, nil)
	defer s.Stop()

	baseline := runtime.NumGoroutine()

	// 调度一个长期运行的任务
	err := s.ScheduleCustomTask("test-task", time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleCustomTask failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // 等待 goroutine 启动

	// 移除任务
	err = s.RemoveTask("test-task")
	if err != nil {
		t.Fatalf("RemoveTask failed: %v", err)
	}

	// 等待 goroutine 有机会退出
	time.Sleep(20 * time.Millisecond)
	runtime.Gosched()

	after := runtime.NumGoroutine()
	if after > baseline {
		t.Fatalf("goroutine leak detected: before=%d, after=%d (expected <=%d)",
			baseline, after, baseline)
	}
}

// TestSchedulerServiceHotReloadDoesNotDuplicateGoroutine 验证热更新（RemoveTask + ScheduleCustomTask）
// 不会产生重复的 goroutine。
func TestSchedulerServiceHotReloadDoesNotDuplicateGoroutine(t *testing.T) {
	s := NewSchedulerService(nil, nil, nil)
	defer s.Stop()

	baseline := runtime.NumGoroutine()

	// 第一次调度
	err := s.ScheduleCustomTask("hot-task", time.Hour, func() {})
	if err != nil {
		t.Fatalf("first ScheduleCustomTask failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// 模拟热更新：移除旧任务
	err = s.RemoveTask("hot-task")
	if err != nil {
		t.Fatalf("RemoveTask failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// 重新调度同名的任务
	err = s.ScheduleCustomTask("hot-task", time.Hour, func() {})
	if err != nil {
		t.Fatalf("second ScheduleCustomTask failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// 此时应该只有新任务的一个 goroutine
	after := runtime.NumGoroutine()
	// 允许 baseline + 1（新任务的 goroutine）
	if after > baseline+1 {
		t.Fatalf("goroutine leak after hot-reload: baseline=%d, after=%d (expected <=%d)",
			baseline, after, baseline+1)
	}
}

// TestSchedulerServiceCustomTaskLifecycle 验证 ScheduleCustomTask + RemoveTask + ListTasks 生命周期
func TestSchedulerServiceCustomTaskLifecycle(t *testing.T) {
	s := NewSchedulerService(nil, nil, nil)
	defer s.Stop()

	err := s.ScheduleCustomTask("lifecycle-task", time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleCustomTask failed: %v", err)
	}

	tasks := s.ListTasks()
	found := false
	for _, name := range tasks {
		if name == "lifecycle-task" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ListTasks should include the scheduled task")
	}

	err = s.RemoveTask("lifecycle-task")
	if err != nil {
		t.Fatalf("RemoveTask failed: %v", err)
	}

	// 验证任务已从列表中移除
	tasks = s.ListTasks()
	for _, name := range tasks {
		if name == "lifecycle-task" {
			t.Fatal("RemoveTask should remove the task from ListTasks")
		}
	}
}