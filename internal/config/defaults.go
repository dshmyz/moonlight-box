package config

import "time"

func setDefaults(v interface {
	SetDefault(key string, value any)
}) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 9081)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 30*time.Second)
	v.SetDefault("server.idle_timeout", 60*time.Second) // 空闲连接超时，减少服务器关闭时等待时间
	v.SetDefault("server.static_dir", "./cmd/registry/dist")
	v.SetDefault("server.max_upload_size", 200*1024*1024) // 200MB

	// Database
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./data/registry.db")
	v.SetDefault("database.log_level", "warn")
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.conn_max_lifetime", 1*time.Hour)
	v.SetDefault("database.conn_max_idle_time", 30*time.Minute)

	// Storage
	v.SetDefault("storage.backend", "local")
	v.SetDefault("storage.local.base_path", "./data/packages")
	v.SetDefault("storage.local.max_size_gb", 100)

	// Auth
	v.SetDefault("auth.jwt_secret", "change-me-in-production")
	v.SetDefault("auth.token_expiry", 24*time.Hour)
	v.SetDefault("auth.refresh_expiry", 168*time.Hour) // 7 days
	v.SetDefault("auth.min_password_len", 8)
	v.SetDefault("auth.max_login_attempts", 5)
	v.SetDefault("auth.lockout_duration", 15*time.Minute)

	// Security
	v.SetDefault("security.enabled", true)
	v.SetDefault("security.scan_on_upload", false) // MVP 暂不启用
	v.SetDefault("security.block_critical", true)
	v.SetDefault("security.block_high", true)

	// Cache
	v.SetDefault("cache.enabled", true)
	v.SetDefault("cache.default_ttl", 24*time.Hour)
	v.SetDefault("cache.max_size_gb", 2)
	v.SetDefault("cache.eviction_policy", "lru")

	// Logging
	v.SetDefault("logging.level", "debug")
	v.SetDefault("logging.format", "console")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.log_retention_days", 30)         // 默认保留30天
	v.SetDefault("logging.cleanup_interval", 24*time.Hour) // 默认每24小时清理一次

	// Proxy
	v.SetDefault("proxy.default_timeout", 30*time.Second)
	v.SetDefault("proxy.connect_timeout", 10*time.Second)
	v.SetDefault("proxy.large_file_threshold", 50*1024*1024)
	v.SetDefault("proxy.max_redirects", 10)
	v.SetDefault("proxy.insecure_skip_verify", false)

	// Proxy Health Check
	v.SetDefault("proxy.health_check.enabled", true)
	v.SetDefault("proxy.health_check.interval", 30*time.Second)
	v.SetDefault("proxy.health_check.timeout", 5*time.Second)
	v.SetDefault("proxy.health_check.failure_threshold", 3)

	// Seed Data
	v.SetDefault("seed.enabled", true)
	v.SetDefault("seed.load_test_data", false)
}
