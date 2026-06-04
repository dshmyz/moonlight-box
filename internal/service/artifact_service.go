package service

import (
	"context"
	"encoding/json"
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
// 如果 coordinates 已存在则更新，否则新建。
func (s *ArtifactService) Save(ctx context.Context, artifact *runtime.Artifact) error {
	if err := runtime.ValidateArtifactForStore(artifact); err != nil {
		return err
	}

	modelArtifact := s.toModelArtifact(artifact)
	coordsJSON, _ := json.Marshal(artifact.Coordinates)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Artifact
		isNew := false

		err := tx.Where("repository_id = ?", modelArtifact.RepositoryID).
			Where("format = ?", artifact.Format).
			Where("coordinates = ?", coordsJSON).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// 精确匹配失败，尝试用核心坐标匹配（去掉 path 字段）
			coreCoords := make(map[string]string)
			for k, v := range artifact.Coordinates {
				if k != "path" {
					coreCoords[k] = v
				}
			}
			if len(coreCoords) != len(artifact.Coordinates) {
				coreCoordsJSON, _ := json.Marshal(coreCoords)
				err = tx.Where("repository_id = ?", modelArtifact.RepositoryID).
					Where("format = ?", artifact.Format).
					Where("coordinates = ?", coreCoordsJSON).
					First(&existing).Error
			}
		}

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

		// 同步 packages 聚合表
		if syncErr := s.syncPackageAfterSave(tx, modelArtifact, isNew); syncErr != nil {
			return syncErr
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

		for _, artifact := range artifacts {
			modelArtifact := s.toModelArtifact(artifact)
			coordsJSON, _ := json.Marshal(artifact.Coordinates)
			isNew := false

			var existing model.Artifact
			err := tx.Where("repository_id = ?", modelArtifact.RepositoryID).
				Where("format = ?", artifact.Format).
				Where("coordinates = ?", coordsJSON).
				First(&existing).Error

			if err == gorm.ErrRecordNotFound {
				// 回退到"核心坐标"匹配
				coreCoords := make(map[string]string)
				for k, v := range artifact.Coordinates {
					if k != "path" {
						coreCoords[k] = v
					}
				}
				if len(coreCoords) != len(artifact.Coordinates) {
					coreCoordsJSON, _ := json.Marshal(coreCoords)
					err = tx.Where("repository_id = ?", modelArtifact.RepositoryID).
						Where("format = ?", artifact.Format).
						Where("coordinates = ?", coreCoordsJSON).
						First(&existing).Error
				}
			}

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

			if err := s.syncBlobRefs(tx, modelArtifact.ID, artifact.BlobRefs); err != nil {
				return err
			}

			// 去重后同步 packages（批量场景下同名的多个 artifact 只需同步一次）
			var scanRepoID uint
			scanUint(artifact.RepositoryID, &scanRepoID)
			pkgKey := packageKey(scanRepoID, artifact.Format, artifact.Coordinates["name"])
			if !seenPackages[pkgKey] {
				seenPackages[pkgKey] = true
				if syncErr := s.syncPackageAfterSave(tx, modelArtifact, isNew); syncErr != nil {
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
	coordsJSON, _ := json.Marshal(key.Coordinates)
	var repoID uint
	scanUint(key.RepositoryID, &repoID)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var artifact model.Artifact
		err := tx.Where("repository_id = ?", repoID).
			Where("format = ?", key.Format).
			Where("coordinates = ?", coordsJSON).
			First(&artifact).Error
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

	// 清空 packages 表
	if err := s.db.WithContext(ctx).Exec("DELETE FROM packages").Error; err != nil {
		return err
	}

	// 聚合 artifacts 数据
	query := `
		INSERT INTO packages (repository_id, format, name, version_count, latest_version, created_at, updated_at)
		SELECT
			repository_id,
			format,
			JSON_UNQUOTE(JSON_EXTRACT(coordinates, '$.name')) AS name,
			COUNT(*) AS version_count,
			COALESCE(
				(SELECT JSON_UNQUOTE(JSON_EXTRACT(a2.coordinates, '$.version'))
				 FROM artifacts a2
				 WHERE a2.repository_id = a.repository_id
				   AND a2.format = a.format
				   AND JSON_UNQUOTE(JSON_EXTRACT(a2.coordinates, '$.name')) = JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name'))
				 ORDER BY a2.updated_at DESC
				 LIMIT 1),
				''
			) AS latest_version,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
		FROM artifacts a
		WHERE JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name')) IS NOT NULL
		  AND JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name')) != ''
		GROUP BY repository_id, format, JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name'))
	`

	if err := s.db.WithContext(ctx).Exec(query).Error; err != nil {
		return err
	}

	util.WithFields(logrus.Fields{
		util.LogKeyModule: "artifact-service",
	}).Info("Packages table rebuilt successfully")

	return nil
}

// ========== 内部辅助方法 ==========

// syncPackageAfterSave 在 artifact 创建/更新后同步 packages 表
func (s *ArtifactService) syncPackageAfterSave(tx *gorm.DB, artifact *model.Artifact, isNew bool) error {
	name := extractCoordsValue(artifact.Coordinates, "name")
	if name == "" {
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
		pkg.LatestVersion = extractCoordsValue(artifact.Coordinates, "version")
		pkg.CreatedAt = artifact.CreatedAt
		pkg.UpdatedAt = artifact.UpdatedAt
		pkg.License = extractCoordsValue(artifact.Metadata, "license")
		pkg.Description = extractCoordsValue(artifact.Metadata, "description")
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
		updates["latest_version"] = extractCoordsValue(artifact.Coordinates, "version")
	}
	return tx.Model(pkg).Updates(updates).Error
}

// syncPackageAfterDelete 在 artifact 删除后同步 packages 表
func (s *ArtifactService) syncPackageAfterDelete(tx *gorm.DB, artifact *model.Artifact) error {
	name := extractCoordsValue(artifact.Coordinates, "name")
	if name == "" {
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

	if pkg.VersionCount > 1 {
		updates := map[string]interface{}{
			"version_count": gorm.Expr("version_count - 1"),
		}
		deletedVersion := extractCoordsValue(artifact.Coordinates, "version")
		if deletedVersion == pkg.LatestVersion {
			// 重新计算最新版本
			var latestArtifact model.Artifact
			if err := tx.Where("repository_id = ? AND format = ?",
				artifact.RepositoryID, artifact.Format).
				Where("JSON_UNQUOTE(JSON_EXTRACT(coordinates, '$.name')) = ?", name).
				Order("updated_at DESC").
				First(&latestArtifact).Error; err == nil {
				updates["latest_version"] = extractCoordsValue(latestArtifact.Coordinates, "version")
			}
		}
		return tx.Model(pkg).Updates(updates).Error
	}

	return tx.Delete(pkg).Error
}

// syncBlobRefs 同步 artifact 与 blob 的关联关系
func (s *ArtifactService) syncBlobRefs(tx *gorm.DB, artifactID uint, blobRefs []runtime.BlobRef) error {
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

// toModelArtifact 将 runtime.Artifact 转换为 model.Artifact
func (s *ArtifactService) toModelArtifact(t *runtime.Artifact) *model.Artifact {
	coords := make(model.JSONB)
	for k, v := range t.Coordinates {
		coords[k] = v
	}

	metadata := make(model.JSONB)
	for k, v := range t.Properties {
		metadata[k] = v
	}

	if filename, _ := coords["filename"].(string); filename != "" && metadata["download_path"] == nil {
		if p, _ := coords["path"].(string); p != "" {
			if p == filename || strings.HasSuffix(p, "/"+filename) {
				metadata["download_path"] = p
			} else {
				metadata["download_path"] = strings.TrimRight(p, "/") + "/" + filename
			}
		} else {
			metadata["download_path"] = filename
		}
	}

	var repoID uint
	scanUint(t.RepositoryID, &repoID)

	return &model.Artifact{
		RepositoryID: repoID,
		Format:       t.Format,
		Kind:         t.Kind,
		Coordinates:  coords,
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
			Where("JSON_UNQUOTE(JSON_EXTRACT(coordinates, '$.name')) = ?", name).
			Count(&count).Error; err != nil {
			return err
		}

		var latest model.Artifact
		if err := tx.Where("repository_id = ? AND format = ?", repoID, format).
			Where("JSON_UNQUOTE(JSON_EXTRACT(coordinates, '$.name')) = ?", name).
			Order("updated_at DESC").First(&latest).Error; err != nil {
			return err
		}

		latestVersion := extractCoordsValue(latest.Coordinates, "version")

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

func extractCoordsValue(coords model.JSONB, key string) string {
	if coords == nil {
		return ""
	}
	v, ok := coords[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
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

// 导入 fmt（已在辅助函数中使用）
