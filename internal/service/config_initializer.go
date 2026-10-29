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

	// 只初始化「运行时真的会读取」的配置项。
	// 死配置（能改但没人读）和由专属页面维护的重复项不再 seed，避免系统配置页出现无效项。
	defaultConfigs := []struct {
		key         string
		value       string
		valueType   string
		category    string
		description string
		isSensitive bool
	}{
		// 安全扫描：SecurityScanner.LoadSecurityConfig 读取
		{"security.scan_on_upload", "true", "bool", "security", "上传时扫描包", false},
		{"security.block_critical", "true", "bool", "security", "阻止严重漏洞包", false},
		{"security.block_high", "true", "bool", "security", "阻止高危漏洞包", false},
		{"security.block_medium", "false", "bool", "security", "阻止中危漏洞包", false},
		{"scheduler.cron.security_scan", "0 23 * * *", "string", "security", "全量安全扫描计划（默认每天23点）", false},

		// 远端健康检查：main.go 与 proxy.HealthChecker 读取
		{"health_check.enabled", "true", "bool", "network", "启用健康检查", false},
		{"health_check.interval", "30", "int", "network", "健康检查间隔（秒）", false},
		{"health_check.timeout", "5", "int", "network", "健康检查超时时间（秒）", false},
		{"health_check.failure_threshold", "3", "int", "network", "健康检查失败阈值", false},
		{"health_check.block_on_unhealthy", "false", "bool", "network", "不健康时是否阻断请求", false},

		// CAS 登录：由 CAS 设置页维护
		{"cas.enabled", "false", "bool", "login", "启用 CAS 单点登录", false},
		{"cas.server_url", "", "string", "login", "CAS 服务器地址", false},
		{"cas.service_url", "", "string", "login", "Service URL", false},
		{"cas.login_path", DefaultCASLoginPath, "string", "login", "CAS 登录路径", false},
		{"cas.validate_path", DefaultCASValidatePath, "string", "login", "CAS 验证路径", false},
		{"cas.allowed_hosts", "", "string", "login", "允许自动推导 Service URL 的域名白名单（逗号分隔，如 repo-a.corp.com,*.corp.com）", false},

		// 下载日志清理：由下载日志页维护
		{"log_cleanup.enabled", "true", "bool", "logging", "启用下载日志自动清理", false},
		{"log_cleanup.retention_days", "30", "int", "logging", "下载日志保留天数", false},

		// 定时任务：调度由定时任务页维护
		{"cleanup.interval", "24h", "string", "maven", "清理任务执行间隔（如 24h, 12h）", false},

		// Maven SNAPSHOT 清理：参数由定时任务页维护
		{"maven_snapshot_cleanup.enabled", "true", "bool", "maven", "启用 Maven SNAPSHOT 自动清理", false},
		{"maven_snapshot_cleanup.keep_last", "5", "int", "maven", "每个 SNAPSHOT 版本保留最近构建数", false},
		{"maven_snapshot_cleanup.max_age_days", "90", "int", "maven", "SNAPSHOT 保留天数", false},

		// 备份开关：由备份管理页读取
		{"backup.enabled", "true", "bool", "storage", "启用自动备份", false},
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
