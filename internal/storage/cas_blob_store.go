package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CASBlobStore struct {
	backend Backend
	db      *gorm.DB
}

type movableBackend interface {
	Move(ctx context.Context, oldKey, newKey string) error
}

type countingReader struct {
	reader io.Reader
	n      int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.n += int64(n)
	return n, err
}

func NewCASBlobStore(backend Backend, db *gorm.DB) *CASBlobStore {
	return &CASBlobStore{
		backend: backend,
		db:      db,
	}
}

func (s *CASBlobStore) Put(reader io.Reader) (runtime.BlobRef, error) {
	return s.PutContext(context.Background(), reader)
}

func (s *CASBlobStore) PutContext(ctx context.Context, reader io.Reader) (runtime.BlobRef, error) {
	hasher := sha256.New()
	counter := &countingReader{reader: reader}
	teeReader := io.TeeReader(counter, hasher)

	tempPath := fmt.Sprintf("temp/%s", uuid.New().String())
	if err := s.backend.Put(ctx, tempPath, teeReader, 0); err != nil {
		return runtime.BlobRef{}, err
	}

	digest := hex.EncodeToString(hasher.Sum(nil))
	size := counter.n

	var existingBlob model.Blob
	err := s.db.WithContext(ctx).Where("algorithm = ? AND digest = ?", "sha256", digest).First(&existingBlob).Error
	if err == nil {
		_ = s.backend.Delete(ctx, tempPath)
		return runtime.BlobRef{
			BlobID:    existingBlob.ID,
			Algorithm: "sha256",
			Digest:    digest,
			Size:      existingBlob.Size,
		}, nil
	}

	casPath := s.buildCASPath("sha256", digest)

	if mover, ok := s.backend.(movableBackend); ok {
		if err := mover.Move(ctx, tempPath, casPath); err != nil {
			return runtime.BlobRef{}, err
		}
	} else {
		rc, err := s.backend.Get(ctx, tempPath)
		if err != nil {
			return runtime.BlobRef{}, err
		}
		defer rc.Close()

		if err := s.backend.Put(ctx, casPath, rc, size); err != nil {
			return runtime.BlobRef{}, err
		}
		_ = s.backend.Delete(ctx, tempPath)
	}

	blob := &model.Blob{
		Algorithm:   "sha256",
		Digest:      digest,
		Size:        size,
		StoragePath: casPath,
	}
	if err := s.db.WithContext(ctx).Create(blob).Error; err != nil {
		return runtime.BlobRef{}, err
	}

	return runtime.BlobRef{
		BlobID:    blob.ID,
		Algorithm: "sha256",
		Digest:    digest,
		Size:      size,
	}, nil
}

func (s *CASBlobStore) Open(ref runtime.BlobRef) (io.ReadCloser, error) {
	return s.OpenContext(context.Background(), ref)
}

func (s *CASBlobStore) OpenContext(ctx context.Context, ref runtime.BlobRef) (io.ReadCloser, error) {
	var blob model.Blob
	if err := s.db.WithContext(ctx).First(&blob, ref.BlobID).Error; err != nil {
		return nil, err
	}
	return s.backend.Get(ctx, blob.StoragePath)
}

func (s *CASBlobStore) Stat(ref runtime.BlobRef) (*runtime.BlobMetadata, error) {
	var blob model.Blob
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
	return s.DeleteContext(context.Background(), ref)
}

func (s *CASBlobStore) DeleteContext(ctx context.Context, ref runtime.BlobRef) error {
	var blob model.Blob
	if err := s.db.WithContext(ctx).First(&blob, ref.BlobID).Error; err != nil {
		return err
	}

	var refCount int64
	if err := s.db.WithContext(ctx).Table("artifact_blobs").Where("blob_id = ?", blob.ID).Count(&refCount).Error; err != nil {
		return err
	}
	if refCount > 0 {
		return nil
	}

	if err := s.backend.Delete(ctx, blob.StoragePath); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&blob).Error
}

func (s *CASBlobStore) buildCASPath(algorithm, digest string) string {
	if len(digest) < 4 {
		return fmt.Sprintf("blobs/%s/%s", algorithm, digest)
	}
	return fmt.Sprintf("blobs/%s/%s/%s/%s", algorithm, digest[:2], digest[2:4], digest)
}

func (s *CASBlobStore) Exists(algorithm, digest string) (bool, error) {
	var count int64
	err := s.db.Model(&model.Blob{}).
		Where("algorithm = ? AND digest = ?", algorithm, digest).
		Count(&count).Error
	return count > 0, err
}
