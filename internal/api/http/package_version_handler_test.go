package http

import (
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
				License    string                 `json:"license"`
				Attributes map[string]interface{} `json:"attributes"`
				Files      []struct {
					Filename    string                 `json:"filename"`
					DownloadURL string                 `json:"download_url"`
					Path        string                 `json:"path"`
					RemotePath  string                 `json:"remote_path"`
					Attributes  map[string]interface{} `json:"attributes"`
				} `json:"files"`
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
	if version.Attributes["license"] != "MIT" {
		t.Fatalf("attributes.license = %#v", version.Attributes["license"])
	}
	files := version.Files
	if len(files) != 1 {
		t.Fatalf("expected only downloadable file, got %d files: %+v", len(files), files)
	}
	if files[0].Filename != "left-pad-1.0.0.tgz" {
		t.Fatalf("filename = %q", files[0].Filename)
	}
	if files[0].DownloadURL != "/repository/npm-proxy/left-pad/-/left-pad-1.0.0.tgz" {
		t.Fatalf("download_url = %q", files[0].DownloadURL)
	}
	if files[0].Path != "left-pad/-" || files[0].RemotePath != "left-pad/-/left-pad-1.0.0.tgz" {
		t.Fatalf("unexpected file paths: %+v", files[0])
	}
	if files[0].Attributes["default_visible"] != "true" {
		t.Fatalf("file default_visible = %#v", files[0].Attributes["default_visible"])
	}
	if files[0].Attributes["display_group"] != "current" {
		t.Fatalf("file display_group = %#v", files[0].Attributes["display_group"])
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
				Files          []struct {
					Filename       string `json:"filename"`
					ChecksumSHA256 string `json:"checksum_sha256"`
					SizeBytes      int64  `json:"size_bytes"`
				} `json:"files"`
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
	if len(resp.Data.Versions[0].Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(resp.Data.Versions[0].Files))
	}
	if got := resp.Data.Versions[0].Files[0].ChecksumSHA256; got != "abc123" {
		t.Fatalf("file checksum_sha256 = %q, want abc123", got)
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
