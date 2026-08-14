package http

import (
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListVersionsSkipsMetadataOnlyArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now()
	versionOnly := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "version",
		Name:         "left-pad",
		Version:      "1.0.0",
		Attributes:   model.JSONB{"published_at": now.Format(time.RFC3339), "license": "MIT"},
	}
	tarball := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "tarball",
		Name:         "left-pad",
		Version:      "1.0.0",
		Path:         "left-pad/-",
		Filename:     "left-pad-1.0.0.tgz",
		RemotePath:   "left-pad/-/left-pad-1.0.0.tgz",
		Qualifiers:   model.JSONB{"package_type": "tarball"},
		Attributes:   model.JSONB{"default_visible": "true", "display_group": "current"},
	}
	legacyReleaseMetadata := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "release",
		Name:         "left-pad",
		Version:      "1.0.0",
		Filename:     "Release",
		RemotePath:   "left-pad/Release",
	}
	if err := db.Create(&versionOnly).Error; err != nil {
		t.Fatalf("create version artifact: %v", err)
	}
	if err := db.Create(&tarball).Error; err != nil {
		t.Fatalf("create tarball artifact: %v", err)
	}
	if err := db.Create(&legacyReleaseMetadata).Error; err != nil {
		t.Fatalf("create legacy release metadata: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.GET("/api/packages/:type/versions", handler.ListVersions)

	req := httptest.NewRequest(stdhttp.MethodGet, "/api/packages/npm/versions?name=left-pad&repository_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Versions []struct {
				License   string `json:"license"`
				FileCount int    `json:"file_count"`
			} `json:"versions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(resp.Data.Versions))
	}
	version := resp.Data.Versions[0]
	if version.License != "MIT" {
		t.Fatalf("license = %q", version.License)
	}
	if version.FileCount != 1 {
		t.Fatalf("file_count = %d, want 1 (only the downloadable tarball counts; version placeholder and release metadata rows are excluded, consistent with ListVersionFiles)", version.FileCount)
	}
}

func TestListVersionsUsesArtifactChecksumWhenBlobMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "pypi-proxy", PackageType: "pypi", Type: "proxy"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	art := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "pypi",
		Kind:         "package-file",
		Name:         "requests",
		Version:      "2.28.0",
		Path:         "packages/ab/cd",
		Filename:     "requests-2.28.0.tar.gz",
		RemotePath:   "packages/ab/cd/requests-2.28.0.tar.gz",
		SizeBytes:    12345,
		Checksums:    model.JSONB{"sha256": "abc123"},
	}
	if err := db.Create(&art).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.GET("/api/packages/:type/versions", handler.ListVersions)

	req := httptest.NewRequest(stdhttp.MethodGet, "/api/packages/pypi/versions?name=requests&repository_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Versions []struct {
				ChecksumSHA256 string `json:"checksum_sha256"`
				SizeBytes      int64  `json:"size_bytes"`
				FileCount      int    `json:"file_count"`
			} `json:"versions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(resp.Data.Versions))
	}
	if got := resp.Data.Versions[0].ChecksumSHA256; got != "abc123" {
		t.Fatalf("version checksum_sha256 = %q, want abc123", got)
	}
	if got := resp.Data.Versions[0].SizeBytes; got != 12345 {
		t.Fatalf("version size_bytes = %d, want 12345", got)
	}
	if got := resp.Data.Versions[0].FileCount; got != 1 {
		t.Fatalf("version file_count = %d, want 1", got)
	}
}

func TestListVersionsUsesPackageVersionSummaryWhenAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "maven-local", PackageType: "maven", Type: "hosted"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	artifact := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "maven",
		Kind:         "artifact",
		Namespace:    "com.example",
		Name:         "com.example:lib",
		Version:      "1.0.0",
		Path:         "com/example/lib/1.0.0",
		Filename:     "lib-1.0.0.jar",
		RemotePath:   "com/example/lib/1.0.0/lib-1.0.0.jar",
		Attributes:   model.JSONB{"license": "artifact-license", "published_at": "2020-01-01T00:00:00Z"},
		SizeBytes:    10,
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	artifactWithoutSummary := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "maven",
		Kind:         "artifact",
		Namespace:    "com.example",
		Name:         "com.example:lib",
		Version:      "2.0.0",
		Path:         "com/example/lib/2.0.0",
		Filename:     "lib-2.0.0.jar",
		RemotePath:   "com/example/lib/2.0.0/lib-2.0.0.jar",
		Attributes:   model.JSONB{"license": "artifact-license"},
		SizeBytes:    20,
	}
	if err := db.Create(&artifactWithoutSummary).Error; err != nil {
		t.Fatalf("create artifact without summary: %v", err)
	}
	publishedAt := time.Date(2026, 6, 30, 8, 9, 10, 0, time.UTC)
	if err := db.Create(&model.PackageVersion{
		RepositoryID:     repo.ID,
		Format:           "maven",
		PackageName:      "com.example:other",
		Version:          "0.1.0",
		Status:           "published",
		LatestArtifactAt: publishedAt,
		FileCount:        1,
	}).Error; err != nil {
		t.Fatalf("create unrelated package version: %v", err)
	}
	summary := model.PackageVersion{
		RepositoryID:     repo.ID,
		Format:           "maven",
		PackageName:      "com.example:lib",
		Namespace:        "com.example",
		Version:          "1.0.0",
		Status:           "deprecated",
		PublishedAt:      &publishedAt,
		LatestArtifactAt: publishedAt,
		FileCount:        1,
		FilesDownloaded:  false,
		SizeBytes:        456,
		DownloadCount:    7,
		License:          "summary-license",
		ChecksumSHA256:   "summary-sha",
	}
	if err := db.Create(&summary).Error; err != nil {
		t.Fatalf("create package version: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.GET("/api/packages/:type/versions", handler.ListVersions)

	req := httptest.NewRequest(stdhttp.MethodGet, "/api/packages/maven/versions?name=com.example:lib&repository_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Versions []struct {
				ID             uint   `json:"id"`
				RepositoryID   uint   `json:"repository_id"`
				Version        string `json:"version"`
				Status         string `json:"status"`
				PublishedAt    string `json:"published_at"`
				SizeBytes      int64  `json:"size_bytes"`
				ChecksumSHA256 string `json:"checksum_sha256"`
				License        string `json:"license"`
				DownloadCount  int64  `json:"download_count"`
				FileCount      int    `json:"file_count"`
			} `json:"versions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Versions) != 2 {
		t.Fatalf("expected summary path to merge unsummarized artifact versions, got %d", len(resp.Data.Versions))
	}
	var version *struct {
		ID             uint   `json:"id"`
		RepositoryID   uint   `json:"repository_id"`
		Version        string `json:"version"`
		Status         string `json:"status"`
		PublishedAt    string `json:"published_at"`
		SizeBytes      int64  `json:"size_bytes"`
		ChecksumSHA256 string `json:"checksum_sha256"`
		License        string `json:"license"`
		DownloadCount  int64  `json:"download_count"`
		FileCount      int    `json:"file_count"`
	}
	for i := range resp.Data.Versions {
		if resp.Data.Versions[i].Version == "1.0.0" {
			version = &resp.Data.Versions[i]
		}
		if resp.Data.Versions[i].Version == "2.0.0" && resp.Data.Versions[i].FileCount != 1 {
			t.Fatalf("unsummarized version file_count = %d, want 1", resp.Data.Versions[i].FileCount)
		}
	}
	if version == nil {
		t.Fatalf("summarized version 1.0.0 missing from response: %+v", resp.Data.Versions)
	}
	if version.Version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version.Version)
	}
	if version.ID != summary.ID {
		t.Fatalf("id = %d, want package version summary id %d", version.ID, summary.ID)
	}
	if version.RepositoryID != repo.ID {
		t.Fatalf("repository_id = %d, want %d", version.RepositoryID, repo.ID)
	}
	if version.Status != "deprecated" {
		t.Fatalf("status = %q, want deprecated", version.Status)
	}
	if version.PublishedAt != "2026-06-30T08:09:10Z" {
		t.Fatalf("published_at = %q, want 2026-06-30T08:09:10Z", version.PublishedAt)
	}
	if version.SizeBytes != 456 {
		t.Fatalf("size_bytes = %d, want 456", version.SizeBytes)
	}
	if version.ChecksumSHA256 != "summary-sha" {
		t.Fatalf("checksum_sha256 = %q, want summary-sha", version.ChecksumSHA256)
	}
	if version.License != "summary-license" {
		t.Fatalf("license = %q, want summary-license", version.License)
	}
	if version.DownloadCount != 7 {
		t.Fatalf("download_count = %d, want 7", version.DownloadCount)
	}
	if version.FileCount != 1 {
		t.Fatalf("file_count = %d, want 1", version.FileCount)
	}
}

func TestDeprecateVersionSyncsPackageVersionSummaryStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	artifact := model.Artifact{
		RepositoryID: 1,
		Format:       "maven",
		Kind:         "artifact",
		Name:         "com.example:lib",
		Version:      "1.0.0",
		Path:         "com/example/lib/1.0.0",
		Filename:     "lib-1.0.0.jar",
		RemotePath:   "com/example/lib/1.0.0/lib-1.0.0.jar",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := db.Create(&model.PackageVersion{
		RepositoryID:     1,
		Format:           "maven",
		PackageName:      "com.example:lib",
		Version:          "1.0.0",
		Status:           "published",
		LatestArtifactAt: artifact.UpdatedAt,
		FileCount:        1,
	}).Error; err != nil {
		t.Fatalf("create package version: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.POST("/api/packages/versions/:id/deprecate", handler.DeprecateVersion)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/packages/versions/"+strconv.Itoa(int(artifact.ID))+"/deprecate", bytes.NewBufferString(`{"reason":"bad release"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var summary model.PackageVersion
	if err := db.Where("repository_id = ? AND format = ? AND package_name = ? AND version = ?", 1, "maven", "com.example:lib", "1.0.0").
		First(&summary).Error; err != nil {
		t.Fatalf("query package version: %v", err)
	}
	if summary.Status != "deprecated" {
		t.Fatalf("summary status = %q, want deprecated", summary.Status)
	}
}

func TestDeprecatePackageVersionByCoordinateUpdatesAllArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	artifacts := []model.Artifact{
		{RepositoryID: 1, Format: "maven", Kind: "artifact", Name: "com.example:lib", Version: "1.0.0", Filename: "lib-1.0.0.jar", RemotePath: "com/example/lib/1.0.0/lib-1.0.0.jar"},
		{RepositoryID: 1, Format: "maven", Kind: "artifact", Name: "com.example:lib", Version: "1.0.0", Filename: "lib-1.0.0.pom", RemotePath: "com/example/lib/1.0.0/lib-1.0.0.pom"},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.POST("/api/packages/:type/versions/deprecate", handler.DeprecatePackageVersion)

	reqBody := `{"repository_id":1,"name":"com.example:lib","version":"1.0.0","reason":"bad release"}`
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/packages/maven/versions/deprecate", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var updated []model.Artifact
	if err := db.Where("repository_id = ? AND format = ? AND name = ? AND version = ?", 1, "maven", "com.example:lib", "1.0.0").
		Order("filename").Find(&updated).Error; err != nil {
		t.Fatalf("query artifacts: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(updated))
	}
	for _, artifact := range updated {
		if artifact.Metadata["status"] != "deprecated" {
			t.Fatalf("%s status = %#v, want deprecated", artifact.Filename, artifact.Metadata["status"])
		}
	}
}

func TestLegacyDeprecateVersionUpdatesWholeVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}, &model.PackageVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	jar := model.Artifact{RepositoryID: 1, Format: "maven", Kind: "artifact", Name: "com.example:lib", Version: "1.0.0", Filename: "lib-1.0.0.jar", RemotePath: "com/example/lib/1.0.0/lib-1.0.0.jar"}
	pom := model.Artifact{RepositoryID: 1, Format: "maven", Kind: "artifact", Name: "com.example:lib", Version: "1.0.0", Filename: "lib-1.0.0.pom", RemotePath: "com/example/lib/1.0.0/lib-1.0.0.pom"}
	if err := db.Create(&jar).Error; err != nil {
		t.Fatalf("create jar: %v", err)
	}
	if err := db.Create(&pom).Error; err != nil {
		t.Fatalf("create pom: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.POST("/api/packages/versions/:id/deprecate", handler.DeprecateVersion)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/packages/versions/"+strconv.Itoa(int(jar.ID))+"/deprecate", bytes.NewBufferString(`{"reason":"bad release"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var updatedPom model.Artifact
	if err := db.First(&updatedPom, pom.ID).Error; err != nil {
		t.Fatalf("load pom: %v", err)
	}
	if updatedPom.Metadata["status"] != "deprecated" {
		t.Fatalf("pom status = %#v, want deprecated", updatedPom.Metadata["status"])
	}
}

func TestListVersionFilesDecoratesMavenSnapshotDefaultVisibleFromFilenames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "maven-snapshots", PackageType: "maven", Type: "hosted"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	artifacts := []model.Artifact{
		{
			RepositoryID: repo.ID,
			Format:       "maven",
			Kind:         "artifact",
			Namespace:    "com.example",
			Name:         "com.example:lib",
			Version:      "1.0-SNAPSHOT",
			Path:         "com/example/lib/1.0-SNAPSHOT",
			Filename:     "lib-1.0-20230101.120000-1.jar",
			RemotePath:   "com/example/lib/1.0-SNAPSHOT/lib-1.0-20230101.120000-1.jar",
			Qualifiers:   model.JSONB{"group": "com.example", "artifact": "lib"},
			Attributes:   model.JSONB{"default_visible": "true", "display_group": "stale"},
		},
		{
			RepositoryID: repo.ID,
			Format:       "maven",
			Kind:         "artifact",
			Namespace:    "com.example",
			Name:         "com.example:lib",
			Version:      "1.0-SNAPSHOT",
			Path:         "com/example/lib/1.0-SNAPSHOT",
			Filename:     "lib-1.0-20230102.120000-2.jar",
			RemotePath:   "com/example/lib/1.0-SNAPSHOT/lib-1.0-20230102.120000-2.jar",
			Qualifiers:   model.JSONB{"group": "com.example", "artifact": "lib"},
		},
		{
			RepositoryID: repo.ID,
			Format:       "maven",
			Kind:         "artifact",
			Namespace:    "com.example",
			Name:         "com.example:lib",
			Version:      "1.0-SNAPSHOT",
			Path:         "com/example/lib/1.0-SNAPSHOT",
			Filename:     "lib-1.0-20230101.120000-1-sources.jar",
			RemotePath:   "com/example/lib/1.0-SNAPSHOT/lib-1.0-20230101.120000-1-sources.jar",
			Qualifiers:   model.JSONB{"group": "com.example", "artifact": "lib", "classifier": "sources"},
		},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.GET("/api/packages/:type/versions/files", handler.ListVersionFiles)

	req := httptest.NewRequest(stdhttp.MethodGet, "/api/packages/maven/versions/files?name=com.example:lib&version=1.0-SNAPSHOT&repository_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Files []struct {
				Filename   string                 `json:"filename"`
				Attributes map[string]interface{} `json:"attributes"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	files := map[string]map[string]interface{}{}
	for _, file := range resp.Data.Files {
		files[file.Filename] = file.Attributes
	}

	assertAttr := func(filename, key string, want interface{}) {
		t.Helper()
		if got := files[filename][key]; got != want {
			t.Fatalf("%s attributes[%s] = %#v, want %#v", filename, key, got, want)
		}
	}
	assertMissingAttr := func(filename, key string) {
		t.Helper()
		if _, ok := files[filename][key]; ok {
			t.Fatalf("%s attributes[%s] is present: %#v", filename, key, files[filename][key])
		}
	}

	assertMissingAttr("lib-1.0-20230101.120000-1.jar", "default_visible")
	assertMissingAttr("lib-1.0-20230101.120000-1.jar", "display_group")
	assertAttr("lib-1.0-20230102.120000-2.jar", "default_visible", "true")
	assertAttr("lib-1.0-20230102.120000-2.jar", "display_group", "20230102.120000-2")
	assertAttr("lib-1.0-20230101.120000-1-sources.jar", "default_visible", "true")
	assertAttr("lib-1.0-20230101.120000-1-sources.jar", "display_group", "20230101.120000-1")
}

func TestListVersionFilesReturnsFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	blob := model.Blob{
		Algorithm:   "sha256",
		Digest:      "deadbeef",
		Size:        98765,
		StoragePath: "/data/blobs/deadbeef",
	}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}

	artifact := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "tarball",
		Name:         "left-pad",
		Version:      "1.0.0",
		Path:         "left-pad/-",
		Filename:     "left-pad-1.0.0.tgz",
		RemotePath:   "left-pad/-/left-pad-1.0.0.tgz",
		SizeBytes:    12345,
		Checksums:    model.JSONB{"sha256": "abc123"},
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	if err := db.Create(&model.ArtifactBlob{
		ArtifactID: artifact.ID,
		BlobID:     blob.ID,
		Position:   0,
	}).Error; err != nil {
		t.Fatalf("create artifact blob: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.GET("/api/packages/:type/versions/files", handler.ListVersionFiles)

	req := httptest.NewRequest(stdhttp.MethodGet, "/api/packages/npm/versions/files?name=left-pad&version=1.0.0&repository_id="+strconv.Itoa(int(repo.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Files []struct {
				Filename       string `json:"filename"`
				DownloadURL    string `json:"download_url"`
				SizeBytes      int64  `json:"size_bytes"`
				ChecksumSHA256 string `json:"checksum_sha256"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Files) != 1 {
		t.Fatalf("expected 1 file, got %d: %+v", len(resp.Data.Files), resp.Data.Files)
	}
	file := resp.Data.Files[0]
	if file.Filename != "left-pad-1.0.0.tgz" {
		t.Fatalf("filename = %q, want left-pad-1.0.0.tgz", file.Filename)
	}
	if file.DownloadURL != "/repository/npm-proxy/left-pad/-/left-pad-1.0.0.tgz" {
		t.Fatalf("download_url = %q", file.DownloadURL)
	}
	if file.SizeBytes != 98765 {
		t.Fatalf("size_bytes = %d, want 98765 (from blob)", file.SizeBytes)
	}
	if file.ChecksumSHA256 != "deadbeef" {
		t.Fatalf("checksum_sha256 = %q, want deadbeef (from blob digest)", file.ChecksumSHA256)
	}
}

func TestDeletePackageUsesPackageAggregateID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Package{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "maven-local", PackageType: "maven", Type: "local"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	pkg := model.Package{RepositoryID: repo.ID, Format: "maven", Name: "com.example:app", VersionCount: 2}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	other := model.Artifact{RepositoryID: repo.ID, Format: "maven", Kind: "artifact", Name: "com.example:other", Version: "1.0.0", RemotePath: "com/example/other/1.0.0/other-1.0.0.jar"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other artifact: %v", err)
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		art := model.Artifact{RepositoryID: repo.ID, Format: "maven", Kind: "artifact", Name: "com.example:app", Version: version, RemotePath: "com/example/app/" + version + "/app-" + version + ".jar"}
		if err := db.Create(&art).Error; err != nil {
			t.Fatalf("create app artifact: %v", err)
		}
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.DELETE("/api/packages/:id", handler.DeletePackage)

	req := httptest.NewRequest(stdhttp.MethodDelete, "/api/packages/"+strconv.Itoa(int(pkg.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var remainingApp int64
	if err := db.Model(&model.Artifact{}).Where("name = ?", "com.example:app").Count(&remainingApp).Error; err != nil {
		t.Fatalf("count app artifacts: %v", err)
	}
	if remainingApp != 0 {
		t.Fatalf("expected app artifacts deleted, got %d remaining", remainingApp)
	}
	var remainingPkg int64
	if err := db.Model(&model.Package{}).Where("id = ?", pkg.ID).Count(&remainingPkg).Error; err != nil {
		t.Fatalf("count package: %v", err)
	}
	if remainingPkg != 0 {
		t.Fatalf("expected package aggregate deleted, got %d remaining", remainingPkg)
	}
	var otherCount int64
	if err := db.Model(&model.Artifact{}).Where("id = ?", other.ID).Count(&otherCount).Error; err != nil {
		t.Fatalf("count other artifact: %v", err)
	}
	if otherCount != 1 {
		t.Fatalf("expected unrelated artifact preserved, got %d", otherCount)
	}
}

func TestDeletePackageAcceptsLegacyRouteWildcardName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Package{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pkg := model.Package{RepositoryID: 7, Format: "npm", Name: "left-pad"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	artifact := model.Artifact{
		RepositoryID: 7,
		Format:       "npm",
		Kind:         "tarball",
		Name:         "left-pad",
		Version:      "1.0.0",
		Filename:     "left-pad-1.0.0.tgz",
		RemotePath:   "left-pad/-/left-pad-1.0.0.tgz",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.DELETE("/api/packages/:type", handler.DeletePackage)

	req := httptest.NewRequest(stdhttp.MethodDelete, "/api/packages/"+strconv.Itoa(int(pkg.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var remaining int64
	if err := db.Model(&model.Artifact{}).Where("repository_id = ? AND format = ? AND name = ?", 7, "npm", "left-pad").Count(&remaining).Error; err != nil {
		t.Fatalf("count artifacts: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected package artifacts deleted, got %d remaining", remaining)
	}
}

func TestDeletePackageFallsBackToArtifactID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Package{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	artifact := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "npm",
		Kind:         "artifact",
		Name:         "left-pad",
		Version:      "1.0.0",
		RemotePath:   "left-pad/-/left-pad-1.0.0.tgz",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.DELETE("/api/packages/:id", handler.DeletePackage)

	req := httptest.NewRequest(stdhttp.MethodDelete, "/api/packages/"+strconv.Itoa(int(artifact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var remaining int64
	if err := db.Model(&model.Artifact{}).Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "npm", "left-pad").Count(&remaining).Error; err != nil {
		t.Fatalf("count artifacts: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected artifacts deleted, got %d remaining", remaining)
	}
}

func TestDeletePackageSupportsCoordinateFallbackWhenIDIsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Package{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "pypi-proxy", PackageType: "pypi", Type: "proxy"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	pkg := model.Package{RepositoryID: repo.ID, Format: "pypi", Name: "requests", VersionCount: 1}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	artifact := model.Artifact{
		RepositoryID: repo.ID,
		Format:       "pypi",
		Kind:         "artifact",
		Name:         "requests",
		Version:      "2.31.0",
		RemotePath:   "packages/requests-2.31.0.tar.gz",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.DELETE("/api/packages/:id", handler.DeletePackage)

	req := httptest.NewRequest(stdhttp.MethodDelete, "/api/packages/0?type=pypi&name=requests&repository_id="+strconv.Itoa(int(repo.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var remainingArtifacts int64
	if err := db.Model(&model.Artifact{}).Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "pypi", "requests").Count(&remainingArtifacts).Error; err != nil {
		t.Fatalf("count artifacts: %v", err)
	}
	if remainingArtifacts != 0 {
		t.Fatalf("expected artifacts deleted, got %d remaining", remainingArtifacts)
	}
	var remainingPackages int64
	if err := db.Model(&model.Package{}).Where("repository_id = ? AND format = ? AND name = ?", repo.ID, "pypi", "requests").Count(&remainingPackages).Error; err != nil {
		t.Fatalf("count packages: %v", err)
	}
	if remainingPackages != 0 {
		t.Fatalf("expected package deleted, got %d remaining", remainingPackages)
	}
}

func TestDeleteVersionUpdatesPackageAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Package{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := model.Repository{Name: "maven-local", PackageType: "maven", Type: "local"}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}
	pkg := model.Package{RepositoryID: repo.ID, Format: "maven", Name: "com.example:app", VersionCount: 2, LatestVersion: "2.0.0"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatalf("create package: %v", err)
	}
	oldArtifact := model.Artifact{RepositoryID: repo.ID, Format: "maven", Kind: "artifact", Name: "com.example:app", Version: "1.0.0", RemotePath: "com/example/app/1.0.0/app-1.0.0.jar"}
	newArtifact := model.Artifact{RepositoryID: repo.ID, Format: "maven", Kind: "artifact", Name: "com.example:app", Version: "2.0.0", RemotePath: "com/example/app/2.0.0/app-2.0.0.jar"}
	if err := db.Create(&oldArtifact).Error; err != nil {
		t.Fatalf("create old artifact: %v", err)
	}
	if err := db.Create(&newArtifact).Error; err != nil {
		t.Fatalf("create new artifact: %v", err)
	}

	handler := NewPackageVersionHandler(db)
	router := gin.New()
	router.DELETE("/api/packages/versions/:id", handler.DeleteVersion)

	req := httptest.NewRequest(stdhttp.MethodDelete, "/api/packages/versions/"+strconv.Itoa(int(newArtifact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != stdhttp.StatusNoContent {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var updated model.Package
	if err := db.First(&updated, pkg.ID).Error; err != nil {
		t.Fatalf("load package: %v", err)
	}
	if updated.VersionCount != 1 {
		t.Fatalf("VersionCount = %d, want 1", updated.VersionCount)
	}
	if updated.LatestVersion != "1.0.0" {
		t.Fatalf("LatestVersion = %q, want 1.0.0", updated.LatestVersion)
	}
}
