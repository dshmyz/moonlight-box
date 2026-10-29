package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newHealTestStore(t *testing.T) (*CASBlobStore, *LocalStorage, *gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dir := t.TempDir()
	backend, err := NewLocalStorage(dir, 0)
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	return NewCASBlobStore(backend, db), backend, db, dir
}

// corruptBlobFile 把 blob 对应的磁盘文件截断，模拟历史损坏（磁盘满/进程被杀/旧版本缺陷）。
func corruptBlobFile(t *testing.T, db *gorm.DB, dir string, blobID uint, keepBytes int) {
	t.Helper()
	var blob model.Blob
	if err := db.First(&blob, blobID).Error; err != nil {
		t.Fatalf("load blob: %v", err)
	}
	full := filepath.Join(dir, blob.StoragePath)
	content, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read blob file: %v", err)
	}
	if err := os.WriteFile(full, content[:keepBytes], 0644); err != nil {
		t.Fatalf("truncate blob file: %v", err)
	}
}

// 坏文件在读取时必须被识别为不可用，而不是把截断的字节发给客户端。
func TestCASBlobStoreOpenRejectsSizeMismatch(t *testing.T) {
	store, _, db, dir := newHealTestStore(t)
	ctx := context.Background()
	content := bytes.Repeat([]byte("tarball-"), 200)

	ref, err := store.PutContext(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutContext: %v", err)
	}
	corruptBlobFile(t, db, dir, ref.BlobID, 100)

	if _, err := store.OpenContext(ctx, ref); !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("Open err = %v, want ErrNotFound", err)
	}
}

// 悬空探测必须能识别"文件在但内容坏了"，否则 ProxyRuntime 会跳过重新回源。
func TestCASBlobStoreRefsExistDetectsSizeMismatch(t *testing.T) {
	store, _, db, dir := newHealTestStore(t)
	ctx := context.Background()
	content := bytes.Repeat([]byte("tarball-"), 200)

	ref, err := store.PutContext(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutContext: %v", err)
	}

	ok, err := store.RefsExist(ctx, []runtime.BlobRef{ref})
	if err != nil || !ok {
		t.Fatalf("RefsExist(intact) = (%v, %v), want (true, nil)", ok, err)
	}

	corruptBlobFile(t, db, dir, ref.BlobID, 100)
	ok, err = store.RefsExist(ctx, []runtime.BlobRef{ref})
	if err != nil {
		t.Fatalf("RefsExist(corrupt): %v", err)
	}
	if ok {
		t.Fatal("RefsExist(corrupt) = true, want false")
	}
}

// 再次回源同样内容时，必须用下载好的完整内容覆盖坏文件，而不是直接复用。
func TestCASBlobStorePutRepairsCorruptExistingFile(t *testing.T) {
	store, _, db, dir := newHealTestStore(t)
	ctx := context.Background()
	content := bytes.Repeat([]byte("tarball-"), 200)

	ref, err := store.PutContext(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutContext: %v", err)
	}
	corruptBlobFile(t, db, dir, ref.BlobID, 100)

	ref2, err := store.PutContext(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutContext(repair): %v", err)
	}
	if ref2.Size != int64(len(content)) {
		t.Fatalf("ref2.Size = %d, want %d", ref2.Size, len(content))
	}
	if ref2.BlobID != ref.BlobID {
		t.Fatalf("repair created new blob row (%d -> %d), want reuse", ref.BlobID, ref2.BlobID)
	}

	rc, err := store.OpenContext(ctx, ref2)
	if err != nil {
		t.Fatalf("Open after repair: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read after repair: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("repaired content mismatch: got %d bytes, want %d", len(got), len(content))
	}
}

// 完整文件仍按 digest 去重复用，自愈逻辑不能把正常去重干掉。
func TestCASBlobStorePutStillReusesIntactFile(t *testing.T) {
	store, _, db, _ := newHealTestStore(t)
	ctx := context.Background()
	content := bytes.Repeat([]byte("tarball-"), 200)

	ref1, err := store.PutContext(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutContext #1: %v", err)
	}
	ref2, err := store.PutContext(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutContext #2: %v", err)
	}
	if ref1.BlobID != ref2.BlobID {
		t.Fatalf("dedupe broken: blob ids %d vs %d", ref1.BlobID, ref2.BlobID)
	}
	var count int64
	db.Model(&model.Blob{}).Count(&count)
	if count != 1 {
		t.Fatalf("blob rows = %d, want 1", count)
	}
}

// Size 为 0 的历史行无法判定完整性，不能因为校验就拒绝读取。
func TestCASBlobStoreOpenAllowsLegacyZeroSizeRow(t *testing.T) {
	store, backend, db, _ := newHealTestStore(t)
	ctx := context.Background()
	content := []byte("legacy-content")
	path := "blobs/sha256/le/ga/legacy"
	if err := backend.Put(ctx, path, bytes.NewReader(content), 0); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}
	blob := &model.Blob{Algorithm: "sha256", Digest: "legacy", Size: 0, StoragePath: path}
	if err := db.Create(blob).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	rc, err := store.OpenContext(ctx, runtime.BlobRef{BlobID: blob.ID})
	if err != nil {
		t.Fatalf("OpenContext(legacy) = %v, want nil", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, content) {
		t.Fatalf("legacy content = %q, want %q", got, content)
	}
}
