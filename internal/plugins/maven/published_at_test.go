package maven

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// FetchRemote 解析 <snapshotVersions> 时，文件行应携带从 updated
// （与快照文件名 timestamp 同源）转换的 published_at，作为该构建的真实发布时间。
func TestFetchRemote_SnapshotFileCarriesPublishedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata modelVersion="1.1.0">
  <groupId>com.example</groupId>
  <artifactId>lib</artifactId>
  <version>1.0-SNAPSHOT</version>
  <versioning>
    <snapshot>
      <timestamp>20260604.090000</timestamp>
      <buildNumber>2</buildNumber>
    </snapshot>
    <snapshotVersions>
      <snapshotVersion>
        <extension>jar</extension>
        <value>1.0-20260604.090000-2</value>
        <updated>20260604090000</updated>
      </snapshotVersion>
    </snapshotVersions>
    <lastUpdated>20260605120000</lastUpdated>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/example/lib/1.0-SNAPSHOT/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}

	// 文件行（kind=artifact，对应 jar）：published_at 必须是 snapshotVersion.updated，
	// 不是 metadata 的 lastUpdated。
	var fileRow *runtime.Artifact
	for _, a := range arts {
		if a.Filename == "lib-1.0-20260604.090000-2.jar" {
			fileRow = a
			break
		}
	}
	if fileRow == nil {
		t.Fatalf("expected snapshot file artifact in %+v", arts)
	}
	if got := fileRow.Attributes["published_at"]; got != "2026-06-04T09:00:00Z" {
		t.Errorf("file row published_at = %q, want 2026-06-04T09:00:00Z", got)
	}
}

// NormalizeAsset（proxy 回源单个文件 / 本地上传归一化）应从时间戳文件名
// 解析出 published_at；非 SNAPSHOT 或无时间戳文件名不写该属性。
func TestNormalizeAssetSnapshotPublishedAt(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)

	// 时间戳快照文件名 → 解析出构建时间
	a, err := p.NormalizeAsset(context.Background(), runtime.NormalizeInput{
		RepositoryID: "1",
		RemotePath:   "com/example/lib/1.0-SNAPSHOT/lib-1.0-20260604.090000-2.jar",
	})
	if err != nil {
		t.Fatalf("NormalizeAsset failed: %v", err)
	}
	if got := a.Attributes["published_at"]; got != "2026-06-04T09:00:00Z" {
		t.Errorf("snapshot published_at = %q, want 2026-06-04T09:00:00Z", got)
	}

	// 无时间戳文件名 → 不写
	a, err = p.NormalizeAsset(context.Background(), runtime.NormalizeInput{
		RepositoryID: "1",
		RemotePath:   "com/example/lib/1.0-SNAPSHOT/lib-1.0-SNAPSHOT.jar",
	})
	if err != nil {
		t.Fatalf("NormalizeAsset failed: %v", err)
	}
	if got := a.Attributes["published_at"]; got != "" {
		t.Errorf("plain snapshot published_at = %q, want empty", got)
	}

	// release 版本 → 不写
	a, err = p.NormalizeAsset(context.Background(), runtime.NormalizeInput{
		RepositoryID: "1",
		RemotePath:   "com/example/lib/1.2.3/lib-1.2.3.jar",
	})
	if err != nil {
		t.Fatalf("NormalizeAsset failed: %v", err)
	}
	if got := a.Attributes["published_at"]; got != "" {
		t.Errorf("release published_at = %q, want empty", got)
	}
}

// handleUpload（local mvn deploy 路径）端到端：上传时间戳快照文件后，
// artifacts 表中该行携带文件名解析出的 published_at。
func TestHandleUploadSnapshotCarriesPublishedAt(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	artifactSvc := service.NewArtifactService(db)
	metadataStore := storage.NewMetadataStoreWithArtifactService(db, artifactSvc)
	backend, err := storage.NewLocalStorage(t.TempDir(), 1)
	if err != nil {
		t.Fatalf("local storage: %v", err)
	}
	repoRuntime := &runtime.HostedRuntime{
		MetadataStore:  metadataStore,
		BlobStore:      storage.NewCASBlobStore(backend, db),
		RepositoryID:   "1",
		AllowOverwrite: true,
		AllowDelete:    true,
	}

	ctx, w := newCtx(http.MethodPut, "com/example/lib/1.0-SNAPSHOT/lib-1.0-20260604.090000-2.jar", strings.NewReader("jar-content"))
	if err := p.Handle(ctx, repoRuntime); err != nil {
		t.Fatalf("Handle upload failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201", w.Code)
	}

	var artifact model.Artifact
	if err := db.Where("format = ? AND filename = ?", "maven", "lib-1.0-20260604.090000-2.jar").
		First(&artifact).Error; err != nil {
		t.Fatalf("load artifact: %v", err)
	}
	if got := artifact.Attributes["published_at"]; got != "2026-06-04T09:00:00Z" {
		t.Errorf("uploaded artifact published_at = %q, want 2026-06-04T09:00:00Z", got)
	}
}
