package config

import (
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
	Proxy    ProxyConfig    `mapstructure:"proxy"`
	AI       AIConfig       `mapstructure:"ai"`   // AI 配置
	Seed     SeedConfig     `mapstructure:"seed"` // 初始化数据配置
}

type SeedConfig struct {
	Enabled      bool `mapstructure:"enabled"`        // 是否启用种子数据
	LoadTestData bool `mapstructure:"load_test_data"` // 是否加载测试包数据
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"` // debug, release, test
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // sqlite, postgres
	DSN    string `mapstructure:"dsn"`
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
	Output string `mapstructure:"output"`
}

type ProxyConfig struct {
	DefaultTimeout     time.Duration     `mapstructure:"default_timeout"`
	ConnectTimeout     time.Duration     `mapstructure:"connect_timeout"`
	LargeFileThreshold int64             `mapstructure:"large_file_threshold"`
	MaxRedirects       int               `mapstructure:"max_redirects"`
	InsecureSkipVerify bool              `mapstructure:"insecure_skip_verify"`
	DNSMapping         map[string]string `mapstructure:"dns_mapping"`
	HealthCheck        HealthCheckConfig `mapstructure:"health_check"`
}

type HealthCheckConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Interval         time.Duration `mapstructure:"interval"`
	Timeout          time.Duration `mapstructure:"timeout"`
	FailureThreshold int           `mapstructure:"failure_threshold"`
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

	// 支持环境变量覆盖
	v.AutomaticEnv()
	v.SetEnvPrefix("MOONLIGHT")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	globalConfig = &cfg
	return &cfg, nil
}

func Get() *Config {
	return globalConfig
}
