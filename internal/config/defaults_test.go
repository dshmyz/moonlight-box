package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultProxySkipsTLSVerification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if !cfg.Proxy.InsecureSkipVerify {
		t.Fatal("expected proxy.insecure_skip_verify default to true")
	}
}

func TestDefaultServerWriteTimeoutDisabledForLargeDownloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if cfg.Server.WriteTimeout != 0 {
		t.Fatalf("server.write_timeout = %s, want 0 for long package downloads", cfg.Server.WriteTimeout)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Fatalf("server.read_timeout = %s, want 30s", cfg.Server.ReadTimeout)
	}
}
