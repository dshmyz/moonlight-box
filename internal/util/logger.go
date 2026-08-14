package util

import (
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	SampleRate    float64            `json:"sample_rate"`    // 0.0~1.0，1.0 表示不采样（默认）
	SampledModules map[string]float64 `json:"sample_by_module"` // module → 专属采样率，未列出用全局
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

// autoDeriveSplitFiles 当 output 是文件路径时，自动启用分文件并从同目录派生各日志路径。
// 用户只需配置 logging.output，无需手动设置 enable_split_files / sql_log_file 等字段。
func autoDeriveSplitFiles(cfg *LoggerConfig) {
	empty := ""
	stdout := "stdout"
	stderr := "stderr"
	if cfg.Output == empty || cfg.Output == stdout || cfg.Output == stderr {
		return
	}
	dir := filepath.Dir(cfg.Output)
	if !cfg.EnableSplitFiles {
		cfg.EnableSplitFiles = true
	}
	sqlLog := filepath.Join(dir, "sql.log")
	errLog := filepath.Join(dir, "error.log")
	accLog := filepath.Join(dir, "access.log")
	if cfg.SqlLogFile == empty {
		cfg.SqlLogFile = sqlLog
	}
	if cfg.ErrorLogFile == empty {
		cfg.ErrorLogFile = errLog
	}
	if cfg.AccessLogFile == empty {
		cfg.AccessLogFile = accLog
	}
}

func initLoggers(cfg *LoggerConfig) error {
	if cfg == nil {
		cfg = &LoggerConfig{
			Level:  "info",
			Format: "console",
			Output: "stdout",
		}
	}

	// output 为文件路径时，自动启用分文件并派生各日志路径
	autoDeriveSplitFiles(cfg)

	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}

	// 配置 logrus 包级标准 logger——所有 logrus.XXX / util.XXX 调用都走这一个实例。
	// 这样配置的 output/level/format 对全部代码生效，不再有"独立实例不生效"的分裂。
	logrus.SetLevel(level)
	logrus.SetFormatter(&samplingFormatter{base: newFormatter(cfg.Format)})
	logrus.SetOutput(getWriter(cfg.Output, cfg.LogRetentionDays))

	// 采样 hook：对 Debug/Info/Warn 全局采样，sample_by_module 可指定 module 专属采样率
	sampling := &samplingHook{
		rate:        cfg.SampleRate,
		moduleRates: cfg.SampledModules,
	}
	if sampling.rate <= 0 {
		sampling.rate = 1.0 // 默认不采样
	}
	if sampling.rate < 1.0 || len(sampling.moduleRates) > 0 {
		logrus.AddHook(sampling)
	}

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

// ---------- sampling hook ----------

// samplingHook 按 sample_rate 全局采样 Debug/Info/Warn。
// sample_by_module 中的 module 使用独立采样率，未列出的 module 用全局 rate。
// 通过在 entry.Data 中设置 _sampled_drop 标记，由 samplingFormatter 决定不输出。
type samplingHook struct {
	rate           float64            // 全局采样率 0.0~1.0
	moduleRates    map[string]float64 // module → 专属采样率
	counter        uint64
}

var sampledDropKey = "_sampled_drop"

func (h *samplingHook) Levels() []logrus.Level {
	return []logrus.Level{logrus.DebugLevel, logrus.InfoLevel, logrus.WarnLevel}
}

func (h *samplingHook) Fire(entry *logrus.Entry) error {
	// 确定本条日志使用的采样率：优先 module 专属，其次全局
	rate := h.rate
	if mod, ok := entry.Data[LogKeyModule].(string); ok {
		if mr, exists := h.moduleRates[mod]; exists {
			rate = mr
		}
	}
	// rate=1.0 表示不采样，直接放行
	if rate >= 1.0 {
		return nil
	}
	// rate=0.0 表示全部丢弃
	if rate <= 0.0 {
		h.drop(entry)
		return nil
	}
	// 全局采样：atomic 计数 + 概率判定
	atomic.AddUint64(&h.counter, 1)
	if rand.Float64() >= rate {
		h.drop(entry)
	}
	return nil
}

func (h *samplingHook) drop(entry *logrus.Entry) {
	// 创建新 map 并替换，避免和 errorHook 并发读写同一 map
	newData := make(logrus.Fields, len(entry.Data)+1)
	for k, v := range entry.Data {
		newData[k] = v
	}
	newData[sampledDropKey] = true
	entry.Data = newData
}

// samplingFormatter 包装 base Formatter，跳过被 samplingHook 标记的条目。
type samplingFormatter struct {
	base logrus.Formatter
}

func (f *samplingFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if dropped, ok := entry.Data[sampledDropKey]; ok && dropped.(bool) {
		return nil, nil // 静默丢弃
	}
	return f.base.Format(entry)
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
