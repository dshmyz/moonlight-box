package service

import (
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type LogBatcher struct {
	mu           sync.Mutex
	logs         []*model.ProxyDownloadLog
	logRepo      *repository.ProxyDownloadLogRepository
	batchSize    int
	flushInterval time.Duration
	stopCh       chan struct{}
	flushing     bool
}

func NewLogBatcher(logRepo *repository.ProxyDownloadLogRepository, batchSize int, flushInterval time.Duration) *LogBatcher {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	batcher := &LogBatcher{
		logs:          make([]*model.ProxyDownloadLog, 0, batchSize),
		logRepo:       logRepo,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}

	go batcher.flushLoop()

	return batcher
}

func (b *LogBatcher) Record(log *model.ProxyDownloadLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, log)

	if len(b.logs) >= b.batchSize && !b.flushing {
		b.flushing = true
		go b.flush()
	}
}

func (b *LogBatcher) flushLoop() {
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.stopCh:
			b.flush()
			return
		}
	}
}

func (b *LogBatcher) flush() {
	b.mu.Lock()
	logs := b.logs
	b.logs = make([]*model.ProxyDownloadLog, 0, b.batchSize)
	b.flushing = false
	b.mu.Unlock()

	if len(logs) == 0 {
		return
	}

	if err := b.logRepo.BatchCreate(logs); err != nil {
		logrus.Errorf("failed to batch create proxy download logs: %v", err)
	} else {
		logrus.Debugf("batch created %d proxy download logs", len(logs))
	}
}

func (b *LogBatcher) Stop() {
	close(b.stopCh)
}
