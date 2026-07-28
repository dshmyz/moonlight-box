package util

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestErrorHookWritesToErrorLog 验证 error hook 把 Error 级别日志复制写到独立 writer
func TestErrorHookWritesToErrorLog(t *testing.T) {
	var errorBuf bytes.Buffer
	hook := newErrorHook(&errorBuf, &logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// 用独立 logger 测试 hook（不污染包级 logger）
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(&bytes.Buffer{}) // 主输出丢弃
	logger.AddHook(hook)

	logger.WithField("module", "test").Error("something failed")

	out := errorBuf.String()
	if !strings.Contains(out, "something failed") {
		t.Fatalf("error log should contain message, got: %s", out)
	}
	if !strings.Contains(out, "level=error") {
		t.Fatalf("error log should contain level=error, got: %s", out)
	}
	if !strings.Contains(out, "module=test") {
		t.Fatalf("error log should contain module field, got: %s", out)
	}
}

// TestErrorHookIgnoresLowerLevels 验证 hook 只对 Error/Fatal/Panic 生效，Info/Warn 不写入
func TestErrorHookIgnoresLowerLevels(t *testing.T) {
	var errorBuf bytes.Buffer
	hook := newErrorHook(&errorBuf, &logrus.TextFormatter{})

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(&bytes.Buffer{})
	logger.AddHook(hook)

	logger.Info("info message")
	logger.Warn("warn message")

	if errorBuf.Len() != 0 {
		t.Fatalf("error log should be empty for Info/Warn, got: %s", errorBuf.String())
	}
}
