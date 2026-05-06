package service

import (
	"context"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type DownloadCountKey struct {
	PackageID   uint
	VersionID   uint
	FileID      uint
}

type DownloadCountBatcher struct {
	mu          sync.Mutex
	counts      map[DownloadCountKey]int64
	pkgRepo     *repository.PackageRepository
	flushInterval time.Duration
	stopCh      chan struct{}
}

func NewDownloadCountBatcher(pkgRepo *repository.PackageRepository, flushInterval time.Duration) *DownloadCountBatcher {
	batcher := &DownloadCountBatcher{
		counts:      make(map[DownloadCountKey]int64),
		pkgRepo:     pkgRepo,
		flushInterval: flushInterval,
		stopCh:      make(chan struct{}),
	}

	go batcher.flushLoop()

	return batcher
}

func (b *DownloadCountBatcher) Increment(pkgID, versionID, fileID uint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := DownloadCountKey{
		PackageID: pkgID,
		VersionID: versionID,
		FileID:    fileID,
	}
	b.counts[key]++
}

func (b *DownloadCountBatcher) flushLoop() {
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

func (b *DownloadCountBatcher) flush() {
	b.mu.Lock()
	counts := b.counts
	b.counts = make(map[DownloadCountKey]int64)
	b.mu.Unlock()

	if len(counts) == 0 {
		return
	}

	ctx := context.Background()
	for key, count := range counts {
		if err := b.batchIncrement(ctx, key, count); err != nil {
			logrus.Errorf("failed to batch increment download count: %v", err)
		}
	}
}

func (b *DownloadCountBatcher) batchIncrement(ctx context.Context, key DownloadCountKey, count int64) error {
	return b.pkgRepo.IncrementDownloadCountByAmount(ctx, key.PackageID, key.VersionID, key.FileID, count)
}

func (b *DownloadCountBatcher) Stop() {
	close(b.stopCh)
}
