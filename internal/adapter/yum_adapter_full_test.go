package adapter

import (
	"context"
	"net/http/httptest"
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

func setupYumAdapter(t *testing.T) (*YumAdapter, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
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
	repoRepo := repository.NewRepositoryRepository(db)

	storageSvc, err := service.NewStorageService(storageBackendRepo, "", 0)
	if err != nil {
		t.Fatalf("failed to create storage service: %v", err)
	}

	backend := &model.StorageBackend{
		Name:      "test-local-yum",
		Type:      model.StorageTypeLocal,
		IsDefault: true,
		Status:    model.StatusActive,
		IsActive:  true,
		Config: model.StorageBackendConfig{
			Local: &model.LocalConfig{
				BasePath:  "/tmp/test-yum-storage",
				MaxSizeGB: 1,
			},
		},
	}
	db.Create(backend)

	storageSvc.RefreshBackends()

	auditSvc := service.NewAuditService()

	pkgCache := cache.NewPackageCache(pkgRepo, 5*time.Minute)

	adapter := NewYumAdapter(repoRepo, storageSvc, auditSvc, pkgCache)
	return adapter, db
}

func TestYumAdapter_Type(t *testing.T) {
	adapter, _ := setupYumAdapter(t)
	assert.Equal(t, YumType, adapter.Type())
}


func TestYumAdapter_ParsePath(t *testing.T) {
	adapter, _ := setupYumAdapter(t)

	tests := []struct {
		name            string
		path            string
		expectedName    string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "valid path with version",
			path:            "nginx/1.20.1",
			expectedName:    "nginx",
			expectedVersion: "1.20.1",
			expectError:     false,
		},
		{
			name:            "valid path without version",
			path:            "nginx",
			expectedName:    "nginx",
			expectedVersion: "",
			expectError:     false,
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

// Upload 方法已移除，上传现在由 RepoRouter 统一处理
// func TestYumAdapter_UploadRPM(t *testing.T) {
// 	adapter, db := setupYumAdapter(t)
//
// 	ctx := context.Background()
// 	rpmContent := []byte("fake rpm content for test")
//
// 	req := &UploadRequest{
// 		Package:  bytes.NewReader(rpmContent),
// 		Filename: "nginx-1.20.1-1.el9.x86_64.rpm",
// 		Size:     int64(len(rpmContent)),
// 		Metadata: map[string]interface{}{
// 			"name":     "nginx-1.20.1-1.el9.x86_64.rpm",
// 			"repo":     "test-repo",
// 			"filename": "nginx-1.20.1-1.el9.x86_64.rpm",
// 		},
// 		UploadedBy: 1,
// 	}
//
// 	result, err := adapter.Upload(ctx, req)
// 	assert.Nil(t, err)
// 	assert.NotNil(t, result)
// 	assert.NotEmpty(t, result.StorageKey)
//
// 	var pkg model.Package
// 	err = db.Where("name = ?", "nginx-1.20.1-1.el9.x86_64.rpm").First(&pkg).Error
// 	assert.Nil(t, err)
// 	assert.Equal(t, model.PackageTypeYum, pkg.Type)
// }

func TestYumAdapter_GetMetadata(t *testing.T) {
	adapter, db := setupYumAdapter(t)

	pkg := &model.Package{
		Name:        "nginx",
		Type:        model.PackageTypeYum,
		Description: "Nginx web server",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "1.20.1",
		Status:      model.StatusPublished,
		StoragePath: "yum/nginx/1.20.1",
	}
	db.Create(version)

	meta, err := adapter.GetMetadata(context.Background(), "nginx")
	assert.Nil(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, "nginx", meta.Name)
	assert.Equal(t, YumType, meta.Type)
	assert.Len(t, meta.Versions, 1)
	assert.Equal(t, "1.20.1", meta.Versions[0].Version)
}

func TestYumAdapter_GetMetadata_NotFound(t *testing.T) {
	adapter, _ := setupYumAdapter(t)

	_, err := adapter.GetMetadata(context.Background(), "nonexistent-package")
	assert.Error(t, err)
}

func TestYumAdapter_ListVersions(t *testing.T) {
	adapter, db := setupYumAdapter(t)

	pkg := &model.Package{
		Name:        "httpd",
		Type:        model.PackageTypeYum,
		Description: "Apache HTTP Server",
	}
	db.Create(pkg)

	versions := []model.PackageVersion{
		{PackageID: pkg.ID, Version: "2.4.51", Status: model.StatusPublished, StoragePath: "path1"},
		{PackageID: pkg.ID, Version: "2.4.52", Status: model.StatusPublished, StoragePath: "path2"},
		{PackageID: pkg.ID, Version: "2.4.53", Status: model.StatusPublished, StoragePath: "path3"},
	}
	for _, v := range versions {
		db.Create(&v)
	}

	result, err := adapter.ListVersions(context.Background(), "httpd")
	assert.Nil(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "2.4.51")
	assert.Contains(t, result, "2.4.52")
	assert.Contains(t, result, "2.4.53")
}

func TestYumAdapter_Delete(t *testing.T) {
	adapter, db := setupYumAdapter(t)

	pkg := &model.Package{
		Name:        "deletable-pkg",
		Type:        model.PackageTypeYum,
		Description: "Deletable package",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "yum/deletable-pkg/1.0.0",
	}
	db.Create(version)

	identity := &PackageIdentity{
		Name:    "deletable-pkg",
		Version: "1.0.0",
		Type:    YumType,
	}

	err := adapter.Delete(context.Background(), identity)
	assert.Nil(t, err)

	var count int64
	db.Model(&model.PackageVersion{}).Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestYumAdapter_DownloadRPM(t *testing.T) {
	adapter, _ := setupYumAdapter(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/yum/test-repo/Packages/nginx-1.20.1-1.el9.x86_64.rpm", nil)
	c.Params = gin.Params{
		{Key: "repo", Value: "test-repo"},
		{Key: "path", Value: "/nginx-1.20.1-1.el9.x86_64.rpm"},
	}

	adapter.DownloadRPM(c)

	assert.Equal(t, 404, w.Code)
}

func TestYumAdapter_RepoDataFile(t *testing.T) {
	adapter, _ := setupYumAdapter(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/yum/test-repo/repodata/repomd.xml", nil)
	c.Params = gin.Params{
		{Key: "repo", Value: "test-repo"},
		{Key: "path", Value: "/repomd.xml"},
	}

	adapter.RepoDataFile(c)

	assert.Equal(t, 404, w.Code)
}

func TestYumAdapter_GenerateRepomdXML(t *testing.T) {
	adapter, _ := setupYumAdapter(t)

	ctx := context.Background()
	xml, err := adapter.generateRepomdXML(ctx, "test-repo")
	assert.Nil(t, err)
	assert.NotEmpty(t, xml)
	assert.Contains(t, xml, "<?xml")
	assert.Contains(t, xml, "repomd")
}

func TestParseRpmFilename_Extended(t *testing.T) {
	tests := []struct {
		name            string
		filename        string
		expectedName    string
		expectedVersion string
		expectedRelease string
		expectedArch    string
	}{
		{
			name:            "standard rpm",
			filename:        "nginx-1.20.1-1.el9.x86_64.rpm",
			expectedName:    "nginx",
			expectedVersion: "1.20.1",
			expectedRelease: "1.el9",
			expectedArch:    "x86_64",
		},
		{
			name:            "complex name",
			filename:        "python3-pip-21.2.3-5.el9.noarch.rpm",
			expectedName:    "python3-pip",
			expectedVersion: "21.2.3",
			expectedRelease: "5.el9",
			expectedArch:    "noarch",
		},
		{
			name:            "aarch64 architecture",
			filename:        "kernel-5.14.0-284.el9.aarch64.rpm",
			expectedName:    "kernel",
			expectedVersion: "5.14.0",
			expectedRelease: "284.el9",
			expectedArch:    "aarch64",
		},
		{
			name:            "multi-part name",
			filename:        "java-11-openjdk-headless-11.0.20.0.8-2.el9.x86_64.rpm",
			expectedName:    "java-11-openjdk-headless",
			expectedVersion: "11.0.20.0.8",
			expectedRelease: "2.el9",
			expectedArch:    "x86_64",
		},
		{
			name:            "i686 architecture",
			filename:        "glibc-2.34-60.el9.i686.rpm",
			expectedName:    "glibc",
			expectedVersion: "2.34",
			expectedRelease: "60.el9",
			expectedArch:    "i686",
		},
		{
			name:            "armv7hl architecture",
			filename:        "systemd-250-12.el9.armv7hl.rpm",
			expectedName:    "systemd",
			expectedVersion: "250",
			expectedRelease: "12.el9",
			expectedArch:    "armv7hl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, version, release, arch := parseRpmFilename(tt.filename)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedVersion, version)
			assert.Equal(t, tt.expectedRelease, release)
			assert.Equal(t, tt.expectedArch, arch)
		})
	}
}

func TestDetectRpmArch_Extended(t *testing.T) {
	tests := []struct {
		filename     string
		expectedArch string
	}{
		{"package-1.0.0-1.x86_64.rpm", "x86_64"},
		{"package-1.0.0-1.aarch64.rpm", "aarch64"},
		{"package-1.0.0-1.noarch.rpm", "noarch"},
		{"package-1.0.0-1.i686.rpm", "i686"},
		{"package-1.0.0-1.armv7hl.rpm", "armv7hl"},
		{"package-1.0.0-1.unknown.rpm", "x86_64"},
		{"package-1.0.0-1.ppc64le.rpm", "x86_64"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			arch := detectRpmArch(tt.filename)
			assert.Equal(t, tt.expectedArch, arch)
		})
	}
}

func TestUnmarshalMetadata(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected map[string]interface{}
	}{
		{
			name:     "empty string",
			data:     "",
			expected: nil,
		},
		{
			name:     "valid json",
			data:     `{"repo": "test-repo", "arch": "x86_64"}`,
			expected: map[string]interface{}{"repo": "test-repo", "arch": "x86_64"},
		},
		{
			name:     "invalid json",
			data:     "invalid json",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unmarshalMetadata(tt.data)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				for k, v := range tt.expected {
					assert.Equal(t, v, result[k])
				}
			}
		})
	}
}
