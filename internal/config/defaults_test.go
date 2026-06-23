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

func TestConfigFileWriteTimeoutOverridesDefault(t *testing.T) {
	// 默认写 timeout=0s（无超时）。但如果用户在配置文件中显式设置了 write_timeout: 30s，
	// Viper 会用它覆盖默认值 0s。此测试验证这一点：生产环境如果从旧配置迁移必须删除或改为 0s。
	path := filepath.Join(t.TempDir(), "config.yaml")
	yamlContent := []byte("server:\n  write_timeout: 30s\n")
	if err := os.WriteFile(path, yamlContent, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Fatalf("config file write_timeout = %s, want 30s (config file must override default 0s)", cfg.Server.WriteTimeout)
	}
}

func TestConfigFileWriteTimeoutZero(t *testing.T) {
	// 验证显式写 write_timeout: 0s 也能正常工作（无超时）
	path := filepath.Join(t.TempDir(), "config.yaml")
	yamlContent := []byte("server:\n  write_timeout: 0s\n")
	if err := os.WriteFile(path, yamlContent, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.WriteTimeout != 0 {
		t.Fatalf("zero write_timeout = %s, want 0s", cfg.Server.WriteTimeout)
	}
}

func TestExampleConfigHasWriteTimeoutZero(t *testing.T) {
	// 验证示例配置文件中 write_timeout 已经设为 0s（无超时）
	path := filepath.Join("..", "..", "configs", "config.example.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("config.example.yaml not found, skipping")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config.example.yaml: %v", err)
	}
	if cfg.Server.WriteTimeout != 0 {
		t.Fatalf("config.example.yaml write_timeout = %s, want 0s", cfg.Server.WriteTimeout)
	}
}
