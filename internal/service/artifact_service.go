package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/database/dialect"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// existingQueryChunkSize 限制单条 SELECT 语句中 IN 子句的参数数量，
// 避免 SQLite "too many SQL variables" 错误（SQLite 默认上限 999/32766）。
// 500 留出足够余量给 repository_id 等其它绑定参数。
const existingQueryChunkSize = 500

// ArtifactService 统一的制品管理服务，封装 artifact 与 blob 关联的创建/更新/删除，
// 并自动同步 packages 聚合表，确保所有写入入口的一致性。
//
// 使用方式：
//
//	所有需要写入 artifacts 表的地方（metadata_store、migration executor 等）
//	都应通过此服务操作，而非直接操作 DB。
type ArtifactService struct {
	db                       *gorm.DB
	onCacheInvalid           func() // 可选：packages 表变更后清除搜索缓存的回调
	packageVersionTableOnce  sync.Once
	packageVersionTableReady bool

	// packageRecalcWorker 相关字段用于异步重算 packages 聚合表。
	// SaveBatch 事务提交后，把 seenPackages 投递到 recalcCh，
	// worker goroutine 用独立 db 连接执行 recalcPackageVersions，
	// 避免在事务内做每包 2 次全表扫描导致长事务持锁。
	recalcCh   chan map[string]bool
	recalcStop chan struct{}
	recalcDone chan struct{}
	recalcOnce sync.Once
}

func NewArtifactService(db *gorm.DB) *ArtifactService {
	s := &ArtifactService{db: db}
	s.startPackageRecalcWorker()
	return s
}

// startPackageRecalcWorker 启动异步 worker 处理 packages 聚合表重算。
// worker 用独立 db 连接（不在 SaveBatch 事务内），避免长事务持锁。
// 同一包短时间内多次提交会去重（map 覆盖），worker 按节流间隔执行。
func (s *ArtifactService) startPackageRecalcWorker() {
	s.recalcCh = make(chan map[string]bool, 256)
	s.recalcStop = make(chan struct{})
	s.recalcDone = make(chan struct{})
	util.SafeGo("artifact-service.recalc-worker", s.recalcLoop)
}

func (s *ArtifactService) recalcLoop() {
	defer close(s.recalcDone)
	// 合并 200ms 内到达的多个 seenPackages，避免短时间内对同一包重复重算
	const mergeWindow = 200 * time.Millisecond
	var pending map[string]bool
	var timerC <-chan time.Time
	var timer *time.Timer
	for {
		select {
		case <-s.recalcStop:
			// 关闭时停止 timer，避免 200ms 内部 goroutine 短暂泄漏
			if timer != nil {
				timer.Stop()
			}
			// 关闭时处理剩余任务
			if pending != nil {
				s.executePackageRecalc(pending)
			}
			// 排空 channel，避免丢失已提交任务
			for {
				select {
				case p := <-s.recalcCh:
					if pending == nil {
						pending = p
					} else {
						for k := range p {
							pending[k] = true
						}
					}
					continue
				default:
					if pending != nil {
						s.executePackageRecalc(pending)
					}
					return
				}
			}
		case p := <-s.recalcCh:
			if pending == nil {
				pending = p
				if timer == nil {
					timer = time.NewTimer(mergeWindow)
					timerC = timer.C
				} else {
					timer.Reset(mergeWindow)
				}
			} else {
				for k := range p {
					pending[k] = true
				}
			}
		case <-timerC:
			s.executePackageRecalc(pending)
			pending = nil
			timerC = nil
		}
	}
}

// executePackageRecalc 用独立 db 连接执行 packages 重算，失败仅记录日志。
func (s *ArtifactService) executePackageRecalc(seenPackages map[string]bool) {
	if len(seenPackages) == 0 {
		return
	}
	// 用独立 db 连接（不在事务内），避免长事务持锁
	if err := s.recalcPackageVersions(s.db, seenPackages); err != nil {
		util.WithFields(logrus.Fields{
			util.LogKeyModule: "artifact-service",
			"packageCount":    len(seenPackages),
		}).WithError(err).Warn("async package recalc failed")
	}
}

// Stop 优雅关闭异步 worker，等待剩余任务完成。
func (s *ArtifactService) Stop() {
	s.recalcOnce.Do(func() {
		close(s.recalcStop)
		<-s.recalcDone
	})
}

// SetCacheInvalidationCallback 设置缓存失效回调
func (s *ArtifactService) SetCacheInvalidationCallback(fn func()) {
	s.onCacheInvalid = fn
}

func (s *ArtifactService) notifyCacheInvalidation() {
	if s.onCacheInvalid != nil {
		s.onCacheInvalid()
	}
}

// Save 创建或更新一个 artifact，同步 blob 关联，并自动更新 packages 聚合表。
// 如果 identity_key 已存在则更新，否则新建。
func (s *ArtifactService) Save(ctx context.Context, artifact *runtime.Artifact) error {
	if err := runtime.ValidateArtifactForStore(artifact); err != nil {
		return err
	}

	modelArtifact := s.toModelArtifact(artifact)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Artifact
		isNew := false

		err := tx.Where("repository_id = ?", modelArtifact.RepositoryID).
			Where("format = ?", artifact.Format).
			Where("identity_key = ?", modelArtifact.IdentityKey).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			isNew = true
			if err := tx.Create(modelArtifact).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			modelArtifact.ID = existing.ID
			if err := tx.Save(modelArtifact).Error; err != nil {
				return err
			}
		}

		// 同步 blob 关联
		if err := s.syncBlobRefs(tx, modelArtifact.ID, artifact.BlobRefs); err != nil {
			return err
		}

		hasBlobRefs := hasUsableBlobRefs(artifact.BlobRefs)

		// 同步 packages 聚合表
		if syncErr := s.syncPackageAfterSave(tx, modelArtifact, isNew, hasBlobRefs); syncErr != nil {
			return syncErr
		}
		if shouldAggregatePackageArtifact(modelArtifact, hasBlobRefs) {
			if err := s.recalcPackageVersionSummary(tx, modelArtifact.RepositoryID, modelArtifact.Format, modelArtifact.Name, modelArtifact.Version); err != nil {
				return err
			}
			if err := s.recalcPackageVersions(tx, map[string]bool{
				packageKey(modelArtifact.RepositoryID, modelArtifact.Format, modelArtifact.Name): true,
			}); err != nil {
				return err
			}
		}

		return nil
	})
	if err == nil {
		s.notifyCacheInvalidation()
	}
	return err
}

// SaveBatch 批量创建或更新 artifacts，自动同步 packages 聚合表。
func (s *ArtifactService) SaveBatch(ctx context.Context, artifacts []*runtime.Artifact) error {
	if len(artifacts) == 0 {
		return nil
	}

	for _, a := range artifacts {
		if err := runtime.ValidateArtifactForStore(a); err != nil {
			return err
		}
	}

	// seenPackages 提升到事务外，事务提交后供异步 worker 使用。
	seenPackages := make(map[string]bool) // 用于批量更新 packages 去重

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seenPackageVersions := make(map[string]bool)

		// 批量查询已有记录，避免逐条 SELECT
		modelArtifacts := make([]*model.Artifact, len(artifacts))
		for i, artifact := range artifacts {
			modelArtifacts[i] = s.toModelArtifact(artifact)
		}

		identityKeys := make([]string, 0, len(artifacts))
		repoIDSet := make(map[uint]bool)
		for _, ma := range modelArtifacts {
			if ma.IdentityKey != "" {
				identityKeys = append(identityKeys, ma.IdentityKey)
			}
			repoIDSet[ma.RepositoryID] = true
		}

		existingMap := make(map[string]model.Artifact) // key: identity_key
		if len(identityKeys) > 0 {
			repoIDs := make([]uint, 0, len(repoIDSet))
			for id := range repoIDSet {
				repoIDs = append(repoIDs, id)
			}
			// 分块查询，避免 SQLite "too many SQL variables" 错误
			for _, chunk := range chunkStrings(identityKeys, existingQueryChunkSize) {
				var existing []model.Artifact
				if err := tx.Where("repository_id IN ? AND identity_key IN ?", repoIDs, chunk).
					Find(&existing).Error; err != nil {
					return err
				}
				for _, e := range existing {
					existingMap[e.IdentityKey] = e
				}
			}
		}

		type indexedArtifact struct {
			model *model.Artifact
			index int
		}
		var toCreate []indexedArtifact
		var toUpdate []indexedArtifact

		for i, ma := range modelArtifacts {
			if existing, ok := existingMap[ma.IdentityKey]; ok {
				ma.ID = existing.ID
				toUpdate = append(toUpdate, indexedArtifact{model: ma, index: i})
			} else {
				toCreate = append(toCreate, indexedArtifact{model: ma, index: i})
			}
		}

		// 批量 INSERT
		if len(toCreate) > 0 {
			createBatch := make([]*model.Artifact, len(toCreate))
			for i, ia := range toCreate {
				createBatch[i] = ia.model
			}
			if err := tx.CreateInBatches(createBatch, 100).Error; err != nil {
				return err
			}
			for _, ia := range toCreate {
				if err := s.syncBlobRefs(tx, ia.model.ID, artifacts[ia.index].BlobRefs); err != nil {
					return err
				}
				// 同步 packages
				var scanRepoID uint
				scanUint(artifacts[ia.index].RepositoryID, &scanRepoID)
				pkgKey := packageKey(scanRepoID, artifacts[ia.index].Format, artifacts[ia.index].Name)
				hasBlobRefs := hasUsableBlobRefs(artifacts[ia.index].BlobRefs)
				if shouldAggregatePackageArtifact(ia.model, hasBlobRefs) {
					seenPackageVersions[packageVersionKey(scanRepoID, artifacts[ia.index].Format, artifacts[ia.index].Name, artifacts[ia.index].Version)] = true
					if !seenPackages[pkgKey] {
						seenPackages[pkgKey] = true
						if syncErr := s.syncPackageAfterSave(tx, ia.model, true, hasBlobRefs); syncErr != nil {
							return syncErr
						}
					}
				}
			}
		}

		// 批量 UPDATE
		for _, ia := range toUpdate {
			if err := tx.Save(ia.model).Error; err != nil {
				return err
			}
			if err := s.syncBlobRefs(tx, ia.model.ID, artifacts[ia.index].BlobRefs); err != nil {
				return err
			}
			// 同步 packages
			var scanRepoID uint
			scanUint(artifacts[ia.index].RepositoryID, &scanRepoID)
			pkgKey := packageKey(scanRepoID, artifacts[ia.index].Format, artifacts[ia.index].Name)
			hasBlobRefs := hasUsableBlobRefs(artifacts[ia.index].BlobRefs)
			if shouldAggregatePackageArtifact(ia.model, hasBlobRefs) {
				seenPackageVersions[packageVersionKey(scanRepoID, artifacts[ia.index].Format, artifacts[ia.index].Name, artifacts[ia.index].Version)] = true
				if !seenPackages[pkgKey] {
					seenPackages[pkgKey] = true
					if syncErr := s.syncPackageAfterSave(tx, ia.model, false, hasBlobRefs); syncErr != nil {
						return syncErr
					}
				}
			}
		}

		if err := s.recalcPackageVersionSummaries(tx, seenPackageVersions); err != nil {
			return err
		}
		// seenPackages 副本投递给异步 worker；原 map 供事务内去重使用。
		// packages 表是 read model，弱一致性可接受；事务提交后异步重算 version_count/latest_version，
		// 避免每包 2 次全表扫描在事务内执行导致长事务持锁。
		return nil
	})
	if err == nil {
		s.notifyCacheInvalidation()
		// 投递 seenPackages 副本到异步 worker。channel 满时阻塞最多 100ms，
		// 超时则降级为同步执行，保证 packages 聚合表最终一致。
		if len(seenPackages) > 0 {
			packagesCopy := make(map[string]bool, len(seenPackages))
			for k := range seenPackages {
				packagesCopy[k] = true
			}
			select {
			case s.recalcCh <- packagesCopy:
			case <-time.After(100 * time.Millisecond):
				// channel 满，降级同步执行
				s.executePackageRecalc(packagesCopy)
			}
		}
	}
	return err
}

// Delete 删除 artifact 及其 blob 关联，并同步更新 packages 聚合表。
func (s *ArtifactService) Delete(ctx context.Context, key runtime.ArtifactKey) error {
	if key.IdentityKey == "" && key.RemotePath == "" && key.Name == "" && key.Version == "" && key.Path == "" && key.Filename == "" {
		return runtime.ErrNotFound
	}
	var repoID uint
	scanUint(key.RepositoryID, &repoID)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var artifact model.Artifact
		db := tx.Where("repository_id = ?", repoID).
			Where("format = ?", key.Format).
			Model(&model.Artifact{})
		if key.IdentityKey != "" {
			db = db.Where("identity_key = ?", key.IdentityKey)
		} else if key.RemotePath != "" {
			db = db.Where("remote_path = ?", key.RemotePath)
		} else {
			if key.Name != "" {
				db = db.Where("name = ?", key.Name)
			}
			if key.Version != "" {
				db = db.Where("version = ?", key.Version)
			}
			if key.Path != "" {
				db = db.Where("path = ?", key.Path)
			}
			if key.Filename != "" {
				db = db.Where("filename = ?", key.Filename)
			}
		}
		err := db.First(&artifact).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return runtime.ErrNotFound
			}
			return err
		}

		// 删除 blob 关联
		if err := tx.Where("artifact_id = ?", artifact.ID).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}

		// 删除 artifact
		if err := tx.Delete(&artifact).Error; err != nil {
			return err
		}

		if shouldAggregatePackageArtifactForDelete(&artifact) {
			if err := s.recalcPackageVersionSummary(tx, artifact.RepositoryID, artifact.Format, artifact.Name, artifact.Version); err != nil {
				return err
			}
		}

		// 同步 packages 聚合表
		return s.syncPackageAfterDelete(tx, &artifact)
	})
	if err == nil {
		s.notifyCacheInvalidation()
	}
	return err
}

func (s *ArtifactService) DeletePackage(ctx context.Context, repoID uint, format, name string) error {
	if repoID == 0 || format == "" || name == "" {
		return runtime.ErrNotFound
	}
	return s.deletePackageWhere(ctx, "repository_id = ? AND format = ? AND name = ?", repoID, format, name)
}

func (s *ArtifactService) DeletePackageByCoordinates(ctx context.Context, repoID uint, format, name string) error {
	if format == "" || name == "" {
		return runtime.ErrNotFound
	}
	if repoID > 0 {
		return s.DeletePackage(ctx, repoID, format, name)
	}
	return s.deletePackageWhere(ctx, "format = ? AND name = ?", format, name)
}

func (s *ArtifactService) DeletePackageVersionByCoordinates(ctx context.Context, repoID uint, format, name, version string) error {
	if format == "" || name == "" || version == "" {
		return runtime.ErrNotFound
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		artifacts, err := findPackageVersionArtifacts(tx, repoID, format, name, version)
		if err != nil {
			return err
		}
		if len(artifacts) == 0 {
			return runtime.ErrNotFound
		}

		artifactIDs := make([]uint, 0, len(artifacts))
		representatives := make(map[string]model.Artifact)
		for _, artifact := range artifacts {
			artifactIDs = append(artifactIDs, artifact.ID)
			key := packageKey(artifact.RepositoryID, artifact.Format, artifact.Name)
			if _, ok := representatives[key]; !ok {
				representatives[key] = artifact
			}
		}

		if err := tx.Where("artifact_id IN ?", artifactIDs).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", artifactIDs).Delete(&model.Artifact{}).Error; err != nil {
			return err
		}

		for _, artifact := range representatives {
			if err := s.recalcPackageVersionSummary(tx, artifact.RepositoryID, artifact.Format, artifact.Name, artifact.Version); err != nil {
				return err
			}
			if err := s.syncPackageAfterDelete(tx, &artifact); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.notifyCacheInvalidation()
	}
	return err
}

func (s *ArtifactService) deletePackageWhere(ctx context.Context, where string, args ...interface{}) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var artifactIDs []uint
		if err := tx.Model(&model.Artifact{}).
			Where(where, args...).
			Pluck("id", &artifactIDs).Error; err != nil {
			return err
		}
		if len(artifactIDs) > 0 {
			if err := tx.Where("artifact_id IN ?", artifactIDs).Delete(&model.ArtifactBlob{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", artifactIDs).Delete(&model.Artifact{}).Error; err != nil {
				return err
			}
		}
		if s.hasPackageVersionTable(tx) {
			if err := deletePackageVersionsMatchingPackageWhere(tx, where, args...); err != nil {
				return err
			}
		}
		return tx.Where(where, args...).Delete(&model.Package{}).Error
	})
	if err == nil {
		s.notifyCacheInvalidation()
	}
	return err
}

// RebuildPackages 重建整个 packages 表（从 artifacts 表全量聚合）
func (s *ArtifactService) RebuildPackages(ctx context.Context) error {
	util.WithFields(logrus.Fields{
		util.LogKeyModule: "artifact-service",
	}).Info("Starting to rebuild packages table")

	// 检查 packages 表是否存在
	hasTable := s.db.Migrator().HasTable(&model.Package{})
	if !hasTable {
		util.WithFields(logrus.Fields{
			util.LogKeyModule: "artifact-service",
		}).Info("Packages table does not exist, skipping rebuild")
		return nil
	}

	licenseExpr := dialect.JSONTextExpr(s.db.Dialector.Name(), "a3.attributes", "license")
	descriptionExpr := dialect.JSONTextExpr(s.db.Dialector.Name(), "a4.attributes", "description")

	// 聚合 artifacts 数据
	query := `
		INSERT INTO packages (repository_id, format, name, version_count, latest_version, license, description, created_at, updated_at)
		SELECT
			repository_id,
			format,
			name,
			COUNT(DISTINCT version) AS version_count,
			COALESCE(
				(SELECT a2.version
				 FROM artifacts a2
				 WHERE a2.repository_id = a.repository_id
				   AND a2.format = a.format
				   AND a2.name = a.name
				   AND a2.version IS NOT NULL
				   AND a2.version != ''
				   AND (a2.kind IS NULL OR a2.kind NOT IN ('metadata', 'checksum', 'directory'))
				   AND (
				     a2.format != 'go'
				     OR a2.kind = 'version'
				     OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = a2.id)
				   )
				 ORDER BY a2.updated_at DESC
				 LIMIT 1),
				''
				) AS latest_version,
			COALESCE(
				(SELECT ` + licenseExpr + `
				 FROM artifacts a3
				 WHERE a3.repository_id = a.repository_id
				   AND a3.format = a.format
				   AND a3.name = a.name
				   AND ` + licenseExpr + ` IS NOT NULL
				   AND ` + licenseExpr + ` != ''
				 ORDER BY a3.updated_at DESC
				 LIMIT 1),
				''
			) AS license,
			COALESCE(
				(SELECT ` + descriptionExpr + `
				 FROM artifacts a4
				 WHERE a4.repository_id = a.repository_id
				   AND a4.format = a.format
				   AND a4.name = a.name
				   AND ` + descriptionExpr + ` IS NOT NULL
				   AND ` + descriptionExpr + ` != ''
				 ORDER BY a4.updated_at DESC
				 LIMIT 1),
				''
			) AS description,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
		FROM artifacts a
		WHERE name IS NOT NULL
			  AND name != ''
			  AND version IS NOT NULL
			  AND version != ''
			  AND (kind IS NULL OR kind NOT IN ('metadata', 'checksum', 'directory'))
			  AND (
			    format != 'go'
			    OR kind = 'version'
			    OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = a.id)
			  )
			  AND NOT (
			    format = 'yum'
		    AND (
		      remote_path LIKE 'repodata/%'
		      OR remote_path LIKE '%/repodata/%'
		      OR path = 'repodata'
		      OR path LIKE '%/repodata'
		      OR filename = 'repomd.xml'
		    )
		  )
		GROUP BY repository_id, format, name
	`

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM packages").Error; err != nil {
			return err
		}
		return tx.Exec(query).Error
	})
	if err != nil {
		return err
	}

	util.WithFields(logrus.Fields{
		util.LogKeyModule: "artifact-service",
	}).Info("Packages table rebuilt successfully")

	return nil
}

// RebuildPackageVersions 重建整个 package_versions 表（从 artifacts 表全量聚合）。
// package_versions 是可重建 read model，artifacts 仍是 source of truth。
func (s *ArtifactService) RebuildPackageVersions(ctx context.Context) error {
	util.WithFields(logrus.Fields{
		util.LogKeyModule: "artifact-service",
	}).Info("Starting to rebuild package_versions table")

	if !s.db.Migrator().HasTable(&model.PackageVersion{}) {
		util.WithFields(logrus.Fields{
			util.LogKeyModule: "artifact-service",
		}).Info("Package versions table does not exist, skipping rebuild")
		return nil
	}

	type versionKeyRow struct {
		RepositoryID uint
		Format       string
		Name         string
		Version      string
	}
	var rows []versionKeyRow
	if err := s.db.WithContext(ctx).Model(&model.Artifact{}).
		Select("repository_id, format, name, version").
		Where("name != ''").
		Where("version != ''").
		Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
		Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
		Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
			"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
		Group("repository_id, format, name, version").
		Find(&rows).Error; err != nil {
		return err
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM package_versions").Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := s.recalcPackageVersionSummary(tx, row.RepositoryID, row.Format, row.Name, row.Version); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	util.WithFields(logrus.Fields{
		util.LogKeyModule: "artifact-service",
	}).Info("Package versions table rebuilt successfully")

	return nil
}

func (s *ArtifactService) RefreshPackageVersionSummary(ctx context.Context, repoID uint, format, name, version string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.recalcPackageVersionSummary(tx, repoID, format, name, version)
	})
}

func (s *ArtifactService) UpdatePackageVersionStatus(ctx context.Context, repoID uint, format, name, version, status, reason string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "published", "deprecated", "yanked":
	default:
		return fmt.Errorf("unsupported package version status %q", status)
	}
	if format == "" || name == "" || version == "" {
		return runtime.ErrNotFound
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		artifacts, err := findPackageVersionArtifacts(tx, repoID, format, name, version)
		if err != nil {
			return err
		}
		if len(artifacts) == 0 {
			return runtime.ErrNotFound
		}

		seenRepos := make(map[uint]bool)
		for i := range artifacts {
			artifact := artifacts[i]
			applyArtifactVersionStatus(&artifact, status, reason)
			if err := tx.Model(&model.Artifact{}).Where("id = ?", artifact.ID).Update("metadata", artifact.Metadata).Error; err != nil {
				return err
			}
			seenRepos[artifact.RepositoryID] = true
		}

		for artifactRepoID := range seenRepos {
			if err := s.recalcPackageVersionSummary(tx, artifactRepoID, format, name, version); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.notifyCacheInvalidation()
	}
	return err
}

// ========== 内部辅助方法 ==========

// syncPackageAfterSave 在 artifact 创建/更新后同步 packages 表
func (s *ArtifactService) syncPackageAfterSave(tx *gorm.DB, artifact *model.Artifact, isNew bool, hasBlobRefs bool) error {
	name := artifact.Name
	if name == "" {
		return nil
	}
	if !shouldAggregatePackageArtifact(artifact, hasBlobRefs) {
		return nil
	}

	pkg := &model.Package{
		RepositoryID: artifact.RepositoryID,
		Format:       artifact.Format,
		Name:         name,
	}

	result := tx.Where("repository_id = ? AND format = ? AND name = ?",
		artifact.RepositoryID, artifact.Format, name).First(pkg)

	if result.Error == gorm.ErrRecordNotFound {
		// 创建新记录
		pkg.VersionCount = 1
		pkg.LatestVersion = artifact.Version
		pkg.CreatedAt = artifact.CreatedAt
		pkg.UpdatedAt = artifact.UpdatedAt
		pkg.License = extractJSONBString(artifact.Attributes, "license")
		pkg.Description = extractJSONBString(artifact.Attributes, "description")
		if pkg.CreatedAt.IsZero() {
			pkg.CreatedAt = time.Now()
		}
		if pkg.UpdatedAt.IsZero() {
			pkg.UpdatedAt = time.Now()
		}
		return tx.Create(pkg).Error
	} else if result.Error != nil {
		return result.Error
	}

	// 更新现有记录
	updates := map[string]interface{}{
		"updated_at": artifact.UpdatedAt,
	}
	if isNew {
		updates["version_count"] = gorm.Expr("version_count + 1")
	}
	if artifact.UpdatedAt.After(pkg.UpdatedAt) {
		updates["latest_version"] = artifact.Version
	}
	if license := extractJSONBString(artifact.Attributes, "license"); license != "" && license != pkg.License {
		updates["license"] = license
	}
	if description := extractJSONBString(artifact.Attributes, "description"); description != "" && description != pkg.Description {
		updates["description"] = description
	}
	return tx.Model(pkg).Updates(updates).Error
}

// syncPackageAfterDelete 在 artifact 删除后同步 packages 表
func (s *ArtifactService) syncPackageAfterDelete(tx *gorm.DB, artifact *model.Artifact) error {
	name := artifact.Name
	if name == "" {
		return nil
	}
	if !shouldAggregatePackageArtifactForDelete(artifact) {
		return nil
	}

	pkg := &model.Package{}
	result := tx.Where("repository_id = ? AND format = ? AND name = ?",
		artifact.RepositoryID, artifact.Format, name).First(pkg)

	if result.Error == gorm.ErrRecordNotFound {
		return nil
	} else if result.Error != nil {
		return result.Error
	}

	var remaining int64
	if err := tx.Model(&model.Artifact{}).
		Where("repository_id = ? AND format = ?", artifact.RepositoryID, artifact.Format).
		Where("name = ?", name).
		Where("version != ''").
		Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
		Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
		Count(&remaining).Error; err != nil {
		return err
	}
	if remaining > 0 {
		return s.recalcPackageVersions(tx, map[string]bool{
			packageKey(artifact.RepositoryID, artifact.Format, name): true,
		})
	}

	return tx.Delete(pkg).Error
}

func (s *ArtifactService) recalcPackageVersionSummaries(tx *gorm.DB, seenVersions map[string]bool) error {
	for key := range seenVersions {
		parts := strings.SplitN(key, "|", 4)
		if len(parts) != 4 {
			continue
		}
		var repoID uint
		scanUint(parts[0], &repoID)
		if err := s.recalcPackageVersionSummary(tx, repoID, parts[1], parts[2], parts[3]); err != nil {
			return err
		}
	}
	return nil
}

func (s *ArtifactService) recalcPackageVersionSummary(tx *gorm.DB, repoID uint, format, name, version string) error {
	if repoID == 0 || format == "" || name == "" || version == "" {
		return nil
	}
	if !s.hasPackageVersionTable(tx) {
		return nil
	}

	var artifacts []model.Artifact
	if err := tx.Model(&model.Artifact{}).
		Where("repository_id = ? AND format = ?", repoID, format).
		Where("name = ? AND version = ?", name, version).
		Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
		Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
		Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
			"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
		Order("updated_at DESC").
		Find(&artifacts).Error; err != nil {
		return err
	}
	if len(artifacts) == 0 {
		return tx.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", repoID, format, name, version).
			Delete(&model.PackageVersion{}).Error
	}

	artifactIDs := make([]uint, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifactIDs = append(artifactIDs, artifact.ID)
	}
	blobSizes := make(map[uint]int64)
	hasDownloadedFiles := false
	if len(artifactIDs) > 0 {
		var blobRows []struct {
			ArtifactID uint
			Size       int64
		}
		if err := tx.Table("artifact_blobs AS ab").
			Select("ab.artifact_id, b.size").
			Joins("JOIN blobs b ON b.id = ab.blob_id").
			Where("ab.artifact_id IN ?", artifactIDs).
			Scan(&blobRows).Error; err != nil {
			return err
		}
		for _, row := range blobRows {
			hasDownloadedFiles = true
			blobSizes[row.ArtifactID] += row.Size
		}
	}

	summary := model.PackageVersion{
		RepositoryID: repoID,
		Format:       format,
		PackageName:  name,
		Version:      version,
		Status:       packageVersionStatus(artifacts),
		FileCount:    len(artifacts),
	}
	for _, artifact := range artifacts {
		if summary.Namespace == "" {
			summary.Namespace = artifact.Namespace
		}
		if artifact.CreatedAt.Before(summary.CreatedAt) || summary.CreatedAt.IsZero() {
			summary.CreatedAt = artifact.CreatedAt
		}
		if artifact.UpdatedAt.After(summary.LatestArtifactAt) {
			summary.LatestArtifactAt = artifact.UpdatedAt
		}
		summary.SizeBytes += firstNonZeroInt64(blobSizes[artifact.ID], artifact.SizeBytes)
		if summary.ChecksumSHA256 == "" {
			summary.ChecksumSHA256 = extractJSONBString(artifact.Checksums, "sha256")
		}
		if summary.License == "" {
			summary.License = extractJSONBString(artifact.Attributes, "license")
		}
		if summary.PublishedAt == nil {
			if published := parseRFC3339Ptr(extractJSONBString(artifact.Attributes, "published_at")); published != nil {
				summary.PublishedAt = published
			}
		}
	}
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now()
	}
	if summary.LatestArtifactAt.IsZero() {
		summary.LatestArtifactAt = time.Now()
	}
	summary.UpdatedAt = summary.LatestArtifactAt
	summary.FilesDownloaded = hasDownloadedFiles

	var existing model.PackageVersion
	err := tx.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", repoID, format, name, version).
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return tx.Create(&summary).Error
	}
	if err != nil {
		return err
	}

	return tx.Model(&existing).Updates(map[string]interface{}{
		"namespace":          summary.Namespace,
		"status":             summary.Status,
		"published_at":       summary.PublishedAt,
		"latest_artifact_at": summary.LatestArtifactAt,
		"file_count":         summary.FileCount,
		"files_downloaded":   summary.FilesDownloaded,
		"size_bytes":         summary.SizeBytes,
		"license":            summary.License,
		"checksum_sha256":    summary.ChecksumSHA256,
		"updated_at":         summary.UpdatedAt,
	}).Error
}

func (s *ArtifactService) hasPackageVersionTable(tx *gorm.DB) bool {
	s.packageVersionTableOnce.Do(func() {
		s.packageVersionTableReady = tx.Migrator().HasTable(&model.PackageVersion{})
	})
	return s.packageVersionTableReady
}

// syncBlobRefs 同步 artifact 与 blob 的关联关系。
// 如果 blobRefs 为空，说明此次更新不涉及 blob（如元数据刷新），保留已有关联不变。
func (s *ArtifactService) syncBlobRefs(tx *gorm.DB, artifactID uint, blobRefs []runtime.BlobRef) error {
	if len(blobRefs) == 0 {
		return nil
	}
	if err := tx.Where("artifact_id = ?", artifactID).Delete(&model.ArtifactBlob{}).Error; err != nil {
		return err
	}
	for i, ref := range blobRefs {
		if ref.BlobID == 0 {
			continue
		}
		ab := &model.ArtifactBlob{
			ArtifactID: artifactID,
			BlobID:     ref.BlobID,
			Position:   i,
		}
		if err := tx.Create(ab).Error; err != nil {
			return err
		}
	}
	return nil
}

// toModelArtifact 将 runtime.Artifact 转换为 model.Artifact。
// 前置条件：调用方必须确保 artifact 已通过 ValidateArtifactForStore 归一化。
func (s *ArtifactService) toModelArtifact(t *runtime.Artifact) *model.Artifact {

	metadata := make(model.JSONB)
	for k, v := range t.Properties {
		metadata[k] = v
	}

	var repoID uint
	scanUint(t.RepositoryID, &repoID)

	return &model.Artifact{
		RepositoryID: repoID,
		Format:       t.Format,
		Kind:         t.Kind,
		IdentityKey:  t.IdentityKey,
		Name:         t.Name,
		Namespace:    t.Namespace,
		Version:      t.Version,
		Path:         t.Path,
		Filename:     t.Filename,
		RemotePath:   t.RemotePath,
		DownloadURL:  t.DownloadURL,
		Extension:    t.Extension,
		ContentType:  t.ContentType,
		SizeBytes:    t.SizeBytes,
		Checksums:    stringMapToJSONB(t.Checksums),
		Qualifiers:   stringMapToJSONB(t.Qualifiers),
		Attributes:   stringMapToJSONB(t.Attributes),
		Metadata:     metadata,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

// ========== 辅助函数 ==========

// recalcPackageVersions 批量写入后，精确重新统计涉及 packages 的 version_count 和 latest_version
func (s *ArtifactService) recalcPackageVersions(tx *gorm.DB, seenPackages map[string]bool) error {
	for pkgKeyStr := range seenPackages {
		// pkgKey 格式: "repoID|format|name"
		parts := strings.SplitN(pkgKeyStr, "|", 3)
		if len(parts) != 3 {
			continue
		}
		var repoID uint
		scanUint(parts[0], &repoID)
		format := parts[1]
		name := parts[2]

		var count int64
		if err := tx.Model(&model.Artifact{}).
			Where("repository_id = ? AND format = ?", repoID, format).
			Where("name = ?", name).
			Where("version != ''").
			Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
			Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
			Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
				"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
			Distinct("version").
			Count(&count).Error; err != nil {
			return err
		}

		var latest model.Artifact
		latestVersion := ""
		if err := tx.Where("repository_id = ? AND format = ?", repoID, format).
			Where("name = ?", name).
			Where("version != ''").
			Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
			Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
			Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
				"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
			Order("updated_at DESC").First(&latest).Error; err != nil {
			// 没有 version 的 artifact（如 generic/raw 文件）查不到记录属正常，
			// latest_version 留空即可，不应让上传事务失败。
			if err != gorm.ErrRecordNotFound {
				return err
			}
		} else {
			latestVersion = latest.Version
		}

		if err := tx.Model(&model.Package{}).
			Where("repository_id = ? AND format = ? AND name = ?", repoID, format, name).
			Updates(map[string]interface{}{
				"version_count":  count,
				"latest_version": latestVersion,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func extractJSONBString(data model.JSONB, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func shouldAggregatePackageArtifact(artifact *model.Artifact, hasBlobRefs bool) bool {
	if artifact == nil || artifact.Name == "" {
		return false
	}
	if runtime.IsCatalogExcludedKind(artifact.Kind) {
		return false
	}
	if isYumRepodataArtifact(artifact.Format, artifact.Path, artifact.Filename, artifact.RemotePath) {
		return false
	}
	if artifact.Format == "go" && artifact.Kind != runtime.KindVersion && !hasBlobRefs {
		return false
	}
	return true
}

func shouldAggregatePackageArtifactForDelete(artifact *model.Artifact) bool {
	if artifact == nil || artifact.Name == "" {
		return false
	}
	if runtime.IsCatalogExcludedKind(artifact.Kind) {
		return false
	}
	if isYumRepodataArtifact(artifact.Format, artifact.Path, artifact.Filename, artifact.RemotePath) {
		return false
	}
	return true
}

func catalogExcludedKinds() []string {
	return []string{runtime.KindMetadata, runtime.KindChecksum, runtime.KindDirectory}
}

func hasUsableBlobRefs(refs []runtime.BlobRef) bool {
	for _, ref := range refs {
		if ref.BlobID > 0 {
			return true
		}
	}
	return false
}

func isYumRepodataArtifact(format, artifactPath, filename, remotePath string) bool {
	if format != "yum" {
		return false
	}
	artifactPath = strings.Trim(strings.ReplaceAll(artifactPath, "\\", "/"), "/")
	remotePath = strings.Trim(strings.ReplaceAll(remotePath, "\\", "/"), "/")
	filename = strings.Trim(filename, "/")
	return strings.HasPrefix(remotePath, "repodata/") ||
		strings.Contains(remotePath, "/repodata/") ||
		artifactPath == "repodata" ||
		strings.HasSuffix(artifactPath, "/repodata") ||
		filename == "repomd.xml"
}

func findPackageVersionArtifacts(tx *gorm.DB, repoID uint, format, name, version string) ([]model.Artifact, error) {
	db := tx.Model(&model.Artifact{}).
		Where("format = ?", format).
		Where("name = ? AND version = ?", name, version).
		Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
		Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
		Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
			"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
		Order("updated_at DESC")
	if repoID > 0 {
		db = db.Where("repository_id = ?", repoID)
	}
	var artifacts []model.Artifact
	return artifacts, db.Find(&artifacts).Error
}

func applyArtifactVersionStatus(artifact *model.Artifact, status, reason string) {
	if artifact.Metadata == nil {
		artifact.Metadata = make(model.JSONB)
	}
	artifact.Metadata["status"] = status
	switch status {
	case "deprecated":
		artifact.Metadata["deprecation_reason"] = reason
		delete(artifact.Metadata, "yank_reason")
	case "yanked":
		artifact.Metadata["yank_reason"] = reason
		delete(artifact.Metadata, "deprecation_reason")
	case "published":
		delete(artifact.Metadata, "deprecation_reason")
		delete(artifact.Metadata, "yank_reason")
	}
}

func packageVersionStatus(artifacts []model.Artifact) string {
	status := "published"
	for _, artifact := range artifacts {
		switch artifactStatus(artifact) {
		case "yanked":
			return "yanked"
		case "deprecated":
			status = "deprecated"
		}
	}
	return status
}

func artifactStatus(artifact model.Artifact) string {
	status := extractJSONBString(artifact.Metadata, "status")
	if status == "" {
		status = extractJSONBString(artifact.Attributes, "status")
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "yanked":
		return "yanked"
	case "deprecated":
		return "deprecated"
	default:
		return "published"
	}
}

func packageKey(repoID uint, format, name string) string {
	return fmt.Sprintf("%d|%s|%s", repoID, format, name)
}

func packageVersionKey(repoID uint, format, name, version string) string {
	if version == "" {
		return ""
	}
	return fmt.Sprintf("%d|%s|%s|%s", repoID, format, name, version)
}

func deletePackageVersionsMatchingPackageWhere(tx *gorm.DB, where string, args ...interface{}) error {
	// 精确替换列名 "name" 为 "package_name"，避免误替换含 "name" 子串的其他列名
	where = replaceColumnName(where, "name", "package_name")
	return tx.Where(where, args...).Delete(&model.PackageVersion{}).Error
}

// replaceColumnName 精确替换 SQL WHERE 子句中的列名（按词边界匹配）
func replaceColumnName(where, oldCol, newCol string) string {
	// 匹配 oldCol 作为独立标识符：前面是空格/开头/逗号，后面是空格/?/>=/<=/!=/</>/结尾
	re := regexp.MustCompile(`(?i)(^|[\s,])` + regexp.QuoteMeta(oldCol) + `([\s?=<>,)]|$)`)
	return re.ReplaceAllStringFunc(where, func(match string) string {
		return strings.Replace(match, oldCol, newCol, 1)
	})
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func parseRFC3339Ptr(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func scanUint(s string, v *uint) {
	if s == "" {
		return
	}
	_, _ = fmt.Sscanf(s, "%d", v)
}

func stringMapToJSONB(src map[string]string) model.JSONB {
	if len(src) == 0 {
		return nil
	}
	dst := make(model.JSONB, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// chunkStrings 将切片按 size 分块，用于限制 SQL IN 子句的参数数量。
func chunkStrings(s []string, size int) [][]string {
	if size <= 0 || len(s) == 0 {
		return [][]string{s}
	}
	var chunks [][]string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}
