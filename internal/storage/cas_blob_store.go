package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
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
	findErr := s.db.WithContext(ctx).Where("algorithm = ? AND digest = ?", "sha256", digest).First(&existingBlob).Error
	// 注册表命中且文件确实存在才允许复用。磁盘清理/存储迁移可能造成 blobs 表有记录
	// 而文件已丢失：此时必须补写文件（复用原记录），否则新下载的内容被丢弃、
	// Open 永远失败，且每次重新下载都会撞上同一悬空记录，无法自愈。
	if findErr == nil {
		if exists, existsErr := s.backend.Exists(ctx, existingBlob.StoragePath); existsErr == nil && exists {
			_ = s.backend.Delete(ctx, tempPath)
			return runtime.BlobRef{
				BlobID:    existingBlob.ID,
				Algorithm: "sha256",
				Digest:    digest,
				Size:      existingBlob.Size,
			}, nil
		}
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
	if findErr == nil {
		// 悬空记录自愈：文件补写完成后复用原 blob ID，避免重复行
		blob = &existingBlob
		blob.StoragePath = casPath
		blob.Size = size
	}
	if err := s.db.WithContext(ctx).Save(blob).Error; err != nil {
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
	rc, err := s.backend.Get(ctx, blob.StoragePath)
	if err != nil {
		// 文件丢失映射为 runtime.ErrNotFound：上层据此清理引用/重新回源，
		// 而不是把原始错误当硬错误返回 500。覆盖 Local（ErrPackageNotFound）、
		// os.ErrNotExist 及 S3 SDK 的 404（匹配方式与 S3Storage.Exists 一致）。
		if isBlobMissingErr(err) {
			return nil, runtime.ErrNotFound
		}
		return nil, err
	}
	return rc, nil
}

func isBlobMissingErr(err error) bool {
	if errors.Is(err, util.ErrPackageNotFound) || errors.Is(err, os.ErrNotExist) {
		return true
	}
	msg := err.Error()
	// 只匹配 S3 SDK 的规范形态（"StatusCode: 404" / 错误码 "NotFound"）。
	// 裸 "404" 子串会误命中含十六进制 digest 的本地 CAS 路径（digest 可含 "404"），
	// 把真实 I/O 错误误判成"文件丢失"。
	return strings.Contains(msg, "NotFound") || strings.Contains(msg, "StatusCode: 404")
}

// RefsExist 校验一组 BlobRef 指向的文件是否真实存在（backend.Exists：本地为
// os.Stat，S3 为 HEAD 请求）。供 runtime 在跳过下载前探测悬空引用。
// 返回约定：(false, nil) = 确定悬空（行缺失/文件确认不存在）；(false, err) = 探测
// 本身失败（网络抖动等），调用方应视为"不确定"而非"悬空"。
func (s *CASBlobStore) RefsExist(ctx context.Context, refs []runtime.BlobRef) (bool, error) {
	for _, ref := range refs {
		var blob model.Blob
		if err := s.db.WithContext(ctx).First(&blob, ref.BlobID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 引用指向的 blob 行不存在：确定悬空
				return false, nil
			}
			return false, err
		}
		exists, err := s.backend.Exists(ctx, blob.StoragePath)
		if err != nil || !exists {
			return false, err
		}
	}
	return true, nil
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
