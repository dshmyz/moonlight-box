package util

import (
	"github.com/sirupsen/logrus"
)

// LogFields 通用日志字段键名常量
const (
	LogKeyModule       = "module"
	LogKeyPkgType      = "pkg_type"
	LogKeyPkgName      = "pkg_name"
	LogKeyPkgVersion   = "pkg_version"
	LogKeyRepo         = "repo"
	LogKeyRepoType     = "repo_type"
	LogKeyUser         = "user"
	LogKeyUserID       = "user_id"
	LogKeyRequestID    = "request_id"
	LogKeyTraceID      = "trace_id"
	LogKeyOperation    = "operation"
	LogKeyDuration     = "duration_ms"
	LogKeyError        = "error"
	LogKeyStatusCode   = "status_code"
	LogKeyBytes        = "bytes"
	LogKeyCacheHit     = "cache_hit"
	LogKeyFromRemote   = "from_remote"
)

// Module 模块标识常量
const (
	ModuleDownload   = "download"
	ModuleUpload     = "upload"
	ModuleDepParse   = "dep_parse"
	ModuleProxy      = "proxy"
	ModuleCache      = "cache"
	ModuleStorage    = "storage"
	ModuleAuth       = "auth"
	ModuleSecurity   = "security"
	ModuleMigration  = "migration"
	ModuleAI         = "ai"
	ModuleHealth     = "health"
	ModuleWebhook    = "webhook"
)

// LogLevel 日志级别
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LoggerBuilder 结构化日志构建器
type LoggerBuilder struct {
	entry *logrus.Entry
}

// NewLogger 创建日志构建器
func NewLogger() *LoggerBuilder {
	return &LoggerBuilder{
		entry: logrus.WithFields(logrus.Fields{}),
	}
}

// WithModule 设置模块标识
func (b *LoggerBuilder) WithModule(module string) *LoggerBuilder {
	b.entry = b.entry.WithField(LogKeyModule, module)
	return b
}

// WithPackage 设置包相关信息
func (b *LoggerBuilder) WithPackage(pkgType, pkgName, pkgVersion string) *LoggerBuilder {
	fields := logrus.Fields{}
	if pkgType != "" {
		fields[LogKeyPkgType] = pkgType
	}
	if pkgName != "" {
		fields[LogKeyPkgName] = pkgName
	}
	if pkgVersion != "" {
		fields[LogKeyPkgVersion] = pkgVersion
	}
	b.entry = b.entry.WithFields(fields)
	return b
}

// WithRepo 设置仓库信息
func (b *LoggerBuilder) WithRepo(repoName, repoType string) *LoggerBuilder {
	fields := logrus.Fields{}
	if repoName != "" {
		fields[LogKeyRepo] = repoName
	}
	if repoType != "" {
		fields[LogKeyRepoType] = repoType
	}
	b.entry = b.entry.WithFields(fields)
	return b
}

// WithUser 设置用户信息
func (b *LoggerBuilder) WithUser(username string, userID uint) *LoggerBuilder {
	fields := logrus.Fields{}
	if username != "" {
		fields[LogKeyUser] = username
	}
	if userID > 0 {
		fields[LogKeyUserID] = userID
	}
	b.entry = b.entry.WithFields(fields)
	return b
}

// WithRequest 设置请求追踪信息
func (b *LoggerBuilder) WithRequest(requestID, traceID string) *LoggerBuilder {
	fields := logrus.Fields{}
	if requestID != "" {
		fields[LogKeyRequestID] = requestID
	}
	if traceID != "" {
		fields[LogKeyTraceID] = traceID
	}
	b.entry = b.entry.WithFields(fields)
	return b
}

// WithError 设置错误信息
func (b *LoggerBuilder) WithError(err error) *LoggerBuilder {
	if err != nil {
		b.entry = b.entry.WithField(LogKeyError, err.Error())
	}
	return b
}

// WithField 添加自定义字段
func (b *LoggerBuilder) WithField(key string, value interface{}) *LoggerBuilder {
	b.entry = b.entry.WithField(key, value)
	return b
}

// WithFields 添加多个自定义字段
func (b *LoggerBuilder) WithFields(fields logrus.Fields) *LoggerBuilder {
	b.entry = b.entry.WithFields(fields)
	return b
}

// Debug 记录调试日志
func (b *LoggerBuilder) Debug(msg string) {
	b.entry.Debug(msg)
}

// Info 记录信息日志
func (b *LoggerBuilder) Info(msg string) {
	b.entry.Info(msg)
}

// Warn 记录警告日志
func (b *LoggerBuilder) Warn(msg string) {
	b.entry.Warn(msg)
}

// Error 记录错误日志
func (b *LoggerBuilder) Error(msg string) {
	b.entry.Error(msg)
}

// Debugf 记录格式化调试日志（兼容旧代码）
func (b *LoggerBuilder) Debugf(format string, args ...interface{}) {
	b.entry.Debugf(format, args...)
}

// Infof 记录格式化信息日志（兼容旧代码）
func (b *LoggerBuilder) Infof(format string, args ...interface{}) {
	b.entry.Infof(format, args...)
}

// Warnf 记录格式化警告日志（兼容旧代码）
func (b *LoggerBuilder) Warnf(format string, args ...interface{}) {
	b.entry.Warnf(format, args...)
}

// Errorf 记录格式化错误日志（兼容旧代码）
func (b *LoggerBuilder) Errorf(format string, args ...interface{}) {
	b.entry.Errorf(format, args...)
}

// ShouldLog 根据采样率判断是否应该记录日志
// 仅对 Warn/Error 级别生效，配置 sample_rate=0 表示不记录，1 表示全部记录
func ShouldLog(module string, level LogLevel, sampleRate float64, sampleByModule map[string]float64) bool {
	// Debug/Info 级别始终记录
	if level == LogLevelDebug || level == LogLevelInfo {
		return true
	}
	
	// 检查模块级采样配置
	if rate, ok := sampleByModule[module]; ok {
		return sampleByModuleCheck(rate)
	}
	
	// 使用全局采样率
	return sampleByModuleCheck(sampleRate)
}

func sampleByModuleCheck(rate float64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	// 简单随机采样（生产环境可替换为更复杂的采样策略）
	// 注意：这里使用简单的随机数，实际生产建议使用分布式采样算法
	return true // 临时返回true，实际使用时需要引入随机数生成
}
