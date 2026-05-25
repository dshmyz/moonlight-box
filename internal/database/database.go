package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Initialize(cfg *config.Config) error {
	var dialector gorm.Dialector

	util.WithFields(logrus.Fields{
		util.LogKeyModule: "database",
		"driver":          cfg.Database.Driver,
	}).Info("Initializing database connection")

	switch cfg.Database.Driver {
	case "postgres":
		dsn := cfg.Database.DSN
		dialector = postgres.Open(dsn)
	case "sqlite":
		fallthrough
	default:
		dsn := cfg.Database.DSN
		if err := ensureDataDirectory(dsn); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
		connParams := "_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate"
		if strings.Contains(dsn, "?") {
			dsn = dsn + "&" + connParams
		} else {
			dsn = dsn + "?" + connParams
		}
		dialector = sqlite.Open(dsn)
	}

	gormConfig := &gorm.Config{
		Logger: buildGormLogger(cfg.Database.LogLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		util.WithFields(logrus.Fields{
			util.LogKeyModule: "database",
			"driver":          cfg.Database.Driver,
			util.LogKeyError:  err,
		}).Error("Failed to connect to database")
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		util.WithFields(logrus.Fields{
			util.LogKeyModule: "database",
			util.LogKeyError:  err,
		}).Error("Failed to get underlying sql.DB")
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if cfg.Database.Driver == "sqlite" {
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(4)
		sqlDB.SetConnMaxLifetime(time.Hour)
		sqlDB.SetConnMaxIdleTime(30 * time.Minute)

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

	util.WithFields(logrus.Fields{
		util.LogKeyModule:    "database",
		"driver":             cfg.Database.Driver,
		"max_open_conns":     cfg.Database.MaxOpenConns,
		"max_idle_conns":     cfg.Database.MaxIdleConns,
		"conn_max_lifetime":  cfg.Database.ConnMaxLifetime,
		"conn_max_idle_time": cfg.Database.ConnMaxIdleTime,
	}).Info("Database connection established")

	return nil
}

func ensureDataDirectory(dsn string) error {
	if strings.HasPrefix(dsn, "file:") {
		dsn = strings.TrimPrefix(dsn, "file:")
	}
	idx := strings.Index(dsn, "?")
	if idx != -1 {
		dsn = dsn[:idx]
	}
	dir := dsn
	if strings.HasSuffix(dir, ".db") {
		dir = strings.TrimSuffix(dir, "/registry.db")
	}
	if dir == "." || dir == "./" || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

// gormLogger 自定义 GORM 日志记录器，支持分文件输出
type gormLogger struct {
	LogLevel logger.LogLevel
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &gormLogger{LogLevel: level}
}

func (l *gormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Info {
		util.GetLogger(util.LogTypeSQL).WithFields(logrus.Fields{
			util.LogKeyModule: "gorm",
		}).Infof(msg, data...)
	}
}

func (l *gormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		util.GetLogger(util.LogTypeSQL).WithFields(logrus.Fields{
			util.LogKeyModule: "gorm",
		}).Warnf(msg, data...)
	}
}

func (l *gormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	util.GetLogger(util.LogTypeSQL).WithFields(logrus.Fields{
		util.LogKeyModule: "gorm",
		util.LogKeyError:  fmt.Errorf(msg, data...),
	}).Error("GORM error")
}

func (l *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := logrus.Fields{
		util.LogKeyModule: "gorm",
		"duration_ms":     elapsed.Milliseconds(),
		"rows_affected":   rows,
	}

	// 只在配置允许时记录完整 SQL
	// 生产环境建议关闭或采样记录，避免日志过大
	if l.LogLevel >= logger.Info {
		fields["sql"] = sql
	}

	entry := util.GetLogger(util.LogTypeSQL).WithFields(fields)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		fields[util.LogKeyError] = err
		entry.WithFields(fields).Error("SQL execution failed")
		return
	}

	// 慢查询日志（>1秒）
	if elapsed > time.Second {
		entry.WithFields(fields).Warn("Slow query detected")
		return
	}

	// 普通查询日志（仅在 debug/info 级别记录）
	if l.LogLevel >= logger.Info {
		entry.WithFields(fields).Debug("SQL executed")
	}
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
	case "debug":
		logLevel = logger.Info // GORM debug 会输出大量参数，用 info 级别控制
	default:
		logLevel = logger.Warn
	}
	return &gormLogger{LogLevel: logLevel}
}

func GetDB() *gorm.DB {
	return DB
}

func Close() error {
	util.CloseLoggers()
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

		util.WithFields(logrus.Fields{
			util.LogKeyModule: "database",
			"wait_count":      newWaits,
			"wait_duration":   s.WaitDuration.String(),
			"open_conns":      s.OpenConnections,
			"in_use":          s.InUse,
			"idle":            s.Idle,
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
