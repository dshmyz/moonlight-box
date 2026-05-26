package service

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/storage"
	"github.com/stretchr/testify/assert"
)

func setupTestStorageService(t *testing.T) (*StorageService, string) {
	testDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(testDir) })

	storageSvc, err := NewStorageService(nil, testDir, 1)
	if err != nil {
		t.Fatalf("failed to create storage service: %v", err)
	}

	localBackend, _ := storage.NewLocalStorage(testDir, 1)
	storageSvc.SetDefaultBackendForTest(localBackend)

	return storageSvc, testDir
}

func TestBuildKey_WithRepoName(t *testing.T) {
	storageSvc, _ := setupTestStorageService(t)

	tests := []struct {
		name     string
		repoName string
		pkgType  string
		pkgName  string
		version  string
		expected string
	}{
		{
			name:     "npm repo with npm type",
			repoName: "npm",
			pkgType:  "npm",
			pkgName:  "lodash",
			version:  "4.17.21",
			expected: "npm/npm/lodash/4.17.21",
		},
		{
			name:     "scoped npm package",
			repoName: "npm",
			pkgType:  "npm",
			pkgName:  "@scope/package",
			version:  "1.0.0",
			expected: "npm/npm/@scope/package/1.0.0",
		},
		{
			name:     "maven package",
			repoName: "my-repo",
			pkgType:  "maven",
			pkgName:  "com/example/mylib",
			version:  "1.0.0",
			expected: "maven/my-repo/com/example/mylib/1.0.0",
		},
		{
			name:     "pypi package",
			repoName: "pypi-cache",
			pkgType:  "pypi",
			pkgName:  "requests",
			version:  "2.28.0",
			expected: "pypi/pypi-cache/requests/2.28.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := storageSvc.buildKey(tt.repoName, tt.pkgType, tt.pkgName, tt.version)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestBuildKey_WithoutRepoName(t *testing.T) {
	storageSvc, _ := setupTestStorageService(t)

	tests := []struct {
		name     string
		pkgType  string
		pkgName  string
		version  string
		expected string
	}{
		{
			name:     "npm package without repo",
			pkgType:  "npm",
			pkgName:  "lodash",
			version:  "4.17.21",
			expected: "npm/lodash/4.17.21",
		},
		{
			name:     "scoped npm without repo",
			pkgType:  "npm",
			pkgName:  "@scope/package",
			version:  "1.0.0",
			expected: "npm/@scope/package/1.0.0",
		},
		{
			name:     "maven without repo",
			pkgType:  "maven",
			pkgName:  "com/example/mylib",
			version:  "1.0.0",
			expected: "maven/com/example/mylib/1.0.0",
		},
		{
			name:     "go path is not normalized by storage service",
			pkgType:  "go",
			pkgName:  "example.com/module",
			version:  "v1.0.0.zip",
			expected: "go/example.com/module/v1.0.0.zip",
		},
		{
			name:     "empty version is omitted",
			pkgType:  "_meta_cache",
			pkgName:  "npm/package.json",
			version:  "",
			expected: "_meta_cache/npm/package.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := storageSvc.buildKey("", tt.pkgType, tt.pkgName, tt.version)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestStoreAndGetPackage_WithRepoName(t *testing.T) {
	storageSvc, testDir := setupTestStorageService(t)

	ctx := context.Background()
	repoName := "npm"
	pkgType := "npm"
	pkgName := "test-package"
	version := "1.0.0"
	content := []byte(`{"name":"test-package","version":"1.0.0"}`)

	// Store package
	storageKey, err := storageSvc.StorePackageWithBackend(ctx, repoName, pkgType, pkgName, version, bytes.NewReader(content), int64(len(content)), 0)
	assert.Nil(t, err)
	assert.Equal(t, "npm/npm/test-package/1.0.0", storageKey)

	// Verify file exists at expected location
	expectedPath := filepath.Join(testDir, "npm", "npm", "test-package", "1.0.0")
	_, err = os.Stat(expectedPath)
	assert.Nil(t, err, "file should exist at %s", expectedPath)

	// Get package back - must use same repoName
	reader, size, err := storageSvc.GetPackageWithBackend(ctx, repoName, pkgType, pkgName, version, 0)
	assert.Nil(t, err)
	assert.Equal(t, int64(len(content)), size)

	// Read content
	buf := make([]byte, size)
	n, _ := reader.Read(buf)
	reader.Close()
	assert.Equal(t, content, buf[:n])
}

func TestStoreAndGetPackage_DifferentRepos(t *testing.T) {
	storageSvc, testDir := setupTestStorageService(t)

	ctx := context.Background()
	content := []byte(`{"name":"test-package","version":"1.0.0"}`)

	// Store to repo "npm"
	_, err := storageSvc.StorePackageWithBackend(ctx, "npm", "npm", "test-package", "1.0.0", bytes.NewReader(content), int64(len(content)), 0)
	assert.Nil(t, err)

	// Store to repo "npm-cache"
	_, err = storageSvc.StorePackageWithBackend(ctx, "npm-cache", "npm", "test-package", "1.0.0", bytes.NewReader(content), int64(len(content)), 0)
	assert.Nil(t, err)

	// Verify both files exist at different locations
	path1 := filepath.Join(testDir, "npm", "npm", "test-package", "1.0.0")
	path2 := filepath.Join(testDir, "npm", "npm-cache", "test-package", "1.0.0")

	_, err = os.Stat(path1)
	assert.Nil(t, err, "file should exist at %s", path1)

	_, err = os.Stat(path2)
	assert.Nil(t, err, "file should exist at %s", path2)

	// Verify they are different files
	assert.NotEqual(t, path1, path2)
}

func TestMetadataCacheSetUsesActualBodySize(t *testing.T) {
	storageSvc, _ := setupTestStorageService(t)
	cache := NewMetadataCache(storageSvc)
	ctx := context.Background()

	body := []byte(`{"name":"demo","versions":{"1.0.0":{}}}`)
	err := cache.Set(ctx, "npm-proxy", "npm", "demo", bytes.NewReader(body), -1, time.Minute)
	assert.NoError(t, err)

	reader, size, err := cache.Get(ctx, "npm-proxy", "npm", "demo")
	assert.NoError(t, err)
	defer reader.Close()

	cachedBody, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, body, cachedBody)
	assert.Equal(t, int64(len(body)), size)
}
