package runtime

import (
	"context"
	"io"
)

// HostedUploadSession 实现 UploadSession 接口
// 提供事务性上传：先暂存 blob 和 artifact，Commit 时一次性写入
type HostedUploadSession struct {
	metadataStore MetadataStore
	blobStore     BlobStore
	artifacts     []*Artifact // 支持多个 artifact（如 npm 的 tarball + metadata）
	createdBlobs  []BlobRef
	aborted       bool
}

func NewHostedUploadSession(metadataStore MetadataStore, blobStore BlobStore) *HostedUploadSession {
	return &HostedUploadSession{
		metadataStore: metadataStore,
		blobStore:     blobStore,
	}
}

func (s *HostedUploadSession) PutBlob(ctx context.Context, blob io.Reader) (BlobRef, error) {
	if s.aborted {
		return BlobRef{}, ErrReadOnly
	}
	ref, err := s.blobStore.Put(blob)
	if err != nil {
		return BlobRef{}, err
	}
	s.createdBlobs = append(s.createdBlobs, ref)
	return ref, nil
}

func (s *HostedUploadSession) PutArtifact(ctx context.Context, artifact *Artifact) error {
	if s.aborted {
		return ErrReadOnly
	}
	s.artifacts = append(s.artifacts, artifact)
	return nil
}

func (s *HostedUploadSession) Commit(ctx context.Context) error {
	if s.aborted {
		return ErrReadOnly
	}
	if len(s.artifacts) == 0 {
		return ErrInvalidUpload
	}
	return s.metadataStore.BatchPut(ctx, s.artifacts)
}

func (s *HostedUploadSession) Abort(ctx context.Context) error {
	s.aborted = true
	// 清理本次 session 创建过但尚未提交的 blob。
	for _, ref := range s.createdBlobs {
		s.blobStore.Delete(ref)
	}
	s.createdBlobs = nil
	s.artifacts = nil
	return nil
}
