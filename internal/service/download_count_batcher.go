package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/gorm"
)

// DownloadCountKey 下载计数的聚合键。
// 包含仓库级别和包级别的维度，flush 时分别更新 repositories 和 packages 表。
type DownloadCountKey struct {
	RepoID      uint
	Format      string
	PackageName string
	Version     string
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
	util.SafeGo("download-count-batcher.flush-loop", batcher.flushLoop)
	return batcher
}

// Increment 记录一次下载计数。
// repoID: 仓库 ID；format: 包格式（npm/maven/pypi 等）；
// name: 包名；version: 版本号。
func (b *DownloadCountBatcher) Increment(repoID uint, format, name, version string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := DownloadCountKey{RepoID: repoID, Format: format, PackageName: name, Version: version}
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
	b.batchUpdateCounts(context.Background(), counts)
}

func (b *DownloadCountBatcher) batchUpdateCounts(ctx context.Context, counts map[DownloadCountKey]int64) {
	// 按仓库 ID 聚合，更新 repositories.download_count
	repoMap := make(map[uint]int64)
	// 按 (repoID, format, name) 聚合，更新 packages.download_count
	pkgMap := make(map[string]int64) // key: "repoID|format|name"

	for key, count := range counts {
		if key.RepoID > 0 {
			repoMap[key.RepoID] += count
		}
		if key.RepoID > 0 && key.Format != "" && key.PackageName != "" {
			pkgKey := fmt.Sprintf("%d|%s|%s", key.RepoID, key.Format, key.PackageName)
			pkgMap[pkgKey] += count
		}
	}

	if len(repoMap) > 0 {
		b.batchUpdateRepoCounts(ctx, repoMap)
	}
	if len(pkgMap) > 0 {
		b.batchUpdatePackageCounts(ctx, pkgMap)
	}
}

// batchUpdateRepoCounts 批量更新 repositories.download_count
func (b *DownloadCountBatcher) batchUpdateRepoCounts(ctx context.Context, counts map[uint]int64) {
	sqlDB, err := b.db.DB()
	if err != nil {
		slog.Error("failed to get sql.DB", "error", err)
		return
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		return
	}
	defer tx.Rollback()

	if len(counts) <= 10 {
		stmt, err := tx.Prepare("UPDATE repositories SET download_count = download_count + ? WHERE id = ?")
		if err != nil {
			slog.Error("failed to prepare statement", "table", "repositories", "error", err)
			return
		}
		defer stmt.Close()
		for id, count := range counts {
			if _, err := stmt.ExecContext(ctx, count, id); err != nil {
				slog.Error("failed to update repo count", "id", id, "error", err)
			}
		}
	} else {
		ids := make([]string, 0, len(counts))
		caseClauses := make([]string, 0, len(counts))
		idList := make([]uint, 0, len(counts))
		for id, count := range counts {
			ids = append(ids, "?")
			caseClauses = append(caseClauses, fmt.Sprintf("WHEN %d THEN %d", id, count))
			idList = append(idList, id)
		}
		batchSQL := fmt.Sprintf("UPDATE repositories SET download_count = download_count + CASE id %s ELSE 0 END WHERE id IN (%s)",
			strings.Join(caseClauses, " "), strings.Join(ids, ","))
		args := make([]interface{}, len(idList))
		for i, id := range idList {
			args[i] = id
		}
		if _, err := tx.ExecContext(ctx, batchSQL, args...); err != nil {
			slog.Error("failed to batch update repo counts", "error", err)
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit repo counts transaction", "error", err)
	}
}

// batchUpdatePackageCounts 批量更新 packages.download_count
func (b *DownloadCountBatcher) batchUpdatePackageCounts(ctx context.Context, pkgMap map[string]int64) {
	sqlDB, err := b.db.DB()
	if err != nil {
		slog.Error("failed to get sql.DB", "error", err)
		return
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		return
	}
	defer tx.Rollback()

	// 检查 packages 表是否存在，避免启动初期表未创建时报错
	var tableCount int
	if err := tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='packages'").Scan(&tableCount); err != nil {
		// 可能是 PostgreSQL，尝试另一种检查
		if err := tx.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_name = 'packages'").Scan(&tableCount); err != nil {
			tableCount = 0
		}
	}
	if tableCount == 0 {
		slog.Debug("packages table not found, skipping package download count update")
		tx.Rollback()
		return
	}

	stmt, err := tx.Prepare("UPDATE packages SET download_count = download_count + ? WHERE repository_id = ? AND format = ? AND name = ?")
	if err != nil {
		slog.Error("failed to prepare packages update statement", "error", err)
		return
	}
	defer stmt.Close()

	for pkgKeyStr, cnt := range pkgMap {
		parts := strings.SplitN(pkgKeyStr, "|", 3)
		if len(parts) != 3 {
			continue
		}
		var repoID uint
		fmt.Sscanf(parts[0], "%d", &repoID)
		format := parts[1]
		name := parts[2]
		if _, err := stmt.ExecContext(ctx, cnt, repoID, format, name); err != nil {
			slog.Error("failed to update package download count", "repoID", repoID, "format", format, "name", name, "error", err)
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit package counts transaction", "error", err)
	}
}

func (b *DownloadCountBatcher) Stop() {
	close(b.stopCh)
}
