package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 回归测试：UpdateLastUsed 必须正确落库。
// 曾因硬编码列名 "last_used_at" 与 GORM 迁移出的实际列 "last_used" 不一致，
// 导致每次校验都静默报 "no such column: last_used_at"，last_used 从未真正写入。
func TestUpdateLastUsed_Persists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.APIToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewAPITokenRepository(db)
	now := time.Now()
	token := &model.APIToken{
		UserID:    1,
		Name:      "ci",
		TokenHash: []byte("0123456789abcdef0123456789abcdef"),
		Prefix:    "mlb_abc1234",
		LastUsed:  &now,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}

	// 更新前 last_used 已经有值，先清掉以验证 UpdateLastUsed 真正写入
	if err := db.Model(token).Update("last_used", nil).Error; err != nil {
		t.Fatalf("clear last_used: %v", err)
	}

	if err := repo.UpdateLastUsed(token.ID); err != nil {
		t.Fatalf("UpdateLastUsed error: %v", err)
	}

	var got model.APIToken
	if err := db.First(&got, token.ID).Error; err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if got.LastUsed == nil {
		t.Fatal("UpdateLastUsed did not persist last_used (nil after update)")
	}
	// 时间应接近 now
	if time.Since(*got.LastUsed) > 5*time.Second {
		t.Errorf("last_used too old: %v (now=%v)", got.LastUsed, time.Now())
	}
}

// 确认 GORM 迁移出的列名确实是 last_used，防止再次引入列名不匹配。
func TestAPITokenMigratedColumnIsLastUsed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.APIToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rows, err := db.Raw("PRAGMA table_info(api_tokens)").Rows()
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if name == "last_used_at" {
			t.Fatal("迁移出的列名是 last_used_at，与 repo 中更新逻辑不一致的风险回归")
		}
		if name == "last_used" {
			found = true
		}
	}
	if !found {
		t.Fatal("api_tokens 表缺少 last_used 列（GORM 命名与预期不符）")
	}
}
