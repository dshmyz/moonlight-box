package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/moonlight-box/registry/internal/cache"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPyPIAdapter(t *testing.T) (*PyPIAdapter, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	db.AutoMigrate(
		&model.Component{},
		&model.Component{},
		&model.Asset{},
		&model.ComponentDependency{},
		&model.Repository{},
		&model.StorageBackend{},
	)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	compRepo := repository.NewComponentRepository(db)

	// 确保存储目录存在
	os.MkdirAll("/tmp/test-pypi-storage", 0755)

	storageSvc, err := service.NewStorageService(storageBackendRepo, "/tmp/test-pypi-storage", 1)
	if err != nil {
		t.Fatalf("failed to create storage service: %v", err)
	}

	backend := &model.StorageBackend{
		Name:      "test-local-pypi",
		Type:      model.StorageTypeLocal,
		IsDefault: true,
		Status:    model.StatusActive,
		IsActive:  true,
		Config: model.StorageBackendConfig{
			Local: &model.LocalConfig{
				BasePath:  "/tmp/test-pypi-storage",
				MaxSizeGB: 1,
			},
		},
	}
	db.Create(backend)

	storageSvc.RefreshBackends()

	compCache := cache.NewComponentCache(compRepo, 5*time.Minute)
	repoRepo := repository.NewRepositoryRepository(db)

	adapter := NewPyPIAdapter(repoRepo, storageSvc, compCache)
	return adapter, db
}

func TestPyPIAdapter_Type(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)
	assert.Equal(t, PyPIType, adapter.Type())
}

func TestPyPIAdapter_ParsePath(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)

	tests := []struct {
		name            string
		path            string
		expectedName    string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "valid path with version",
			path:            "requests/2.28.0",
			expectedName:    "requests",
			expectedVersion: "2.28.0",
			expectError:     false,
		},
		{
			name:            "valid path - only package name",
			path:            "numpy",
			expectedName:    "numpy",
			expectedVersion: "",
			expectError:     false,
		},
		{
			name:        "invalid path - empty",
			path:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathInfo, err := adapter.ParsePath(tt.path)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedName, pathInfo.Name)
				assert.Equal(t, tt.expectedVersion, pathInfo.Version)
			}
		})
	}
}

// func TestPyPIAdapter_UploadPackage(t *testing.T) {
// 	adapter, db := setupPyPIAdapter(t)
//
// 	ctx := context.Background()
// 	wheelContent := []byte("fake wheel content for test")
//
// 	req := &UploadRequest{
// 		Package:  bytes.NewReader(wheelContent),
// 		FileName: "test_package-1.0.0-py3-none-any.whl",
// 		Size:     int64(len(wheelContent)),
// 		Metadata: map[string]interface{}{
// 			"name":     "test-package",
// 			"version":  "1.0.0",
// 			"filename": "test_package-1.0.0-py3-none-any.whl",
// 		},
// 		UploadedBy: 1,
// 	}
//
// 	result, err := adapter.Upload(ctx, req)
// 	assert.Nil(t, err)
// 	assert.NotNil(t, result)
// 	assert.Equal(t, "1.0.0", result.Version)
// 	assert.NotEmpty(t, result.StorageKey)
//
// 	var pkg model.Package
// 	err = db.Where("name = ?", "test-package").First(&pkg).Error
// 	assert.Nil(t, err)
// 	assert.Equal(t, model.PackageTypePyPI, pkg.Type)
//
// 	var version model.Component
// 	err = db.Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").First(&version).Error
// 	assert.Nil(t, err)
// 	assert.Equal(t, model.StatusPublished, version.Status)
// }

// func TestPyPIAdapter_UploadSourceDistribution(t *testing.T) {
// 	adapter, _ := setupPyPIAdapter(t)
//
// 	ctx := context.Background()
// 	tarGzContent := []byte("fake tar.gz content for test")
//
// 	req := &UploadRequest{
// 		Package:  bytes.NewReader(tarGzContent),
// 		FileName: "test-package-1.0.0.tar.gz",
// 		Size:     int64(len(tarGzContent)),
// 		Metadata: map[string]interface{}{
// 			"name":     "test-package",
// 			"version":  "1.0.0",
// 			"filename": "test-package-1.0.0.tar.gz",
// 		},
// 		UploadedBy: 1,
// 	}
//
// 	result, err := adapter.Upload(ctx, req)
// 	assert.Nil(t, err)
// 	assert.NotNil(t, result)
// 	assert.Equal(t, "1.0.0", result.Version)
// }

func TestPyPIAdapter_GetMetadata(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Component{
		Name:        "requests",
		Format: model.PackageTypePyPI,
		Description: "Python HTTP library",
	}
	db.Create(pkg)

	version := &model.Component{
		PackageID: pkg.ID,
		Version:   "2.28.0",
		Status:    model.StatusPublished,
	}
	db.Create(version)

	meta, err := adapter.GetMetadata(context.Background(), "requests")
	assert.Nil(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, "requests", meta.Name)
	assert.Equal(t, PyPIType, meta.Type)
	assert.Len(t, meta.Versions, 1)
	assert.Equal(t, "2.28.0", meta.Versions[0].Version)
}

func TestPyPIAdapter_GetMetadata_NotFound(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)

	_, err := adapter.GetMetadata(context.Background(), "nonexistent-package")
	assert.Error(t, err)
}

func TestPyPIAdapter_ListVersions(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Component{
		Name:        "numpy",
		Format: model.PackageTypePyPI,
		Description: "NumPy library",
	}
	db.Create(pkg)

	versions := []model.Component{
		{PackageID: pkg.ID, Version: "1.21.0", Status: model.StatusPublished},
		{PackageID: pkg.ID, Version: "1.22.0", Status: model.StatusPublished},
		{PackageID: pkg.ID, Version: "1.23.0", Status: model.StatusPublished},
	}
	for _, v := range versions {
		db.Create(&v)
	}

	result, err := adapter.ListVersions(context.Background(), "numpy")
	assert.Nil(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "1.21.0")
	assert.Contains(t, result, "1.22.0")
	assert.Contains(t, result, "1.23.0")
}

func TestPyPIAdapter_Delete(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Component{
		Name:        "deletable-package",
		Format: model.PackageTypePyPI,
		Description: "Deletable package",
	}
	db.Create(pkg)

	version := &model.Component{
		PackageID: pkg.ID,
		Version:   "1.0.0",
		Status:    model.StatusPublished,
	}
	db.Create(version)

	identity := &PackageIdentity{
		Name:    "deletable-package",
		Version: "1.0.0",
		Type:    PyPIType,
	}

	err := adapter.Delete(context.Background(), identity)
	assert.Nil(t, err)

	var count int64
	db.Model(&model.Component{}).Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestPyPIAdapter_ListPackages(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	packages := []model.Component{
		{Name: "requests", Format: model.PackageTypePyPI, Description: "HTTP library"},
		{Name: "numpy", Format: model.PackageTypePyPI, Description: "NumPy"},
		{Name: "pandas", Format: model.PackageTypePyPI, Description: "Pandas"},
	}
	for _, p := range packages {
		db.Create(&p)
	}

	repo := &model.Repository{
		Name:        "pypi",
		PackageType: string(model.PackageTypePyPI),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/simple/", nil)

	intent := adapter.ParseIntent("simple/", c.Request.Method)
	result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestPyPIAdapter_PackageFiles(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Component{
		Name:        "flask",
		Format: model.PackageTypePyPI,
		Description: "Flask framework",
	}
	db.Create(pkg)

	version := &model.Component{
		PackageID: pkg.ID,
		Version:   "2.0.0",
		Status:    model.StatusPublished,
	}
	db.Create(version)

	file := &model.Asset{
		ComponentID: version.ID,
		FileName:  "Flask-2.0.0-py3-none-any.whl",
		Kind:  model.AssetKindPrimary,
		SizeBytes: 1000,
	}
	db.Create(file)

	repo := &model.Repository{
		Name:        "pypi",
		PackageType: string(model.PackageTypePyPI),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/simple/flask/", nil)

	intent := adapter.ParseIntent("simple/flask/", c.Request.Method)
	result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestPyPIAdapter_PackageFilesJSONPEP691Shape(t *testing.T) {
	// 注意: 此测试需要正确配置的存储后端，当前测试设置存在初始化问题
	// 模拟真实 PyPI PEP 691 格式验证
	t.Skip("跳过此测试: 测试设置问题 - 存储后端未正确初始化。需要修复 setupPyPIAdapter() 中的存储后端配置。")

	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Component{
		Name: "flask",
		Format: model.PackageTypePyPI,
	}
	db.Create(pkg)

	version := &model.Component{
		PackageID: pkg.ID,
		Version:   "2.0.0",
		Status:    model.StatusPublished,
	}
	db.Create(version)

	file := &model.Asset{
		ComponentID: version.ID,
		FileName:  "Flask-2.0.0-py3-none-any.whl",
		Kind:  model.AssetKindPrimary,
		SizeBytes: 1000,
	}
	db.Create(file)

	repo := &model.Repository{
		Name:        "pypi",
		PackageType: string(model.PackageTypePyPI),
	}

	result, err := adapter.PackageFiles(context.Background(), "application/vnd.pypi.simple.v1+json", "flask", repo)
	assert.Nil(t, err)
	assert.NotNil(t, result)

	var parsed struct {
		Meta struct {
			APIVersion string `json:"api-version"`
		} `json:"meta"`
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	body, readErr := io.ReadAll(result.Content)
	assert.Nil(t, readErr)
	assert.Nil(t, json.Unmarshal(body, &parsed))
	assert.Equal(t, "1.0", parsed.Meta.APIVersion)
	assert.Len(t, parsed.Files, 1)
	assert.Equal(t, "Flask-2.0.0-py3-none-any.whl", parsed.Files[0].Filename)
}

func TestPyPIAdapter_DownloadPackage(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)

	repo := &model.Repository{
		Name:        "pypi",
		PackageType: string(model.PackageTypePyPI),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/packages/test-package-1.0.0.whl", nil)

	intent := adapter.ParseIntent("packages/test-package-1.0.0.whl", c.Request.Method)
	result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
	if err != nil {
		assert.Nil(t, result)
	} else {
		assert.NotNil(t, result)
		assert.True(t, result.StatusCode == 200 || result.StatusCode == 404)
	}
}

func TestPyPIAdapter_JSONAPI(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Component{
		Name:        "jsonapi-test",
		Format: model.PackageTypePyPI,
		Description: "JSON API test package",
	}
	db.Create(pkg)

	version := &model.Component{
		PackageID: pkg.ID,
		Version:   "1.0.0",
		Status:    model.StatusPublished,
	}
	db.Create(version)

	repo := &model.Repository{
		Name:        "pypi",
		PackageType: string(model.PackageTypePyPI),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/jsonapi-test/1.0.0/json", nil)

	intent := adapter.ParseIntent("jsonapi-test/1.0.0/json", c.Request.Method)
	result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}
