package util

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	mainLogger    *logrus.Logger
	sqlLogger     *logrus.Logger
	errorLogger   *logrus.Logger
	accessLogger  *logrus.Logger
	loggerOnce    sync.Once
	logFiles      = make(map[string]*lumberjack.Logger)
	logFilesMu    sync.Mutex
	logConfig     *LoggerConfig
)

// LoggerConfig 日志配置（解耦config包依赖）
type LoggerConfig struct {
	Level            string
	Format           string
	Output           string
	EnableSplitFiles bool
	SqlLogFile       string
	ErrorLogFile     string
	AccessLogFile    string
	LogRetentionDays int
}

// LogType 日志类型枚举
type LogType string

const (
	LogTypeMain   LogType = "main"
	LogTypeSQL    LogType = "sql"
	LogTypeError  LogType = "error"
	LogTypeAccess LogType = "access"
)

// InitLogger 初始化日志系统，支持分文件输出
func InitLogger(cfg *LoggerConfig) error {
	var initErr error
	loggerOnce.Do(func() {
		logConfig = cfg
		initErr = initLoggers(cfg)
	})
	return initErr
}

func initLoggers(cfg *LoggerConfig) error {
	if cfg == nil {
		cfg = &LoggerConfig{
			Level:            "info",
			Format:           "console",
			Output:           "stdout",
			EnableSplitFiles: false,
		}
	}

	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}

	// 初始化主日志
	mainLogger = setupLogger(level, cfg.Format, cfg.Output, cfg.LogRetentionDays)

	// 如果启用分文件日志，初始化各类型日志
	if cfg.EnableSplitFiles {
		sqlLogger = setupLogger(
			getLogLevel(cfg.Level, "sql"),
			cfg.Format,
			defaultIfEmpty(cfg.SqlLogFile, "./logs/sql.log"),
			cfg.LogRetentionDays,
		)
		errorLogger = setupLogger(
			logrus.ErrorLevel, // 错误日志始终记录 error 及以上
			cfg.Format,
			defaultIfEmpty(cfg.ErrorLogFile, "./logs/error.log"),
			cfg.LogRetentionDays,
		)
		accessLogger = setupLogger(
			logrus.InfoLevel, // 访问日志记录 info 及以上
			cfg.Format,
			defaultIfEmpty(cfg.AccessLogFile, "./logs/access.log"),
			cfg.LogRetentionDays,
		)
	}

	logrus.WithFields(logrus.Fields{
		"level":            level.String(),
		"format":           cfg.Format,
		"output":           cfg.Output,
		"split_files":      cfg.EnableSplitFiles,
		"sql_log_file":     cfg.SqlLogFile,
		"error_log_file":   cfg.ErrorLogFile,
		"access_log_file":  cfg.AccessLogFile,
	}).Info("Logger initialized")

	return nil
}

func getLogLevel(baseLevel, logType string) logrus.Level {
	// SQL 日志默认只记录 warn 及以上，避免刷屏
	levels := map[string]map[string]logrus.Level{
		"debug": {
			"sql":    logrus.DebugLevel,
			"error":  logrus.ErrorLevel,
			"access": logrus.InfoLevel,
		},
		"info": {
			"sql":    logrus.WarnLevel,
			"error":  logrus.ErrorLevel,
			"access": logrus.InfoLevel,
		},
		"warn": {
			"sql":    logrus.WarnLevel,
			"error":  logrus.ErrorLevel,
			"access": logrus.WarnLevel,
		},
		"error": {
			"sql":    logrus.ErrorLevel,
			"error":  logrus.ErrorLevel,
			"access": logrus.ErrorLevel,
		},
	}
	if m, ok := levels[baseLevel]; ok {
		if l, ok := m[logType]; ok {
			return l
		}
	}
	return logrus.InfoLevel
}

func defaultIfEmpty(val, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}

func setupLogger(level logrus.Level, format, output string, retentionDays int) *logrus.Logger {
	l := logrus.New()
	l.SetLevel(level)

	// 设置格式化器
	switch format {
	case "json":
		l.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		l.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// 设置输出（支持文件轮转）
	l.SetOutput(getWriter(output, retentionDays))

	return l
}

func getWriter(output string, retentionDays int) io.Writer {
	switch output {
	case "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		// 文件输出，使用 lumberjack 支持轮转
		ensureLogDir(output)
		return getRotatingWriter(output, retentionDays)
	}
}

func ensureLogDir(path string) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		_ = os.MkdirAll(dir, 0755)
	}
}

func getRotatingWriter(path string, retentionDays int) *lumberjack.Logger {
	logFilesMu.Lock()
	defer logFilesMu.Unlock()

	if writer, ok := logFiles[path]; ok {
		return writer
	}

	maxSize := 100 // MB
	maxBackups := 10
	maxAge := retentionDays
	if maxAge <= 0 {
		maxAge = 7
	}

	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   true,
		LocalTime:  true,
	}
	logFiles[path] = writer
	return writer
}

// GetLogger 获取指定类型的日志记录器
func GetLogger(logType LogType) *logrus.Logger {
	if logType == LogTypeMain || logConfig == nil || !logConfig.EnableSplitFiles {
		return mainLogger
	}
	switch logType {
	case LogTypeSQL:
		if sqlLogger != nil {
			return sqlLogger
		}
	case LogTypeError:
		if errorLogger != nil {
			return errorLogger
		}
	case LogTypeAccess:
		if accessLogger != nil {
			return accessLogger
		}
	}
	return mainLogger
}

// SQL 日志专用方法
func LogSQL(query string, args ...interface{}) {
	if sqlLogger == nil {
		return
	}
	sqlLogger.WithFields(logrus.Fields{
		LogKeyModule: "gorm",
		"query":      query,
		"args":       args,
	}).Debug("SQL executed")
}

// 错误日志专用方法
func LogError(module, msg string, err error, fields ...logrus.Fields) {
	l := errorLogger
	if l == nil {
		l = mainLogger
	}
	entry := l.WithFields(logrus.Fields{
		LogKeyModule: module,
		LogKeyError:  err,
	})
	for _, f := range fields {
		entry = entry.WithFields(f)
	}
	entry.Error(msg)
}

// 访问日志专用方法
func LogAccess(method, path, clientIP string, statusCode int, duration time.Duration, fields ...logrus.Fields) {
	l := accessLogger
	if l == nil {
		l = mainLogger
	}
	entry := l.WithFields(logrus.Fields{
		LogKeyModule:     "access",
		"method":         method,
		"path":           path,
		"client_ip":      clientIP,
		LogKeyStatusCode: statusCode,
		LogKeyDuration:   duration.Milliseconds(),
	})
	for _, f := range fields {
		entry = entry.WithFields(f)
	}
	if statusCode >= 400 {
		entry.Warn("HTTP request completed")
	} else {
		entry.Info("HTTP request completed")
	}
}

// 关闭所有日志文件
func CloseLoggers() {
	logFilesMu.Lock()
	defer logFilesMu.Unlock()
	for _, writer := range logFiles {
		_ = writer.Close()
	}
	logFiles = make(map[string]*lumberjack.Logger)
}

// 全局日志代理方法（兼容旧代码）
func Debug(args ...interface{})                 { mainLogger.Debug(args...) }
func Info(args ...interface{})                  { mainLogger.Info(args...) }
func Warn(args ...interface{})                  { mainLogger.Warn(args...) }
func Error(args ...interface{})                 { mainLogger.Error(args...) }
func Debugf(format string, args ...interface{}) { mainLogger.Debugf(format, args...) }
func Infof(format string, args ...interface{})  { mainLogger.Infof(format, args...) }
func Warnf(format string, args ...interface{})  { mainLogger.Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { mainLogger.Errorf(format, args...) }

func WithFields(fields logrus.Fields) *logrus.Entry { return mainLogger.WithFields(fields) }
func WithField(key string, value interface{}) *logrus.Entry {
	return mainLogger.WithField(key, value)
}
func WithError(err error) *logrus.Entry { return mainLogger.WithError(err) }
func WithTime(t time.Time) *logrus.Entry { return mainLogger.WithTime(t) }

// SetLevel 设置全局日志级别
func SetLevel(level logrus.Level) { mainLogger.SetLevel(level) }

// AddHook 添加日志钩子
func AddHook(hook logrus.Hook) { mainLogger.AddHook(hook) }
