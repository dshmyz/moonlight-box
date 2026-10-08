package util

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// slowWriter 每次写耗时 delay，用于模拟慢盘。
type slowWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	delay time.Duration
}

func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *slowWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestAsyncWriterFlushOnClose 验证 close 排空全部队列，不丢已投递的日志。
func TestAsyncWriterFlushOnClose(t *testing.T) {
	out := &slowWriter{delay: time.Millisecond}
	w := newAsyncWriter(out)

	const lines = 100
	for i := 0; i < lines; i++ {
		if _, err := io.WriteString(w, "line\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	w.close()

	got := out.String()
	if gotCount := strings.Count(got, "line"); gotCount != lines {
		t.Fatalf("flushed %d lines, want %d", gotCount, lines)
	}
	if w.dropped.Load() != 0 {
		t.Fatalf("dropped = %d, want 0", w.dropped.Load())
	}
}

// TestAsyncWriterDropsWhenFull 验证队列满时丢弃并计数，调用方永不阻塞。
func TestAsyncWriterDropsWhenFull(t *testing.T) {
	release := make(chan struct{})
	out := &blockingWriter{release: release}
	w := newAsyncWriter(out)
	defer func() { close(release); w.close() }()

	// 塞满队列（后台写被阻塞）：1 条被消费进阻塞写，8192 条占满队列，其余应丢弃
	for i := 0; i < asyncLogQueueSize+100; i++ {
		start := time.Now()
		_, _ = io.WriteString(w, "drop-me\n")
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Fatalf("Write blocked for %s, want non-blocking", elapsed)
		}
	}
	// 8292 条总数：1 条被消费进阻塞写，8192 条占满队列，恰好丢弃 99 条
	if w.dropped.Load() != 99 {
		t.Fatalf("dropped = %d, want 99", w.dropped.Load())
	}
}

// TestAsyncWriterOrderPreserved 验证单写 goroutine 保持日志顺序。
func TestAsyncWriterOrderPreserved(t *testing.T) {
	out := &bytes.Buffer{}
	w := newAsyncWriter(out)

	for i := 0; i < 50; i++ {
		_, _ = io.WriteString(w, string(rune('a'+i%26)))
	}
	w.close()

	if out.Len() != 50 {
		t.Fatalf("written %d bytes, want 50", out.Len())
	}
}

type blockingWriter struct {
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	return len(p), nil
}
