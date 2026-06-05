package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
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
		DownloadPath: "left-pad/-/left-pad-1.0.0.tgz",
		Qualifiers:   model.JSONB{"package_type": "tarball"},
	}
	if err := db.Create(&versionOnly).Error; err != nil {
		t.Fatalf("create version artifact: %v", err)
	}
	if err := db.Create(&tarball).Error; err != nil {
		t.Fatalf("create tarball artifact: %v", err)
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
					Filename     string `json:"filename"`
					DownloadURL  string `json:"download_url"`
					Path         string `json:"path"`
					RemotePath   string `json:"remote_path"`
					DownloadPath string `json:"download_path"`
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
	if files[0].Path != "left-pad/-" || files[0].RemotePath != "left-pad/-/left-pad-1.0.0.tgz" || files[0].DownloadPath != "left-pad/-/left-pad-1.0.0.tgz" {
		t.Fatalf("unexpected file paths: %+v", files[0])
	}
}
