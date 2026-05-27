package gomod

import "testing"

func TestEncodeGoPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/gin-gonic/gin/@v/list", "github.com/gin-gonic/gin/@v/list"},
		{"github.com/Azure/azure-sdk-go/@v/list", "github.com/!azure/azure-sdk-go/@v/list"},
		{"github.com/BurntSushi/toml/@latest", "github.com/!burnt!sushi/toml/@latest"},
		{"github.com/foo/bar/@v/v1.0.0.zip", "github.com/foo/bar/@v/v1.0.0.zip"},
		{"", ""},
	}
	for _, tt := range tests {
		got := encodeGoPath(tt.input)
		if got != tt.expected {
			t.Errorf("encodeGoPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
