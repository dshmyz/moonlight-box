package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type contextKey string

func TestHostedUploadSessionAbortDeletesAllCreatedBlobs(t *testing.T) {
	blobStore := &uploadSessionBlobStore{}
	session := NewHostedUploadSession(&uploadSessionMetadataStore{}, blobStore)

	first, err := session.PutBlob(context.Background(), strings.NewReader("first"))
	if err != nil {
		t.Fatalf("put first blob: %v", err)
	}
	if err := session.PutArtifact(context.Background(), NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "first.txt",
		Filename: "first.txt",
		BlobRefs: []BlobRef{first},
	})); err != nil {
		t.Fatalf("put first artifact: %v", err)
	}

	second, err := session.PutBlob(context.Background(), strings.NewReader("second"))
	if err != nil {
		t.Fatalf("put second blob: %v", err)
	}
	if err := session.PutArtifact(context.Background(), NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "second.txt",
		Filename: "second.txt",
		BlobRefs: []BlobRef{second},
	})); err != nil {
		t.Fatalf("put second artifact: %v", err)
	}

	if err := session.Abort(context.Background()); err != nil {
		t.Fatalf("abort: %v", err)
	}

	if len(blobStore.deleted) != 2 {
		t.Fatalf("expected 2 deleted blobs, got %d", len(blobStore.deleted))
	}
	if blobStore.deleted[0].Digest != first.Digest || blobStore.deleted[1].Digest != second.Digest {
		t.Fatalf("deleted refs = %+v, want [%+v %+v]", blobStore.deleted, first, second)
	}
}

func TestHostedUploadSessionCommitUsesBatchPut(t *testing.T) {
	storeErr := errors.New("batch failed")
	metadataStore := &uploadSessionMetadataStore{batchErr: storeErr}
	session := NewHostedUploadSession(metadataStore, &uploadSessionBlobStore{})

	ref, err := session.PutBlob(context.Background(), strings.NewReader("first"))
	if err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if err := session.PutArtifact(context.Background(), NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "first.txt",
		Filename: "first.txt",
		BlobRefs: []BlobRef{ref},
	})); err != nil {
		t.Fatalf("put artifact: %v", err)
	}

	err = session.Commit(context.Background())
	if !errors.Is(err, storeErr) {
		t.Fatalf("commit error = %v, want %v", err, storeErr)
	}
	if metadataStore.putCalls != 0 {
		t.Fatalf("commit should not call Put one by one, got %d Put calls", metadataStore.putCalls)
	}
	if metadataStore.batchCalls != 1 {
		t.Fatalf("commit should call BatchPut once, got %d calls", metadataStore.batchCalls)
	}
}

// TestHostedUploadSessionCommitCleansUpBlobsOnBatchPutFailure 验证 P0-A 修复：
// BatchPut 失败时 Commit 必须自动清理已写入的 blob，避免 orphan blob 永久残留。
// 此前实现仅返回错误不调 Abort，导致磁盘泄漏。
func TestHostedUploadSessionCommitCleansUpBlobsOnBatchPutFailure(t *testing.T) {
	storeErr := errors.New("batch failed")
	metadataStore := &uploadSessionMetadataStore{batchErr: storeErr}
	blobStore := &uploadSessionBlobStore{}
	session := NewHostedUploadSession(metadataStore, blobStore)

	// 写入两个 blob（模拟 npm 发布时 tarball + metadata 的多 blob 场景）
	firstRef, err := session.PutBlob(context.Background(), strings.NewReader("first"))
	if err != nil {
		t.Fatalf("put first blob: %v", err)
	}
	secondRef, err := session.PutBlob(context.Background(), strings.NewReader("second"))
	if err != nil {
		t.Fatalf("put second blob: %v", err)
	}
	if err := session.PutArtifact(context.Background(), NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "pkg",
		Filename: "pkg.txt",
		BlobRefs: []BlobRef{firstRef},
	})); err != nil {
		t.Fatalf("put artifact: %v", err)
	}

	err = session.Commit(context.Background())
	if !errors.Is(err, storeErr) {
		t.Fatalf("commit error = %v, want %v", err, storeErr)
	}
	if len(blobStore.deleted) != 2 {
		t.Fatalf("expected 2 blobs deleted after Commit failure, got %d (orphan blob leak)", len(blobStore.deleted))
	}
	deletedDigests := map[string]bool{
		blobStore.deleted[0].Digest: true,
		blobStore.deleted[1].Digest: true,
	}
	if !deletedDigests[firstRef.Digest] || !deletedDigests[secondRef.Digest] {
		t.Fatalf("deleted refs = %+v, want both %q and %q", blobStore.deleted, firstRef.Digest, secondRef.Digest)
	}
}

// TestHostedUploadSessionCommitCleansUpBlobsWhenNoArtifacts 验证 P0-A 修复：
// 当 session 有 blob 但未 PutArtifact 时 Commit 返回 ErrInvalidUpload，
// 此时也应自动清理已写入的 blob（否则孤立的 blob 同样会泄漏）。
func TestHostedUploadSessionCommitCleansUpBlobsWhenNoArtifacts(t *testing.T) {
	blobStore := &uploadSessionBlobStore{}
	session := NewHostedUploadSession(&uploadSessionMetadataStore{}, blobStore)

	ref, err := session.PutBlob(context.Background(), strings.NewReader("orphan"))
	if err != nil {
		t.Fatalf("put blob: %v", err)
	}

	err = session.Commit(context.Background())
	if !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("commit error = %v, want ErrInvalidUpload", err)
	}
	if len(blobStore.deleted) != 1 {
		t.Fatalf("expected 1 blob deleted after Commit with no artifacts, got %d (orphan blob leak)", len(blobStore.deleted))
	}
	if blobStore.deleted[0].Digest != ref.Digest {
		t.Fatalf("deleted ref = %+v, want %q", blobStore.deleted[0], ref.Digest)
	}
}

// TestHostedUploadSessionCommitSuccessDoesNotDeleteBlobs 验证 Commit 成功时不应误删 blob。
// 防止 P0-A 修复引入回归：成功路径必须保持 blob 不变。
func TestHostedUploadSessionCommitSuccessDoesNotDeleteBlobs(t *testing.T) {
	blobStore := &uploadSessionBlobStore{}
	session := NewHostedUploadSession(&uploadSessionMetadataStore{}, blobStore)

	ref, err := session.PutBlob(context.Background(), strings.NewReader("content"))
	if err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if err := session.PutArtifact(context.Background(), NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "pkg",
		Filename: "pkg.txt",
		BlobRefs: []BlobRef{ref},
	})); err != nil {
		t.Fatalf("put artifact: %v", err)
	}

	if err := session.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(blobStore.deleted) != 0 {
		t.Fatalf("expected 0 deletions on successful Commit, got %d", len(blobStore.deleted))
	}
}

func TestHostedUploadSessionPutBlobUsesContextAwareBlobStore(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("request"), "upload-ctx")
	blobStore := &contextUploadSessionBlobStore{}
	session := NewHostedUploadSession(&uploadSessionMetadataStore{}, blobStore)

	if _, err := session.PutBlob(ctx, strings.NewReader("first")); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if got := blobStore.contextValue; got != "upload-ctx" {
		t.Fatalf("context value = %v, want upload-ctx", got)
	}
	if blobStore.putCalls != 0 {
		t.Fatalf("fallback Put called %d times, want 0", blobStore.putCalls)
	}
}

type uploadSessionMetadataStore struct {
	putCalls   int
	batchCalls int
	batchErr   error
	artifacts  []*Artifact
}

func (s *uploadSessionMetadataStore) Get(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return nil, ErrNotFound
}

func (s *uploadSessionMetadataStore) Put(ctx context.Context, artifact *Artifact) error {
	s.putCalls++
	s.artifacts = append(s.artifacts, artifact)
	return nil
}

func (s *uploadSessionMetadataStore) BatchPut(ctx context.Context, artifacts []*Artifact) error {
	s.batchCalls++
	if s.batchErr != nil {
		return s.batchErr
	}
	s.artifacts = append(s.artifacts, artifacts...)
	return nil
}

func (s *uploadSessionMetadataStore) Delete(ctx context.Context, key ArtifactKey) error {
	return nil
}

func (s *uploadSessionMetadataStore) Query(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return s.artifacts, nil
}

type uploadSessionBlobStore struct {
	next    int
	deleted []BlobRef
}

func (s *uploadSessionBlobStore) Put(reader io.Reader) (BlobRef, error) {
	_, _ = io.ReadAll(reader)
	s.next++
	return BlobRef{Algorithm: "sha256", Digest: string(rune('a' + s.next - 1)), Size: int64(s.next)}, nil
}

func (s *uploadSessionBlobStore) Open(ref BlobRef) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (s *uploadSessionBlobStore) Stat(ref BlobRef) (*BlobMetadata, error) {
	return &BlobMetadata{Algorithm: ref.Algorithm, Digest: ref.Digest, Size: ref.Size}, nil
}

func (s *uploadSessionBlobStore) Delete(ref BlobRef) error {
	s.deleted = append(s.deleted, ref)
	return nil
}

type contextUploadSessionBlobStore struct {
	uploadSessionBlobStore
	contextValue any
	putCalls     int
}

func (s *contextUploadSessionBlobStore) Put(reader io.Reader) (BlobRef, error) {
	s.putCalls++
	return s.uploadSessionBlobStore.Put(reader)
}

func (s *contextUploadSessionBlobStore) PutContext(ctx context.Context, reader io.Reader) (BlobRef, error) {
	s.contextValue = ctx.Value(contextKey("request"))
	return s.uploadSessionBlobStore.Put(reader)
}
