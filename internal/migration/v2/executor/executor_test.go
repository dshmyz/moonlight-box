package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/plugins/maven"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildMigratedArtifactUsesRuntimeFileShape(t *testing.T) {
	item := &domain.MigrationItem{
		SourceRepository: "pypi-hosted",
		SourcePath:       "packages/ab/cd/requests-2.28.0-py3-none-any.whl",
		SourceFormat:     "pypi",
		SourceName:       "requests",
		SourceVersion:    "2.28.0",
		TargetPath:       "custom/requests-2.28.0-py3-none-any.whl",
	}
	asset := source.AssetStream{
		ContentType: "application/octet-stream",
		Size:        1234,
	}

	artifact := buildMigratedArtifact(42, item, asset, runtime.BlobRef{BlobID: 7, Algorithm: "sha256", Digest: "abc123", Size: 1234}, map[string]string{
		"sha256": "abc123",
	}, nil)

	if artifact.RepositoryID != "42" {
		t.Fatalf("repository id = %q, want 42", artifact.RepositoryID)
	}
	if artifact.Kind != runtime.KindFile {
		t.Fatalf("kind = %q, want %q", artifact.Kind, runtime.KindFile)
	}
	if artifact.Name != "requests" || artifact.Version != "2.28.0" || artifact.Format != "pypi" {
		t.Fatalf("artifact identity = %s/%s/%s, want pypi/requests/2.28.0", artifact.Format, artifact.Name, artifact.Version)
	}
	if artifact.RemotePath != "packages/ab/cd/requests-2.28.0-py3-none-any.whl" {
		t.Fatalf("remote path = %q", artifact.RemotePath)
	}
	if artifact.DownloadPath != "custom/requests-2.28.0-py3-none-any.whl" {
		t.Fatalf("download path = %q", artifact.DownloadPath)
	}
	if artifact.Path != "packages/ab/cd" {
		t.Fatalf("path = %q, want packages/ab/cd", artifact.Path)
	}
	if artifact.Filename != "requests-2.28.0-py3-none-any.whl" {
		t.Fatalf("filename = %q", artifact.Filename)
	}
	if artifact.ContentType != "application/octet-stream" || artifact.SizeBytes != 1234 {
		t.Fatalf("content fields = %q/%d", artifact.ContentType, artifact.SizeBytes)
	}
	if artifact.Checksums["sha256"] != "abc123" {
		t.Fatalf("sha256 checksum = %q", artifact.Checksums["sha256"])
	}
	if artifact.Attributes["source_repository"] != "pypi-hosted" {
		t.Fatalf("source repository attribute = %q", artifact.Attributes["source_repository"])
	}
	if len(artifact.BlobRefs) != 1 || artifact.BlobRefs[0].BlobID != 7 {
		t.Fatalf("blob refs = %#v", artifact.BlobRefs)
	}
}

func TestBuildMigratedArtifactNormalizesFormatAndKeepsRemotePathRelative(t *testing.T) {
	item := &domain.MigrationItem{
		SourceRepository: "maven-releases",
		SourcePath:       "com/acme/demo/1.0.0/demo-1.0.0.jar",
		SourceFormat:     "maven2",
		SourceName:       "com.acme:demo",
		SourceVersion:    "1.0.0",
	}

	artifact := buildMigratedArtifact(42, item, source.AssetStream{}, runtime.BlobRef{BlobID: 7}, nil, nil)

	if artifact.Format != "maven" {
		t.Fatalf("format = %q, want maven", artifact.Format)
	}
	if artifact.RemotePath != "com/acme/demo/1.0.0/demo-1.0.0.jar" {
		t.Fatalf("remote path = %q", artifact.RemotePath)
	}
	if artifact.DownloadURL != "" {
		t.Fatalf("download url leaked into artifact = %q", artifact.DownloadURL)
	}
}

func TestBuildMigratedArtifactUsesMavenPathParserForSnapshotArtifacts(t *testing.T) {
	item := &domain.MigrationItem{
		SourceRepository: "maven-snapshots",
		SourcePath:       "com/acme/demo/1.0-SNAPSHOT/demo-1.0-20260608.123456-2-sources.jar",
		SourceFormat:     "maven",
		SourceName:       "demo",
		SourceVersion:    "",
	}

	artifact := buildMigratedArtifact(42, item, source.AssetStream{}, runtime.BlobRef{BlobID: 7}, nil, map[string]runtime.ArtifactNormalizer{
		"maven": maven.NewMavenPlugin(),
	})

	if artifact.Kind != runtime.KindArtifact {
		t.Fatalf("kind = %q, want artifact", artifact.Kind)
	}
	if artifact.Namespace != "com.acme" {
		t.Fatalf("namespace = %q, want com.acme", artifact.Namespace)
	}
	if artifact.Name != "com.acme:demo" {
		t.Fatalf("name = %q, want com.acme:demo", artifact.Name)
	}
	if artifact.Version != "1.0-SNAPSHOT" {
		t.Fatalf("version = %q, want 1.0-SNAPSHOT", artifact.Version)
	}
	if artifact.Qualifiers["group"] != "com.acme" || artifact.Qualifiers["artifact"] != "demo" {
		t.Fatalf("qualifiers = %#v", artifact.Qualifiers)
	}
	if artifact.Qualifiers["classifier"] != "sources" {
		t.Fatalf("classifier = %q, want sources", artifact.Qualifiers["classifier"])
	}
}

func TestBuildMigratedArtifactUsesMavenPathParserForSnapshotMetadata(t *testing.T) {
	item := &domain.MigrationItem{
		SourceRepository: "maven-snapshots",
		SourcePath:       "com/acme/demo/1.0-SNAPSHOT/maven-metadata.xml",
		SourceFormat:     "maven",
		SourceName:       "demo",
	}

	artifact := buildMigratedArtifact(42, item, source.AssetStream{}, runtime.BlobRef{BlobID: 7}, nil, map[string]runtime.ArtifactNormalizer{
		"maven": maven.NewMavenPlugin(),
	})

	if artifact.Kind != runtime.KindMetadata {
		t.Fatalf("kind = %q, want metadata", artifact.Kind)
	}
	if artifact.Name != "com.acme:demo" {
		t.Fatalf("name = %q, want com.acme:demo", artifact.Name)
	}
	if artifact.Version != "1.0-SNAPSHOT" {
		t.Fatalf("version = %q, want 1.0-SNAPSHOT", artifact.Version)
	}
	if artifact.Path != "com/acme/demo/1.0-SNAPSHOT" {
		t.Fatalf("path = %q, want snapshot metadata directory", artifact.Path)
	}
}

func TestPutMigratedBlobUsesCASStorageAndReusesExistingDigest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatal(err)
	}
	backend, err := storage.NewLocalStorage(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}

	first, err := putMigratedBlob(context.Background(), db, backend, strings.NewReader("same-content"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := putMigratedBlob(context.Background(), db, backend, strings.NewReader("same-content"))
	if err != nil {
		t.Fatal(err)
	}

	if first.BlobID != second.BlobID {
		t.Fatalf("blob ids = %d/%d, want reused blob", first.BlobID, second.BlobID)
	}
	if first.Digest == "" || first.Algorithm != "sha256" {
		t.Fatalf("blob ref = %#v", first)
	}

	var blob model.Blob
	if err := db.First(&blob, first.BlobID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(blob.StoragePath, "blobs/sha256/") {
		t.Fatalf("storage path = %q, want CAS blobs/sha256 path", blob.StoragePath)
	}

	var count int64
	if err := db.Model(&model.Blob{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("blob count = %d, want 1", count)
	}
}
