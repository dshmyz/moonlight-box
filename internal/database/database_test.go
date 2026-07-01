package database

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
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

func TestInitializeSQLiteUsesConcurrentReadConnection(t *testing.T) {
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
	// WAL 模式支持多读单写，允许多个读连接并发，写串行由 _txlock=immediate 保证
	if stats.MaxOpenConnections != 4 {
		t.Fatalf("sqlite MaxOpenConnections = %d, want 4 for concurrent reads under WAL", stats.MaxOpenConnections)
	}
}

func TestArtifactAutoMigrateCreatesLookupIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Artifact{}); err != nil {
		t.Fatalf("auto migrate artifacts: %v", err)
	}

	for _, indexName := range []string{
		"idx_artifacts_repo_format_remote_path",
		"idx_artifacts_repo_format_name",
		"idx_artifacts_repo_format_name_version",
		"idx_artifacts_repo_format_filename",
		"idx_artifacts_repo_format_kind_name_version",
	} {
		if !sqliteIndexExists(t, db, indexName) {
			t.Fatalf("expected SQLite index %s to exist", indexName)
		}
	}
}

func TestArtifactRemotePathUsesBoundedVarcharForMySQLIndexes(t *testing.T) {
	parsed, err := schema.Parse(&model.Artifact{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse artifact schema: %v", err)
	}
	field := parsed.LookUpField("RemotePath")
	if field == nil {
		t.Fatal("RemotePath field not found")
	}
	if got := field.TagSettings["TYPE"]; got != "varchar(1024)" {
		t.Fatalf("RemotePath TYPE = %q, want varchar(1024)", got)
	}
	if got := field.TagSettings["INDEX"]; !strings.Contains(got, "length:512") {
		t.Fatalf("RemotePath INDEX tag = %q, want prefix length 512", got)
	}
}

func TestDialectorForConfigSupportsMySQLDriver(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.Driver = "mysql"
	cfg.Database.DSN = "registry:secret@tcp(127.0.0.1:3306)/moonlight?parseTime=true"

	dialector, err := dialectorForConfig(cfg)
	if err != nil {
		t.Fatalf("build mysql dialector: %v", err)
	}
	if got := dialector.Name(); got != "mysql" {
		t.Fatalf("dialector = %q, want mysql", got)
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

func sqliteIndexExists(t *testing.T, db *gorm.DB, indexName string) bool {
	t.Helper()
	var count int
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", indexName).Scan(&count).Error; err != nil {
		t.Fatalf("check sqlite index: %v", err)
	}
	return count > 0
}
