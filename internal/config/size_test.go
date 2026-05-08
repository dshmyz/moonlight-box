package config

import (
	"testing"
)

func TestParseSize(t *testing.T) {
	testCases := []struct {
		input    string
		expected int64
		shouldErr bool
	}{
		{"50MB", 50 * 1024 * 1024, false},
		{"50m", 50 * 1024 * 1024, false},
		{"50MB", 50 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"100K", 100 * 1024, false},
		{"500B", 500, false},
		{"1024", 1024, false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			size, err := ParseSize(tc.input)
			if tc.shouldErr && err == nil {
				t.Errorf("expected error for input %q", tc.input)
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tc.input, err)
			}
			if size != tc.expected {
				t.Errorf("ParseSize(%q) = %d, want %d", tc.input, size, tc.expected)
			}
		})
	}
}