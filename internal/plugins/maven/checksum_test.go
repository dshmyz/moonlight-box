package maven

import (
	"strings"
	"testing"
)

func TestParseChecksumRequest(t *testing.T) {
	tests := []struct {
		filename     string
		wantOriginal string
		wantAlgo     checksumAlgo
		wantOK       bool
	}{
		{"my-lib-1.0.0.jar.sha1", "my-lib-1.0.0.jar", checksumSHA1, true},
		{"my-lib-1.0.0.jar.md5", "my-lib-1.0.0.jar", checksumMD5, true},
		{"my-lib-1.0.0.jar.sha256", "my-lib-1.0.0.jar", checksumSHA256, true},
		{"my-lib-1.0.0.pom.sha1", "my-lib-1.0.0.pom", checksumSHA1, true},
		{"maven-metadata.xml.sha1", "maven-metadata.xml", checksumSHA1, true},
		{"my-lib-1.0.0.jar", "", "", false},
		{"my-lib-1.0.0.pom", "", "", false},
		{"maven-metadata.xml", "", "", false},
		{"sources.jar.sha1", "sources.jar", checksumSHA1, true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			original, algo, ok := parseChecksumRequest(tt.filename)
			if ok != tt.wantOK {
				t.Fatalf("parseChecksumRequest(%q) ok = %v, want %v", tt.filename, ok, tt.wantOK)
			}
			if original != tt.wantOriginal {
				t.Errorf("original = %q, want %q", original, tt.wantOriginal)
			}
			if algo != tt.wantAlgo {
				t.Errorf("algo = %q, want %q", algo, tt.wantAlgo)
			}
		})
	}
}

func TestComputeChecksum(t *testing.T) {
	content := "hello world"

	sha1Val, err := computeChecksum(strings.NewReader(content), checksumSHA1)
	if err != nil {
		t.Fatalf("computeChecksum sha1 error: %v", err)
	}
	if sha1Val != "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed" {
		t.Errorf("sha1 = %q, want %q", sha1Val, "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed")
	}

	md5Val, err := computeChecksum(strings.NewReader(content), checksumMD5)
	if err != nil {
		t.Fatalf("computeChecksum md5 error: %v", err)
	}
	if md5Val != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Errorf("md5 = %q, want %q", md5Val, "5eb63bbbe01eeed093cb22bb8f5acdc3")
	}

	sha256Val, err := computeChecksum(strings.NewReader(content), checksumSHA256)
	if err != nil {
		t.Fatalf("computeChecksum sha256 error: %v", err)
	}
	if sha256Val != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Errorf("sha256 = %q, want %q", sha256Val, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	}
}

func TestFormatMavenChecksum(t *testing.T) {
	result := formatMavenChecksum("abc123", "my-lib-1.0.0.jar")
	want := "abc123  my-lib-1.0.0.jar\n"
	if result != want {
		t.Errorf("formatMavenChecksum = %q, want %q", result, want)
	}
}
