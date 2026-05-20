package adapter

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/cache"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGoAdapterFull(t *testing.T) (*GoAdapter, *gorm.DB) {
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

	adapter := NewGoAdapter(storageSvc, pkgCache)
	return adapter, db
}

func TestGoAdapter_ParsePath_VersionList(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/list")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "list", pathInfo.Filename)
	assert.Equal(t, "github.com/user/repo", pathInfo.StorageName)
	assert.Equal(t, "github.com/user/repo/@v/list", pathInfo.RemotePath)
}

func TestGoAdapter_ParsePath_Latest(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@latest")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "latest", pathInfo.Filename)
	assert.Equal(t, "github.com/user/repo", pathInfo.StorageName)
	assert.Equal(t, "github.com/user/repo/@latest", pathInfo.RemotePath)
}

func TestGoAdapter_ParsePath_VersionInfo(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/v1.0.0.info")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "v1.0.0", pathInfo.Version)
	assert.Equal(t, "v1.0.0.info", pathInfo.Filename)
	assert.Equal(t, "github.com/user/repo", pathInfo.StorageName)
	assert.Equal(t, "@v/v1.0.0.info", pathInfo.StorageVersion)
	assert.Equal(t, "github.com/user/repo/@v/v1.0.0.info", pathInfo.RemotePath)
}

func TestGoAdapter_ParsePath_GoMod(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/v1.0.0.mod")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "v1.0.0", pathInfo.Version)
	assert.Equal(t, "v1.0.0.mod", pathInfo.Filename)
	assert.Equal(t, "@v/v1.0.0.mod", pathInfo.StorageVersion)
}

func TestGoAdapter_ParsePath_ZipFile(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/v1.0.0.zip")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "v1.0.0", pathInfo.Version)
	assert.Equal(t, "v1.0.0.zip", pathInfo.Filename)
	assert.Equal(t, "@v/v1.0.0.zip", pathInfo.StorageVersion)
}

func TestGoAdapter_ParsePath_SimpleModule(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("simple-module/@v/v1.0.0.info")
	assert.Nil(t, err)
	assert.Equal(t, "simple-module", pathInfo.Name)
	assert.Equal(t, "v1.0.0", pathInfo.Version)
}

func TestGoAdapter_ParsePath_WithMajorVersion(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/v2.1.0.info")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "v2.1.0", pathInfo.Version)
	assert.Equal(t, "v2.1.0.info", pathInfo.Filename)
}

func TestGoAdapter_ParsePath_PreReleaseVersion(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/v1.0.0-beta.1.info")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "v1.0.0-beta.1", pathInfo.Version)
	assert.Equal(t, "v1.0.0-beta.1.info", pathInfo.Filename)
}

func TestGoAdapter_ParsePath_FallbackFormat(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/v1.0.0.info")
	assert.Nil(t, err)
	assert.Equal(t, "github.com/user/repo", pathInfo.Name)
	assert.Equal(t, "v1.0.0.info", pathInfo.Filename)
	assert.Equal(t, "v1.0.0.info", pathInfo.Version)
}

func TestGoAdapter_ParsePath_PathTraversal(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	tests := []struct {
		name string
		path string
	}{
		{"path with double dot", "../etc/passwd/@v/list"},
		{"path with traversal in middle", "github.com/user/../../etc/passwd/@v/list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adapter.ParsePath(tt.path)
			assert.NotNil(t, err)
			assert.Contains(t, err.Error(), "path traversal")
		})
	}
}

func TestGoAdapter_ParsePath_InvalidPath_TooFewParts(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	_, err := adapter.ParsePath("single")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid go module path")
}

func TestGoAdapter_ParsePath_InvalidPath_MissingVersionFile(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	_, err := adapter.ParsePath("github.com/user/repo/@v/")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid go module version file")
}

func TestGoAdapter_ParsePath_InvalidPath_NoExtension(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	pathInfo, err := adapter.ParsePath("github.com/user/repo/@v/v1.0.0")
	if err != nil {
		assert.Contains(t, err.Error(), "invalid go module version file")
		return
	}
	assert.NotNil(t, pathInfo)
}

func TestGoAdapter_ParseIntent_ListVersions(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	intent := adapter.ParseIntent("github.com/user/repo/@v/list", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestList, intent.Type)
	assert.Equal(t, "github.com/user/repo", intent.Name)
	assert.Equal(t, "list", intent.Filename)
}

func TestGoAdapter_ParseIntent_LatestVersion(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	intent := adapter.ParseIntent("github.com/user/repo/@latest", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestMetadata, intent.Type)
	assert.Equal(t, "github.com/user/repo", intent.Name)
	assert.Equal(t, "latest", intent.Filename)
}

func TestGoAdapter_ParseIntent_DownloadInfo(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	intent := adapter.ParseIntent("github.com/user/repo/@v/v1.0.0.info", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestDownload, intent.Type)
	assert.Equal(t, "github.com/user/repo", intent.Name)
	assert.Equal(t, "v1.0.0", intent.Version)
	assert.Equal(t, "v1.0.0.info", intent.Filename)
}

func TestGoAdapter_ParseIntent_DownloadMod(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	intent := adapter.ParseIntent("github.com/user/repo/@v/v1.0.0.mod", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestDownload, intent.Type)
	assert.Equal(t, "v1.0.0.mod", intent.Filename)
}

func TestGoAdapter_ParseIntent_DownloadZip(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	intent := adapter.ParseIntent("github.com/user/repo/@v/v1.0.0.zip", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestDownload, intent.Type)
	assert.Equal(t, "v1.0.0.zip", intent.Filename)
}

func TestGoAdapter_HandleDelete(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/test", nil)

	deleteCtx := &types.DeleteContext{
		Name:    "test-module",
		Version: "v1.0.0",
	}

	err := adapter.HandleDelete(c, deleteCtx)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "cannot be deleted")
}

func TestDecodeGoModulePath_Normal(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/user/repo", "github.com/user/repo"},
		{"example.com!repo", "example.comRepo"},
		{"github.com!user!repo", "github.comUserRepo"},
		{"mixed!case!path", "mixedCasePath"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := decodeGoModulePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildGoDependenciesFromMod_ValidMod(t *testing.T) {
	modContent := []byte(`module github.com/test/repo

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/stretchr/testify v1.8.4
)

require github.com/go-sql-driver/mysql v1.7.1
`)

	deps := buildGoDependenciesFromMod(modContent)
	assert.Len(t, deps, 3)

	foundGin := false
	foundTestify := false
	foundMySQL := false
	for _, dep := range deps {
		switch dep.DepName {
		case "github.com/gin-gonic/gin":
			foundGin = true
			assert.Equal(t, "v1.9.1", dep.DepVersionConstraint)
			assert.False(t, dep.IsOptional)
		case "github.com/stretchr/testify":
			foundTestify = true
			assert.Equal(t, "v1.8.4", dep.DepVersionConstraint)
		case "github.com/go-sql-driver/mysql":
			foundMySQL = true
			assert.Equal(t, "v1.7.1", dep.DepVersionConstraint)
		}
	}
	assert.True(t, foundGin, "gin dependency not found")
	assert.True(t, foundTestify, "testify dependency not found")
	assert.True(t, foundMySQL, "mysql dependency not found")
}

func TestBuildGoDependenciesFromMod_EmptyMod(t *testing.T) {
	modContent := []byte(`module github.com/test/repo

go 1.21
`)

	deps := buildGoDependenciesFromMod(modContent)
	assert.Empty(t, deps)
}

func TestBuildGoDependenciesFromMod_WithIndirect(t *testing.T) {
	modContent := []byte(`module github.com/test/repo

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	golang.org/x/net v0.17.0 // indirect
)
`)

	deps := buildGoDependenciesFromMod(modContent)
	assert.Len(t, deps, 2)

	for _, dep := range deps {
		if dep.DepName == "golang.org/x/net" {
			assert.True(t, dep.IsOptional)
		}
	}
}

func TestParseVersionList(t *testing.T) {
	input := `v1.0.0
v1.1.0
v2.0.0
v2.0.0-beta.1`

	result := parseVersionList(input)
	assert.Equal(t, []string{
		"v1.0.0",
		"v1.1.0",
		"v2.0.0",
		"v2.0.0-beta.1",
	}, result)
}

func TestParseVersionList_EmptyLines(t *testing.T) {
	input := `
v1.0.0


v1.1.0
`

	result := parseVersionList(input)
	assert.Equal(t, []string{
		"v1.0.0",
		"v1.1.0",
	}, result)
}

func TestParseVersionList_SingleVersion(t *testing.T) {
	input := "v1.0.0"

	result := parseVersionList(input)
	assert.Equal(t, []string{"v1.0.0"}, result)
}

func TestGoAdapter_HandleGet_UnsupportedType(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	repo := &model.Repository{
		Name: "test-repo",
		Type: model.RepoTypeLocal,
	}

	intent := &types.RequestIntent{
		Type:     types.RequestDistTags,
		Name:     "test",
		Path:     "/test",
		Extra:    make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := adapter.HandleGet(ctx, repo, intent)
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unsupported request type")
}

func TestHandlePut_GoAdapter_MissingFile(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

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

func TestHandlePut_GoAdapter_MissingNameOrVersion(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := strings.NewReader("--boundary\r\n" +
		"Content-Disposition: form-data; name=\"file\"; filename=\"test.mod\"\r\n" +
		"Content-Type: text/plain\r\n\r\n" +
		"module test\r\n" +
		"--boundary--\r\n")

	c.Request = httptest.NewRequest("PUT", "/", body)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")

	publishCtx := &types.PublishContext{}

	result, err := adapter.HandlePut(c, publishCtx)
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "missing module name or version")
}

func TestGoAdapter_ListVersionsWithTime_NotFound(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	ctx := context.Background()
	versions, err := adapter.ListVersionsWithTime(ctx, "nonexistent/package")
	assert.NotNil(t, err)
	assert.Nil(t, versions)
}

func TestGoAdapter_ParseIntent_PathWithSlashPrefix(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	intent := adapter.ParseIntent("/github.com/user/repo/@v/list", "GET")
	assert.NotNil(t, intent)
	assert.Equal(t, types.RequestList, intent.Type)
	assert.Equal(t, "github.com/user/repo", intent.Name)
}

func TestGoAdapter_ParseIntent_ComplexPaths(t *testing.T) {
	adapter, _ := setupGoAdapterFull(t)

	tests := []struct {
		name         string
		path         string
		expectedType types.RequestType
		expectedName string
		expectedVer  string
		expectedFile string
	}{
		{
			name:         "version info with patch",
			path:         "golang.org/x/text/@v/v0.14.0.info",
			expectedType: types.RequestDownload,
			expectedName: "golang.org/x/text",
			expectedVer:  "v0.14.0",
			expectedFile: "v0.14.0.info",
		},
		{
			name:         "go mod file",
			path:         "gorm.io/gorm/@v/v1.25.5.mod",
			expectedType: types.RequestDownload,
			expectedName: "gorm.io/gorm",
			expectedVer:  "v1.25.5",
			expectedFile: "v1.25.5.mod",
		},
		{
			name:         "zip download",
			path:         "google.golang.org/grpc/@v/v1.59.0.zip",
			expectedType: types.RequestDownload,
			expectedName: "google.golang.org/grpc",
			expectedVer:  "v1.59.0",
			expectedFile: "v1.59.0.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := adapter.ParseIntent(tt.path, "GET")
			assert.Equal(t, tt.expectedType, intent.Type)
			assert.Equal(t, tt.expectedName, intent.Name)
			assert.Equal(t, tt.expectedVer, intent.Version)
			assert.Equal(t, tt.expectedFile, intent.Filename)
		})
	}
}
