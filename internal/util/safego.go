package util

import (
	"fmt"
	"runtime"

	"github.com/sirupsen/logrus"
)

// SafeGo 启动一个带 panic 恢复的后台 goroutine。
//
// 单个后台 goroutine 内的 panic 会被捕获并记录，而不会导致整个进程退出。
// name 用于在日志中定位 panic 的来源（如 "scheduler.backup"、"batcher.download-count"）。
//
// 适用于常驻循环和任务型后台 goroutine。对于已有的具名方法 goroutine，
// 也可在方法内部直接 `defer util.RecoverPanic("name")`。
func SafeGo(name string, fn func()) {
	go func() {
		defer RecoverPanic(name)
		fn()
	}()
}

// RecoverPanic 捕获并记录当前 goroutine 的 panic，用于 defer 调用。
// 当日志系统尚未初始化时，回退到标准输出，确保 panic 不会被静默吞掉。
func RecoverPanic(name string) {
	if r := recover(); r != nil {
		buf := make([]byte, 8192)
		n := runtime.Stack(buf, false)
		stack := string(buf[:n])

		WithFields(logrus.Fields{
			"module":    "safego",
			"goroutine": name,
			"panic":     r,
			"stack":     stack,
		}).Error("recovered from panic in background goroutine")
		// 兜底：即使日志未初始化也保证 panic 可见
		fmt.Printf("PANIC in goroutine %q: %v\nstack:\n%s\n", name, r, stack)
	}
}
