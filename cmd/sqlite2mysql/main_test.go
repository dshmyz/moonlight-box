package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// 目标 MySQL 列/表名必须加反引号：一旦出现保留字列名（如 rank、usage），
// 裸拼接的 ORDER BY / SELECT COUNT 会直接语法错误。
func TestQuoteCols(t *testing.T) {
	if got := quoteCols([]string{"id", "name"}); got != "`id`, `name`" {
		t.Errorf("quoteCols = %q, want %q", got, "`id`, `name`")
	}
}

func TestQuoteIdentEscapesBacktick(t *testing.T) {
	if got := quoteIdent("we`ird"); got != "`we``ird`" {
		t.Errorf("quoteIdent = %q, want %q", got, "`we``ird`")
	}
}

// parseSpec 输出的主键列名必须能原样进 SQL：验证 quoteCols 是恒等包裹。
func TestQuoteColsRoundTripsSchemaNames(t *testing.T) {
	names := []string{"artifact_id", "blob_id", "position", "rank", "usage"}
	got := quoteCols(names)
	want := "`artifact_id`, `blob_id`, `position`, `rank`, `usage`"
	if got != want {
		t.Errorf("quoteCols = %q, want %q", got, want)
	}
}

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// SQLite 历史数据可能存在 created_at 为 NULL 的行（建表早于 not null 约束）。
// NOT NULL 列回填 1970-01-01，可空列零值时间置 NULL，主键保留、行数一致。
func TestCopyTableBackfillsNullTimes(t *testing.T) {
	src := openTestDB(t, "src.db")
	dst := openTestDB(t, "dst.db")
	if err := dst.AutoMigrate(&model.Artifact{}); err != nil {
		t.Fatalf("migrate dst: %v", err)
	}
	// 源表模拟旧版本 schema：created_at/updated_at 可空（SQLite 不追溯新加的 NOT NULL 约束，
	// 老表里会留有 NULL 行）
	if err := src.Exec(`CREATE TABLE artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repository_id INTEGER NOT NULL, format TEXT NOT NULL, kind TEXT,
		identity_key TEXT NOT NULL, name TEXT, namespace TEXT, version TEXT,
		path TEXT, filename TEXT, remote_path TEXT, download_url TEXT,
		extension TEXT, content_type TEXT, size_bytes INTEGER DEFAULT 0,
		checksums JSON, qualifiers JSON, attributes JSON, metadata JSON,
		created_at DATETIME, updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create legacy src table: %v", err)
	}

	// 正常行（钩子生成 IdentityKey/时间戳）
	normal := model.Artifact{RepositoryID: 1, Format: "npm", Kind: "file", Name: "a",
		RemotePath: "a/a-1.0.0.tgz", SizeBytes: 10}
	if err := src.Create(&normal).Error; err != nil {
		t.Fatalf("seed normal: %v", err)
	}
	// 历史脏数据行：created_at/updated_at 为 NULL
	if err := src.Exec(`INSERT INTO artifacts
		(repository_id, format, kind, name, remote_path, identity_key, size_bytes, metadata, created_at, updated_at)
		VALUES (1, 'npm', 'file', 'b', 'b/b-1.0.0.tgz', 'file/b/b-1.0.0.tgz', 20, '{}', NULL, NULL)`).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	spec, err := parseSpec(src, &model.Artifact{})
	if err != nil {
		t.Fatalf("parseSpec: %v", err)
	}
	copied := make(map[string]int64)
	fixed, err := copyTable(src, dst, spec, 1, copied)
	if err != nil {
		t.Fatalf("copyTable: %v", err)
	}
	if copied["artifacts"] != 2 {
		t.Fatalf("copied = %d, want 2", copied["artifacts"])
	}
	if fixed != 2 { // 两处：created_at + updated_at
		t.Fatalf("fixed = %d, want 2", fixed)
	}

	epoch := time.Unix(0, 0)
	var legacy model.Artifact
	if err := dst.Where("name = ?", "b").First(&legacy).Error; err != nil {
		t.Fatalf("load legacy row: %v", err)
	}
	if !legacy.CreatedAt.Truncate(time.Second).Equal(epoch) || !legacy.UpdatedAt.Truncate(time.Second).Equal(epoch) {
		t.Errorf("legacy CreatedAt/UpdatedAt = %v/%v, want epoch %v", legacy.CreatedAt, legacy.UpdatedAt, epoch)
	}
	if legacy.ID != 2 {
		t.Errorf("legacy ID = %d, want 2 (主键必须保留)", legacy.ID)
	}

	var normalRow model.Artifact
	if err := dst.Where("name = ?", "a").First(&normalRow).Error; err != nil {
		t.Fatalf("load normal row: %v", err)
	}
	if normalRow.CreatedAt.IsZero() {
		t.Errorf("normal row CreatedAt 被误判为零值")
	}
}

// Repository.Config 带 serializer:json：map 插入会绕过 gorm 序列化器，
// 必须手动调用，否则裸 struct 丢给驱动报 unsupported type。
func TestCopyTableSerializesJSONFields(t *testing.T) {
	src := openTestDB(t, "src.db")
	dst := openTestDB(t, "dst.db")
	for _, db := range []*gorm.DB{src, dst} {
		if err := db.AutoMigrate(&model.Repository{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	// Config 非 nil
	withConfig := model.Repository{Name: "npm-proxy", Type: model.RepoTypeProxy, PackageType: "npm",
		Config: &model.RepositoryConfig{RemoteURL: "https://registry.npmjs.org"}}
	if err := src.Create(&withConfig).Error; err != nil {
		t.Fatalf("seed with config: %v", err)
	}
	// Config 为 nil（指针空值 → NULL）
	noConfig := model.Repository{Name: "raw-hosted", Type: model.RepoTypeLocal, PackageType: "raw"}
	if err := src.Create(&noConfig).Error; err != nil {
		t.Fatalf("seed without config: %v", err)
	}

	spec, err := parseSpec(src, &model.Repository{})
	if err != nil {
		t.Fatalf("parseSpec: %v", err)
	}
	copied := make(map[string]int64)
	if _, err := copyTable(src, dst, spec, 1, copied); err != nil {
		t.Fatalf("copyTable: %v", err)
	}

	var got model.Repository
	if err := dst.Where("name = ?", "npm-proxy").First(&got).Error; err != nil {
		t.Fatalf("load with-config repo: %v", err)
	}
	if got.Config == nil || got.Config.RemoteURL != "https://registry.npmjs.org" {
		t.Errorf("Config roundtrip 失败: %+v", got.Config)
	}
	var noCfg model.Repository
	if err := dst.Where("name = ?", "raw-hosted").First(&noCfg).Error; err != nil {
		t.Fatalf("load no-config repo: %v", err)
	}
	if noCfg.Config != nil {
		t.Errorf("nil Config 应保持 NULL，得到: %+v", noCfg.Config)
	}
}
