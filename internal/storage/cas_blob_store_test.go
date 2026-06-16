package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCASBlobStoreUsesBackendMoveWhenAvailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatalf("migrate blobs: %v", err)
	}
	backend := newMovingMemoryBackend()
	store := NewCASBlobStore(backend, db)

	ref, err := store.Put(bytes.NewBufferString("cas-content"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if ref.Size != int64(len("cas-content")) {
		t.Fatalf("size = %d, want %d", ref.Size, len("cas-content"))
	}
	if backend.moveCalls != 1 {
		t.Fatalf("Move called %d times, expected 1", backend.moveCalls)
	}
	if backend.getCalls != 0 {
		t.Fatalf("Get called %d times during Put, expected 0 when Move is available", backend.getCalls)
	}
	if backend.putCalls != 1 {
		t.Fatalf("Put called %d times, expected only temp write", backend.putCalls)
	}
}

func TestCASBlobStorePutContextPassesContextToBackend(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "cas-ctx")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatalf("migrate blobs: %v", err)
	}
	backend := newMovingMemoryBackend()
	backend.contextKey = contextKey("request")
	store := NewCASBlobStore(backend, db)

	if _, err := store.PutContext(ctx, bytes.NewBufferString("cas-content")); err != nil {
		t.Fatalf("PutContext failed: %v", err)
	}
	if got := backend.contextValue; got != "cas-ctx" {
		t.Fatalf("backend context value = %v, want cas-ctx", got)
	}
}

type movingMemoryBackend struct {
	files        map[string][]byte
	putCalls     int
	getCalls     int
	moveCalls    int
	contextKey   any
	contextValue any
}

func newMovingMemoryBackend() *movingMemoryBackend {
	return &movingMemoryBackend{files: map[string][]byte{}}
}

func (b *movingMemoryBackend) Name() string               { return "moving-memory" }
func (b *movingMemoryBackend) Init(basePath string) error { return nil }

func (b *movingMemoryBackend) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	b.putCalls++
	if b.contextKey != nil && b.contextValue == nil {
		b.contextValue = ctx.Value(b.contextKey)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	b.files[key] = body
	return nil
}

func (b *movingMemoryBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b.getCalls++
	body, ok := b.files[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (b *movingMemoryBackend) Delete(ctx context.Context, key string) error {
	delete(b.files, key)
	return nil
}

func (b *movingMemoryBackend) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := b.files[key]
	return ok, nil
}

func (b *movingMemoryBackend) Size(ctx context.Context, key string) (int64, error) {
	return int64(len(b.files[key])), nil
}

func (b *movingMemoryBackend) List(ctx context.Context, prefix string) ([]Entry, error) {
	return nil, nil
}

func (b *movingMemoryBackend) Browse(ctx context.Context, path string) ([]BrowseEntry, error) {
	return nil, nil
}

func (b *movingMemoryBackend) Close() error { return nil }
func (b *movingMemoryBackend) BasePath() string {
	return filepath.Clean("/")
}

func (b *movingMemoryBackend) Move(ctx context.Context, oldKey, newKey string) error {
	b.moveCalls++
	body, ok := b.files[oldKey]
	if !ok {
		return os.ErrNotExist
	}
	b.files[newKey] = body
	delete(b.files, oldKey)
	return nil
}
