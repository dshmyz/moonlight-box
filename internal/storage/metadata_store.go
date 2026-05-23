package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/types"
	"gorm.io/gorm"
)

type MetadataStore struct {
	db *gorm.DB
}

func NewMetadataStore(db *gorm.DB) *MetadataStore {
	return &MetadataStore{db: db}
}

func (s *MetadataStore) Get(ctx context.Context, key types.ArtifactKey) (*types.Artifact, error) {
	var artifact model.Artifact

	coordsJSON, _ := json.Marshal(key.Coordinates)

	err := s.db.WithContext(ctx).
		Where("repository_id = ?", key.RepositoryID).
		Where("format = ?", key.Format).
		Where("coordinates = ?", coordsJSON).
		First(&artifact).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, types.ErrNotFound
		}
		return nil, err
	}

	return s.toTypesArtifact(&artifact), nil
}

func (s *MetadataStore) Put(ctx context.Context, artifact *types.Artifact) error {
	modelArtifact := s.toModelArtifact(artifact)

	var existing model.Artifact
	coordsJSON, _ := json.Marshal(artifact.Coordinates)

	err := s.db.WithContext(ctx).
		Where("repository_id = ?", artifact.RepositoryID).
		Where("format = ?", artifact.Format).
		Where("coordinates = ?", coordsJSON).
		First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		return s.db.WithContext(ctx).Create(modelArtifact).Error
	} else if err != nil {
		return err
	}

	modelArtifact.ID = existing.ID
	return s.db.WithContext(ctx).Save(modelArtifact).Error
}

func (s *MetadataStore) Delete(ctx context.Context, key types.ArtifactKey) error {
	coordsJSON, _ := json.Marshal(key.Coordinates)

	result := s.db.WithContext(ctx).
		Where("repository_id = ?", key.RepositoryID).
		Where("format = ?", key.Format).
		Where("coordinates = ?", coordsJSON).
		Delete(&model.Artifact{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return types.ErrNotFound
	}
	return nil
}

func (s *MetadataStore) List(ctx context.Context, repoID string) ([]*types.Artifact, error) {
	var artifacts []model.Artifact

	err := s.db.WithContext(ctx).
		Where("repository_id = ?", repoID).
		Find(&artifacts).Error

	if err != nil {
		return nil, err
	}

	result := make([]*types.Artifact, len(artifacts))
	for i, a := range artifacts {
		result[i] = s.toTypesArtifact(&a)
	}
	return result, nil
}

func (s *MetadataStore) toTypesArtifact(m *model.Artifact) *types.Artifact {
	var coords map[string]string
	if m.Coordinates != nil {
		coords = make(map[string]string)
		for k, v := range m.Coordinates {
			if str, ok := v.(string); ok {
				coords[k] = str
			}
		}
	}

	return &types.Artifact{
		ID:           fmt.Sprintf("%d", m.ID),
		RepositoryID: fmt.Sprintf("%d", m.RepositoryID),
		Format:       m.Format,
		Coordinates:  coords,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func (s *MetadataStore) toModelArtifact(t *types.Artifact) *model.Artifact {
	coords := make(model.JSONB)
	for k, v := range t.Coordinates {
		coords[k] = v
	}

	var repoID uint
	fmt.Sscanf(t.RepositoryID, "%d", &repoID)

	return &model.Artifact{
		RepositoryID: repoID,
		Format:       t.Format,
		Coordinates:  coords,
	}
}
