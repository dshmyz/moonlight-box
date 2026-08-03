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
	// RepositoryID 为 artifact 落库时强制使用的仓库 ID。
	// 组合仓库发布时 plugin 传入的是组合库 ID，但实际写入到 hosted 成员的存储；
	// 与读路径（HostedRuntime.QueryArtifacts/GetArtifact 强制使用成员 ID）保持一致，
	// 写路径也必须把 artifact 归到成员自身的 ID，否则发布成功却无法按成员 ID 查询到。
	RepositoryID string // 为空表示不覆写
	artifacts   []*Artifact  // 支持多个 artifact（如 npm 的 tarball + metadata）
	createdBlobs []BlobRef
	aborted      bool
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
	var (
		ref BlobRef
		err error
	)
	if store, ok := s.blobStore.(ContextBlobPutter); ok {
		ref, err = store.PutContext(ctx, blob)
	} else {
		ref, err = s.blobStore.Put(blob)
	}
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
	// 任何失败路径都必须清理已写入的 blob，否则会留下 orphan blob 永久占用存储。
	// 调用方只需感知 Commit 返回的错误，无需关心 blob 回收——资源清理是 Session 的职责。
	var commitErr error
	if len(s.artifacts) == 0 {
		commitErr = ErrInvalidUpload
	} else {
		// 覆写 artifact 的 RepositoryID 为成员自身 ID，确保与读路径查询一致
		if s.RepositoryID != "" {
			for _, a := range s.artifacts {
				a.RepositoryID = s.RepositoryID
			}
		}
		commitErr = s.metadataStore.BatchPut(ctx, s.artifacts)
	}
	if commitErr != nil {
		// Abort 会清理 createdBlobs 并标记 aborted，忽略其错误（清理失败无法恢复，只能记录）。
		_ = s.Abort(ctx)
		return commitErr
	}
	// 成功后清空 createdBlobs，防止后续误调用 Abort 删掉已提交的 blob。
	s.createdBlobs = nil
	s.artifacts = nil
	return nil
}

func (s *HostedUploadSession) Abort(ctx context.Context) error {
	s.aborted = true
	// 清理本次 session 创建过但尚未提交的 blob。
	for _, ref := range s.createdBlobs {
		if store, ok := s.blobStore.(ContextBlobDeleter); ok {
			_ = store.DeleteContext(ctx, ref)
		} else {
			_ = s.blobStore.Delete(ref)
		}
	}
	s.createdBlobs = nil
	s.artifacts = nil
	return nil
}
