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
	logRepo       *repository.DownloadLogRepository
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}
	logCh         chan *model.DownloadLog
	once          sync.Once
}

func NewLogBatcher(logRepo *repository.DownloadLogRepository, batchSize int, flushInterval time.Duration) *LogBatcher {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	batcher := &LogBatcher{
		logRepo:       logRepo,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		logCh:         make(chan *model.DownloadLog, batchSize*4),
	}

	util.SafeGo("log-batcher.flush-loop", batcher.flushLoop)

	return batcher
}

func (b *LogBatcher) Record(log *model.DownloadLog) {
	if log == nil {
		return
	}
	select {
	case b.logCh <- log:
	default:
		util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
			util.LogKeyModule: "service",
		}).Warn("download log channel is full, dropping log")
	}
}

func (b *LogBatcher) flushLoop() {
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	defer close(b.doneCh)
	logs := make([]*model.DownloadLog, 0, b.batchSize)

	for {
		select {
		case log := <-b.logCh:
			logs = append(logs, log)
			if len(logs) >= b.batchSize {
				b.flushLogs(logs)
				logs = make([]*model.DownloadLog, 0, b.batchSize)
			}
		case <-ticker.C:
			if len(logs) > 0 {
				b.flushLogs(logs)
				logs = make([]*model.DownloadLog, 0, b.batchSize)
			}
		case <-b.stopCh:
			for {
				select {
				case log := <-b.logCh:
					logs = append(logs, log)
					if len(logs) >= b.batchSize {
						b.flushLogs(logs)
						logs = make([]*model.DownloadLog, 0, b.batchSize)
					}
				default:
					if len(logs) > 0 {
						b.flushLogs(logs)
					}
					return
				}
			}
		}
	}
}

func (b *LogBatcher) flush() {
	logs := make([]*model.DownloadLog, 0, b.batchSize)
	for {
		select {
		case log := <-b.logCh:
			logs = append(logs, log)
			if len(logs) >= b.batchSize {
				b.flushLogs(logs)
				logs = make([]*model.DownloadLog, 0, b.batchSize)
			}
		default:
			if len(logs) > 0 {
				b.flushLogs(logs)
			}
			return
		}
	}
}

func (b *LogBatcher) flushLogs(logs []*model.DownloadLog) {
	if len(logs) == 0 {
		return
	}
	if b.logRepo == nil {
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
	b.once.Do(func() {
		close(b.stopCh)
		<-b.doneCh
	})
}
