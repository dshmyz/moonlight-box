package adapter

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/moonlight-box/registry/internal/cache"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
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
	return db
}

func setupNpmAdapter(t *testing.T) (*NpmAdapter, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	pkgRepo := repository.NewPackageRepository(db)

	testDir, err := os.MkdirTemp("", "npm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(testDir) })

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

	adapter := NewNpmAdapter(storageSvc, pkgCache)
	return adapter, db
}

func TestNpmAdapter_Type(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)
	assert.Equal(t, NpmType, adapter.Type())
}

func TestNpmAdapter_ParsePath_Scoped(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("@scope/package")
	assert.Nil(t, err)
	assert.Equal(t, "@scope/package", pathInfo.Name)
}

func TestNpmAdapter_ParsePath_ScopedWithVersion(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("@scope/package/1.0.0")
	assert.Nil(t, err)
	assert.Equal(t, "@scope/package", pathInfo.Name)
	assert.Equal(t, "1.0.0", pathInfo.Version)
}

func TestNpmAdapter_ParseIntent_ScopedDistTags(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	intent := adapter.ParseIntent("-/package/@scope/package/dist-tags", "GET")

	assert.Equal(t, types.RequestDistTags, intent.Type)
	assert.Equal(t, "@scope/package", intent.Name)
}

func TestNpmAdapter_ParseIntent_ScopedDistTagUpdate(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	intent := adapter.ParseIntent("-/package/@scope/package/dist-tags/latest", "PUT")

	assert.Equal(t, types.RequestDistTagUpdate, intent.Type)
	assert.Equal(t, "@scope/package", intent.Name)
	assert.Equal(t, "latest", intent.Extra["tag"])
}

func TestNpmAdapter_ParsePath_NonScoped(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("express")
	assert.Nil(t, err)
	assert.Equal(t, "express", pathInfo.Name)
	assert.Empty(t, pathInfo.Version)
}

func TestNpmAdapter_ParsePath_NonScopedWithVersion(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("express/4.17.1")
	assert.Nil(t, err)
	assert.Equal(t, "express", pathInfo.Name)
	assert.Equal(t, "4.17.1", pathInfo.Version)
}

func TestNpmAdapter_ParsePath_Tarball_Unscoped(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("express/-/express-4.17.1.tgz")
	assert.Nil(t, err)
	assert.Equal(t, "express", pathInfo.Name)
	assert.Equal(t, "4.17.1", pathInfo.Version)
	assert.Equal(t, "express-4.17.1.tgz", pathInfo.Filename)
	assert.Equal(t, "express/express-4.17.1.tgz", pathInfo.StorageName)
	assert.Equal(t, "express/-/express-4.17.1.tgz", pathInfo.RemotePath)
}

func TestNpmAdapter_ParsePath_Tarball_Scoped(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("@scope/package/-/package-1.0.0.tgz")
	assert.Nil(t, err)
	assert.Equal(t, "@scope/package", pathInfo.Name)
	assert.Equal(t, "1.0.0", pathInfo.Version)
	assert.Equal(t, "package-1.0.0.tgz", pathInfo.Filename)
	assert.Equal(t, "@scope/package/package-1.0.0.tgz", pathInfo.StorageName)
	assert.Equal(t, "@scope/package/-/package-1.0.0.tgz", pathInfo.RemotePath)
}

func TestNpmAdapter_ParsePath_Tarball_WithoutDash(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("express/express-4.17.1.tgz")
	assert.Nil(t, err)
	assert.Equal(t, "express", pathInfo.Name)
	assert.Equal(t, "4.17.1", pathInfo.Version)
	assert.Equal(t, "express-4.17.1.tgz", pathInfo.Filename)
	assert.Equal(t, "express/express-4.17.1.tgz", pathInfo.StorageName)
	assert.Equal(t, "express/express-4.17.1.tgz", pathInfo.RemotePath)
}

func TestNpmAdapter_ParsePath_Tarball_PrereleaseVersion(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("lodash/-/lodash-1.0.0-beta.1.tgz")
	assert.Nil(t, err)
	assert.Equal(t, "lodash", pathInfo.Name)
	assert.Equal(t, "1.0.0-beta.1", pathInfo.Version)
	assert.Equal(t, "lodash-1.0.0-beta.1.tgz", pathInfo.Filename)
	assert.Equal(t, "lodash/lodash-1.0.0-beta.1.tgz", pathInfo.StorageName)
	assert.Equal(t, "lodash/-/lodash-1.0.0-beta.1.tgz", pathInfo.RemotePath)
}

func TestNpmAdapter_ParsePath_Tarball_ScopedPrereleaseVersion(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("@scope/package/-/package-2.0.0-alpha+sha.digest.tgz")
	assert.Nil(t, err)
	assert.Equal(t, "@scope/package", pathInfo.Name)
	assert.Equal(t, "2.0.0-alpha+sha.digest", pathInfo.Version)
	assert.Equal(t, "package-2.0.0-alpha+sha.digest.tgz", pathInfo.Filename)
	assert.Equal(t, "@scope/package/package-2.0.0-alpha+sha.digest.tgz", pathInfo.StorageName)
	assert.Equal(t, "@scope/package/-/package-2.0.0-alpha+sha.digest.tgz", pathInfo.RemotePath)
}

func TestNpmAdapter_ParsePath_Tarball_PackageNameWithDashes(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	pathInfo, err := adapter.ParsePath("my-package/-/my-package-3.0.0.tgz")
	assert.Nil(t, err)
	assert.Equal(t, "my-package", pathInfo.Name)
	assert.Equal(t, "3.0.0", pathInfo.Version)
	assert.Equal(t, "my-package-3.0.0.tgz", pathInfo.Filename)
	assert.Equal(t, "my-package/my-package-3.0.0.tgz", pathInfo.StorageName)
	assert.Equal(t, "my-package/-/my-package-3.0.0.tgz", pathInfo.RemotePath)
}

func TestNpmAdapter_ParsePath_Tarball_MultipleVersions(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	tests := []struct {
		path    string
		name    string
		version string
	}{
		{"express/-/express-4.17.1.tgz", "express", "4.17.1"},
		{"express/-/express-4.0.0.tgz", "express", "4.0.0"},
		{"express/-/express-3.21.2.tgz", "express", "3.21.2"},
		{"react/-/react-18.2.0.tgz", "react", "18.2.0"},
		{"react/-/react-16.8.6.tgz", "react", "16.8.6"},
		{"@babel/core/-/core-7.24.0.tgz", "@babel/core", "7.24.0"},
		{"@babel/core/-/core-7.23.9.tgz", "@babel/core", "7.23.9"},
		{"typescript/-/typescript-5.4.0.tgz", "typescript", "5.4.0"},
		{"lodash/-/lodash-4.17.21.tgz", "lodash", "4.17.21"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			pathInfo, err := adapter.ParsePath(tt.path)
			assert.Nil(t, err)
			assert.Equal(t, tt.name, pathInfo.Name)
			assert.Equal(t, tt.version, pathInfo.Version)
			assert.Equal(t, tt.path, pathInfo.RemotePath)
		})
	}
}

func TestExtractVersionFromTarball(t *testing.T) {
	tests := []struct {
		filename string
		pkgName  string
		want     string
	}{
		{"express-4.17.1.tgz", "express", "4.17.1"},
		{"lodash-4.17.21.tgz", "lodash", "4.17.21"},
		{"my-package-1.0.0.tgz", "my-package", "1.0.0"},
		{"lodash-1.0.0-beta.1.tgz", "lodash", "1.0.0-beta.1"},
		{"package-2.0.0-alpha+sha.digest.tgz", "package", "2.0.0-alpha+sha.digest"},
		{"package-1.0.0.tgz", "@scope/package", "1.0.0"},
		{"core-7.24.0.tgz", "@babel/core", "7.24.0"},
		{"1.0.0.tgz", "unknown", "1.0.0"},
		{"no-version-pattern.tgz", "something", "no-version-pattern"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractVersionFromTarball(tt.filename, tt.pkgName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateRevision(t *testing.T) {
	rev1 := generateRevision()
	rev2 := generateRevision()
	assert.NotEmpty(t, rev1)
	assert.NotEmpty(t, rev2)
}

func TestGetDescription(t *testing.T) {
	t.Skip("getDescription function not available")
	/*
		tests := []struct {
			name     string
			meta     map[string]interface{}
			expected string
		}{
			{
				name:     "with description",
				meta:     map[string]interface{}{"description": "test package"},
				expected: "test package",
			},
			{
				name:     "without description",
				meta:     map[string]interface{}{"name": "test"},
				expected: "",
			},
			{
				name:     "empty description",
				meta:     map[string]interface{}{"description": ""},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getDescription(tt.meta)
				assert.Equal(t, tt.expected, result)
			})
		}
	*/
}

func TestMarshalMetadata(t *testing.T) {
	meta := map[string]interface{}{
		"name":        "test",
		"version":     "1.0.0",
		"description": "test package",
	}

	result := marshalMetadata(meta)
	assert.NotEmpty(t, result)

	var decoded map[string]interface{}
	err := json.Unmarshal([]byte(result), &decoded)
	assert.Nil(t, err)
	assert.Equal(t, "test", decoded["name"])
	assert.Equal(t, "1.0.0", decoded["version"])
}

func TestNpmAdapter_GetVersion_NotFound(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	_, err := adapter.GetMetadata(context.Background(), "lodash")

	assert.Error(t, err)
	assert.True(t, util.IsErr(err, util.ErrPackageNotFound))
}

func TestNpmAdapter_ListVersions(t *testing.T) {
	adapter, db := setupNpmAdapter(t)

	pkg := &model.Package{
		Name:        "test-pkg",
		Type:        model.PackageTypeNPM,
		Description: "Test package",
	}
	db.Create(pkg)

	versions, err := adapter.ListVersions(context.Background(), "test-pkg")
	assert.Nil(t, err)
	assert.Empty(t, versions)
}

func TestNpmAdapter_GetMetadata_NotFound(t *testing.T) {
	adapter, _ := setupNpmAdapter(t)

	ctx := context.Background()
	_, err := adapter.GetMetadata(ctx, "nonexistent")
	assert.NotNil(t, err)
	assert.True(t, util.IsErr(err, util.ErrPackageNotFound))
}

func TestNpmAdapter_Delete(t *testing.T) {
	adapter, db := setupNpmAdapter(t)

	pkg := &model.Package{
		Name:        "delete-test",
		Type:        model.PackageTypeNPM,
		Description: "Delete test package",
	}
	db.Create(pkg)

	version := &model.PackageVersion{
		PackageID: pkg.ID,
		Version:   "1.0.0",
		Status:    model.StatusPublished,
		SizeBytes: 1000,
	}
	db.Create(version)

	identity := &PackageIdentity{
		Name:    "delete-test",
		Version: "1.0.0",
		Type:    NpmType,
	}

	err := adapter.Delete(context.Background(), identity)
	assert.Nil(t, err)
}
