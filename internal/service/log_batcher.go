package service

import (
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
)

type LogBatcher struct {
	mu            sync.Mutex
	logs          []*model.DownloadLog
	logRepo       *repository.DownloadLogRepository
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	flushing      bool
}

func NewLogBatcher(logRepo *repository.DownloadLogRepository, batchSize int, flushInterval time.Duration) *LogBatcher {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	batcher := &LogBatcher{
		logs:          make([]*model.DownloadLog, 0, batchSize),
		logRepo:       logRepo,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}

	util.SafeGo("log-batcher.flush-loop", batcher.flushLoop)

	return batcher
}

func (b *LogBatcher) Record(log *model.DownloadLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, log)

	if len(b.logs) >= b.batchSize && !b.flushing {
		b.flushing = true
		util.SafeGo("log-batcher.flush", b.flush)
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
	b.logs = make([]*model.DownloadLog, 0, b.batchSize)
	b.flushing = false
	b.mu.Unlock()

	if len(logs) == 0 {
		return
	}

	if err := b.logRepo.BatchCreate(logs); err != nil {
		util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
			util.LogKeyModule:  "service",
			util.LogKeyPkgType: "go",
			util.LogKeyError:   err,
		}).Error("failed to batch create download logs")
	} else {
		util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
			util.LogKeyModule:  "service",
			util.LogKeyPkgType: "go",
			"count":            len(logs),
		}).Debug("batch created download logs")
	}
}

func (b *LogBatcher) Stop() {
	close(b.stopCh)
}
