package adapter

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAptAdapter(t *testing.T) (*AptAdapter, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	db.AutoMigrate(
		&model.Package{},
		&model.PackageVersion{},
		&model.PackageFile{},
		&model.PackageDependency{},
		&model.Repository{},
		&model.StorageBackend{},
	)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	pkgRepo := repository.NewPackageRepository(db)

	testDir := t.TempDir()

	configJSON := `{"local":{"base_path":"` + testDir + `","max_size_gb":1}}`
	backend := &model.StorageBackend{
		Name:       "test-local",
		Type:       model.StorageTypeLocal,
		IsDefault:  true,
		Status:     model.StatusActive,
		IsActive:   true,
		ConfigJSON: configJSON,
		Config: model.StorageBackendConfig{
			Local: &model.LocalConfig{
				BasePath:  testDir,
				MaxSizeGB: 1,
			},
		},
	}
	db.Create(backend)

	storageSvc, err := service.NewStorageService(storageBackendRepo, testDir, 1)
	if err != nil {
		t.Fatalf("failed to create storage service: %v", err)
	}

	pkgCache := cache.NewPackageCache(pkgRepo, 5*time.Minute)

	adapter := NewAptAdapter(storageSvc, pkgCache)
	return adapter, db
}

func TestAptAdapter_Type(t *testing.T) {
	adapter, _ := setupAptAdapter(t)
	assert.Equal(t, AptType, adapter.Type())
}

func TestAptAdapter_ParsePath_DebFile(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	pathInfo, err := adapter.ParsePath("pool/main/nginx/nginx_1.20.1-1_amd64.deb")
	assert.Nil(t, err)
	assert.Equal(t, "pool/main/nginx", pathInfo.Name)
	assert.Equal(t, "1.20.1-1", pathInfo.Version)
	assert.Equal(t, "nginx_1.20.1-1_amd64.deb", pathInfo.Filename)
	assert.Equal(t, "pool/main/nginx", pathInfo.StorageName)
	assert.Equal(t, "nginx_1.20.1-1_amd64.deb", pathInfo.StorageVersion)
	assert.Equal(t, "pool/main/nginx/nginx_1.20.1-1_amd64.deb", pathInfo.RemotePath)
}

func TestAptAdapter_ParsePath_SimpleDeb(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	pathInfo, err := adapter.ParsePath("pool/main/mypackage/mypackage_1.0.0_amd64.deb")
	assert.Nil(t, err)
	assert.Equal(t, "pool/main/mypackage", pathInfo.Name)
	assert.Equal(t, "1.0.0", pathInfo.Version)
	assert.Equal(t, "mypackage_1.0.0_amd64.deb", pathInfo.Filename)
}

func TestAptAdapter_ParsePath_WithSubdirectory(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	pathInfo, err := adapter.ParsePath("pool/contrib/python3/python3-pip_21.2.3-5_all.deb")
	assert.Nil(t, err)
	assert.Equal(t, "pool/contrib/python3", pathInfo.Name)
	assert.Equal(t, "21.2.3-5", pathInfo.Version)
	assert.Equal(t, "python3-pip_21.2.3-5_all.deb", pathInfo.Filename)
}

func TestAptAdapter_ParsePath_PathTraversal(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	tests := []struct {
		name string
		path string
	}{
		{"path with double dot", "../etc/passwd.deb"},
		{"path with traversal in middle", "pool/main/../../etc/test.deb"},
		{"Windows absolute path", "C:\\Windows\\System32\\test.deb"},
		{"Windows UNC path", "\\\\server\\share\\test.deb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adapter.ParsePath(tt.path)
			assert.NotNil(t, err)
		})
	}
}

func TestAptAdapter_ParsePath_NullCharacter(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	_, err := adapter.ParsePath("pool/main/test\x00file.deb")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "null characters not allowed")
}

func TestAptAdapter_ParsePath_TooFewParts(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	_, err := adapter.ParsePath("single")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid apt path")
}

func TestValidateAptPath_ValidPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		err  bool
	}{
		{"valid pool path", "pool/main/nginx/nginx_1.0.deb", false},
		{"valid dists path", "dists/stable/Release", false},
		{"simple filename", "package_1.0.deb", false},
		{"with hyphens", "my-package_2.0-1_amd64.deb", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAptPath(tt.path)
			if tt.err {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestIsValidDebFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"nginx_1.20.1-1_amd64.deb", true},
		{"python3-pip_21.2.3-5_all.deb", true},
		{"package_1.0.deb", true},
		{"my-package_2.0.0-1_arm64.deb", true},
		{".deb", false},
		{"package.rpm", false},
		{"package", false},
		{"", false},
		{"123package_1.0.deb", false}, // starts with number
		{"-package_1.0.deb", false},   // starts with hyphen
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isValidDebFilename(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseDebPackageName(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"nginx_1.20.1-1_amd64.deb", "nginx"},
		{"python3-pip_21.2.3-5_all.deb", "python3-pip"},
		{"mysql-server_8.0.25-1_amd64.deb", "mysql-server"},
		{"simple_1.0.deb", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := parseDebPackageName(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseDebPackageVersion(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"nginx_1.20.1-1_amd64.deb", "1.20.1-1"},
		{"python3-pip_21.2.3-5_all.deb", "21.2.3-5"},
		{"mysql-server_8.0.25-1_amd64.deb", "8.0.25-1"},
		{"simple_1.0.deb", "1.0"},
		{"nodeb_1.0", "1.0.0"}, // fallback when no version found
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := parseDebPackageVersion(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAptAdapter_ParseIntent_PoolDownload(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("pool/main/nginx/nginx_1.20.1-1_amd64.deb", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestDownload, intent.Type)
	assert.Equal(t, "nginx", intent.Name)
	assert.Equal(t, "1.20.1-1", intent.Version)
	assert.Equal(t, "nginx_1.20.1-1_amd64.deb", intent.Filename)
}

func TestAptAdapter_ParseIntent_ReleaseFile(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("dists/stable/Release", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestMetadata, intent.Type)
	assert.Equal(t, "stable", intent.Extra["dist"])
	assert.Equal(t, "Release", intent.Extra["file"])
}

func TestAptAdapter_ParseIntent_InReleaseFile(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("dists/stable/InRelease", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestMetadata, intent.Type)
	assert.Equal(t, "stable", intent.Extra["dist"])
	assert.Equal(t, "InRelease", intent.Extra["file"])
}

func TestAptAdapter_ParseIntent_ReleaseGPG(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("dists/stable/Release.gpg", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestMetadata, intent.Type)
	assert.Equal(t, "Release.gpg", intent.Extra["file"])
}

func TestAptAdapter_ParseIntent_PackagesFile(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("dists/stable/main/binary-amd64/Packages", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestMetadata, intent.Type)
	assert.Equal(t, "Packages", intent.Filename)
}

func TestAptAdapter_ParseIntent_PackagesGzFile(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("dists/stable/main/binary-amd64/Packages.gz", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestMetadata, intent.Type)
	assert.Equal(t, "Packages.gz", intent.Filename)
}

func TestAptAdapter_ParseIntent_PathWithSlashPrefix(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	intent := adapter.ParseIntent("/pool/main/nginx/nginx_1.0.deb", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestDownload, intent.Type)
	assert.Equal(t, "nginx", intent.Name)
}

func TestAptAdapter_HandleGet_PoolDownload_NotFound(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	repo := &model.Repository{
		Name: "test-repo",
		Type: model.RepoTypeLocal,
	}

	intent := &types.RequestIntent{
		Type:     types.RequestDownload,
		Path:     "pool/main/nonexistent/package_1.0.deb",
		Name:     "nonexistent",
		Version:  "1.0",
		Filename: "package_1.0.deb",
		Extra:    make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := adapter.HandleGet(ctx, repo, intent)
	if err == nil && result.StatusCode == 404 {
		return
	}
	assert.NotNil(t, err)
}

func TestAptAdapter_HandleGet_DistsRelease(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	repo := &model.Repository{
		Name: "test-repo",
		Type: model.RepoTypeLocal,
	}

	intent := &types.RequestIntent{
		Type: types.RequestMetadata,
		Path: "dists/stable/Release",
		Extra: map[string]interface{}{
			"dist": "stable",
			"file": "Release",
		},
	}

	ctx := context.Background()
	result, err := adapter.HandleGet(ctx, repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
	assert.Contains(t, result.ContentType, "text/plain")
}

func TestAptAdapter_HandleGet_DistsInRelease(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	repo := &model.Repository{
		Name: "test-repo",
		Type: model.RepoTypeLocal,
	}

	intent := &types.RequestIntent{
		Type: types.RequestMetadata,
		Path: "dists/stable/InRelease",
		Extra: map[string]interface{}{
			"dist": "stable",
			"file": "InRelease",
		},
	}

	ctx := context.Background()
	result, err := adapter.HandleGet(ctx, repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestAptAdapter_HandleGet_DistsReleaseGPG(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	repo := &model.Repository{
		Name: "test-repo",
		Type: model.RepoTypeLocal,
	}

	intent := &types.RequestIntent{
		Type: types.RequestMetadata,
		Path: "dists/stable/Release.gpg",
		Extra: map[string]interface{}{
			"file": "Release.gpg",
		},
	}

	ctx := context.Background()
	result, err := adapter.HandleGet(ctx, repo, intent)
	if err != nil || result.StatusCode == 404 {
		return
	}
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestAptAdapter_HandleGet_NotFound(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	repo := &model.Repository{
		Name: "test-repo",
		Type: model.RepoTypeLocal,
	}

	intent := &types.RequestIntent{
		Type:     types.RequestDownload,
		Path:     "unknown/path/file.txt",
		Name:     "unknown",
		Filename: "file.txt",
		Extra:    make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := adapter.HandleGet(ctx, repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 404, result.StatusCode)
}

func TestHandlePut_AptAdapter_MissingFile(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(""))
	c.Request.Header.Set("Content-Type", "multipart/form-data")

	publishCtx := &types.PublishContext{}

	result, err := adapter.HandlePut(c, publishCtx)
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "missing file")
}

func TestHandlePut_AptAdapter_InvalidFileType(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := strings.NewReader("--boundary\r\n" +
		"Content-Disposition: form-data; name=\"file\"; filename=\"test.rpm\"\r\n" +
		"Content-Type: application/octet-stream\r\n\r\n" +
		"dummy content\r\n" +
		"--boundary--\r\n")

	c.Request = httptest.NewRequest("PUT", "/", body)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")

	publishCtx := &types.PublishContext{}

	result, err := adapter.HandlePut(c, publishCtx)
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid file type")
}

func TestFormatPackageEntry(t *testing.T) {
	entry := AptPackageEntry{
		Package:       "nginx",
		Version:       "1.20.1-1",
		Architecture:  "amd64",
		Maintainer:    "Test Maintainer <test@example.com>",
		Description:   "High performance web server",
		Section:       "web",
		Priority:      "optional",
		InstalledSize: "10240",
		Filename:      "pool/main/nginx/nginx_1.20.1-1_amd64.deb",
		Size:          2048000,
	}

	result := formatPackageEntry(entry)
	assert.Contains(t, result, "Package: nginx")
	assert.Contains(t, result, "Version: 1.20.1-1")
	assert.Contains(t, result, "Architecture: amd64")
	assert.Contains(t, result, "Maintainer: Test Maintainer <test@example.com>")
	assert.Contains(t, result, "Description: High performance web server")
	assert.Contains(t, result, "Section: web")
	assert.Contains(t, result, "Priority: optional")
	assert.Contains(t, result, "Installed-Size: 10240")
	assert.Contains(t, result, "Filename: pool/main/nginx/nginx_1.20.1-1_amd64.deb")
	assert.Contains(t, result, "Size: 2048000")
}

func TestAptAdapter_ParseIntent_ComplexPoolPaths(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	tests := []struct {
		name         string
		path         string
		expectedName string
		expectedVer  string
		expectedFile string
	}{
		{
			name:         "amd64 package",
			path:         "pool/main/g/gcc/gcc-12_12.1.0-1_amd64.deb",
			expectedName: "gcc-12",
			expectedVer:  "12.1.0-1",
			expectedFile: "gcc-12_12.1.0-1_amd64.deb",
		},
		{
			name:         "arm64 package",
			path:         "pool/main/r/rustc/rustc_1.65.0-1_arm64.deb",
			expectedName: "rustc",
			expectedVer:  "1.65.0-1",
			expectedFile: "rustc_1.65.0-1_arm64.deb",
		},
		{
			name:         "all architecture package",
			path:         "pool/main/p/python3-doc/python3-doc_3.10.6-1_all.deb",
			expectedName: "python3-doc",
			expectedVer:  "3.10.6-1",
			expectedFile: "python3-doc_3.10.6-1_all.deb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := adapter.ParseIntent(tt.path, "GET")
			assert.Equal(t, types.RequestDownload, intent.Type)
			assert.Equal(t, tt.expectedName, intent.Name)
			assert.Equal(t, tt.expectedVer, intent.Version)
			assert.Equal(t, tt.expectedFile, intent.Filename)
		})
	}
}

func TestAptAdapter_ParseIntent_MultipleDistributions(t *testing.T) {
	adapter, _ := setupAptAdapter(t)

	tests := []struct {
		name         string
		path         string
		expectedDist string
	}{
		{"stable distribution", "dists/stable/Release", "stable"},
		{"testing distribution", "dists/testing/Release", "testing"},
		{"unstable distribution", "dists/unstable/Release", "unstable"},
		{"codename distribution", "dists/jammy/Release", "jammy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := adapter.ParseIntent(tt.path, "GET")
			assert.Equal(t, types.RequestMetadata, intent.Type)
			assert.Equal(t, tt.expectedDist, intent.Extra["dist"])
		})
	}
}
