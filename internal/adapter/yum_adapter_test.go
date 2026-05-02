package adapter

import (
	"testing"
)

func TestParseRpmFilename(t *testing.T) {
	tests := []struct {
		name            string
		filename        string
		expectedName    string
		expectedVersion string
		expectedRelease string
		expectedArch    string
	}{
		{
			name:            "standard rpm",
			filename:        "nginx-1.20.1-1.el9.x86_64.rpm",
			expectedName:    "nginx",
			expectedVersion: "1.20.1",
			expectedRelease: "1.el9",
			expectedArch:    "x86_64",
		},
		{
			name:            "complex name",
			filename:        "python3-pip-21.2.3-5.el9.noarch.rpm",
			expectedName:    "python3-pip",
			expectedVersion: "21.2.3",
			expectedRelease: "5.el9",
			expectedArch:    "noarch",
		},
		{
			name:            "aarch64 architecture",
			filename:        "kernel-5.14.0-284.el9.aarch64.rpm",
			expectedName:    "kernel",
			expectedVersion: "5.14.0",
			expectedRelease: "284.el9",
			expectedArch:    "aarch64",
		},
		{
			name:            "multi-part name",
			filename:        "java-11-openjdk-headless-11.0.20.0.8-2.el9.x86_64.rpm",
			expectedName:    "java-11-openjdk-headless",
			expectedVersion: "11.0.20.0.8",
			expectedRelease: "2.el9",
			expectedArch:    "x86_64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, version, release, arch := parseRpmFilename(tt.filename)
			if name != tt.expectedName {
				t.Errorf("expected name %s, got %s", tt.expectedName, name)
			}
			if version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, version)
			}
			if release != tt.expectedRelease {
				t.Errorf("expected release %s, got %s", tt.expectedRelease, release)
			}
			if arch != tt.expectedArch {
				t.Errorf("expected arch %s, got %s", tt.expectedArch, arch)
			}
		})
	}
}

func TestDetectRpmArch(t *testing.T) {
	tests := []struct {
		filename     string
		expectedArch string
	}{
		{"package-1.0.0-1.x86_64.rpm", "x86_64"},
		{"package-1.0.0-1.aarch64.rpm", "aarch64"},
		{"package-1.0.0-1.noarch.rpm", "noarch"},
		{"package-1.0.0-1.i686.rpm", "i686"},
		{"package-1.0.0-1.armv7hl.rpm", "armv7hl"},
		{"package-1.0.0-1.unknown.rpm", "x86_64"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			arch := detectRpmArch(tt.filename)
			if arch != tt.expectedArch {
				t.Errorf("expected arch %s, got %s", tt.expectedArch, arch)
			}
		})
	}
}
