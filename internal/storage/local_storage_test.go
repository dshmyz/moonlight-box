package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStoragePutReadsFullContent(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir, 100)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}

	ctx := context.Background()
	content := strings.Repeat("hello-world-", 1000) // ~12KB
	reader := strings.NewReader(content)

	err = s.Put(ctx, "test/file.txt", reader, int64(len(content)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := s.Get(ctx, "test/file.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer got.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(got)
	if err != nil {
		t.Fatalf("Read from Get failed: %v", err)
	}

	if buf.String() != content {
		t.Fatalf("content mismatch: got %d bytes, want %d bytes", buf.Len(), len(content))
	}
}

func TestLocalStoragePutOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir, 100)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}

	ctx := context.Background()

	// 写入初始内容
	err = s.Put(ctx, "test/overwrite.txt", strings.NewReader("initial"), 7)
	if err != nil {
		t.Fatalf("first Put failed: %v", err)
	}

	// 覆盖写入
	err = s.Put(ctx, "test/overwrite.txt", strings.NewReader("overwritten"), 11)
	if err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	got, err := s.Get(ctx, "test/overwrite.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer got.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(got)
	if buf.String() != "overwritten" {
		t.Fatalf("got %q, want %q", buf.String(), "overwritten")
	}
}

func TestLocalStoragePutRejectsShortWrite(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir, 100)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}

	ctx := context.Background()
	content := "short"
	// 声明更大的 size 但实际写入更少
	err = s.Put(ctx, "test/short.txt", strings.NewReader(content), 999)
	if err == nil {
		t.Fatal("expected error for short write, got nil")
	}

	// 验证文件被清理
	exists, _ := s.Exists(ctx, "test/short.txt")
	if exists {
		t.Fatal("short-write file should have been cleaned up")
	}
}

func TestLocalStoragePutMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir, 100)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}

	ctx := context.Background()
	files := map[string]string{
		"a/1.txt": "aaa",
		"b/2.txt": "bbb",
		"c/3.txt": "ccc",
	}

	for key, content := range files {
		err := s.Put(ctx, key, strings.NewReader(content), int64(len(content)))
		if err != nil {
			t.Fatalf("Put(%q) failed: %v", key, err)
		}
	}

	entries, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// List 返回直接子目录
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Key] = true
	}
	if !found["a"] || !found["b"] || !found["c"] {
		t.Fatalf("List entries: got %v, want all dirs", entries)
	}
}

// BenchmarkLocalStoragePutAllocs 基准测试：测量 Put 的内存分配次数
func BenchmarkLocalStoragePutAllocs(b *testing.B) {
	dir := b.TempDir()
	s, err := NewLocalStorage(dir, 100)
	if err != nil {
		b.Fatalf("NewLocalStorage failed: %v", err)
	}

	ctx := context.Background()
	content := []byte(strings.Repeat("data", 256)) // 1KB
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := filepath.Join("bench", "file.txt")
		reader := bytes.NewReader(content)
		err := s.Put(ctx, key, reader, int64(len(content)))
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
		// 清理以便下次写入
		s.Delete(ctx, key)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}