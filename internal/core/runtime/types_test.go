package runtime

import "testing"

func TestNewArtifactNormalizesRemotePath(t *testing.T) {
	a := NewArtifact(ArtifactSpec{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
	})

	if a.Path != "packages/ab/cd" {
		t.Fatalf("expected path from remote_path, got %q", a.Path)
	}
	if a.Filename != "requests-2.28.0.tar.gz" {
		t.Fatalf("expected filename from remote_path, got %q", a.Filename)
	}
	if a.RemotePath != "packages/ab/cd/requests-2.28.0.tar.gz" {
		t.Fatalf("expected remote_path preserved, got %q", a.RemotePath)
	}
	if a.Name != "requests" || a.Version != "2.28.0" {
		t.Fatalf("expected strong fields preserved, got name=%q version=%q", a.Name, a.Version)
	}
	if a.Properties["remote_path"] != "packages/ab/cd/requests-2.28.0.tar.gz" {
		t.Fatalf("expected remote_path mirrored to properties, got %#v", a.Properties)
	}
	if a.IdentityKey == "" {
		t.Fatal("expected identity key")
	}
}

func TestNewArtifactKeepsQualifiers(t *testing.T) {
	a := NewArtifact(ArtifactSpec{
		Format:   "npm",
		Kind:     "tarball",
		Name:     "@scope/pkg",
		Version:  "1.0.0",
		Path:     "@scope/pkg/-",
		Filename: "pkg-1.0.0.tgz",
		Qualifiers: map[string]string{
			"package_type": "tarball",
		},
	})

	if a.Name != "@scope/pkg" {
		t.Fatalf("expected name, got %q", a.Name)
	}
	if a.Version != "1.0.0" {
		t.Fatalf("expected version, got %q", a.Version)
	}
	if a.RemotePath != "@scope/pkg/-/pkg-1.0.0.tgz" {
		t.Fatalf("expected remote_path from path and filename, got %q", a.RemotePath)
	}
	if a.Qualifiers["package_type"] != "tarball" {
		t.Fatalf("expected qualifier preserved, got %#v", a.Qualifiers)
	}
}

func TestValidateArtifactForStoreRejectsInvalidPaths(t *testing.T) {
	tests := []struct {
		name string
		spec ArtifactSpec
	}{
		{
			name: "filename contains slash",
			spec: ArtifactSpec{
				Format:   "generic",
				Kind:     "file",
				Name:     "bad",
				Filename: "dir/bad.txt",
			},
		},
		{
			name: "path includes filename",
			spec: ArtifactSpec{
				Format:   "generic",
				Kind:     "file",
				Name:     "bad",
				Path:     "dir/bad.txt",
				Filename: "bad.txt",
			},
		},
		{
			name: "relative download url",
			spec: ArtifactSpec{
				Format:      "pypi",
				Kind:        "package-file",
				Name:        "requests",
				Version:     "2.28.0",
				Filename:    "requests-2.28.0.tar.gz",
				DownloadURL: "../../packages/requests-2.28.0.tar.gz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewArtifact(tt.spec)
			if err := ValidateArtifactForStore(a); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
