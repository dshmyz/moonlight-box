package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/mavenutil"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// MavenSnapshotCleanup 实现 ScheduledTask，清理过期的 Maven SNAPSHOT 构建。
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
	dryRun     bool
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
		dryRun:     true, // 默认仅预览：先看会删什么，确认后再关闭 dry_run 真正执行
	}
}

func (t *MavenSnapshotCleanup) Name() string { return "maven_snapshot" }

// LoadConfig 从 SystemConfigService 加载配置（启动时调用一次）。
func (t *MavenSnapshotCleanup) LoadConfig() {
	t.loadConfig()
}

// Reload 实现 ScheduledTask.Reload，热更新配置。
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

// ConfigFields 实现 ConfigurableTask，声明可配置参数。
func (t *MavenSnapshotCleanup) ConfigFields() []TaskConfigField {
	cfg := t.getConfig()
	return []TaskConfigField{
		{Key: "enabled", Label: "启用自动清理", Kind: ConfigKindBool, Value: cfg.Enabled},
		{Key: "dry_run", Label: "仅预览不删除", Kind: ConfigKindBool, Value: cfg.DryRun},
		{Key: "keep_last", Label: "每个 SNAPSHOT 保留构建数", Kind: ConfigKindInt, Value: cfg.KeepLast},
		{Key: "max_age_days", Label: "SNAPSHOT 保留天数", Kind: ConfigKindInt, Value: cfg.MaxAgeDays},
	}
}

// UpdateConfig 实现 ConfigurableTask，校验并持久化配置。
func (t *MavenSnapshotCleanup) UpdateConfig(values map[string]any, updatedBy uint) error {
	if t.configSvc == nil {
		return fmt.Errorf("maven snapshot cleanup: config service unavailable")
	}
	enabled, err := configBoolField(values, "enabled")
	if err != nil {
		return err
	}
	dryRun, err := configBoolField(values, "dry_run")
	if err != nil {
		return err
	}
	keepLast, err := configIntField(values, "keep_last")
	if err != nil {
		return err
	}
	maxAgeDays, err := configIntField(values, "max_age_days")
	if err != nil {
		return err
	}
	if err := t.configSvc.Set("maven_snapshot_cleanup.enabled", strconv.FormatBool(enabled), "bool", "maven", "启用 Maven SNAPSHOT 自动清理", false, updatedBy); err != nil {
		return fmt.Errorf("set maven_snapshot_cleanup.enabled: %w", err)
	}
	if err := t.configSvc.Set("maven_snapshot_cleanup.dry_run", strconv.FormatBool(dryRun), "bool", "maven", "仅预览不删除（确认后关闭再真正执行）", false, updatedBy); err != nil {
		return fmt.Errorf("set maven_snapshot_cleanup.dry_run: %w", err)
	}
	if err := t.configSvc.Set("maven_snapshot_cleanup.keep_last", strconv.Itoa(keepLast), "int", "maven", "每个 SNAPSHOT 版本保留最近构建数", false, updatedBy); err != nil {
		return fmt.Errorf("set maven_snapshot_cleanup.keep_last: %w", err)
	}
	if err := t.configSvc.Set("maven_snapshot_cleanup.max_age_days", strconv.Itoa(maxAgeDays), "int", "maven", "SNAPSHOT 保留天数", false, updatedBy); err != nil {
		return fmt.Errorf("set maven_snapshot_cleanup.max_age_days: %w", err)
	}
	t.loadConfig()
	return nil
}

// Run 实现 ScheduledTask.Run，执行一次 SNAPSHOT 清理。
func (t *MavenSnapshotCleanup) Run(ctx context.Context) (int, error) {
	cfg := t.getConfig()
	if !cfg.Enabled {
		return 0, nil
	}

	logrus.WithFields(logrus.Fields{
		"module":       "maven_snapshot",
		"keep_last":    cfg.KeepLast,
		"max_age_days": cfg.MaxAgeDays,
		"dry_run":      cfg.DryRun,
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
		deleted, err := t.cleanupRepo(ctx, repo.ID, repo.Name, keepLast, maxAgeDays, cfg.DryRun)
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
	DryRun     bool
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
	if v, err := t.configSvc.Get("maven_snapshot_cleanup.dry_run"); err == nil {
		t.dryRun = v.Value == "true" || v.Value == "1"
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
		DryRun:     t.dryRun,
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

func (t *MavenSnapshotCleanup) cleanupRepo(ctx context.Context, repoID uint, repoName string, keepLast, maxAgeDays int, dryRun bool) (int, error) {
	var artifacts []snapshotArtifact
	// 同时选中 artifact 与 checksum 两类行：一个 SNAPSHOT 构建的 jar 和它的 .sha1/.md5 走
	// 同一条保留判定（文件名都能解析出相同的时间戳+构建号），删除时一并删掉，避免留下
	// 指向已删除 jar 的孤儿 checksum 行。
	//
	// 候选 = 两种存储布局的并集（精确判定仍由 Go 侧 ParseSnapshotBuild 把关，SQL 只做候选过滤）：
	//   - 常规布局：目录是 <base>-SNAPSHOT/（version 列为基础形式 "1.0-SNAPSHOT"）
	//   - 唯一快照布局：目录/version 都是时间戳形式 "1.0-20260703.033633-1"
	// Maven 定义：时间戳形式只存在于快照构建（release 版本不会用该命名），故按版本形状亦为快照语义。
	versionFilter := mavenSnapshotTimestampedFilter(t.db.Dialector.Name())
	err := t.db.WithContext(ctx).
		Model(&model.Artifact{}).
		Where("repository_id = ? AND format = ? AND kind IN ? AND (remote_path LIKE ? OR "+versionFilter+")",
			repoID, "maven", []string{"artifact", "checksum"}, "%-SNAPSHOT/%").
		Select("id, repository_id, name, version, filename, remote_path, created_at").
		Find(&artifacts).Error
	if err != nil {
		return 0, fmt.Errorf("query snapshots: %w", err)
	}
	if len(artifacts) == 0 {
		return 0, nil
	}

	// 按 (name, baseVersion) 分组：时间戳版本与基础版本归到同一组，
	// 避免每个时间戳版本成为单例组（单例组永远满足不了 i >= keepLast，永不清理）。
	type gavKey struct {
		Name        string
		BaseVersion string
	}
	type parsedArtifact struct {
		artifact snapshotArtifact
		build    mavenutil.SnapshotBuild
	}
	groups := make(map[gavKey][]parsedArtifact)
	for _, a := range artifacts {
		b, ok := mavenutil.ParseSnapshotBuild(a.Name, a.Version, a.Filename)
		if !ok {
			continue // 无法解析的文件保留（安全起见）
		}
		key := gavKey{Name: a.Name, BaseVersion: b.BaseVersion}
		groups[key] = append(groups[key], parsedArtifact{artifact: a, build: b})
	}

	var toDelete []snapshotArtifact
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)

	for _, group := range groups {
		// 按构建（timestamp+buildnum）分桶：一个构建的 jar/checksum/pom 视为整体，
		// 同保留同删除，保证 keepLast 按"构建数"计数而非行数。
		type buildKey struct {
			Timestamp string
			BuildNum  int
		}
		buckets := make(map[buildKey][]parsedArtifact)
		var ordered []buildKey
		for _, pa := range group {
			k := buildKey{Timestamp: pa.build.Timestamp, BuildNum: pa.build.BuildNum}
			if _, ok := buckets[k]; !ok {
				ordered = append(ordered, k)
			}
			buckets[k] = append(buckets[k], pa)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].Timestamp != ordered[j].Timestamp {
				return ordered[i].Timestamp > ordered[j].Timestamp
			}
			return ordered[i].BuildNum > ordered[j].BuildNum
		})

		// 保留：前 keepLast 个构建 或 在 maxAgeDays 天内
		for i, k := range ordered {
			if i >= keepLast && !buckets[k][0].build.TimestampT.After(cutoff) {
				for _, pa := range buckets[k] {
					toDelete = append(toDelete, pa.artifact)
				}
			}
		}
	}

	if len(toDelete) == 0 {
		return 0, nil
	}

	// dry-run 模式：只预览会删除的行，不真正删除。默认开启，确认无误后关闭 dry_run 再启用。
	if dryRun {
		previewLog := make([]string, 0, len(toDelete))
		for _, a := range toDelete {
			previewLog = append(previewLog, a.Filename)
		}
		const previewLimit = 30
		if len(previewLog) > previewLimit {
			logrus.WithFields(logrus.Fields{
				"module":  "maven_snapshot",
				"repo":    repoName,
				"would_delete": len(toDelete),
				"preview": strings.Join(previewLog[:previewLimit], ", ") + " …",
			}).Info("Maven snapshot cleanup dry-run: 以下行将被删除（未执行）")
		} else {
			logrus.WithFields(logrus.Fields{
				"module":       "maven_snapshot",
				"repo":         repoName,
				"would_delete": len(toDelete),
				"preview":      strings.Join(previewLog, ", "),
			}).Info("Maven snapshot cleanup dry-run: 以下行将被删除（未执行）")
		}
		return len(toDelete), nil
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

// mavenSnapshotTimestampedFilter 生成识别"时间戳形式版本"的 SQL 候选过滤表达式，
// 用于唯一快照布局（version 列形如 "1.0-20260703.033633-1"）的兜底选中。
func mavenSnapshotTimestampedFilter(dialectName string) string {
	switch strings.ToLower(dialectName) {
	case "postgres", "postgresql":
		return "version ~ '\\-[0-9]{8}\\.[0-9]{6}\\-[0-9]+$'"
	case "mysql":
		return "version REGEXP '-[0-9]{8}\\.[0-9]{6}-[0-9]+$'"
	default: // sqlite
		return "version GLOB '*-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9]-[0-9]*'"
	}
}
