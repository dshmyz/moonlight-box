package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
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

// TestCASBlobStoreHealsDanglingBlobRow 验证悬空记录自愈：blobs 表有记录但文件
// 已丢失（磁盘清理/迁移残留）时，重新 Put 同一内容必须补写文件并复用原记录，
// 而不是丢弃新内容导致 Open 永远失败。
func TestCASBlobStoreHealsDanglingBlobRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatalf("migrate blobs: %v", err)
	}
	backend := newMovingMemoryBackend()
	store := NewCASBlobStore(backend, db)

	first, err := store.Put(bytes.NewBufferString("cas-content"))
	if err != nil {
		t.Fatalf("first Put failed: %v", err)
	}

	// 模拟文件丢失：保留 blobs 表记录，删除磁盘文件
	var row model.Blob
	if err := db.First(&row, first.BlobID).Error; err != nil {
		t.Fatalf("load blob row: %v", err)
	}
	delete(backend.files, row.StoragePath)

	// Open 应映射为 runtime.ErrNotFound（供上层自愈），而非原始 package not found
	if _, err := store.Open(first); !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("Open on missing file: err = %v, want runtime.ErrNotFound", err)
	}

	// RefsExist 应报告缺失
	if ok, _ := store.RefsExist(context.Background(), []runtime.BlobRef{first}); ok {
		t.Fatalf("RefsExist on missing file should be false")
	}

	// 重新 Put 同一内容：补写文件、复用原 BlobID、不产生重复行
	second, err := store.Put(bytes.NewBufferString("cas-content"))
	if err != nil {
		t.Fatalf("second Put failed: %v", err)
	}
	if second.BlobID != first.BlobID {
		t.Fatalf("healed Put should reuse blob ID: got %d, want %d", second.BlobID, first.BlobID)
	}
	var count int64
	db.Model(&model.Blob{}).Count(&count)
	if count != 1 {
		t.Fatalf("blob rows = %d, want 1 (no duplicates)", count)
	}
	if _, err := store.Open(second); err != nil {
		t.Fatalf("Open after heal failed: %v", err)
	}
}

// isBlobMissingErr 只匹配 S3 SDK 规范形态（"StatusCode: 404" / 错误码 "NotFound"）：
// 裸 "404" 子串会误命中含十六进制 digest 的本地路径（如 ".../aa/bb4040ff..."），
// 把真实 I/O 错误误判成"文件丢失"送进自愈分支。
func TestIsBlobMissingErr(t *testing.T) {
	digestPath := "/data/cas/sha256/aa/bb4040cc/404aaa404bbbb"
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"os not exist", os.ErrNotExist, true},
		{"wrapped os not exist", fmt.Errorf("open %s: %w", digestPath, os.ErrNotExist), true},
		{"s3 status 404", errors.New("operation error S3: HeadObject, https response error StatusCode: 404, NotFound"), true},
		{"io error on path containing 404", errors.New("read " + digestPath + ": input/output error"), false},
		{"permission denied", errors.New("open " + digestPath + ": permission denied"), false},
		{"connection reset", errors.New("read tcp 1.2.3.4:443: connection reset by peer"), false},
	}
	for _, tc := range cases {
		if got := isBlobMissingErr(tc.err); got != tc.want {
			t.Errorf("%s: isBlobMissingErr(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

// 引用指向的 blob 行不存在 = 确定悬空，RefsExist 应返回 (false, nil)，
// 让 runtime 直接走重下自愈；后端探测错误才返回 err（"不确定"）。
func TestCASBlobStoreRefsExistMissingRowIsDeterminate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatalf("migrate blobs: %v", err)
	}
	store := NewCASBlobStore(newMovingMemoryBackend(), db)

	exists, err := store.RefsExist(context.Background(), []runtime.BlobRef{{BlobID: 999}})
	if err != nil {
		t.Fatalf("RefsExist(missing row) returned err %v, want nil (determinate dangling ref)", err)
	}
	if exists {
		t.Fatal("RefsExist(missing row) = true, want false")
	}
}
