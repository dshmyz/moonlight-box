package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMetadataStoreQueryFiltersQualifiers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	artifacts := []model.Artifact{
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			Name:         "com.example:app",
			Version:      "1.0-SNAPSHOT",
			RemotePath:   "com/example/app/1.0-SNAPSHOT/app-1.0-20260612.120000-1.jar",
			Qualifiers:   model.JSONB{"group": "com.example", "artifact": "app", "classifier": ""},
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         runtime.KindArtifact,
			Name:         "com.example:app",
			Version:      "1.0-SNAPSHOT",
			RemotePath:   "com/example/app/1.0-SNAPSHOT/app-1.0-20260612.120000-1-sources.jar",
			Qualifiers:   model.JSONB{"group": "com.example", "artifact": "app", "classifier": "sources"},
		},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	store := NewMetadataStore(db)
	got, err := store.Query(context.Background(), runtime.ArtifactQuery{
		RepositoryID: "1",
		Format:       "maven",
		Name:         "com.example:app",
		Version:      "1.0-SNAPSHOT",
		Qualifiers: map[string]string{
			"classifier": "sources",
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 sources artifact, got %d", len(got))
	}
	if got[0].Qualifiers["classifier"] != "sources" {
		t.Fatalf("classifier = %q", got[0].Qualifiers["classifier"])
	}
}

func TestMetadataStoreListRejectsUnboundedRepositoryScan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	const repoID = uint(1)
	artifacts := make([]model.Artifact, 1001)
	for i := range artifacts {
		artifacts[i] = model.Artifact{
			RepositoryID: repoID,
			Format:       "generic",
			Kind:         runtime.KindFile,
			IdentityKey:  fmt.Sprintf("file/readme-%04d", i),
			Name:         "readme",
			Version:      "1.0.0",
			Filename:     "readme.txt",
		}
	}
	if err := db.CreateInBatches(artifacts, 100).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	store := NewMetadataStore(db)
	_, err = store.List(context.Background(), "1")
	if err == nil {
		t.Fatal("expected List to reject unbounded repository scan")
	}
	if !strings.Contains(err.Error(), "too many artifacts") {
		t.Fatalf("error = %v, want too many artifacts", err)
	}
}

func TestArtifactAutoMigrateCreatesFormatIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	if !db.Migrator().HasIndex(&model.Artifact{}, "idx_artifact_format") {
		t.Fatal("expected artifacts.format index")
	}
}

// TestBatchPutHandlesManyIdentityKeys 验证当 identity_key 数量超过 SQLite
// 参数上限时，BatchPut 通过分块查询避免 "too many SQL variables" 错误。
func TestBatchPutHandlesManyIdentityKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	store := NewMetadataStore(db)

	// 构造超过 existingQueryChunkSize (500) 且超过 SQLite 默认变量上限 (999) 的批量写入。
	// 这里用 1200 个 artifact，确保即使不分块也会触发 "too many SQL variables"。
	const count = 1200
	artifacts := make([]*runtime.Artifact, count)
	for i := 0; i < count; i++ {
		artifacts[i] = &runtime.Artifact{
			RepositoryID: "1",
			Format:       "pypi",
			Kind:         runtime.KindArtifact,
			IdentityKey:  fmt.Sprintf("pypi:pkg-%04d", i),
			Name:         fmt.Sprintf("pkg-%04d", i),
			Version:      "1.0.0",
			Filename:     fmt.Sprintf("pkg_%04d-1.0.0-py3-none-any.whl", i),
			Path:         fmt.Sprintf("packages/pkg-%04d/1.0.0", i),
		}
	}

	if err := store.BatchPut(context.Background(), artifacts); err != nil {
		t.Fatalf("BatchPut with %d identity keys failed: %v", count, err)
	}

	// 验证写入数量正确
	got, err := store.Query(context.Background(), runtime.ArtifactQuery{
		RepositoryID: "1",
		Format:       "pypi",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != count {
		t.Fatalf("expected %d artifacts, got %d", count, len(got))
	}

	// 再次 BatchPut（全部命中 existing，走 UPDATE 分支）也应成功
	if err := store.BatchPut(context.Background(), artifacts); err != nil {
		t.Fatalf("BatchPut (update path) with %d identity keys failed: %v", count, err)
	}
}
