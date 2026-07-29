package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
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
	db                *gorm.DB
	flushInterval     time.Duration
	stopCh            chan struct{}
	doneCh            chan struct{}
	shards            []downloadCountShard
	packagesTableOk   bool
	packagesTableOnce sync.Once
	versionsTableOk   bool
	versionsTableOnce sync.Once
}

const defaultDownloadCountShardCount = 32

type downloadCountShard struct {
	mu     sync.Mutex
	counts map[DownloadCountKey]int64
}

func NewDownloadCountBatcher(db *gorm.DB, flushInterval time.Duration) *DownloadCountBatcher {
	batcher := &DownloadCountBatcher{
		db:            db,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		shards:        make([]downloadCountShard, defaultDownloadCountShardCount),
	}
	for i := range batcher.shards {
		batcher.shards[i].counts = make(map[DownloadCountKey]int64)
	}
	util.SafeGo("download-count-batcher.flush-loop", batcher.flushLoop)
	return batcher
}

// Increment 记录一次下载计数。
// repoID: 仓库 ID；format: 包格式（npm/maven/pypi 等）；
// name: 包名；version: 版本号。
func (b *DownloadCountBatcher) Increment(repoID uint, format, name, version string) {
	key := DownloadCountKey{RepoID: repoID, Format: format, PackageName: name, Version: version}
	shard := b.shardFor(key)
	shard.mu.Lock()
	shard.counts[key]++
	shard.mu.Unlock()
}

func (b *DownloadCountBatcher) flushLoop() {
	defer close(b.doneCh)
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
	counts := b.drainCounts()
	if len(counts) == 0 {
		return
	}
	b.batchUpdateCounts(context.Background(), counts)
}

func (b *DownloadCountBatcher) drainCounts() map[DownloadCountKey]int64 {
	counts := make(map[DownloadCountKey]int64)
	for i := range b.shards {
		shard := &b.shards[i]
		shard.mu.Lock()
		for key, count := range shard.counts {
			counts[key] += count
		}
		shard.counts = make(map[DownloadCountKey]int64)
		shard.mu.Unlock()
	}
	return counts
}

func (b *DownloadCountBatcher) shardFor(key DownloadCountKey) *downloadCountShard {
	if len(b.shards) == 0 {
		return nil
	}
	return &b.shards[int(hashDownloadCountKey(key)%uint32(len(b.shards)))]
}

func hashDownloadCountKey(key DownloadCountKey) uint32 {
	h := fnv.New32a()
	var buf []byte
	buf = strconv.AppendUint(buf, uint64(key.RepoID), 10)
	_, _ = h.Write(buf)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.Format))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.PackageName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key.Version))
	return h.Sum32()
}

// pkgCountKey packages 表下载计数的聚合键
type pkgCountKey struct {
	RepoID uint
	Format string
	Name   string
}

type pkgVersionCountKey struct {
	RepoID  uint
	Format  string
	Name    string
	Version string
}

func (b *DownloadCountBatcher) batchUpdateCounts(ctx context.Context, counts map[DownloadCountKey]int64) {
	// 按仓库 ID 聚合，更新 repositories.download_count
	repoMap := make(map[uint]int64)
	// 按 (repoID, format, name) 聚合，更新 packages.download_count
	pkgMap := make(map[pkgCountKey]int64)
	// 按 (repoID, format, name, version) 聚合，更新 package_versions.download_count
	versionMap := make(map[pkgVersionCountKey]int64)

	for key, count := range counts {
		if key.RepoID > 0 {
			repoMap[key.RepoID] += count
		}
		if key.RepoID > 0 && key.Format != "" && key.PackageName != "" {
			pk := pkgCountKey{RepoID: key.RepoID, Format: key.Format, Name: key.PackageName}
			pkgMap[pk] += count
		}
		if key.RepoID > 0 && key.Format != "" && key.PackageName != "" && key.Version != "" {
			vk := pkgVersionCountKey{RepoID: key.RepoID, Format: key.Format, Name: key.PackageName, Version: key.Version}
			versionMap[vk] += count
		}
	}

	if len(repoMap) > 0 {
		b.batchUpdateRepoCounts(ctx, repoMap)
	}
	if len(pkgMap) > 0 {
		b.batchUpdatePackageCounts(ctx, pkgMap)
	}
	if len(versionMap) > 0 {
		b.batchUpdatePackageVersionCounts(ctx, versionMap)
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

// checkPackagesTable 检查 packages 表是否存在，结果只查一次并缓存。
// 避免每次 flush 都查询系统表。
func (b *DownloadCountBatcher) checkPackagesTable() bool {
	b.packagesTableOnce.Do(func() {
		b.packagesTableOk = b.checkTable("packages")
		if !b.packagesTableOk {
			slog.Debug("packages table not found, package download count update will be skipped")
		}
	})
	return b.packagesTableOk
}

func (b *DownloadCountBatcher) checkPackageVersionsTable() bool {
	b.versionsTableOnce.Do(func() {
		b.versionsTableOk = b.checkTable("package_versions")
		if !b.versionsTableOk {
			slog.Debug("package_versions table not found, package version download count update will be skipped")
		}
	})
	return b.versionsTableOk
}

func (b *DownloadCountBatcher) checkTable(table string) bool {
	sqlDB, err := b.db.DB()
	if err != nil {
		return false
	}
	var tableCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?", table).Scan(&tableCount); err != nil {
		if err := sqlDB.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_name = ?", table).Scan(&tableCount); err != nil {
			tableCount = 0
		}
	}
	return tableCount > 0
}

// batchUpdatePackageCounts 批量更新 packages.download_count
func (b *DownloadCountBatcher) batchUpdatePackageCounts(ctx context.Context, pkgMap map[pkgCountKey]int64) {
	if !b.checkPackagesTable() {
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

	stmt, err := tx.Prepare("UPDATE packages SET download_count = download_count + ? WHERE repository_id = ? AND format = ? AND name = ?")
	if err != nil {
		slog.Error("failed to prepare packages update statement", "error", err)
		return
	}
	defer stmt.Close()

	for pk, cnt := range pkgMap {
		if _, err := stmt.ExecContext(ctx, cnt, pk.RepoID, pk.Format, pk.Name); err != nil {
			slog.Error("failed to update package download count", "repo_id", pk.RepoID, "format", pk.Format, "name", pk.Name, "error", err)
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit package counts transaction", "error", err)
	}
}

// batchUpdatePackageVersionCounts 批量更新 package_versions.download_count
func (b *DownloadCountBatcher) batchUpdatePackageVersionCounts(ctx context.Context, versionMap map[pkgVersionCountKey]int64) {
	if !b.checkPackageVersionsTable() {
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

	stmt, err := tx.Prepare("UPDATE package_versions SET download_count = download_count + ? WHERE repository_id = ? AND format = ? AND package_name = ? AND version = ?")
	if err != nil {
		slog.Error("failed to prepare package_versions update statement", "error", err)
		return
	}
	defer stmt.Close()

	for pk, cnt := range versionMap {
		if _, err := stmt.ExecContext(ctx, cnt, pk.RepoID, pk.Format, pk.Name, pk.Version); err != nil {
			slog.Error("failed to update package version download count", "repo_id", pk.RepoID, "format", pk.Format, "name", pk.Name, "version", pk.Version, "error", err)
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit package version counts transaction", "error", err)
	}
}

func (b *DownloadCountBatcher) Stop() {
	close(b.stopCh)
	<-b.doneCh
}
