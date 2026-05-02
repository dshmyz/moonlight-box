package service

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type ConfigInitializer struct {
	configRepo *repository.SystemConfigRepository
}

func NewConfigInitializer(configRepo *repository.SystemConfigRepository) *ConfigInitializer {
	return &ConfigInitializer{
		configRepo: configRepo,
	}
}

func (i *ConfigInitializer) InitializeDefaultConfigs() error {
	logrus.Info("Initializing default system configurations")

	defaultConfigs := []struct {
		key         string
		value       string
		valueType   string
		category    string
		description string
		isSensitive bool
	}{
		{"system.name", "Moonlight Registry", "string", "general", "System name", false},
		{"system.description", "Enterprise Package Registry", "string", "general", "System description", false},
		{"system.version", "1.0.0", "string", "general", "System version", false},

		{"backup.enabled", "true", "bool", "storage", "Enable automatic backup", false},
		{"backup.schedule", "0 2 * * *", "string", "storage", "Backup cron schedule (daily at 2 AM)", false},
		{"backup.retention_days", "30", "int", "storage", "Backup retention days", false},
		{"backup.max_count", "10", "int", "storage", "Maximum backup count", false},

		{"security.scan_on_upload", "true", "bool", "security", "Scan packages on upload", false},
		{"security.block_critical", "true", "bool", "security", "Block packages with critical vulnerabilities", false},
		{"security.block_high", "true", "bool", "security", "Block packages with high vulnerabilities", false},
		{"security.block_medium", "false", "bool", "security", "Block packages with medium vulnerabilities", false},

		{"cache.enabled", "true", "bool", "cache", "Enable proxy cache", false},
		{"cache.default_ttl", "24h", "string", "cache", "Default cache TTL", false},
		{"cache.max_size_gb", "100", "int", "cache", "Maximum cache size in GB", false},

		{"webhook.enabled", "true", "bool", "network", "Enable webhook notifications", false},
		{"webhook.timeout", "10s", "string", "network", "Webhook request timeout", false},
		{"webhook.retry_count", "3", "int", "network", "Webhook retry count", false},

		{"storage.default_backend", "local", "string", "storage", "Default storage backend", false},
		{"storage.max_file_size", "1073741824", "int", "storage", "Maximum file size in bytes (1GB)", false},

		{"auth.token_expiry", "24h", "string", "security", "JWT token expiry time", false},
		{"auth.refresh_token_expiry", "168h", "string", "security", "Refresh token expiry time (7 days)", false},
		{"auth.max_login_attempts", "5", "int", "security", "Maximum login attempts before lockout", false},

		{"metrics.enabled", "true", "bool", "general", "Enable Prometheus metrics", false},
		{"metrics.path", "/metrics", "string", "general", "Prometheus metrics endpoint path", false},
	}

	for _, config := range defaultConfigs {
		existing, err := i.configRepo.Get(config.key)
		if err == nil && existing != nil {
			logrus.WithField("key", config.key).Debug("Config already exists, skipping")
			continue
		}

		newConfig := &model.SystemConfig{
			Key:         config.key,
			Value:       config.value,
			ValueType:   config.valueType,
			Category:    config.category,
			Description: config.description,
			IsSensitive: config.isSensitive,
		}

		if err := i.configRepo.Set(newConfig); err != nil {
			logrus.WithError(err).WithField("key", config.key).Error("Failed to initialize config")
			continue
		}

		logrus.WithField("key", config.key).Info("Initialized default config")
	}

	logrus.Info("Default system configurations initialized")
	return nil
}

func (i *ConfigInitializer) GetConfig(key string) (string, error) {
	config, err := i.configRepo.Get(key)
	if err != nil {
		return "", err
	}
	return config.Value, nil
}

func (i *ConfigInitializer) GetConfigAsBool(key string, defaultValue bool) bool {
	config, err := i.configRepo.Get(key)
	if err != nil {
		return defaultValue
	}
	return config.Value == "true"
}

func (i *ConfigInitializer) GetConfigAsInt(key string, defaultValue int) int {
	config, err := i.configRepo.Get(key)
	if err != nil {
		return defaultValue
	}

	var value int
	if _, err := fmt.Sscanf(config.Value, "%d", &value); err != nil {
		return defaultValue
	}
	return value
}
