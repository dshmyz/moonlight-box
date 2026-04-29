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
	DefaultTimeout     time.Duration `mapstructure:"default_timeout"`
	ConnectTimeout     time.Duration `mapstructure:"connect_timeout"`
	LargeFileThreshold int64         `mapstructure:"large_file_threshold"`
	MaxRedirects       int           `mapstructure:"max_redirects"`
	InsecureSkipVerify bool          `mapstructure:"insecure_skip_verify"`
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
