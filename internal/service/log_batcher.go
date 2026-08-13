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
	dailyStatsRepo *repository.DownloadDailyStatsRepository
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}
	logCh         chan *model.DownloadLog
	once          sync.Once
}

func NewLogBatcher(logRepo *repository.DownloadLogRepository, dailyStatsRepo *repository.DownloadDailyStatsRepository, batchSize int, flushInterval time.Duration) *LogBatcher {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	batcher := &LogBatcher{
		logRepo:        logRepo,
		dailyStatsRepo: dailyStatsRepo,
		batchSize:      batchSize,
		flushInterval:  flushInterval,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
		// 容量设为 batchSize 的 20 倍，避免高并发下载时 channel 打满丢日志。
		// 每条 DownloadLog 结构体很小（几百字节），2000 条约 1MB 内存开销。
		logCh: make(chan *model.DownloadLog, batchSize*20),
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
		// channel 已满，短暂等待 100ms 给 flushLoop 消费时间，
		// 超时后丢弃并告警，避免无限阻塞调用方。
		select {
		case b.logCh <- log:
		case <-time.After(100 * time.Millisecond):
			util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
				util.LogKeyModule: "service",
			}).Warn("download log channel full after 100ms, dropping log")
		}
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

func (b *LogBatcher) flushLogs(logs []*model.DownloadLog) {
	if len(logs) == 0 {
		return
	}
	if b.logRepo == nil {
		return
	}

	// 分流：cached 日志不落 download_logs 表（量大、调试无用），
	// 但仍送入聚合表统计缓存命中数。
	persistLogs := make([]*model.DownloadLog, 0, len(logs))
	for _, l := range logs {
		if l.Status != model.DownloadStatusCached {
			persistLogs = append(persistLogs, l)
		}
	}

	if len(persistLogs) > 0 {
		if err := b.logRepo.BatchCreate(persistLogs); err != nil {
			util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
				util.LogKeyModule:  "service",
				util.LogKeyPkgType: "go",
				util.LogKeyError:   err,
			}).Error("failed to batch create download logs")
		} else {
			util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
				util.LogKeyModule:  "service",
				util.LogKeyPkgType: "go",
				"count":            len(persistLogs),
			}).Debug("batch created download logs")
		}
	}

	// 同步增量更新每日聚合表（含 cached）
	if b.dailyStatsRepo != nil {
		if err := b.dailyStatsRepo.BatchIncrByLogs(logs); err != nil {
			util.GetLogger(util.LogTypeMain).WithFields(logrus.Fields{
				util.LogKeyModule: "service",
				util.LogKeyError:  err,
			}).Error("failed to batch update daily stats")
		}
	}
}

func (b *LogBatcher) Stop() {
	b.once.Do(func() {
		close(b.stopCh)
		<-b.doneCh
	})
}
