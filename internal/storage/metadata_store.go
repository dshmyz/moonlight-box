package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/database/dialect"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type MetadataStore struct {
	db          *gorm.DB
	artifactSvc ArtifactServiceAdapter
}

const maxMetadataStoreListItems = 1000

// existingQueryChunkSize 限制单条 SELECT 语句中 IN 子句的参数数量，
// 避免 SQLite "too many SQL variables" 错误（SQLite 默认上限 999/32766）。
// 500 留出足够余量给 repository_id 等其它绑定参数。
const existingQueryChunkSize = 500

// ArtifactServiceAdapter 适配 service.ArtifactService，使 MetadataStore 能调用它。
// 使用接口避免循环依赖。
type ArtifactServiceAdapter interface {
	Save(ctx context.Context, artifact *runtime.Artifact) error
	SaveBatch(ctx context.Context, artifacts []*runtime.Artifact, rejectOverwrite bool) error
	AttachBlob(ctx context.Context, artifact *runtime.Artifact) error
	Delete(ctx context.Context, key runtime.ArtifactKey) error
	BatchDelete(ctx context.Context, repoID uint, artifactIDs []uint) error
}

func NewMetadataStore(db *gorm.DB) *MetadataStore {
	return &MetadataStore{db: db}
}

// NewMetadataStoreWithArtifactService 创建带 ArtifactService 的 MetadataStore
func NewMetadataStoreWithArtifactService(db *gorm.DB, svc ArtifactServiceAdapter) *MetadataStore {
	return &MetadataStore{db: db, artifactSvc: svc}
}

func (s *MetadataStore) Get(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	var repoID uint
	fmt.Sscanf(key.RepositoryID, "%d", &repoID)

	db := s.db.WithContext(ctx).Model(&model.Artifact{}).
		Where("repository_id = ?", repoID).
		Where("format = ?", key.Format)
	if key.IdentityKey != "" {
		db = db.Where("identity_key = ?", key.IdentityKey)
	} else if key.RemotePath != "" {
		db = db.Where("remote_path = ?", key.RemotePath)
	} else if key.Name != "" || key.Version != "" || key.Path != "" || key.Filename != "" {
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
	} else {
		return nil, runtime.ErrNotFound
	}

	// 单条 JOIN 查询：artifact 行与 blob 引用一次取出（原为 2 条查询，读路径 2→1）。
	// COALESCE 兜住 LEFT JOIN 无 blob 时的 NULL（scan 进非指针类型会报错）。
	type artifactBlobRow struct {
		model.Artifact `gorm:"embedded"`
		RefBlobID      uint   `gorm:"column:ref_blob_id"`
		RefAlgorithm   string `gorm:"column:ref_algorithm"`
		RefDigest      string `gorm:"column:ref_digest"`
		RefSize        int64  `gorm:"column:ref_size"`
		RefPosition    int    `gorm:"column:ref_position"`
		RefHasBlob     bool   `gorm:"column:ref_has_blob"`
	}
	var rows []artifactBlobRow
	err := db.
		Select("artifacts.*, COALESCE(ab.blob_id, 0) AS ref_blob_id, COALESCE(b.algorithm, '') AS ref_algorithm, " +
			"COALESCE(b.digest, '') AS ref_digest, COALESCE(b.size, 0) AS ref_size, COALESCE(ab.position, 0) AS ref_position, " +
			"(ab.artifact_id IS NOT NULL) AS ref_has_blob").
		Joins("LEFT JOIN artifact_blobs ab ON ab.artifact_id = artifacts.id").
		Joins("LEFT JOIN blobs b ON b.id = ab.blob_id").
		Order("artifacts.id ASC, ab.position ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, runtime.ErrNotFound
	}

	// 只取第一个 artifact 的全部行（与原 First 按 id 取第一条语义一致），聚合其 blob 引用
	first := rows[0]
	refs := make([]runtime.BlobRef, 0, len(rows))
	for _, r := range rows {
		if r.Artifact.ID != first.Artifact.ID {
			break
		}
		if r.RefHasBlob {
			refs = append(refs, runtime.BlobRef{
				BlobID:    r.RefBlobID,
				Algorithm: r.RefAlgorithm,
				Digest:    r.RefDigest,
				Size:      r.RefSize,
			})
		}
	}
	result := s.toTypesArtifact(&first.Artifact)
	result.BlobRefs = refs
	return result, nil
}

// AttachBlob 回源下载完内容后只补写 blob 关联与内容列（不走整条 Put 的重复查询）。
// 无 ArtifactService 的回退路径直接整条 Put（该路径本就不维护聚合表）。
func (s *MetadataStore) AttachBlob(ctx context.Context, artifact *runtime.Artifact) error {
	if s.artifactSvc != nil {
		return s.artifactSvc.AttachBlob(ctx, artifact)
	}
	return s.Put(ctx, artifact)
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

		err := tx.Where("repository_id = ?", artifact.RepositoryID).
			Where("format = ?", artifact.Format).
			Where("identity_key = ?", modelArtifact.IdentityKey).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			if err := tx.Create(modelArtifact).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			modelArtifact.ID = existing.ID
			if runtime.IsSyncSource(ctx) {
				preserveSyncPublishedAt(existing, modelArtifact)
			}
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

func (s *MetadataStore) BatchPut(ctx context.Context, artifacts []*runtime.Artifact, rejectOverwrite bool) error {
	if s.artifactSvc != nil {
		return s.artifactSvc.SaveBatch(ctx, artifacts, rejectOverwrite)
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
		// 批量查询已有记录，避免逐条 SELECT
		modelArtifacts := make([]*model.Artifact, len(artifacts))
		for i, artifact := range artifacts {
			modelArtifacts[i] = s.toModelArtifact(artifact)
		}

		// 收集所有 identity_key 用于批量查询
		identityKeys := make([]string, 0, len(artifacts))
		repoIDSet := make(map[uint]bool)
		for _, ma := range modelArtifacts {
			if ma.IdentityKey != "" {
				identityKeys = append(identityKeys, ma.IdentityKey)
			}
			repoIDSet[ma.RepositoryID] = true
		}

		// 分块查询已有记录，避免 SQLite "too many SQL variables" 错误
		existingMap := make(map[string]model.Artifact) // key: identity_key
		if len(identityKeys) > 0 {
			repoIDs := make([]uint, 0, len(repoIDSet))
			for id := range repoIDSet {
				repoIDs = append(repoIDs, id)
			}
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

		// 分为新增和更新两组，记录每个 artifact 的索引以便后续同步 blob
		type indexedArtifact struct {
			model *model.Artifact
			index int // 在原始 artifacts 切片中的索引
		}
		var toCreate []indexedArtifact
		var toUpdate []indexedArtifact

		for i, ma := range modelArtifacts {
			if existing, ok := existingMap[ma.IdentityKey]; ok {
				if rejectOverwrite && !runtime.IsCatalogExcludedKind(artifacts[i].Kind) {
					return runtime.ErrOverwriteNotAllowed
				}
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
				if rejectOverwrite && errors.Is(err, gorm.ErrDuplicatedKey) {
					return runtime.ErrOverwriteNotAllowed
				}
				return err
			}
			// 为新建的记录同步 blob 关联（CreateInBatches 后 model.ID 已被填充）
			for _, ia := range toCreate {
				if err := s.syncBlobRefs(tx, ia.model.ID, artifacts[ia.index].BlobRefs); err != nil {
					return err
				}
			}
		}

		// 批量 UPDATE + 同步 blob
		for _, ia := range toUpdate {
			if existing, ok := existingMap[ia.model.IdentityKey]; ok {
				preserveSyncPublishedAt(existing, ia.model)
			}
			if err := tx.Save(ia.model).Error; err != nil {
				return err
			}
			if err := s.syncBlobRefs(tx, ia.model.ID, artifacts[ia.index].BlobRefs); err != nil {
				return err
			}
		}

		return nil
	})
}

// preserveSyncPublishedAt 代理同步路径下，已存在制品的 published_at 保持 first-write-wins：
// 上游 metadata 的 lastUpdated（尤其 Maven）随任意重新部署变化，不能反复覆盖。
// 与 ArtifactService.preservePublishedAt 语义一致（此为无 ArtifactService 的回退路径）。
func preserveSyncPublishedAt(existing model.Artifact, target *model.Artifact) {
	old, _ := existing.Attributes["published_at"].(string)
	if old == "" {
		return
	}
	cur, _ := target.Attributes["published_at"].(string)
	if cur != old {
		if target.Attributes == nil {
			target.Attributes = model.JSONB{}
		}
		target.Attributes["published_at"] = old
	}
}

func (s *MetadataStore) syncBlobRefs(tx *gorm.DB, artifactID uint, blobRefs []runtime.BlobRef) error {
	if len(blobRefs) == 0 {
		return nil
	}
	if err := tx.Where("artifact_id = ?", artifactID).Delete(&model.ArtifactBlob{}).Error; err != nil {
		return err
	}
	// 批量写入 blob 关联，取代逐条 CREATE：缩减写事务内语句数、持锁窗口更短。
	refs := make([]*model.ArtifactBlob, 0, len(blobRefs))
	for i, ref := range blobRefs {
		if ref.BlobID == 0 {
			continue
		}
		refs = append(refs, &model.ArtifactBlob{
			ArtifactID: artifactID,
			BlobID:     ref.BlobID,
			Position:   i,
		})
	}
	if len(refs) == 0 {
		return nil
	}
	return tx.CreateInBatches(refs, 500).Error
}

func (s *MetadataStore) Delete(ctx context.Context, key runtime.ArtifactKey) error {
	if s.artifactSvc != nil {
		return s.artifactSvc.Delete(ctx, key)
	}

	var repoID uint
	fmt.Sscanf(key.RepositoryID, "%d", &repoID)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

		if err := tx.Where("artifact_id = ?", artifact.ID).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&artifact).Error; err != nil {
			return err
		}
		return nil
	})
}

// BatchDelete 批量删除指定 artifact，同步更新 packages/package_versions 聚合表。
func (s *MetadataStore) BatchDelete(ctx context.Context, repoID uint, artifactIDs []uint) error {
	if len(artifactIDs) == 0 {
		return nil
	}
	if s.artifactSvc != nil {
		return s.artifactSvc.BatchDelete(ctx, repoID, artifactIDs)
	}
	// 无 adapter 时直接批量删除（不更新聚合表）
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("artifact_id IN ?", artifactIDs).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", artifactIDs).Delete(&model.Artifact{}).Error
	})
}

func (s *MetadataStore) List(ctx context.Context, repoID string) ([]*runtime.Artifact, error) {
	var artifacts []model.Artifact

	err := s.db.WithContext(ctx).
		Where("repository_id = ?", repoID).
		Limit(maxMetadataStoreListItems + 1).
		Find(&artifacts).Error

	if err != nil {
		return nil, err
	}
	if len(artifacts) > maxMetadataStoreListItems {
		return nil, fmt.Errorf("too many artifacts for repository %s: List is capped at %d items; use Query with filters instead", repoID, maxMetadataStoreListItems)
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
	if query.Kind != "" {
		db = db.Where("kind = ?", query.Kind)
	}
	if query.IdentityKey != "" {
		db = db.Where("identity_key = ?", query.IdentityKey)
	}
	if query.Name != "" {
		db = db.Where("name = ?", query.Name)
	}
	if query.Namespace != "" {
		db = db.Where("namespace = ?", query.Namespace)
	}
	if query.Version != "" {
		db = db.Where("version = ?", query.Version)
	}
	if query.Path != "" {
		db = db.Where("path = ?", query.Path)
	}
	if query.Filename != "" {
		db = db.Where("filename = ?", query.Filename)
	}
	if query.RemotePath != "" {
		db = db.Where("remote_path = ?", query.RemotePath)
	} else if query.RemotePathPrefix != "" {
		db = db.Where("remote_path LIKE ?", query.RemotePathPrefix+"%")
	}
	for k, v := range query.Qualifiers {
		db = db.Where(dialect.JSONTextExpr(s.db.Dialector.Name(), "qualifiers", k)+" = ?", v)
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
	var props map[string]string
	if m.Metadata != nil {
		props = make(map[string]string)
		for k, v := range m.Metadata {
			if str, ok := v.(string); ok {
				props[k] = str
			}
		}
	}

	a := &runtime.Artifact{
		ID:           fmt.Sprintf("%d", m.ID),
		RepositoryID: fmt.Sprintf("%d", m.RepositoryID),
		Format:       m.Format,
		Kind:         m.Kind,
		IdentityKey:  m.IdentityKey,
		Name:         m.Name,
		Namespace:    m.Namespace,
		Version:      m.Version,
		Path:         m.Path,
		Filename:     m.Filename,
		RemotePath:   m.RemotePath,
		DownloadURL:  m.DownloadURL,
		Extension:    m.Extension,
		ContentType:  m.ContentType,
		SizeBytes:    m.SizeBytes,
		Checksums:    jsonbToStringMap(m.Checksums),
		Qualifiers:   jsonbToStringMap(m.Qualifiers),
		Attributes:   jsonbToStringMap(m.Attributes),
		Properties:   props,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
	runtime.NormalizeArtifactForStore(a)
	return a
}

// toModelArtifact 将 runtime.Artifact 转为 model.Artifact。
// 前置条件：调用方必须确保 artifact 已通过 ValidateArtifactForStore 归一化。
// 注意：此函数与 ArtifactService.toModelArtifact 逻辑相同（回退路径专用），
// 如需修改转换逻辑请同步更新两者。
func (s *MetadataStore) toModelArtifact(t *runtime.Artifact) *model.Artifact {

	metadata := make(model.JSONB)
	for k, v := range t.Properties {
		metadata[k] = v
	}

	var repoID uint
	fmt.Sscanf(t.RepositoryID, "%d", &repoID)

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

func (s *MetadataStore) fillBlobRefs(ctx context.Context, artifacts []*runtime.Artifact) {
	artifactIDs := make([]uint, 0, len(artifacts))
	artifactByID := make(map[uint]*runtime.Artifact, len(artifacts))
	for _, a := range artifacts {
		if a == nil || a.ID == "" {
			continue
		}
		artifactID, err := strconv.ParseUint(a.ID, 10, 64)
		if err != nil {
			continue
		}
		id := uint(artifactID)
		artifactIDs = append(artifactIDs, id)
		artifactByID[id] = a
	}
	if len(artifactIDs) == 0 {
		return
	}

	type row struct {
		ArtifactID uint
		BlobID     uint
		Algorithm  string
		Digest     string
		Size       int64
		Position   int
	}
	var rows []row
	err := s.db.WithContext(ctx).
		Table("artifact_blobs AS ab").
		Select("ab.artifact_id, ab.blob_id, b.algorithm, b.digest, b.size, ab.position").
		Joins("JOIN blobs b ON b.id = ab.blob_id").
		Where("ab.artifact_id IN ?", artifactIDs).
		Order("ab.artifact_id ASC, ab.position ASC").
		Scan(&rows).Error
	if err != nil {
		return
	}

	for _, r := range rows {
		a := artifactByID[r.ArtifactID]
		if a == nil {
			continue
		}
		a.BlobRefs = append(a.BlobRefs, runtime.BlobRef{
			BlobID:    r.BlobID,
			Algorithm: r.Algorithm,
			Digest:    r.Digest,
			Size:      r.Size,
		})
	}
}

func jsonbToStringMap(src model.JSONB) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		if str, ok := v.(string); ok {
			dst[k] = str
		}
	}
	return dst
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
