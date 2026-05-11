package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
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
		// - _busy_timeout: 设置锁等待超时时间（毫秒），增加到 30 秒以应对高并发写入
		// - _synchronous=NORMAL: 平衡性能和安全性
		// - _cache_size: 页面缓存大小（KB），负数表示 KB，正数表示页数
		// - _txlock=immediate: 事务开始时立即获取写锁，减少死锁
		if !strings.Contains(dsn, "?") {
			dsn += "?_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&_cache_size=-64000&_txlock=immediate"
		} else if !strings.Contains(dsn, "_journal_mode") {
			dsn += "&_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&_cache_size=-64000&_txlock=immediate"
		}
		dialector = gormlite.Open(dsn)
	}

	gormConfig := &gorm.Config{
		Logger: buildGormLogger(cfg.Database.LogLevel),
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
		// SQLite WAL 模式下支持并发读，写入仍然串行
		// 100 人使用场景：提升连接数以支持并发读操作
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(4)
		sqlDB.SetConnMaxLifetime(time.Hour)
		sqlDB.SetConnMaxIdleTime(30 * time.Minute)

		// 运行时 PRAGMA 优化
		sqlDB.Exec("PRAGMA journal_mode=WAL")
		sqlDB.Exec("PRAGMA synchronous=NORMAL")
		sqlDB.Exec("PRAGMA cache_size=-64000")
		sqlDB.Exec("PRAGMA temp_store=MEMORY")
		sqlDB.Exec("PRAGMA mmap_size=268435456")
		sqlDB.Exec("PRAGMA wal_autocheckpoint=1000")
		sqlDB.Exec("PRAGMA busy_timeout=30000")
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

func buildGormLogger(level string) logger.Interface {
	var logLevel logger.LogLevel
	switch strings.ToLower(level) {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Warn
	}
	return logger.Default.LogMode(logLevel)
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

var lastWaitCount int64

func GetPoolStats() *PoolStats {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return nil
	}
	s := sqlDB.Stats()

	if s.WaitCount > lastWaitCount {
		newWaits := s.WaitCount - lastWaitCount
		lastWaitCount = s.WaitCount

		logrus.WithFields(logrus.Fields{
			"module":        "database",
			"wait_count":    newWaits,
			"wait_duration": s.WaitDuration.String(),
			"open_conns":    s.OpenConnections,
			"in_use":        s.InUse,
			"idle":          s.Idle,
		}).Warn("Database connection pool wait detected, consider increasing max_open_conns or migrating to PostgreSQL")
	}

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
