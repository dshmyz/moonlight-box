package util

import (
	"bytes"
	"strings"
	"sync"
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

// ---------- samplingHook tests ----------

// spyFormatter 记录 Format 被调用次数和内容
type spyFormatter struct {
	mu        sync.Mutex
	formatted []string
}

func (f *spyFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.formatted = append(f.formatted, entry.Message)
	return []byte(entry.Message + "\n"), nil
}

func (f *spyFormatter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.formatted)
}

func TestSamplingRate1AlwaysPasses(t *testing.T) {
	// rate=1.0 → 所有日志都应通过
	hook := &samplingHook{rate: 1.0, moduleRates: map[string]float64{}}
	levels := hook.Levels()
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	for _, lvl := range []logrus.Level{logrus.DebugLevel, logrus.InfoLevel, logrus.WarnLevel} {
		entry := &logrus.Entry{
			Logger:  logrus.StandardLogger(),
			Level:   lvl,
			Data:    logrus.Fields{},
			Message: "test",
		}
		if err := hook.Fire(entry); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, dropped := entry.Data[sampledDropKey]; dropped {
			t.Errorf("rate=1.0 should not drop %s entries", lvl)
		}
	}
}

func TestSamplingRate0AlwaysDrops(t *testing.T) {
	// rate=0.0 → 所有日志都应被丢弃
	hook := &samplingHook{rate: 0.0, moduleRates: map[string]float64{}}
	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Data:    logrus.Fields{},
		Message: "test",
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dropped, ok := entry.Data[sampledDropKey]
	if !ok || !dropped.(bool) {
		t.Error("rate=0.0 should drop all entries")
	}
}

func TestSamplingModuleRateOverridesGlobal(t *testing.T) {
	// 全局 rate=0.0（全部丢弃），但 module "important" rate=1.0（全部保留）
	hook := &samplingHook{
		rate:        0.0,
		moduleRates: map[string]float64{"important": 1.0},
	}
	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Data:    logrus.Fields{LogKeyModule: "important"},
		Message: "important log",
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, dropped := entry.Data[sampledDropKey]; dropped {
		t.Error("module 'important' with rate=1.0 should not be dropped even if global rate=0.0")
	}

	// 不在白名单里的 module 应被全局 rate=0.0 丢弃
	entry2 := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Data:    logrus.Fields{LogKeyModule: "other"},
		Message: "other log",
	}
	_ = hook.Fire(entry2)
	dropped, ok := entry2.Data[sampledDropKey]
	if !ok || !dropped.(bool) {
		t.Error("module 'other' should be dropped by global rate=0.0")
	}
}

func TestSamplingHookNotAddedForLevelError(t *testing.T) {
	// Error/Fatal/Panic 不在 samplingHook.Levels() 里，不应被采样
	hook := &samplingHook{rate: 0.0, moduleRates: map[string]float64{}}
	for _, lvl := range hook.Levels() {
		if lvl == logrus.ErrorLevel {
			t.Error("samplingHook should not hook ErrorLevel")
		}
	}
}

func TestSamplingFormatterDropsMarkedEntries(t *testing.T) {
	spy := &spyFormatter{}
	formatter := &samplingFormatter{base: spy}

	// 正常 entry → 应该 format
	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Data:    logrus.Fields{},
		Message: "visible",
	}
	out, err := formatter.Format(entry)
	if err != nil || string(out) != "visible\n" {
		t.Errorf("expected 'visible\\n', got %q, err=%v", out, err)
	}
	if spy.count() != 1 {
		t.Errorf("expected spy called once, got %d", spy.count())
	}

	// 被标记的 entry → 应该返回 nil
	entry2 := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Data:    logrus.Fields{sampledDropKey: true},
		Message: "hidden",
	}
	out2, err2 := formatter.Format(entry2)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if out2 != nil {
		t.Errorf("expected nil for dropped entry, got %q", out2)
	}
	if spy.count() != 1 {
		t.Errorf("spy should not be called for dropped entry, got %d calls", spy.count())
	}
}
