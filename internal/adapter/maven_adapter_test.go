package adapter

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
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
		&model.PackageDependency{},
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

	pkgCache := cache.NewPackageCache(pkgRepo, 5*time.Minute)

	adapter := NewMavenAdapter(storageSvc, pkgCache)
	return adapter, db
}

func TestMavenAdapter_Type(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)
	assert.Equal(t, MavenType, adapter.Type())
}

func TestMavenAdapter_ParsePath(t *testing.T) {
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
			path:            "com/test/lib/1.0.0/lib-1.0.0.jar",
			expectedName:    "com.test:lib",
			expectedVersion: "1.0.0",
			expectError:     false,
		},
		{
			name:            "valid path with pom",
			path:            "org/springframework/core/5.3.0/core-5.3.0.pom",
			expectedName:    "org.springframework:core",
			expectedVersion: "5.3.0",
			expectError:     false,
		},
		{
			name:        "invalid path too short",
			path:        "com/test/my-lib",
			expectError: true,
		},
		{
			name:        "invalid path",
			path:        "invalid",
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

func TestMavenAdapter_GetMetadata(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	pkg := &model.Package{
		Name:        "com.test/my-lib",
		Type:        model.PackageTypeMaven,
		Description: "Test Maven package",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID: pkg.ID,
		Version:   "1.0.0",
		Status:    model.StatusPublished,
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
		{PackageID: pkg.ID, Version: "1.0.0", Status: model.StatusPublished},
		{PackageID: pkg.ID, Version: "1.1.0", Status: model.StatusPublished},
		{PackageID: pkg.ID, Version: "2.0.0", Status: model.StatusPublished},
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
		PackageID: pkg.ID,
		Version:   "1.0.0",
		Status:    model.StatusPublished,
	}
	db.Create(version)

	identity := &PackageIdentity{
		Name:    "com.test/deletable-lib",
		Version: "1.0.0",
		Type:    MavenType,
	}

	err := adapter.Delete(context.Background(), identity)
	assert.Nil(t, err)

	var count int64
	db.Model(&model.PackageVersion{}).Where("package_id = ? AND version = ?", pkg.ID, "1.0.0").Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestMavenAdapter_HandleMetadataXML(t *testing.T) {
	adapter, db := setupMavenAdapter(t)

	pkg := &model.Package{
		Name:        "com.test.metadata:metadata-lib",
		Type:        model.PackageTypeMaven,
		Description: "Metadata test library",
	}
	db.Create(pkg)

	versions := []model.PackageVersion{
		{PackageID: pkg.ID, Version: "1.0.0", Status: model.StatusPublished},
		{PackageID: pkg.ID, Version: "1.1.0", Status: model.StatusPublished},
		{PackageID: pkg.ID, Version: "2.0.0-SNAPSHOT", Status: model.StatusPublished},
	}
	for _, v := range versions {
		db.Create(&v)
	}

	repo := &model.Repository{
		Name:        "maven2",
		PackageType: string(model.PackageTypeMaven),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/maven2/com/test/metadata/metadata-lib/maven-metadata.xml", nil)

	intent := adapter.ParseIntent("com/test/metadata/metadata-lib/maven-metadata.xml", c.Request.Method)
	result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)

	// Verify XML content through ExtraData
	metadata, ok := result.ExtraData["xml_struct"].(*MavenMetadata)
	assert.True(t, ok)
	assert.Equal(t, "com.test.metadata", metadata.GroupID)
	assert.Equal(t, "metadata-lib", metadata.ArtifactID)
	assert.Equal(t, "2.0.0-SNAPSHOT", metadata.Versioning.Latest)
	assert.Equal(t, "1.1.0", metadata.Versioning.Release)
	assert.Len(t, metadata.Versioning.Versions.Version, 3)
}

func TestMavenAdapter_HandleDownloadArtifact(t *testing.T) {
	adapter, _ := setupMavenAdapter(t)

	repo := &model.Repository{
		Name:        "maven2",
		PackageType: string(model.PackageTypeMaven),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/maven2/com/test/download-lib/1.0.0/download-lib-1.0.0.jar", nil)

	intent := adapter.ParseIntent("com/test/download-lib/1.0.0/download-lib-1.0.0.jar", c.Request.Method)
	result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 404, result.StatusCode)
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
			repo := &model.Repository{
				Name:        "maven2",
				PackageType: string(model.PackageTypeMaven),
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/maven2/"+tt.path, nil)

			intent := adapter.ParseIntent(tt.path, c.Request.Method)
			result, err := adapter.HandleGet(c.Request.Context(), repo, intent)
			assert.Nil(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expected, result.StatusCode)
		})
	}
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

func TestParseMavenSnapshotFilenameWithClassifierAndDashedArtifact(t *testing.T) {
	artifact, ok := parseMavenSnapshotFilename("my-lib-core-1.2.3-20260514.101112-7-sources.jar", "my-lib-core", "1.2.3-SNAPSHOT")
	assert.True(t, ok)
	assert.Equal(t, "jar", artifact.extension)
	assert.Equal(t, "sources", artifact.classifier)
	assert.Equal(t, "1.2.3-20260514.101112-7", artifact.value)
	assert.Equal(t, "20260514101112", artifact.updated)
	assert.Equal(t, "20260514.101112", artifact.timestamp)
	assert.Equal(t, 7, artifact.buildNumber)
}

func TestBuildSnapshotVersionsFromEntriesKeepsLatestPerClassifier(t *testing.T) {
	entries := []storage.Entry{
		{Key: "maven/repo/com/acme/my-lib-core/1.2.3-SNAPSHOT/my-lib-core-1.2.3-20260514.101112-7.jar"},
		{Key: "maven/repo/com/acme/my-lib-core/1.2.3-SNAPSHOT/my-lib-core-1.2.3-20260514.101112-7-sources.jar"},
		{Key: "maven/repo/com/acme/my-lib-core/1.2.3-SNAPSHOT/my-lib-core-1.2.3-20260514.101112-7-javadoc.jar"},
		{Key: "maven/repo/com/acme/my-lib-core/1.2.3-SNAPSHOT/my-lib-core-1.2.3-20260514.101112-7.pom"},
		{Key: "maven/repo/com/acme/my-lib-core/1.2.3-SNAPSHOT/my-lib-core-1.2.3-20260514.111213-8.jar"},
		{Key: "maven/repo/com/acme/my-lib-core/1.2.3-SNAPSHOT/my-lib-core-1.2.3-20260514.111213-8-sources.jar"},
	}

	versions, latestTimestamp, latestBuildNumber := buildSnapshotVersionsFromEntries(entries, "my-lib-core", "1.2.3-SNAPSHOT")

	assert.Equal(t, "20260514.111213", latestTimestamp)
	assert.Equal(t, 8, latestBuildNumber)
	assert.Len(t, versions, 4)

	byKind := map[string]MavenSnapshotVersion{}
	for _, version := range versions {
		byKind[version.Extension+":"+version.Classifier] = version
	}

	assert.Equal(t, "1.2.3-20260514.111213-8", byKind["jar:"].Value)
	assert.Equal(t, "1.2.3-20260514.111213-8", byKind["jar:sources"].Value)
	assert.Equal(t, "1.2.3-20260514.101112-7", byKind["jar:javadoc"].Value)
	assert.Equal(t, "1.2.3-20260514.101112-7", byKind["pom:"].Value)
	assert.Equal(t, "20260514111213", byKind["jar:"].Updated)
}
