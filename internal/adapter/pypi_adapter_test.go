package adapter

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupPyPIAdapter(t *testing.T) (*PyPIAdapter, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(gormlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	db.AutoMigrate(
		&model.Package{},
		&model.PackageVersion{},
		&model.PackageFile{},
		&model.Repository{},
		&model.StorageBackend{},
	)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	pkgRepo := repository.NewPackageRepository(db)

	storageSvc, err := service.NewStorageService(storageBackendRepo, "", 0)
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

	auditSvc := service.NewAuditService()
	repoRepo := repository.NewRepositoryRepository(db)

	adapter := NewPyPIAdapter(pkgRepo, repoRepo, storageSvc, auditSvc)
	return adapter, db
}

func TestPyPIAdapter_Type(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)
	assert.Equal(t, PyPIType, adapter.Type())
}

func TestPyPIAdapter_RoutePrefix(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)
	assert.Equal(t, "/pypi", adapter.RoutePrefix())
}

func TestPyPIAdapter_ParsePackagePath(t *testing.T) {
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
			name:        "invalid path - only package name",
			path:        "numpy",
			expectError: true,
		},
		{
			name:        "invalid path - empty",
			path:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := adapter.ParsePackagePath(tt.path)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedName, identity.Name)
				assert.Equal(t, tt.expectedVersion, identity.Version)
				assert.Equal(t, PyPIType, identity.Type)
			}
		})
	}
}

func TestPyPIAdapter_UploadPackage(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	ctx := context.Background()
	wheelContent := []byte("fake wheel content for test")

	req := &UploadRequest{
		Package:  bytes.NewReader(wheelContent),
		Filename: "test_package-1.0.0-py3-none-any.whl",
		Size:     int64(len(wheelContent)),
		Metadata: map[string]interface{}{
			"name":     "test-package",
			"version":  "1.0.0",
			"filename": "test_package-1.0.0-py3-none-any.whl",
		},
		UploadedBy: 1,
	}

	result, err := adapter.Upload(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "1.0.0", result.Version)
	assert.NotEmpty(t, result.StorageKey)

	var pkg model.Package
	err = db.Where("name = ?", "test-package").First(&pkg).Error
	assert.Nil(t, err)
	assert.Equal(t, model.PackageTypePyPI, pkg.Type)

	var version model.PackageVersion
	err = db.Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").First(&version).Error
	assert.Nil(t, err)
	assert.Equal(t, model.StatusPublished, version.Status)
}

func TestPyPIAdapter_UploadSourceDistribution(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)

	ctx := context.Background()
	tarGzContent := []byte("fake tar.gz content for test")

	req := &UploadRequest{
		Package:  bytes.NewReader(tarGzContent),
		Filename: "test-package-1.0.0.tar.gz",
		Size:     int64(len(tarGzContent)),
		Metadata: map[string]interface{}{
			"name":     "test-package",
			"version":  "1.0.0",
			"filename": "test-package-1.0.0.tar.gz",
		},
		UploadedBy: 1,
	}

	result, err := adapter.Upload(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "1.0.0", result.Version)
}

func TestPyPIAdapter_GetMetadata(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Package{
		Name:        "requests",
		Type:        model.PackageTypePyPI,
		Description: "Python HTTP library",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "2.28.0",
		Status:      model.StatusPublished,
		StoragePath: "pypi/requests/2.28.0",
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

	pkg := &model.Package{
		Name:        "numpy",
		Type:        model.PackageTypePyPI,
		Description: "NumPy library",
	}
	db.Create(pkg)

	versions := []model.PackageVersion{
		{PackageID: pkg.ID, Version: "1.21.0", Status: model.StatusPublished, StoragePath: "path1"},
		{PackageID: pkg.ID, Version: "1.22.0", Status: model.StatusPublished, StoragePath: "path2"},
		{PackageID: pkg.ID, Version: "1.23.0", Status: model.StatusPublished, StoragePath: "path3"},
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

	pkg := &model.Package{
		Name:        "deletable-package",
		Type:        model.PackageTypePyPI,
		Description: "Deletable package",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "pypi/deletable-package/1.0.0",
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
	db.Model(&model.PackageVersion{}).Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestPyPIAdapter_ListPackages(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	packages := []model.Package{
		{Name: "requests", Type: model.PackageTypePyPI, Description: "HTTP library"},
		{Name: "numpy", Type: model.PackageTypePyPI, Description: "NumPy"},
		{Name: "pandas", Type: model.PackageTypePyPI, Description: "Pandas"},
	}
	for _, p := range packages {
		db.Create(&p)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/simple/", nil)

	adapter.ListPackages(c)

	assert.Equal(t, 200, w.Code)
}

func TestPyPIAdapter_PackageFiles(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Package{
		Name:        "flask",
		Type:        model.PackageTypePyPI,
		Description: "Flask framework",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "2.0.0",
		Status:      model.StatusPublished,
		StoragePath: "pypi/flask/2.0.0",
	}
	db.Create(version)

	file := &model.PackageFile{
		VersionID:   version.ID,
		Filename:    "Flask-2.0.0-py3-none-any.whl",
		FileType:    model.FileTypePrimary,
		StoragePath: "pypi/flask/2.0.0/Flask-2.0.0-py3-none-any.whl",
		SizeBytes:   1000,
	}
	db.Create(file)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/simple/flask/", nil)
	c.Params = gin.Params{
		{Key: "package", Value: "flask"},
	}

	adapter.PackageFiles(c)

	assert.Equal(t, 200, w.Code)
}

func TestPyPIAdapter_DownloadPackage(t *testing.T) {
	adapter, _ := setupPyPIAdapter(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/packages/test-package-1.0.0.whl", nil)
	c.Params = gin.Params{
		{Key: "filename", Value: "test-package-1.0.0.whl"},
	}

	adapter.DownloadPackage(c)

	assert.True(t, w.Code == 200 || w.Code == 404)
}

func TestPyPIAdapter_JSONAPI(t *testing.T) {
	adapter, db := setupPyPIAdapter(t)

	pkg := &model.Package{
		Name:        "jsonapi-test",
		Type:        model.PackageTypePyPI,
		Description: "JSON API test package",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "pypi/jsonapi-test/1.0.0",
	}
	db.Create(version)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/pypi/jsonapi-test/1.0.0/json", nil)
	c.Params = gin.Params{
		{Key: "package", Value: "jsonapi-test"},
		{Key: "version", Value: "1.0.0"},
	}

	adapter.JSONAPI(c)

	assert.Equal(t, 200, w.Code)
}
