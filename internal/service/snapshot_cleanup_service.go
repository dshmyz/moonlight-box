package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/mavenutil"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// MavenSnapshotCleanup 实现 CleanupTask，清理过期的 Maven SNAPSHOT 构建。
// 策略：每个 GAV 保留最近 N 个构建 + 保留 M 天内的，满足任一条件即保留。
type MavenSnapshotCleanup struct {
	db        *gorm.DB
	repoRepo  *repository.RepositoryRepository
	store     *storage.MetadataStore
	configSvc *SystemConfigService

	mu         sync.RWMutex
	enabled    bool
	keepLast   int
	maxAgeDays int
}

func NewMavenSnapshotCleanup(
	db *gorm.DB,
	repoRepo *repository.RepositoryRepository,
	store *storage.MetadataStore,
	configSvc *SystemConfigService,
) *MavenSnapshotCleanup {
	return &MavenSnapshotCleanup{
		db:         db,
		repoRepo:   repoRepo,
		store:      store,
		configSvc:  configSvc,
		enabled:    true,
		keepLast:   5,
		maxAgeDays: 90,
	}
}

func (t *MavenSnapshotCleanup) Name() string { return "maven_snapshot" }

// LoadConfig 从 SystemConfigService 加载配置（启动时调用一次）。
func (t *MavenSnapshotCleanup) LoadConfig() {
	t.loadConfig()
}

// Reload 实现 CleanupTask.Reload，热更新配置。
func (t *MavenSnapshotCleanup) Reload() {
	t.loadConfig()
	cfg := t.getConfig()
	logrus.WithFields(logrus.Fields{
		"module":      "maven_snapshot",
		"enabled":     cfg.Enabled,
		"keep_last":   cfg.KeepLast,
		"max_age_days": cfg.MaxAgeDays,
	}).Info("Maven snapshot cleanup config reloaded")
}

func (t *MavenSnapshotCleanup) Stop() {
	logrus.WithField("module", "maven_snapshot").Info("Maven snapshot cleanup stopped")
}

// GetConfig 返回当前配置，供 API 查询使用。
func (t *MavenSnapshotCleanup) GetConfig() (enabled bool, keepLast int, maxAgeDays int) {
	cfg := t.getConfig()
	return cfg.Enabled, cfg.KeepLast, cfg.MaxAgeDays
}

// Cleanup 实现 CleanupTask.Cleanup，执行一次 SNAPSHOT 清理。
func (t *MavenSnapshotCleanup) Cleanup(ctx context.Context) (int, error) {
	cfg := t.getConfig()
	if !cfg.Enabled {
		return 0, nil
	}

	logrus.WithFields(logrus.Fields{
		"module":      "maven_snapshot",
		"keep_last":   cfg.KeepLast,
		"max_age_days": cfg.MaxAgeDays,
	}).Info("Starting Maven snapshot cleanup")

	startTime := time.Now()
	totalDeleted := 0

	repos, err := t.repoRepo.ListContext(ctx, map[string]interface{}{
		"type":         "local",
		"package_type": "maven",
		"enabled":      true,
	}, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("list repositories: %w", err)
	}

	for _, repo := range repos {
		keepLast, maxAgeDays := t.resolveRepoConfig(&repo, cfg)
		deleted, err := t.cleanupRepo(ctx, repo.ID, repo.Name, keepLast, maxAgeDays)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"module": "maven_snapshot",
				"repo":   repo.Name,
			}).Error("Failed to cleanup snapshots")
			continue
		}
		totalDeleted += deleted
	}

	logrus.WithFields(logrus.Fields{
		"module":      "maven_snapshot",
		"deleted":     totalDeleted,
		"duration_ms": time.Since(startTime).Milliseconds(),
	}).Info("Maven snapshot cleanup completed")

	return totalDeleted, nil
}

// --- config helpers ---

type snapshotCleanupConfig struct {
	Enabled    bool
	KeepLast   int
	MaxAgeDays int
}

func (t *MavenSnapshotCleanup) loadConfig() {
	if t.configSvc == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if v, err := t.configSvc.Get("maven_snapshot_cleanup.enabled"); err == nil {
		t.enabled = v.Value == "true" || v.Value == "1"
	}
	if v, err := t.configSvc.Get("maven_snapshot_cleanup.keep_last"); err == nil {
		if n, err := strconv.Atoi(v.Value); err == nil && n > 0 {
			t.keepLast = n
		}
	}
	if v, err := t.configSvc.Get("maven_snapshot_cleanup.max_age_days"); err == nil {
		if n, err := strconv.Atoi(v.Value); err == nil && n > 0 {
			t.maxAgeDays = n
		}
	}
}

func (t *MavenSnapshotCleanup) getConfig() snapshotCleanupConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return snapshotCleanupConfig{
		Enabled:    t.enabled,
		KeepLast:   t.keepLast,
		MaxAgeDays: t.maxAgeDays,
	}
}

func (t *MavenSnapshotCleanup) resolveRepoConfig(repo *model.Repository, cfg snapshotCleanupConfig) (keepLast, maxAgeDays int) {
	keepLast = cfg.KeepLast
	maxAgeDays = cfg.MaxAgeDays
	if repo.Config != nil {
		if repo.Config.SnapshotKeepLast != nil {
			keepLast = *repo.Config.SnapshotKeepLast
		}
		if repo.Config.SnapshotMaxAgeDays != nil {
			maxAgeDays = *repo.Config.SnapshotMaxAgeDays
		}
	}
	return
}

// --- cleanup internals ---

type snapshotArtifact struct {
	ID           uint
	RepositoryID uint
	Name         string
	Version      string
	Filename     string
	RemotePath   string
	CreatedAt    time.Time
}

func (t *MavenSnapshotCleanup) cleanupRepo(ctx context.Context, repoID uint, repoName string, keepLast, maxAgeDays int) (int, error) {
	var artifacts []snapshotArtifact
	// 同时选中 artifact 与 checksum 两类行：一个 SNAPSHOT 构建的 jar 和它的 .sha1/.md5 走
	// 同一条保留判定（文件名都能解析出相同的时间戳+构建号），删除时一并删掉，避免留下
	// 指向已删除 jar 的孤儿 checksum 行。
	err := t.db.WithContext(ctx).
		Model(&model.Artifact{}).
		Where("repository_id = ? AND format = ? AND kind IN ? AND version LIKE ? AND remote_path != ''",
			repoID, "maven", []string{"artifact", "checksum"}, "%-SNAPSHOT").
		Select("id, repository_id, name, version, filename, remote_path, created_at").
		Find(&artifacts).Error
	if err != nil {
		return 0, fmt.Errorf("query snapshots: %w", err)
	}
	if len(artifacts) == 0 {
		return 0, nil
	}

	// 按 (name, version) 分组
	type gavKey struct {
		Name    string
		Version string
	}
	groups := make(map[gavKey][]snapshotArtifact)
	for _, a := range artifacts {
		key := gavKey{Name: a.Name, Version: a.Version}
		groups[key] = append(groups[key], a)
	}

	var toDelete []snapshotArtifact
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)

	for _, group := range groups {
		type indexedBuild struct {
			artifact snapshotArtifact
			build    mavenutil.SnapshotBuild
		}
		var builds []indexedBuild
		for _, a := range group {
			b, ok := mavenutil.ParseSnapshotBuild(a.Name, a.Version, a.Filename)
			if !ok {
				continue // 无法解析的文件保留（安全起见）
			}
			builds = append(builds, indexedBuild{artifact: a, build: b})
		}

		// 按时间戳+构建号降序排序（最新在前）
		sort.Slice(builds, func(i, j int) bool {
			if builds[i].build.Timestamp != builds[j].build.Timestamp {
				return builds[i].build.Timestamp > builds[j].build.Timestamp
			}
			return builds[i].build.BuildNum > builds[j].build.BuildNum
		})

		// 保留：前 keepLast 个 或 在 maxAgeDays 天内
		for i, b := range builds {
			if i >= keepLast && !b.build.TimestampT.After(cutoff) {
				toDelete = append(toDelete, b.artifact)
			}
		}
	}

	if len(toDelete) == 0 {
		return 0, nil
	}

	// 收集 ID，一次性批量删除
	ids := make([]uint, len(toDelete))
	for i, a := range toDelete {
		ids[i] = a.ID
	}

	if err := t.store.BatchDelete(ctx, repoID, ids); err != nil {
		return 0, fmt.Errorf("batch delete snapshots: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"module":  "maven_snapshot",
		"repo":    repoName,
		"deleted": len(toDelete),
		"total":   len(artifacts),
	}).Info("Cleaned up snapshot artifacts")

	return len(toDelete), nil
}
