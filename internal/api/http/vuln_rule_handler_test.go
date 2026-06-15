package http

import (
	"testing"
	"time"
)

func TestVulnDataSourceHTTPClientHasTimeout(t *testing.T) {
	client := newVulnDataSourceHTTPClient()

	if client.Timeout != 10*time.Second {
		t.Fatalf("Timeout = %s, want 10s", client.Timeout)
	}
}
