package adapter

import (
	"bytes"
	"context"
	"encoding/xml"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMavenAdapter(t *testing.T) (*MavenAdapter, *gorm.DB) {
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

	storageSvc, err := service.NewStorageService(storageBackendRepo, "", 0)
	if err != nil {
		t.Fatalf("failed to create storage service: %v", err)
	}

	backend := &model.StorageBackend{
		Name:      "test-local-maven",
		Type:      model.StorageTypeLocal,
		IsDefault: true,
		Status:    model.StatusActive,
		IsActive:  true,
		Config: model.StorageBackendConfig{
			Local: &model.LocalConfig{
				BasePath:  "/tmp/test-maven-storage",
				MaxSizeGB: 1,
			},
		},
	}
	db.Create(backend)

	storageSvc.RefreshBackends()

	auditSvc := service.NewAuditService()

	adapter := NewMavenAdapter(pkgRepo, storageSvc, auditSvc, nil)
	return adapter, db
}

func TestMavenAdapter_Type(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)
	assert.Equal(t, MavenType, adapter.Type())
}

func TestMavenAdapter_RoutePrefix(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)
	assert.Equal(t, "/maven2", adapter.RoutePrefix())
}

func TestMavenAdapter_ParsePackagePath(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	tests := []struct {
		name            string
		path            string
		expectedName    string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "valid path with version",
			path:            "com/test/lib/1.0.0",
			expectedName:    "com/test/lib",
			expectedVersion: "1.0.0",
			expectError:     false,
		},
		{
			name:            "valid path with group and artifact only",
			path:            "com/test/my-lib",
			expectedName:    "com/test",
			expectedVersion: "my-lib",
			expectError:     false,
		},
		{
			name:        "invalid path",
			path:        "invalid",
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
				assert.Equal(t, MavenType, identity.Type)
			}
		})
	}
}

func TestMavenAdapter_Upload_ReleaseVersion(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	ctx := context.Background()
	jarContent := []byte("fake jar content for release")
	req := &UploadRequest{
		Package:  bytes.NewReader(jarContent),
		Filename: "my-lib-1.0.0.jar",
		Size:     int64(len(jarContent)),
		Metadata: map[string]interface{}{
			"groupId":    "com.test",
			"artifactId": "my-lib",
			"version":    "1.0.0",
			"packaging":  "jar",
		},
		UploadedBy: 1,
	}

	result, err := adapter.Upload(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "1.0.0", result.Version)
	assert.NotEmpty(t, result.StorageKey)

	var pkg model.Package
	err = db.Where("name = ?", "com.test/my-lib").First(&pkg).Error
	assert.Nil(t, err)
	assert.Equal(t, model.PackageTypeMaven, pkg.Type)

	var version model.PackageVersion
	err = db.Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").First(&version).Error
	assert.Nil(t, err)
	assert.Equal(t, model.StatusPublished, version.Status)
}

func TestMavenAdapter_Upload_SnapshotVersion(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	ctx := context.Background()
	jarContent := []byte("fake jar content for snapshot")
	req := &UploadRequest{
		Package:  bytes.NewReader(jarContent),
		Filename: "my-lib-1.0-SNAPSHOT.jar",
		Size:     int64(len(jarContent)),
		Metadata: map[string]interface{}{
			"groupId":    "com.test",
			"artifactId": "my-lib",
			"version":    "1.0-SNAPSHOT",
			"packaging":  "jar",
		},
		UploadedBy: 1,
	}

	result, err := adapter.Upload(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "1.0-SNAPSHOT", result.Version)

	var pkg model.Package
	err = db.Where("name = ?", "com.test/my-lib").First(&pkg).Error
	assert.Nil(t, err)

	var version model.PackageVersion
	err = db.Where("package_id = ? AND version = ?", pkg.ID, "1.0-SNAPSHOT").First(&version).Error
	assert.Nil(t, err)
	assert.Equal(t, model.StatusPublished, version.Status)
}

func TestMavenAdapter_Upload_PomFile(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	ctx := context.Background()
	pomContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.test</groupId>
  <artifactId>my-lib</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
</project>`)

	req := &UploadRequest{
		Package:  bytes.NewReader(pomContent),
		Filename: "my-lib-1.0.0.pom",
		Size:     int64(len(pomContent)),
		Metadata: map[string]interface{}{
			"groupId":    "com.test",
			"artifactId": "my-lib",
			"version":    "1.0.0",
			"packaging":  "pom",
		},
		UploadedBy: 1,
	}

	result, err := adapter.Upload(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "1.0.0", result.Version)
}

func TestMavenAdapter_GetMetadata(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	pkg := &model.Package{
		Name:        "com.test/my-lib",
		Type:        model.PackageTypeMaven,
		Description: "Test Maven package",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "maven/com.test/my-lib/1.0.0",
	}
	db.Create(version)

	meta, err := adapter.GetMetadata(context.Background(), "com.test/my-lib")
	assert.Nil(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, "com.test/my-lib", meta.Name)
	assert.Equal(t, MavenType, meta.Type)
	assert.Len(t, meta.Versions, 1)
	assert.Equal(t, "1.0.0", meta.Versions[0].Version)
}

func TestMavenAdapter_GetMetadata_NotFound(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	_, err := adapter.GetMetadata(context.Background(), "com.test/nonexistent")
	assert.Error(t, err)
}

func TestMavenAdapter_ListVersions(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	pkg := &model.Package{
		Name:        "com.test/versioned-lib",
		Type:        model.PackageTypeMaven,
		Description: "Versioned library",
	}
	db.Create(pkg)

	versions := []model.PackageVersion{
		{PackageID: pkg.ID, Version: "1.0.0", Status: model.StatusPublished, StoragePath: "path1"},
		{PackageID: pkg.ID, Version: "1.1.0", Status: model.StatusPublished, StoragePath: "path2"},
		{PackageID: pkg.ID, Version: "2.0.0", Status: model.StatusPublished, StoragePath: "path3"},
	}
	for _, v := range versions {
		db.Create(&v)
	}

	result, err := adapter.ListVersions(context.Background(), "com.test/versioned-lib")
	assert.Nil(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "1.0.0")
	assert.Contains(t, result, "1.1.0")
	assert.Contains(t, result, "2.0.0")
}

func TestMavenAdapter_Delete(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	pkg := &model.Package{
		Name:        "com.test/deletable-lib",
		Type:        model.PackageTypeMaven,
		Description: "Deletable library",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID:   pkg.ID,
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "maven/com.test/deletable-lib/1.0.0",
	}
	db.Create(version)

	identity := &PackageIdentity{
		Name:    "com.test/deletable-lib",
		Version: "1.0.0",
		Type:    MavenType,
	}

	err := adapter.Delete(context.Background(), identity)
	assert.NotNil(t, err)
}

func TestMavenAdapter_HandleMetadataXML(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	pkg := &model.Package{
		Name:        "com.test.metadata/metadata-lib",
		Type:        model.PackageTypeMaven,
		Description: "Metadata test library",
	}
	db.Create(pkg)

	versions := []model.PackageVersion{
		{PackageID: pkg.ID, Version: "1.0.0", Status: model.StatusPublished, StoragePath: "path1"},
		{PackageID: pkg.ID, Version: "1.1.0", Status: model.StatusPublished, StoragePath: "path2"},
		{PackageID: pkg.ID, Version: "2.0.0-SNAPSHOT", Status: model.StatusPublished, StoragePath: "path3"},
	}
	for _, v := range versions {
		db.Create(&v)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/maven2/com/test/metadata/metadata-lib/maven-metadata.xml", nil)

	adapter.handleMetadataXML(c, "com/test/metadata/metadata-lib/maven-metadata.xml")

	assert.Equal(t, 200, w.Code)

	var metadata MavenMetadata
	err := xml.Unmarshal(w.Body.Bytes(), &metadata)
	assert.Nil(t, err)
	assert.Equal(t, "com.test.metadata", metadata.GroupID)
	assert.Equal(t, "metadata-lib", metadata.ArtifactID)
	assert.Equal(t, "2.0.0-SNAPSHOT", metadata.Versioning.Latest)
	assert.Equal(t, "1.1.0", metadata.Versioning.Release)
	assert.Len(t, metadata.Versioning.Versions.Version, 3)
}

func TestMavenAdapter_HandleDownloadArtifact(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/maven2/com/test/download-lib/1.0.0/download-lib-1.0.0.jar", nil)

	adapter.handleDownloadArtifact(c, "com/test/download-lib/1.0.0/download-lib-1.0.0.jar")

	assert.Equal(t, 404, w.Code)
}

func TestMavenAdapter_HandleChecksumRequest(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			name:     "SHA1 checksum",
			path:     "com/test/lib/1.0.0/lib-1.0.0.jar.sha1",
			expected: 404,
		},
		{
			name:     "MD5 checksum",
			path:     "com/test/lib/1.0.0/lib-1.0.0.jar.md5",
			expected: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/maven2/"+tt.path, nil)

			adapter.handleChecksumRequest(c, tt.path)

			assert.Equal(t, tt.expected, w.Code)
		})
	}
}

func TestMavenAdapter_UploadArtifact(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	jarContent := []byte("fake jar content for upload artifact test")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/maven2/com/test/upload-lib/1.0.0/upload-lib-1.0.0.jar", bytes.NewReader(jarContent))
	c.Request.ContentLength = int64(len(jarContent))
	c.Params = gin.Params{
		{Key: "path", Value: "/com/test/upload-lib/1.0.0/upload-lib-1.0.0.jar"},
	}
	c.Set("userID", uint(1))

	adapter.UploadArtifact(c)

	assert.Equal(t, 200, w.Code)
}

func TestIsRelease(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"1.0.0", true},
		{"2.1.3", true},
		{"1.0.0-SNAPSHOT", false},
		{"2.0.0-SNAPSHOT", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := isRelease(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPackaging(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"lib-1.0.0.jar", "jar"},
		{"lib-1.0.0.pom", "pom"},
		{"lib-1.0.0-sources.jar", "jar"},
		{"lib-1.0.0-javadoc.jar", "jar"},
		{"lib-1.0.0.war", "jar"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := getPackaging(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetMavenFileType(t *testing.T) {
	tests := []struct {
		filename string
		expected model.PackageFileType
	}{
		{"lib-1.0.0.jar", model.FileTypePrimary},
		{"lib-1.0.0.pom", model.FileTypePom},
		{"lib-1.0.0-sources.jar", model.FileTypeSources},
		{"lib-1.0.0-javadoc.jar", model.FileTypeJavadoc},
		{"maven-metadata.xml", model.FileTypeMetadata},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := getMavenFileType(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateChecksum(t *testing.T) {
	data := []byte("test data for checksum")

	sha1Result := calculateChecksum(data, "sha1")
	assert.NotEmpty(t, sha1Result)
	assert.Len(t, sha1Result, 40)

	md5Result := calculateChecksum(data, "md5")
	assert.NotEmpty(t, md5Result)
	assert.Len(t, md5Result, 32)
}

func TestGenerateSnapshotTimestamp(t *testing.T) {
	timestamp, buildNumber := generateSnapshotTimestamp()
	assert.NotEmpty(t, timestamp)
	assert.Equal(t, 1, buildNumber)
	assert.Len(t, timestamp, 15)
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}
