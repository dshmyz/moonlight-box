package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type MetadataStore struct {
	db          *gorm.DB
	artifactSvc ArtifactServiceAdapter
}

// ArtifactServiceAdapter 适配 service.ArtifactService，使 MetadataStore 能调用它。
// 使用接口避免循环依赖。
type ArtifactServiceAdapter interface {
	Save(ctx context.Context, artifact *runtime.Artifact) error
	SaveBatch(ctx context.Context, artifacts []*runtime.Artifact) error
	Delete(ctx context.Context, key runtime.ArtifactKey) error
}

func NewMetadataStore(db *gorm.DB) *MetadataStore {
	return &MetadataStore{db: db}
}

// NewMetadataStoreWithArtifactService 创建带 ArtifactService 的 MetadataStore
func NewMetadataStoreWithArtifactService(db *gorm.DB, svc ArtifactServiceAdapter) *MetadataStore {
	return &MetadataStore{db: db, artifactSvc: svc}
}

func (s *MetadataStore) Get(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	var artifact model.Artifact

	coordsJSON, _ := json.Marshal(key.Coordinates)

	var repoID uint
	fmt.Sscanf(key.RepositoryID, "%d", &repoID)

	err := s.db.WithContext(ctx).
		Where("repository_id = ?", repoID).
		Where("format = ?", key.Format).
		Where("coordinates = ?", coordsJSON).
		First(&artifact).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, runtime.ErrNotFound
		}
		return nil, err
	}

	result := s.toTypesArtifact(&artifact)
	s.fillBlobRefs(ctx, []*runtime.Artifact{result})
	return result, nil
}

func (s *MetadataStore) Put(ctx context.Context, artifact *runtime.Artifact) error {
	if s.artifactSvc != nil {
		return s.artifactSvc.Save(ctx, artifact)
	}

	// 回退：直接操作 DB（无 packages 同步）
	if err := runtime.ValidateArtifactForStore(artifact); err != nil {
		return err
	}
	modelArtifact := s.toModelArtifact(artifact)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Artifact
		coordsJSON, _ := json.Marshal(artifact.Coordinates)

		err := tx.Where("repository_id = ?", artifact.RepositoryID).
			Where("format = ?", artifact.Format).
			Where("coordinates = ?", coordsJSON).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
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
		return nil
	})
}

func (s *MetadataStore) BatchPut(ctx context.Context, artifacts []*runtime.Artifact) error {
	if s.artifactSvc != nil {
		return s.artifactSvc.SaveBatch(ctx, artifacts)
	}

	// 回退：直接操作 DB（无 packages 同步）
	if len(artifacts) == 0 {
		return nil
	}

	for _, a := range artifacts {
		if err := runtime.ValidateArtifactForStore(a); err != nil {
			return err
		}
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, artifact := range artifacts {
			modelArtifact := s.toModelArtifact(artifact)

			var existing model.Artifact
			coordsJSON, _ := json.Marshal(artifact.Coordinates)

			err := tx.Where("repository_id = ?", artifact.RepositoryID).
				Where("format = ?", artifact.Format).
				Where("coordinates = ?", coordsJSON).
				First(&existing).Error

			if err == gorm.ErrRecordNotFound {
				// 精确匹配失败，尝试用核心坐标匹配（去掉 path 字段），
				// 以兼容旧记录缺少 path 字段的情况。
				coreCoords := make(map[string]string)
				for k, v := range artifact.Coordinates {
					if k != "path" {
						coreCoords[k] = v
					}
				}
				if len(coreCoords) != len(artifact.Coordinates) {
					coreCoordsJSON, _ := json.Marshal(coreCoords)
					err = tx.Where("repository_id = ?", artifact.RepositoryID).
						Where("format = ?", artifact.Format).
						Where("coordinates = ?", coreCoordsJSON).
						First(&existing).Error
				}
			}

			if err == gorm.ErrRecordNotFound {
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
		}
		return nil
	})
}

func (s *MetadataStore) syncBlobRefs(tx *gorm.DB, artifactID uint, blobRefs []runtime.BlobRef) error {
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

func (s *MetadataStore) Delete(ctx context.Context, key runtime.ArtifactKey) error {
	if s.artifactSvc != nil {
		return s.artifactSvc.Delete(ctx, key)
	}

	// 回退：直接操作 DB（无 packages 同步）
	coordsJSON, _ := json.Marshal(key.Coordinates)

	var repoID uint
	fmt.Sscanf(key.RepositoryID, "%d", &repoID)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

		if err := tx.Where("artifact_id = ?", artifact.ID).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&artifact).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *MetadataStore) List(ctx context.Context, repoID string) ([]*runtime.Artifact, error) {
	var artifacts []model.Artifact

	err := s.db.WithContext(ctx).
		Where("repository_id = ?", repoID).
		Find(&artifacts).Error

	if err != nil {
		return nil, err
	}

	result := make([]*runtime.Artifact, len(artifacts))
	for i, a := range artifacts {
		result[i] = s.toTypesArtifact(&a)
	}
	s.fillBlobRefs(ctx, result)
	return result, nil
}

func (s *MetadataStore) Query(ctx context.Context, query runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	db := s.db.WithContext(ctx).Model(&model.Artifact{})

	if query.RepositoryID != "" {
		var repoID uint
		fmt.Sscanf(query.RepositoryID, "%d", &repoID)
		db = db.Where("repository_id = ?", repoID)
	}

	if query.Format != "" {
		db = db.Where("format = ?", query.Format)
	}

	if len(query.Coordinates) > 0 {
		for k, v := range query.Coordinates {
			pattern := fmt.Sprintf("%%\"%s\":\"%s\"%%", k, v)
			db = db.Where("coordinates LIKE ?", pattern)
		}
	}

	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}

	var artifacts []model.Artifact
	if err := db.Find(&artifacts).Error; err != nil {
		return nil, err
	}

	result := make([]*runtime.Artifact, len(artifacts))
	for i, a := range artifacts {
		result[i] = s.toTypesArtifact(&a)
	}
	s.fillBlobRefs(ctx, result)
	return result, nil
}

func (s *MetadataStore) toTypesArtifact(m *model.Artifact) *runtime.Artifact {
	var coords map[string]string
	if m.Coordinates != nil {
		coords = make(map[string]string)
		for k, v := range m.Coordinates {
			if str, ok := v.(string); ok {
				coords[k] = str
			}
		}
	}

	var props map[string]string
	if m.Metadata != nil {
		props = make(map[string]string)
		for k, v := range m.Metadata {
			if str, ok := v.(string); ok {
				props[k] = str
			}
		}
	}

	return &runtime.Artifact{
		ID:           fmt.Sprintf("%d", m.ID),
		RepositoryID: fmt.Sprintf("%d", m.RepositoryID),
		Format:       m.Format,
		Kind:         m.Kind,
		Coordinates:  coords,
		Properties:   props,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// toModelArtifact 将 runtime.Artifact 转为 model.Artifact。
// 注意：此函数与 ArtifactService.toModelArtifact 逻辑相同（回退路径专用），
// 如需修改转换逻辑请同步更新两者。
func (s *MetadataStore) toModelArtifact(t *runtime.Artifact) *model.Artifact {
	coords := make(model.JSONB)
	for k, v := range t.Coordinates {
		coords[k] = v
	}

	metadata := make(model.JSONB)
	for k, v := range t.Properties {
		metadata[k] = v
	}

	// 自动从 Coordinates 计算 download_path，供前端下载链接使用
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
	fmt.Sscanf(t.RepositoryID, "%d", &repoID)

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

func (s *MetadataStore) fillBlobRefs(ctx context.Context, artifacts []*runtime.Artifact) {
	for _, a := range artifacts {
		if a == nil || a.ID == "" {
			continue
		}
		artifactID, err := strconv.ParseUint(a.ID, 10, 64)
		if err != nil {
			continue
		}

		type row struct {
			BlobID    uint
			Algorithm string
			Digest    string
			Size      int64
			Position  int
		}
		var rows []row
		err = s.db.WithContext(ctx).
			Table("artifact_blobs AS ab").
			Select("ab.blob_id, b.algorithm, b.digest, b.size, ab.position").
			Joins("JOIN blobs b ON b.id = ab.blob_id").
			Where("ab.artifact_id = ?", artifactID).
			Order("ab.position ASC").
			Scan(&rows).Error
		if err != nil {
			continue
		}

		a.BlobRefs = make([]runtime.BlobRef, 0, len(rows))
		for _, r := range rows {
			a.BlobRefs = append(a.BlobRefs, runtime.BlobRef{
				BlobID:    r.BlobID,
				Algorithm: r.Algorithm,
				Digest:    r.Digest,
				Size:      r.Size,
			})
		}
	}
}
