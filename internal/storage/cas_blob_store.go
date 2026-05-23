package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/core/runtime"
	"gorm.io/gorm"
)

type CASBlobStore struct {
	backend Backend
	db      *gorm.DB
}

func NewCASBlobStore(backend Backend, db *gorm.DB) *CASBlobStore {
	return &CASBlobStore{
		backend: backend,
		db:      db,
	}
}

func (s *CASBlobStore) Put(reader io.Reader) (runtime.BlobRef, error) {
	hasher := sha256.New()
	teeReader := io.TeeReader(reader, hasher)

	tempPath := fmt.Sprintf("temp/%s", uuid.New().String())
	if err := s.backend.Put(context.Background(), tempPath, teeReader, -1); err != nil {
		return runtime.BlobRef{}, err
	}

	digest := hex.EncodeToString(hasher.Sum(nil))

	var existingBlob model.BlobV2
	err := s.db.Where("algorithm = ? AND digest = ?", "sha256", digest).First(&existingBlob).Error
	if err == nil {
		s.backend.Delete(context.Background(), tempPath)
		s.db.Model(&existingBlob).UpdateColumn("ref_count", gorm.Expr("ref_count + 1"))
		return runtime.BlobRef{
			BlobID:    existingBlob.ID,
			Algorithm: "sha256",
			Digest:    digest,
			Size:      existingBlob.Size,
		}, nil
	}

	casPath := s.buildCASPath("sha256", digest)
	
	rc, err := s.backend.Get(context.Background(), tempPath)
	if err != nil {
		return runtime.BlobRef{}, err
	}
	defer rc.Close()
	
	if err := s.backend.Put(context.Background(), casPath, rc, -1); err != nil {
		return runtime.BlobRef{}, err
	}
	s.backend.Delete(context.Background(), tempPath)

	blob := &model.BlobV2{
		Algorithm:   "sha256",
		Digest:      digest,
		Size:        0,
		StoragePath: casPath,
	}
	if err := s.db.Create(blob).Error; err != nil {
		return runtime.BlobRef{}, err
	}

	return runtime.BlobRef{
		BlobID:    blob.ID,
		Algorithm: "sha256",
		Digest:    digest,
		Size:      blob.Size,
	}, nil
}

func (s *CASBlobStore) Open(ref runtime.BlobRef) (io.ReadCloser, error) {
	var blob model.BlobV2
	if err := s.db.First(&blob, ref.BlobID).Error; err != nil {
		return nil, err
	}
	return s.backend.Get(context.Background(), blob.StoragePath)
}

func (s *CASBlobStore) Stat(ref runtime.BlobRef) (*runtime.BlobMetadata, error) {
	var blob model.BlobV2
	if err := s.db.First(&blob, ref.BlobID).Error; err != nil {
		return nil, err
	}
	return &runtime.BlobMetadata{
		Algorithm:   blob.Algorithm,
		Digest:      blob.Digest,
		Size:        blob.Size,
		StoragePath: blob.StoragePath,
		CreatedAt:   blob.CreatedAt,
	}, nil
}

func (s *CASBlobStore) Delete(ref runtime.BlobRef) error {
	var blob model.BlobV2
	if err := s.db.First(&blob, ref.BlobID).Error; err != nil {
		return err
	}

	if err := s.backend.Delete(context.Background(), blob.StoragePath); err != nil {
		return err
	}

	return s.db.Delete(&blob).Error
}

func (s *CASBlobStore) buildCASPath(algorithm, digest string) string {
	if len(digest) < 4 {
		return fmt.Sprintf("blobs/%s/%s", algorithm, digest)
	}
	return fmt.Sprintf("blobs/%s/%s/%s/%s", algorithm, digest[:2], digest[2:4], digest)
}

func (s *CASBlobStore) Exists(algorithm, digest string) (bool, error) {
	var count int64
	err := s.db.Model(&model.BlobV2{}).
		Where("algorithm = ? AND digest = ?", algorithm, digest).
		Count(&count).Error
	return count > 0, err
}
