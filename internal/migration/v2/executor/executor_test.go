package executor

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
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

	artifact := buildMigratedArtifact(42, item, asset, 7, map[string]string{
		"sha256": "abc123",
	})

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

	artifact := buildMigratedArtifact(42, item, source.AssetStream{}, 7, nil)

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
