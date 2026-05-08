package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type DownloadCountKey struct {
	PackageID   uint
	VersionID   uint
	FileID      uint
	RepoID      uint
}

type DownloadCountBatcher struct {
	mu            sync.Mutex
	counts        map[DownloadCountKey]int64
	db            *gorm.DB
	flushInterval time.Duration
	stopCh        chan struct{}
}

func NewDownloadCountBatcher(db *gorm.DB, flushInterval time.Duration) *DownloadCountBatcher {
	batcher := &DownloadCountBatcher{
		counts:        make(map[DownloadCountKey]int64),
		db:            db,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}

	go batcher.flushLoop()

	return batcher
}

func (b *DownloadCountBatcher) Increment(pkgID, versionID, fileID, repoID uint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := DownloadCountKey{
		PackageID: pkgID,
		VersionID: versionID,
		FileID:    fileID,
		RepoID:    repoID,
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
	b.batchUpdateCounts(ctx, counts)
}

func (b *DownloadCountBatcher) batchUpdateCounts(ctx context.Context, counts map[DownloadCountKey]int64) {
	repoMap := make(map[uint]int64)
	pkgMap := make(map[uint]int64)
	versionMap := make(map[uint]int64)
	fileMap := make(map[uint]int64)

	for key, count := range counts {
		if key.RepoID > 0 {
			repoMap[key.RepoID] += count
		}
		if key.PackageID > 0 {
			pkgMap[key.PackageID] += count
		}
		if key.VersionID > 0 {
			versionMap[key.VersionID] += count
		}
		if key.FileID > 0 {
			fileMap[key.FileID] += count
		}
	}

	if len(repoMap) > 0 {
		b.batchUpdateRepoDownloadCounts(ctx, repoMap)
	}
	if len(pkgMap) > 0 {
		b.batchUpdatePackageDownloadCounts(ctx, pkgMap)
	}
	if len(versionMap) > 0 {
		b.batchUpdateVersionDownloadCounts(ctx, versionMap)
	}
	if len(fileMap) > 0 {
		b.batchUpdateFileDownloadCounts(ctx, fileMap)
	}
}

func (b *DownloadCountBatcher) batchUpdateRepoDownloadCounts(ctx context.Context, repoCounts map[uint]int64) {
	for repoID, count := range repoCounts {
		if err := b.db.WithContext(ctx).
			Model(&model.Repository{}).
			Where("id = ?", repoID).
			UpdateColumn("download_count", gorm.Expr("download_count + ?", count)).Error; err != nil {

			slog.Error("failed to update repository download count",
				"repo_id", repoID,
				"count", count,
				"error", err)
		}
	}
}

func (b *DownloadCountBatcher) batchUpdatePackageDownloadCounts(ctx context.Context, pkgCounts map[uint]int64) {
	for pkgID, count := range pkgCounts {
		if err := b.db.WithContext(ctx).
			Model(&model.Package{}).
			Where("id = ?", pkgID).
			UpdateColumn("download_count", gorm.Expr("download_count + ?", count)).Error; err != nil {

			slog.Error("failed to update package download count",
				"package_id", pkgID,
				"count", count,
				"error", err)
		}
	}
}

func (b *DownloadCountBatcher) batchUpdateVersionDownloadCounts(ctx context.Context, versionCounts map[uint]int64) {
	for versionID, count := range versionCounts {
		if err := b.db.WithContext(ctx).
			Model(&model.PackageVersion{}).
			Where("id = ?", versionID).
			UpdateColumn("download_count", gorm.Expr("download_count + ?", count)).Error; err != nil {

			slog.Error("failed to update version download count",
				"version_id", versionID,
				"count", count,
				"error", err)
		}
	}
}

func (b *DownloadCountBatcher) batchUpdateFileDownloadCounts(ctx context.Context, fileCounts map[uint]int64) {
	for fileID, count := range fileCounts {
		if err := b.db.WithContext(ctx).
			Model(&model.PackageFile{}).
			Where("id = ?", fileID).
			UpdateColumn("download_count", gorm.Expr("download_count + ?", count)).Error; err != nil {

			slog.Error("failed to update file download count",
				"file_id", fileID,
				"count", count,
				"error", err)
		}
	}
}

func (b *DownloadCountBatcher) Stop() {
	close(b.stopCh)
}
