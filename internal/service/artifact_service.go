package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ArtifactService 统一的制品管理服务，封装 artifact 与 blob 关联的创建/更新/删除，
// 并自动同步 packages 聚合表，确保所有写入入口的一致性。
//
// 使用方式：
//
//	所有需要写入 artifacts 表的地方（metadata_store、migration executor 等）
//	都应通过此服务操作，而非直接操作 DB。
type ArtifactService struct {
	db             *gorm.DB
	onCacheInvalid func() // 可选：packages 表变更后清除搜索缓存的回调
}

func NewArtifactService(db *gorm.DB) *ArtifactService {
	return &ArtifactService{db: db}
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

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seenPackages := make(map[string]bool) // 用于批量更新 packages 去重

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
			var existing []model.Artifact
			if err := tx.Where("repository_id IN ? AND identity_key IN ?", repoIDs, identityKeys).
				Find(&existing).Error; err != nil {
				return err
			}
			for _, e := range existing {
				existingMap[e.IdentityKey] = e
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
				if shouldAggregatePackageArtifact(ia.model, hasBlobRefs) && !seenPackages[pkgKey] {
					seenPackages[pkgKey] = true
					if syncErr := s.syncPackageAfterSave(tx, ia.model, true, hasBlobRefs); syncErr != nil {
						return syncErr
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
			if shouldAggregatePackageArtifact(ia.model, hasBlobRefs) && !seenPackages[pkgKey] {
				seenPackages[pkgKey] = true
				if syncErr := s.syncPackageAfterSave(tx, ia.model, false, hasBlobRefs); syncErr != nil {
					return syncErr
				}
			}
		}

		// 批量结束后，精确更新所有涉及 packages 的 version_count
		return s.recalcPackageVersions(tx, seenPackages)
	})
	if err == nil {
		s.notifyCacheInvalidation()
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

	licenseExpr := jsonTextExpr(s.db, "a3.attributes", "license")
	descriptionExpr := jsonTextExpr(s.db, "a4.attributes", "description")

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
		if err := tx.Where("repository_id = ? AND format = ?", repoID, format).
			Where("name = ?", name).
			Where("version != ''").
			Where("(kind IS NULL OR kind NOT IN ?)", catalogExcludedKinds()).
			Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
			Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
				"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
			Order("updated_at DESC").First(&latest).Error; err != nil {
			return err
		}

		latestVersion := latest.Version

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

func jsonTextExpr(db *gorm.DB, column, key string) string {
	switch db.Dialector.Name() {
	case "postgres":
		return column + "->>'" + key + "'"
	case "mysql":
		return "JSON_UNQUOTE(JSON_EXTRACT(" + column + ", '$." + key + "'))"
	default:
		return "JSON_EXTRACT(" + column + ", '$." + key + "')"
	}
}

func packageKey(repoID uint, format, name string) string {
	return fmt.Sprintf("%d|%s|%s", repoID, format, name)
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
