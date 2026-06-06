package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

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
