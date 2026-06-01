package config

import (
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Security SecurityConfig `mapstructure:"security"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Service  ServiceConfig  `mapstructure:"service"` // 服务层配置
	Proxy    ProxyConfig    `mapstructure:"proxy"`
	AI       AIConfig       `mapstructure:"ai"`   // AI 配置
	Seed     SeedConfig     `mapstructure:"seed"` // 初始化数据配置
}

type SeedConfig struct {
	Enabled      bool `mapstructure:"enabled"`        // 是否启用种子数据
	LoadTestData bool `mapstructure:"load_test_data"` // 是否加载测试包数据
}

type ServerConfig struct {
	Host          string        `mapstructure:"host"`
	Port          int           `mapstructure:"port"`
	Mode          string        `mapstructure:"mode"` // debug, release, test
	ReadTimeout   time.Duration `mapstructure:"read_timeout"`
	WriteTimeout  time.Duration `mapstructure:"write_timeout"`
	StaticDir     string        `mapstructure:"static_dir"` // 前端静态文件目录
	MaxUploadSize int64         `mapstructure:"max_upload_size"`
}

type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"` // sqlite, postgres
	DSN             string        `mapstructure:"dsn"`
	LogLevel        string        `mapstructure:"log_level"`          // silent, error, warn, info
	MaxOpenConns    int           `mapstructure:"max_open_conns"`     // 最大打开连接数
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`     // 最大空闲连接数
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`  // 连接最大生命周期
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"` // 连接最大空闲时间
}

type StorageConfig struct {
	Backend string       `mapstructure:"backend"` // local, s3
	Local   LocalStorage `mapstructure:"local"`
}

type LocalStorage struct {
	BasePath  string `mapstructure:"base_path"`
	MaxSizeGB int64  `mapstructure:"max_size_gb"`
}

type AuthConfig struct {
	JWTSecret        string        `mapstructure:"jwt_secret"`
	TokenExpiry      time.Duration `mapstructure:"token_expiry"`
	RefreshExpiry    time.Duration `mapstructure:"refresh_expiry"`
	MinPasswordLen   int           `mapstructure:"min_password_len"`
	MaxLoginAttempts int           `mapstructure:"max_login_attempts"`
	LockoutDuration  time.Duration `mapstructure:"lockout_duration"`
	CAS              CASConfig     `mapstructure:"cas"`
}

type CASConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ServerURL    string `mapstructure:"server_url"`
	ServiceURL   string `mapstructure:"service_url"`
	LoginPath    string `mapstructure:"login_path"`
	ValidatePath string `mapstructure:"validate_path"`
}

type SecurityConfig struct {
	Enabled       bool `mapstructure:"enabled"`
	ScanOnUpload  bool `mapstructure:"scan_on_upload"`
	BlockCritical bool `mapstructure:"block_critical"`
	BlockHigh     bool `mapstructure:"block_high"`
}

type CacheConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	DefaultTTL     time.Duration `mapstructure:"default_ttl"`
	MaxSizeGB      int64         `mapstructure:"max_size_gb"`
	EvictionPolicy string        `mapstructure:"eviction_policy"` // lru, fifo, ttl
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, console
	Output string `mapstructure:"output"` // 主日志文件路径
	// 分文件日志配置
	// 启用后，不同类型日志写入不同文件，便于管理和轮转
	EnableSplitFiles bool          `mapstructure:"enable_split_files"`
	SqlLogFile       string        `mapstructure:"sql_log_file"`       // SQL日志文件，默认 ./logs/sql.log
	ErrorLogFile     string        `mapstructure:"error_log_file"`     // 错误日志文件，默认 ./logs/error.log
	AccessLogFile    string        `mapstructure:"access_log_file"`    // 访问日志文件，默认 ./logs/access.log
	LogRetentionDays int           `mapstructure:"log_retention_days"` // 日志保留天数
	CleanupInterval  time.Duration `mapstructure:"cleanup_interval"`   // 清理间隔
	// 日志采样配置（避免高频错误日志刷屏）
	// sample_rate: 0=不采样(全部记录), 1=100%采样, 0.1=10%采样
	// 仅对 Warn/Error 级别生效
	// 按 module 配置不同采样率，格式: {"module_name": 0.1}
	SampleRate     float64            `mapstructure:"sample_rate"`
	SampleByModule map[string]float64 `mapstructure:"sample_by_module"`
}

// ServiceConfig 服务层通用配置
type ServiceConfig struct {
	// 依赖解析并发限制（Maven/Go 包下载后异步解析依赖）
	MaxConcurrentDepParse int `mapstructure:"max_concurrent_dep_parse"`
	// 存储操作超时
	StorageOpTimeout time.Duration `mapstructure:"storage_op_timeout"`
	// 数据库操作超时
	DbOpTimeout time.Duration `mapstructure:"db_op_timeout"`
}

type ProxyConfig struct {
	DefaultTimeout        time.Duration     `mapstructure:"default_timeout"`
	ConnectTimeout        time.Duration     `mapstructure:"connect_timeout"`
	LargeFileThreshold    int64             `mapstructure:"large_file_threshold"`
	LargeFileThresholdStr string            `mapstructure:"large_file_threshold_str"`
	MaxRedirects          int               `mapstructure:"max_redirects"`
	InsecureSkipVerify    bool              `mapstructure:"insecure_skip_verify"`
	DNSMapping            map[string]string `mapstructure:"dns_mapping"`
	HealthCheck           HealthCheckConfig `mapstructure:"health_check"`
}

type HealthCheckConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Interval         time.Duration `mapstructure:"interval"`
	Timeout          time.Duration `mapstructure:"timeout"`
	FailureThreshold int           `mapstructure:"failure_threshold"`
	BlockOnUnhealthy bool          `mapstructure:"block_on_unhealthy"`
}

// AI 配置
type AIConfig struct {
	Enabled     bool              `mapstructure:"enabled"`
	Provider    string            `mapstructure:"provider"`
	BaseURL     string            `mapstructure:"base_url"`
	APIKey      string            `mapstructure:"api_key"`
	Model       string            `mapstructure:"model"`
	MaxTokens   int               `mapstructure:"max_tokens"`
	Temperature float64           `mapstructure:"temperature"`
	Timeout     time.Duration     `mapstructure:"timeout"`
	Tools       AIToolsConfig     `mapstructure:"tools"`
	RateLimit   AIRateLimitConfig `mapstructure:"rate_limit"`
	Cache       AICacheConfig     `mapstructure:"cache"`
	Session     AISessionConfig   `mapstructure:"session"`
}

// AI 工具配置
type AIToolsConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	AllowedTools     []string `mapstructure:"allowed_tools"`
	MaxExecutionTime int      `mapstructure:"max_execution_time"`
	EnableAuditLog   bool     `mapstructure:"enable_audit_log"`
}

// AI 限流配置
type AIRateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
	RequestsPerDay    int `mapstructure:"requests_per_day"`
	TokensPerDay      int `mapstructure:"tokens_per_day"`
}

// AI 缓存配置
type AICacheConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	TTL     time.Duration `mapstructure:"ttl"`
	MaxSize int           `mapstructure:"max_size"`
}

// AI 会话配置
type AISessionConfig struct {
	MaxAge      time.Duration `mapstructure:"max_age"`
	MaxMessages int           `mapstructure:"max_messages"`
}

var globalConfig *Config

func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// 设置默认值
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// 使用默认配置
			println("Warning: config file not found, using defaults")
		} else {
			return nil, err
		}
	}

	// 支持环境变量覆盖（如 MOONLIGHT_AUTH_JWT_SECRET 覆盖 auth.jwt_secret）
	// 必须先设置 prefix 与 key replacer，再开启 AutomaticEnv，嵌套 key 才能正确映射
	v.SetEnvPrefix("MOONLIGHT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// 解析带单位的文件大小字符串（如 "50MB", "1GB", "50m"）
	if raw := v.GetString("proxy.large_file_threshold"); raw != "" {
		if size, err := ParseSize(raw); err == nil {
			cfg.Proxy.LargeFileThreshold = size
		}
	}

	globalConfig = &cfg
	return &cfg, nil
}

// ParseSize 解析带单位的文件大小字符串为字节数
// 支持: "50MB", "1GB", "50m", "100k", "500B", 或纯数字
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	multipliers := map[string]int64{
		"B":  1,
		"K":  1024,
		"KB": 1024,
		"M":  1024 * 1024,
		"MB": 1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
	}

	upper := strings.ToUpper(s)

	suffixes := []string{"GB", "MB", "KB", "G", "M", "K", "B"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(upper, suffix) {
			numStr := strings.TrimSuffix(upper, suffix)
			numStr = strings.TrimSpace(numStr)
			if numStr == "" {
				return 0, nil
			}
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, err
			}
			return int64(num * float64(multipliers[suffix])), nil
		}
	}

	// 纯数字
	num, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(num), nil
}

func Get() *Config {
	return globalConfig
}
