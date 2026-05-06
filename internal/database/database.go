package database

import (
	"fmt"
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
		"module":   "database",
		"driver":   cfg.Database.Driver,
	}).Info("Initializing database connection")

	switch cfg.Database.Driver {
	case "postgres":
		dsn := cfg.Database.DSN
		dialector = postgres.Open(dsn)
	case "sqlite":
		fallthrough
	default:
		dsn := cfg.Database.DSN
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

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db

	logrus.WithFields(logrus.Fields{
		"module":          "database",
		"driver":          cfg.Database.Driver,
		"max_idle_conns":  10,
		"max_open_conns":  100,
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
