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
	artifact      *Artifact
	blobRefs      []BlobRef
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
	s.blobRefs = append(s.blobRefs, ref)
	return ref, nil
}

func (s *HostedUploadSession) PutArtifact(ctx context.Context, artifact *Artifact) error {
	if s.aborted {
		return ErrReadOnly
	}
	artifact.BlobRefs = s.blobRefs
	s.artifact = artifact
	return nil
}

func (s *HostedUploadSession) Commit(ctx context.Context) error {
	if s.aborted {
		return ErrReadOnly
	}
	if s.artifact == nil {
		return ErrInvalidUpload
	}
	return s.metadataStore.Put(ctx, s.artifact)
}

func (s *HostedUploadSession) Abort(ctx context.Context) error {
	s.aborted = true
	// 清理已上传的 blob
	for _, ref := range s.blobRefs {
		s.blobStore.Delete(ref)
	}
	s.blobRefs = nil
	s.artifact = nil
	return nil
}
