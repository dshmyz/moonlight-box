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
	sqlLogger     *logrus.Logger
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

	// 配置 logrus 包级标准 logger——所有 logrus.XXX / util.XXX 调用都走这一个实例。
	// 这样配置的 output/level/format 对全部代码生效，不再有“独立实例不生效”的分裂。
	logrus.SetLevel(level)
	logrus.SetFormatter(newFormatter(cfg.Format))
	logrus.SetOutput(getWriter(cfg.Output, cfg.LogRetentionDays))

	// 如果启用分文件日志，初始化 SQL / Access 独立实例，并给主 logger 装 error hook
	if cfg.EnableSplitFiles {
		sqlLogger = setupLogger(
			getLogLevel(cfg.Level, "sql"),
			cfg.Format,
			defaultIfEmpty(cfg.SqlLogFile, "./logs/sql.log"),
			cfg.LogRetentionDays,
		)
		accessLogger = setupLogger(
			logrus.InfoLevel, // 访问日志记录 info 及以上
			cfg.Format,
			defaultIfEmpty(cfg.AccessLogFile, "./logs/access.log"),
			cfg.LogRetentionDays,
		)
		// error hook：把主 logger 的 Error/Fatal/Panic 复制写到 error.log
		// 不再用独立 errorLogger——hook 让 653 处 .Error() 调用自动进 error.log，零调用点改动
		errorWriter := getWriter(defaultIfEmpty(cfg.ErrorLogFile, "./logs/error.log"), cfg.LogRetentionDays)
		logrus.AddHook(newErrorHook(errorWriter, newFormatter(cfg.Format)))
	}

	logrus.WithFields(logrus.Fields{
		"level":           level.String(),
		"format":          cfg.Format,
		"output":          cfg.Output,
		"split_files":     cfg.EnableSplitFiles,
		"sql_log_file":    cfg.SqlLogFile,
		"error_log_file":  cfg.ErrorLogFile,
		"access_log_file": cfg.AccessLogFile,
	}).Info("Logger initialized")

	return nil
}

// newFormatter 按配置创建格式化器
func newFormatter(format string) logrus.Formatter {
	switch format {
	case "json":
		return &logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		}
	default:
		return &logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		}
	}
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
	l.SetFormatter(newFormatter(format))
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
		return logrus.StandardLogger()
	}
	switch logType {
	case LogTypeSQL:
		if sqlLogger != nil {
			return sqlLogger
		}
	case LogTypeAccess:
		if accessLogger != nil {
			return accessLogger
		}
	}
	return logrus.StandardLogger()
}

// errorHook 把 Error/Fatal/Panic 级别的日志复制写到独立的 error.log。
// 用 hook 而非独立 logger，让所有 logrus.XXX / util.XXX 的 .Error() 调用自动进 error.log。
type errorHook struct {
	writer    io.Writer
	formatter logrus.Formatter
}

func newErrorHook(writer io.Writer, formatter logrus.Formatter) *errorHook {
	return &errorHook{writer: writer, formatter: formatter}
}

func (h *errorHook) Levels() []logrus.Level {
	return []logrus.Level{logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel}
}

func (h *errorHook) Fire(entry *logrus.Entry) error {
	// 用 formatter 序列化 entry，写到 error.log
	formatted, err := h.formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = h.writer.Write(formatted)
	return err
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

// 全局日志代理方法：转发到 logrus 包级标准 logger（与 logrus.XXX 完全等价）
func Debug(args ...interface{})                 { logrus.Debug(args...) }
func Info(args ...interface{})                  { logrus.Info(args...) }
func Warn(args ...interface{})                  { logrus.Warn(args...) }
func Error(args ...interface{})                 { logrus.Error(args...) }
func Debugf(format string, args ...interface{}) { logrus.Debugf(format, args...) }
func Infof(format string, args ...interface{})  { logrus.Infof(format, args...) }
func Warnf(format string, args ...interface{})  { logrus.Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { logrus.Errorf(format, args...) }

func WithFields(fields logrus.Fields) *logrus.Entry { return logrus.WithFields(fields) }
func WithField(key string, value interface{}) *logrus.Entry {
	return logrus.WithField(key, value)
}
func WithError(err error) *logrus.Entry { return logrus.WithError(err) }
func WithTime(t time.Time) *logrus.Entry { return logrus.WithTime(t) }

// SetLevel 设置全局日志级别
func SetLevel(level logrus.Level) { logrus.SetLevel(level) }

// AddHook 添加日志钩子
func AddHook(hook logrus.Hook) { logrus.AddHook(hook) }
