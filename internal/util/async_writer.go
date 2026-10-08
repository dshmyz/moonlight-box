package util

import (
	"io"
	"sync"
	"sync/atomic"
)

// asyncWriter 有界异步日志写出器：Write 把日志行投入有界队列即返回，
// 单个后台 goroutine 顺序写出。磁盘慢/满时调用方不再被同步写阻塞
// （历史故障：logrus 全局互斥 + 磁盘 IO 卡顿 → 全站请求排队）。
// 队列满时丢弃该行并计数——日志是可丢数据，丢一行好过挂一个请求。
type asyncWriter struct {
	ch      chan []byte
	closing chan struct{}
	exited  chan struct{}
	out     io.Writer
	dropped atomic.Uint64
}

const asyncLogQueueSize = 8192

var (
	asyncWriters   []*asyncWriter
	asyncWritersMu sync.Mutex
)

func newAsyncWriter(out io.Writer) *asyncWriter {
	w := &asyncWriter{
		ch:      make(chan []byte, asyncLogQueueSize),
		closing: make(chan struct{}),
		exited:  make(chan struct{}),
		out:     out,
	}
	asyncWritersMu.Lock()
	asyncWriters = append(asyncWriters, w)
	asyncWritersMu.Unlock()
	SafeGo("util.async-log", w.loop)
	return w
}

func (w *asyncWriter) loop() {
	defer close(w.exited)
	for {
		select {
		case b := <-w.ch:
			_, _ = w.out.Write(b)
		case <-w.closing:
			// 排空剩余队列后退出
			for {
				select {
				case b := <-w.ch:
					_, _ = w.out.Write(b)
				default:
					return
				}
			}
		}
	}
}

func (w *asyncWriter) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	copy(b, p)
	select {
	case w.ch <- b:
	default:
		w.dropped.Add(1)
	}
	return len(p), nil
}

func (w *asyncWriter) close() {
	close(w.closing)
	<-w.exited
}

// LogDroppedCount 累计因队列满被丢弃的日志行数（供 Prometheus 采集）。
func LogDroppedCount() uint64 {
	asyncWritersMu.Lock()
	defer asyncWritersMu.Unlock()
	var total uint64
	for _, w := range asyncWriters {
		total += w.dropped.Load()
	}
	return total
}

// flushAsyncLoggers 排空所有异步写出器（必须在关闭底层文件之前调用，
// 否则优雅退出时丢尾部日志）。
func flushAsyncLoggers() {
	asyncWritersMu.Lock()
	defer asyncWritersMu.Unlock()
	for _, w := range asyncWriters {
		w.close()
	}
}
