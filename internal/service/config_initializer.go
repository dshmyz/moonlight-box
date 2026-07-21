package service

import (
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
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
		{"system.name", "Moonlight Registry", "string", "general", "系统名称", false},
		{"system.description", "Enterprise Package Registry", "string", "general", "系统描述", false},
		{"system.version", "1.0.0", "string", "general", "系统版本", false},

		{"backup.enabled", "true", "bool", "storage", "启用自动备份", false},
		{"backup.schedule", "0 2 * * *", "string", "storage", "备份计划（每天凌晨2点）", false},
		{"backup.retention_days", "30", "int", "storage", "备份保留天数", false},
		{"backup.max_count", "10", "int", "storage", "最大备份数量", false},

		{"security.scan_on_upload", "true", "bool", "security", "上传时扫描包", false},
		{"security.block_critical", "true", "bool", "security", "阻止严重漏洞包", false},
		{"security.block_high", "true", "bool", "security", "阻止高危漏洞包", false},
		{"security.block_medium", "false", "bool", "security", "阻止中危漏洞包", false},

		{"cache.enabled", "true", "bool", "cache", "启用代理缓存", false},
		{"cache.default_ttl", "24h", "string", "cache", "默认缓存时间", false},
		{"cache.max_size_gb", "100", "int", "cache", "最大缓存大小(GB)", false},

		{"webhook.enabled", "true", "bool", "network", "启用Webhook通知", false},
		{"webhook.timeout", "10s", "string", "network", "Webhook请求超时时间", false},
		{"webhook.retry_count", "3", "int", "network", "Webhook重试次数", false},

		{"storage.default_backend", "local", "string", "storage", "默认存储后端", false},
		{"storage.max_file_size", "1073741824", "int", "storage", "最大文件大小(字节)", false},

		{"auth.token_expiry", "24h", "string", "security", "JWT令牌过期时间", false},
		{"auth.refresh_token_expiry", "168h", "string", "security", "刷新令牌过期时间(7天)", false},
		{"auth.max_login_attempts", "5", "int", "security", "最大登录尝试次数", false},

		{"cas.enabled", "false", "bool", "login", "启用 CAS 单点登录", false},
		{"cas.server_url", "", "string", "login", "CAS 服务器地址", false},
		{"cas.service_url", "", "string", "login", "Service URL", false},
		{"cas.login_path", DefaultCASLoginPath, "string", "login", "CAS 登录路径", false},
		{"cas.validate_path", DefaultCASValidatePath, "string", "login", "CAS 验证路径", false},

		{"metrics.enabled", "true", "bool", "general", "启用Prometheus监控", false},
		{"metrics.path", "/metrics", "string", "general", "Prometheus监控路径", false},

		{"health_check.enabled", "true", "bool", "network", "启用健康检查", false},
		{"health_check.interval", "30", "int", "network", "健康检查间隔（秒）", false},
		{"health_check.timeout", "5", "int", "network", "健康检查超时时间（秒）", false},
		{"health_check.failure_threshold", "3", "int", "network", "健康检查失败阈值", false},
		{"health_check.block_on_unhealthy", "false", "bool", "network", "不健康时是否阻断请求", false},

		{"log_cleanup.enabled", "true", "bool", "logging", "启用下载日志自动清理", false},
		{"log_cleanup.retention_days", "30", "int", "logging", "下载日志保留天数", false},
		{"log_cleanup.interval", "24h", "string", "logging", "清理执行间隔（如 24h, 12h, 1h）", false},
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
