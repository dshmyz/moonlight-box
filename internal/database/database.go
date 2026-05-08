package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/config"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Initialize(cfg *config.Config) error {
	var dialector gorm.Dialector

	logrus.WithFields(logrus.Fields{
		"module": "database",
		"driver": cfg.Database.Driver,
	}).Info("Initializing database connection")

	switch cfg.Database.Driver {
	case "postgres":
		dsn := cfg.Database.DSN
		dialector = postgres.Open(dsn)
	case "sqlite":
		fallthrough
	default:
		dsn := cfg.Database.DSN
		// SQLite 配置优化
		// - _journal_mode=WAL: 启用 Write-Ahead Logging 提高并发性能
		// - _busy_timeout: 设置锁等待超时时间（毫秒）
		// - _synchronous=NORMAL: 平衡性能和安全性
		if !strings.Contains(dsn, "?") {
			dsn += "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
		} else if !strings.Contains(dsn, "_journal_mode") {
			dsn += "&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
		}
		dialector = sqlite.Open(dsn)
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "database",
			"driver": cfg.Database.Driver,
			"error":  err,
		}).Error("Failed to connect to database")
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "database",
			"error":  err,
		}).Error("Failed to get underlying sql.DB")
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if cfg.Database.Driver == "sqlite" {
		// SQLite 不支持多个并发写入，限制最大打开连接数为 1
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
		sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
		sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
		sqlDB.SetConnMaxIdleTime(cfg.Database.ConnMaxIdleTime)
	}

	DB = db

	logrus.WithFields(logrus.Fields{
		"module":             "database",
		"driver":             cfg.Database.Driver,
		"max_open_conns":     cfg.Database.MaxOpenConns,
		"max_idle_conns":     cfg.Database.MaxIdleConns,
		"conn_max_lifetime":  cfg.Database.ConnMaxLifetime,
		"conn_max_idle_time": cfg.Database.ConnMaxIdleTime,
	}).Info("Database connection established")

	return nil
}

func GetDB() *gorm.DB {
	return DB
}

func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

type PoolStats struct {
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration_ms"`
	MaxIdleClosed      int64         `json:"max_idle_closed"`
	MaxIdleTimeClosed  int64         `json:"max_idle_time_closed"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed"`
}

func GetPoolStats() *PoolStats {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return nil
	}
	s := sqlDB.Stats()
	return &PoolStats{
		MaxOpenConnections: s.MaxOpenConnections,
		OpenConnections:    s.OpenConnections,
		InUse:              s.InUse,
		Idle:               s.Idle,
		WaitCount:          s.WaitCount,
		WaitDuration:       s.WaitDuration,
		MaxIdleClosed:      s.MaxIdleClosed,
		MaxIdleTimeClosed:  s.MaxIdleTimeClosed,
		MaxLifetimeClosed:  s.MaxLifetimeClosed,
	}
}
