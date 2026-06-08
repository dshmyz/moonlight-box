package planner

import (
	"encoding/json"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
)

func TestMigrationAssetCheckpointSeparatesDownloadURLFromPath(t *testing.T) {
	checkpoint, err := migrationAssetCheckpointJSON(source.SourceAsset{
		DownloadURL: "https://nexus.example/repository/maven/com/acme/demo/1.0.0/demo-1.0.0.jar",
		Path:        "com/acme/demo/1.0.0/demo-1.0.0.jar",
		Checksum: map[string]string{
			"sha1": "def456",
		},
		ContentType: "application/java-archive",
		FileSize:    99,
	})
	if err != nil {
		t.Fatal(err)
	}

	var decoded domain.AssetCheckpoint
	if err := json.Unmarshal([]byte(checkpoint), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.DownloadURL != "https://nexus.example/repository/maven/com/acme/demo/1.0.0/demo-1.0.0.jar" {
		t.Fatalf("download url = %q", decoded.DownloadURL)
	}
	if decoded.Path != "com/acme/demo/1.0.0/demo-1.0.0.jar" {
		t.Fatalf("path = %q", decoded.Path)
	}
	if decoded.Checksum["sha1"] != "def456" {
		t.Fatalf("checksum = %#v", decoded.Checksum)
	}
}

func TestNexusTargetFormatMapsRepositoryFormatsToPluginFormats(t *testing.T) {
	tests := map[string]string{
		"maven2":  "maven",
		"maven":   "maven",
		"raw":     "generic",
		"generic": "generic",
		"npm":     "npm",
		"pypi":    "pypi",
	}

	for input, want := range tests {
		if got := nexusTargetFormat(input); got != want {
			t.Fatalf("nexusTargetFormat(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestArtifactSourceFormatFallsBackToRepositoryFormat(t *testing.T) {
	comp := source.SourceComponent{
		Format: "",
	}

	if got := artifactSourceFormat(comp, "maven2"); got != "maven" {
		t.Fatalf("artifactSourceFormat(empty component, maven2 repo) = %q, want maven", got)
	}
}
