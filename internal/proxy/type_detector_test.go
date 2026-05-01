package proxy

import "testing"

func TestTypeDetector_Detect(t *testing.T) {
	d := NewTypeDetector()

	tests := []struct {
		path     string
		expected string
	}{
		{"npm/lodash", "npm"},
		{"maven/org/springframework/core", "maven"},
		{"pypi/simple/requests", "pypi"},
		{"go/golang.org/x/text/@v/v0.3.0.mod", "go"},
		{"nuget/odata/FindPackagesById", "nuget"},
		{"yum/repodata/repomd.xml", "yum"},
		{"apt/dists/stable/Release", "apt"},
		{"lodash/-/lodash-4.17.21.tgz", "npm"},
		{"unknown/path/here", "maven"}, // 3+ segments default to maven
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := d.Detect(tt.path)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestTypeDetector_IsSupportedType(t *testing.T) {
	d := NewTypeDetector()

	tests := []struct {
		pkgType      string
		packageTypes string
		expected     bool
	}{
		{"npm", `["npm","maven"]`, true},
		{"pypi", `["npm","maven"]`, false},
		{"npm", `["npm"]`, true},
		{"npm", "", false},
	}

	for _, tt := range tests {
		got := d.IsSupportedType(tt.pkgType, tt.packageTypes)
		if got != tt.expected {
			t.Errorf("IsSupportedType(%q, %q) = %v, want %v",
				tt.pkgType, tt.packageTypes, got, tt.expected)
		}
	}
}
