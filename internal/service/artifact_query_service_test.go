package service

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetVersionsAggregatesMavenFilesByLogicalVersion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	createdAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	artifacts := []model.Artifact{
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         "artifact",
			Name:         "com.example:lib",
			Version:      "1.0.0",
			RemotePath:   "com/example/lib/1.0.0/lib-1.0.0.jar",
			CreatedAt:    createdAt,
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         "artifact",
			Name:         "com.example:lib",
			Version:      "1.0.0",
			RemotePath:   "com/example/lib/1.0.0/lib-1.0.0.pom",
			CreatedAt:    createdAt.Add(time.Minute),
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         "artifact",
			Name:         "com.example:lib",
			Version:      "1.1-SNAPSHOT",
			RemotePath:   "com/example/lib/1.1-SNAPSHOT/lib-1.1-20260612.100000-1.jar",
			CreatedAt:    createdAt.Add(2 * time.Minute),
		},
		{
			RepositoryID: 1,
			Format:       "maven",
			Kind:         "artifact",
			Name:         "com.example:lib",
			Version:      "1.1-SNAPSHOT",
			RemotePath:   "com/example/lib/1.1-SNAPSHOT/lib-1.1-20260612.100000-1.pom",
			CreatedAt:    createdAt.Add(3 * time.Minute),
		},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create artifacts: %v", err)
	}

	versions, err := NewArtifactQueryService(db).GetVersions(context.Background(), 1, "maven", "com.example:lib")
	if err != nil {
		t.Fatalf("GetVersions failed: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 logical versions, got %d: %#v", len(versions), versions)
	}
	if versions[0].Version != "1.1-SNAPSHOT" {
		t.Fatalf("first version = %q, want latest logical snapshot", versions[0].Version)
	}
	if versions[1].Version != "1.0.0" {
		t.Fatalf("second version = %q, want release version", versions[1].Version)
	}
}
