package service

import (
	"context"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

// ArtifactQueryService 基于新 artifacts 表的查询服务
type ArtifactQueryService struct {
	db *gorm.DB
}

// NewArtifactQueryService 创建制品查询服务实例
func NewArtifactQueryService(db *gorm.DB) *ArtifactQueryService {
	return &ArtifactQueryService{db: db}
}

// ArtifactCatalogEntry 制品目录条目
type ArtifactCatalogEntry struct {
	ID             uint      `json:"id"`
	RepositoryID   uint      `json:"repository_id"`
	Format         string    `json:"format"`
	Name           string    `json:"name"`
	VersionCount   int       `json:"version_count"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
	RepositoryName string    `json:"repository_name,omitempty"`
}

// ArtifactVersion 制品版本信息
type ArtifactVersion struct {
	ID        uint               `json:"id"`
	Version   string             `json:"version"`
	Format    string             `json:"format"`
	Kind      string             `json:"kind,omitempty"`
	SizeBytes int64              `json:"size_bytes"`
	CreatedAt time.Time          `json:"created_at"`
	BlobRefs  []ArtifactBlobInfo `json:"blob_refs,omitempty"`
}

// ArtifactBlobInfo blob 摘要信息
type ArtifactBlobInfo struct {
	BlobID    uint   `json:"blob_id"`
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// GetVersions 获取制品的版本列表
func (s *ArtifactQueryService) GetVersions(ctx context.Context, repoID uint, format, name string) ([]ArtifactVersion, error) {
	var artifacts []model.Artifact

	db := s.db.WithContext(ctx).
		Where("repository_id = ?", repoID)
	if format != "" {
		db = db.Where("format = ?", format)
	}
	if name != "" {
		db = db.Where("name = ?", name)
	}

	if err := db.Order("created_at DESC").Find(&artifacts).Error; err != nil {
		return nil, err
	}

	versions := make([]ArtifactVersion, 0, len(artifacts))
	artifactIDs := make([]uint, 0, len(artifacts))
	for _, a := range artifacts {
		artifactIDs = append(artifactIDs, a.ID)
	}

	// 批量查 blob refs
	blobRefMap := make(map[uint][]ArtifactBlobInfo)
	if len(artifactIDs) > 0 {
		type blobRow struct {
			ArtifactID uint
			BlobID     uint
			Algorithm  string
			Digest     string
			Size       int64
		}
		var blobRows []blobRow
		s.db.Table("artifact_blobs AS ab").
			Select("ab.artifact_id, ab.blob_id, b.algorithm, b.digest, b.size").
			Joins("JOIN blobs b ON b.id = ab.blob_id").
			Where("ab.artifact_id IN ?", artifactIDs).
			Order("ab.position ASC").
			Scan(&blobRows)
		for _, br := range blobRows {
			blobRefMap[br.ArtifactID] = append(blobRefMap[br.ArtifactID], ArtifactBlobInfo{
				BlobID:    br.BlobID,
				Algorithm: br.Algorithm,
				Digest:    br.Digest,
				Size:      br.Size,
			})
		}
	}

	for _, a := range artifacts {
		version := a.Version

		var totalSize int64
		for _, ref := range blobRefMap[a.ID] {
			totalSize += ref.Size
		}

		versions = append(versions, ArtifactVersion{
			ID:        a.ID,
			Version:   version,
			Format:    a.Format,
			Kind:      a.Kind,
			SizeBytes: totalSize,
			CreatedAt: a.CreatedAt,
			BlobRefs:  blobRefMap[a.ID],
		})
	}

	return versions, nil
}

// CountByRepository 按仓库统计制品数量
func (s *ArtifactQueryService) CountByRepository(ctx context.Context, repoIDs []uint) (map[uint]int64, error) {
	if len(repoIDs) == 0 {
		return nil, nil
	}

	var rows []struct {
		RepositoryID uint
		Count        int64
	}
	if err := s.db.WithContext(ctx).Model(&model.Artifact{}).
		Select("repository_id, COUNT(*) as count").
		Where("repository_id IN ?", repoIDs).
		Group("repository_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uint]int64)
	for _, r := range rows {
		result[r.RepositoryID] = r.Count
	}
	return result, nil
}

// CountByFormat 按格式统计制品数量
func (s *ArtifactQueryService) CountByFormat(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Format string
		Count  int64
	}
	if err := s.db.WithContext(ctx).Model(&model.Artifact{}).
		Select("format, COUNT(*) as count").
		Group("format").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]int64)
	for _, r := range rows {
		result[r.Format] = r.Count
	}
	return result, nil
}

// GetTopPackages 获取最近创建的制品
func (s *ArtifactQueryService) GetTopPackages(ctx context.Context, limit int) ([]PackageTop, error) {
	var artifacts []model.Artifact
	if err := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&artifacts).Error; err != nil {
		return nil, err
	}

	topPackages := make([]PackageTop, 0, len(artifacts))
	seen := make(map[string]bool)
	for _, a := range artifacts {
		name := a.Name
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		topPackages = append(topPackages, PackageTop{
			Name: name,
			Type: a.Format,
		})
	}
	return topPackages, nil
}

// DeleteArtifact 删除制品
func (s *ArtifactQueryService) DeleteArtifact(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("artifact_id = ?", id).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Artifact{}, id).Error
	})
}
