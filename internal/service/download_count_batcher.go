package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type DownloadCountKey struct {
	ComponentID uint
	AssetID     uint
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

func (b *DownloadCountBatcher) Increment(componentID, assetID, repoID uint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := DownloadCountKey{ComponentID: componentID, AssetID: assetID, RepoID: repoID}
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
	repoMap := make(map[uint]int64)

	for key, count := range counts {
		if key.RepoID > 0 {
			repoMap[key.RepoID] += count
		}
	}

	if len(repoMap) > 0 {
		b.batchUpdateWithSQL(ctx, repoMap, "repositories", "download_count")
	}
}

func (b *DownloadCountBatcher) batchUpdateWithSQL(ctx context.Context, counts map[uint]int64, tableName, columnName string) {
	if len(counts) == 0 {
		return
	}
	// 白名单校验：防止 SQL 注入（虽然当前调用方均为硬编码常量）
	allowedTables := map[string]bool{"repositories": true}
	allowedColumns := map[string]bool{"download_count": true}
	if !allowedTables[tableName] || !allowedColumns[columnName] {
		slog.Error("batchUpdateWithSQL: rejected non-whitelisted table/column", "table", tableName, "column", columnName)
		return
	}
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
		stmt, err := tx.Prepare("UPDATE " + tableName + " SET " + columnName + " = " + columnName + " + ? WHERE id = ?")
		if err != nil {
			slog.Error("failed to prepare statement", "table", tableName, "error", err)
			return
		}
		defer stmt.Close()
		for id, count := range counts {
			if _, err := stmt.ExecContext(ctx, count, id); err != nil {
				slog.Error("failed to update count", "table", tableName, "id", id, "error", err)
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
		batchSQL := fmt.Sprintf("UPDATE %s SET %s = %s + CASE id %s ELSE 0 END WHERE id IN (%s)",
			tableName, columnName, columnName, strings.Join(caseClauses, " "), strings.Join(ids, ","))
		args := make([]interface{}, len(idList))
		for i, id := range idList {
			args[i] = id
		}
		if _, err := tx.ExecContext(ctx, batchSQL, args...); err != nil {
			slog.Error("failed to batch update counts", "table", tableName, "error", err)
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "table", tableName, "error", err)
	}
}

func (b *DownloadCountBatcher) Stop() {
	close(b.stopCh)
}
