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
