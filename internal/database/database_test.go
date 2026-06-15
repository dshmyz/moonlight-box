package database

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGormLoggerTraceDoesNotLogRecordNotFoundAsExecutionFailure(t *testing.T) {
	if err := util.InitLogger(&util.LoggerConfig{Level: "debug", Format: "console", Output: "stdout"}); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	var output bytes.Buffer
	util.GetLogger(util.LogTypeSQL).SetOutput(&output)

	l := &gormLogger{LogLevel: logger.Error}
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT * FROM users WHERE username = ?", 0
	}, gorm.ErrRecordNotFound)

	if strings.Contains(output.String(), "SQL execution failed") {
		t.Fatalf("record not found should not be logged as execution failure: %s", output.String())
	}
}

func TestCleanupLegacyArtifactColumnsDropsCoordinates(t *testing.T) {
	if err := util.InitLogger(&util.LoggerConfig{Level: "debug", Format: "console", Output: "stdout"}); err != nil {
		t.Fatalf("init logger: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE artifacts (
			id integer primary key autoincrement,
			repository_id integer not null,
			format varchar(64) not null,
			identity_key varchar(1024) not null,
			coordinates jsonb not null,
			created_at datetime not null,
			updated_at datetime not null
		)
	`).Error; err != nil {
		t.Fatalf("create legacy artifacts table: %v", err)
	}

	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })

	if err := cleanupLegacyArtifactColumns(); err != nil {
		t.Fatalf("cleanup legacy artifact columns: %v", err)
	}

	if sqliteColumnExists(t, db, "artifacts", "coordinates") {
		t.Fatalf("coordinates should be removed from artifacts")
	}
}

func TestInitializeSQLiteUsesSingleWriterConnection(t *testing.T) {
	if err := util.InitLogger(&util.LoggerConfig{Level: "error", Format: "console", Output: "stdout"}); err != nil {
		t.Fatalf("init logger: %v", err)
	}

	oldDB := DB
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		DB = oldDB
	})

	cfg := &config.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "registry.db")
	cfg.Database.LogLevel = "silent"

	if err := Initialize(cfg); err != nil {
		t.Fatalf("initialize sqlite: %v", err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("sqlite MaxOpenConnections = %d, want 1 to avoid writer lock contention", stats.MaxOpenConnections)
	}
}

func sqliteColumnExists(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var count int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&count).Error; err != nil {
		t.Fatalf("check sqlite column: %v", err)
	}
	return count > 0
}
