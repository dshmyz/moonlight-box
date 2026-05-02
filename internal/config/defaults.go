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

	// Database
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./data/registry.db")

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
	v.SetDefault("cache.max_size_gb", 10)
	v.SetDefault("cache.eviction_policy", "lru")

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "console")
	v.SetDefault("logging.output", "stdout")

	// Proxy
	v.SetDefault("proxy.default_timeout", 30*time.Second)
	v.SetDefault("proxy.connect_timeout", 10*time.Second)
	v.SetDefault("proxy.large_file_threshold", 50*1024*1024)
	v.SetDefault("proxy.max_redirects", 10)
	v.SetDefault("proxy.insecure_skip_verify", false)
	v.SetDefault("proxy.dns_mapping", map[string]string{})

	// AI 配置
	v.SetDefault("ai.enabled", false)
	v.SetDefault("ai.provider", "chatglm")
	v.SetDefault("ai.base_url", "http://localhost:8000/v1")
	v.SetDefault("ai.model", "chatglm3-6b")
	v.SetDefault("ai.max_tokens", 2048)
	v.SetDefault("ai.temperature", 0.7)
	v.SetDefault("ai.timeout", 30*time.Second)
	v.SetDefault("ai.tools.enabled", true)
	v.SetDefault("ai.tools.max_execution_time", 10)
	v.SetDefault("ai.tools.enable_audit_log", true)
	v.SetDefault("ai.rate_limit.requests_per_minute", 20)
	v.SetDefault("ai.rate_limit.requests_per_day", 500)
	v.SetDefault("ai.rate_limit.tokens_per_day", 100000)
	v.SetDefault("ai.cache.enabled", true)
	v.SetDefault("ai.cache.ttl", time.Hour)
	v.SetDefault("ai.cache.max_size", 1000)
	v.SetDefault("ai.session.max_age", 24*time.Hour)
	v.SetDefault("ai.session.max_messages", 50)
}
