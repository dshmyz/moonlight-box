package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ProxyMetadataCacheGC 实现 ScheduledTask，清理 proxy 仓库中"已缓存但从未下载"的元数据行。
// 判定：仓库为 proxy 类型 + 无 blob 引用（内容从未下载进 CAS）+ updated_at 超过 max_age_days。
// 无 blob 意味着删除零内容损失（下次查询会回源重建），且天然不会触碰有内容的缓存与 hosted 数据。
// 删除走 ArtifactService.BatchDelete，保持 packages/package_versions 聚合表同步。
type ProxyMetadataCacheGC struct {
	db          *gorm.DB
	artifactSvc *ArtifactService
	configSvc   *SystemConfigService

	mu         sync.RWMutex
	enabled    bool
	maxAgeDays int
}

// gcBatchSize 每批查询并删除的行数上限，避免巨型 Pluck 与巨型删除事务。
const gcBatchSize = 1000

func NewProxyMetadataCacheGC(db *gorm.DB, artifactSvc *ArtifactService, configSvc *SystemConfigService) *ProxyMetadataCacheGC {
	return &ProxyMetadataCacheGC{
		db:          db,
		artifactSvc: artifactSvc,
		configSvc:   configSvc,
		enabled:     true,
		maxAgeDays:  30,
	}
}

func (t *ProxyMetadataCacheGC) Name() string { return "proxy_metadata_cache_gc" }

// LoadConfig 启动时调用一次，从 SystemConfigService 加载配置。
func (t *ProxyMetadataCacheGC) LoadConfig() {
	t.loadConfig()
}

func (t *ProxyMetadataCacheGC) loadConfig() {
	if t.configSvc == nil {
		return
	}
	if v, err := t.configSvc.Get("proxy_cache_gc.enabled"); err == nil {
		if b, err := strconv.ParseBool(v.Value); err == nil {
			t.mu.Lock()
			t.enabled = b
			t.mu.Unlock()
		}
	}
	if v, err := t.configSvc.Get("proxy_cache_gc.max_age_days"); err == nil {
		if n, err := strconv.Atoi(v.Value); err == nil && n > 0 {
			t.mu.Lock()
			t.maxAgeDays = n
			t.mu.Unlock()
		}
	}
}

func (t *ProxyMetadataCacheGC) getConfig() (enabled bool, maxAgeDays int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled, t.maxAgeDays
}

// ConfigFields 实现 ConfigurableTask，声明可配置参数。
func (t *ProxyMetadataCacheGC) ConfigFields() []TaskConfigField {
	enabled, maxAgeDays := t.getConfig()
	return []TaskConfigField{
		{Key: "enabled", Label: "启用清理", Kind: ConfigKindBool, Value: enabled},
		{Key: "max_age_days", Label: "未下载元数据保留天数", Kind: ConfigKindInt, Value: maxAgeDays},
	}
}

// UpdateConfig 实现 ConfigurableTask，校验并持久化配置。
func (t *ProxyMetadataCacheGC) UpdateConfig(values map[string]any, updatedBy uint) error {
	if t.configSvc == nil {
		return fmt.Errorf("proxy metadata cache gc: config service unavailable")
	}
	enabled, err := configBoolField(values, "enabled")
	if err != nil {
		return err
	}
	maxAgeDays, err := configIntField(values, "max_age_days")
	if err != nil {
		return err
	}
	if err := t.configSvc.Set("proxy_cache_gc.enabled", strconv.FormatBool(enabled), "bool", "scheduler", "启用代理缓存元数据清理", false, updatedBy); err != nil {
		return fmt.Errorf("set proxy_cache_gc.enabled: %w", err)
	}
	if err := t.configSvc.Set("proxy_cache_gc.max_age_days", strconv.Itoa(maxAgeDays), "int", "scheduler", "未下载元数据保留天数", false, updatedBy); err != nil {
		return fmt.Errorf("set proxy_cache_gc.max_age_days: %w", err)
	}
	t.loadConfig()
	return nil
}

// Run 实现 ScheduledTask.Run，删除过期且无 blob 引用的 proxy 元数据行。
func (t *ProxyMetadataCacheGC) Run(ctx context.Context) (int, error) {
	enabled, maxAgeDays := t.getConfig()
	if !enabled {
		return 0, nil
	}
	if t.artifactSvc == nil {
		return 0, errors.New("proxy metadata cache gc: artifact service required")
	}
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	var repos []model.Repository
	if err := t.db.WithContext(ctx).Model(&model.Repository{}).
		Where("type = ?", model.RepoTypeProxy).Find(&repos).Error; err != nil {
		return 0, fmt.Errorf("proxy metadata cache gc: %w", err)
	}

	deleted := 0
	for _, repo := range repos {
		// 分批查询+删除：一次 Pluck 全部 ID 在大库（百万级行）会撑爆内存，
		// 单条 BatchDelete 也会产生巨型事务；每批删完再查，天然翻页。
		for {
			if err := ctx.Err(); err != nil {
				return deleted, fmt.Errorf("proxy metadata cache gc repo %s: %w", repo.Name, err)
			}
			var ids []uint
			if err := t.db.WithContext(ctx).Model(&model.Artifact{}).
				Where("repository_id = ? AND updated_at < ?", repo.ID, cutoff).
				Where("NOT EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id)").
				Limit(gcBatchSize).
				Pluck("id", &ids).Error; err != nil {
				return deleted, fmt.Errorf("proxy metadata cache gc repo %s: %w", repo.Name, err)
			}
			if len(ids) == 0 {
				break
			}
			// 走 ArtifactService.BatchDelete，事务内同步 packages/package_versions 聚合，
			// 避免直接删除 artifact 后聚合表残留孤儿版本行（go 代理 KindVersion 占位行会被聚合）。
			if err := t.artifactSvc.BatchDelete(ctx, repo.ID, ids); err != nil {
				return deleted, fmt.Errorf("proxy metadata cache gc repo %s: %w", repo.Name, err)
			}
			deleted += len(ids)
			if len(ids) < gcBatchSize {
				break
			}
		}
	}

	if deleted > 0 {
		logrus.WithFields(logrus.Fields{
			"module":       "proxy_cache_gc",
			"deleted":      deleted,
			"max_age_days": maxAgeDays,
		}).Info("Proxy metadata cache GC deleted stale rows")
	}
	return deleted, nil
}

func (t *ProxyMetadataCacheGC) Reload() {
	t.loadConfig()
	enabled, maxAgeDays := t.getConfig()
	logrus.WithFields(logrus.Fields{
		"module":       "proxy_cache_gc",
		"enabled":      enabled,
		"max_age_days": maxAgeDays,
	}).Info("Proxy metadata cache GC config reloaded")
}

func (t *ProxyMetadataCacheGC) Stop() {}
