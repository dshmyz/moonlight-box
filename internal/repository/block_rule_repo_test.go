package repository

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestBlockRuleDB 构建内存 SQLite 数据库并迁移 BlockRule 表
func setupTestBlockRuleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

// TestFindAllEnabledConditionalRules 验证 FindAllEnabledConditionalRules 只返回
// ConditionType != "" 的启用规则，不返回无条件规则。
func TestFindAllEnabledConditionalRules(t *testing.T) {
	db := setupTestBlockRuleDB(t)
	repo := NewBlockRuleRepository(db)

	// 无条件的 exact 规则（启用）
	plainExact := model.BlockRule{
		PackageName: "pkg-plain",
		Version:     "1.0.0",
		MatchType:   model.BlockMatchExact,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := repo.Create(&plainExact); err != nil {
		t.Fatalf("create plain exact rule: %v", err)
	}

	// 无条件的 wildcard 规则（启用）
	plainWildcard := model.BlockRule{
		PackageName: "pkg-wild-*",
		Version:     "*",
		MatchType:   model.BlockMatchWildcard,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := repo.Create(&plainWildcard); err != nil {
		t.Fatalf("create plain wildcard rule: %v", err)
	}

	// 带条件的 exact 规则（启用，license 条件）
	condExact := model.BlockRule{
		PackageName:    "pkg-cond",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := repo.Create(&condExact); err != nil {
		t.Fatalf("create conditional exact rule: %v", err)
	}

	// 带条件的 wildcard 规则（启用，publish_time 条件）
	condWildcard := model.BlockRule{
		PackageName:    "pkg-cond-*",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpBefore,
		ConditionValue: "2024-01-01",
	}
	if err := repo.Create(&condWildcard); err != nil {
		t.Fatalf("create conditional wildcard rule: %v", err)
	}

	got, err := repo.FindAllEnabledConditionalRules()
	if err != nil {
		t.Fatalf("FindAllEnabledConditionalRules failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d conditional rules, want 2", len(got))
	}

	// 验证返回的都是带条件的规则
	seenIDs := map[uint]bool{}
	for _, r := range got {
		if r.ConditionType == "" {
			t.Fatalf("rule id=%d has empty ConditionType, should be excluded", r.ID)
		}
		seenIDs[r.ID] = true
	}

	if !seenIDs[condExact.ID] {
		t.Fatalf("conditional exact rule id=%d not returned", condExact.ID)
	}
	if !seenIDs[condWildcard.ID] {
		t.Fatalf("conditional wildcard rule id=%d not returned", condWildcard.ID)
	}
}

// TestFindAllEnabledExactRulesExcludesConditional 验证 FindAllEnabledExactRules
// 不返回条件规则（即只返回 ConditionType 为空的）。
func TestFindAllEnabledExactRulesExcludesConditional(t *testing.T) {
	db := setupTestBlockRuleDB(t)
	repo := NewBlockRuleRepository(db)

	// 无条件的 exact 规则（启用）
	plainExact := model.BlockRule{
		PackageName: "pkg-plain",
		Version:     "1.0.0",
		MatchType:   model.BlockMatchExact,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := repo.Create(&plainExact); err != nil {
		t.Fatalf("create plain exact rule: %v", err)
	}

	// 带条件的 exact 规则（启用）
	condExact := model.BlockRule{
		PackageName:    "pkg-cond",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := repo.Create(&condExact); err != nil {
		t.Fatalf("create conditional exact rule: %v", err)
	}

	got, err := repo.FindAllEnabledExactRules()
	if err != nil {
		t.Fatalf("FindAllEnabledExactRules failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d exact rules, want 1 (conditional rule should be excluded)", len(got))
	}

	if got[0].ID != plainExact.ID {
		t.Fatalf("got rule id=%d, want id=%d (plain exact rule)", got[0].ID, plainExact.ID)
	}

	if got[0].ConditionType != "" {
		t.Fatalf("returned rule id=%d has ConditionType=%q, should be empty",
			got[0].ID, got[0].ConditionType)
	}
}

// TestFindAllEnabledWildcardRulesExcludesConditional 验证 FindAllEnabledWildcardRules
// 不返回条件规则（即只返回 ConditionType 为空的）。
func TestFindAllEnabledWildcardRulesExcludesConditional(t *testing.T) {
	db := setupTestBlockRuleDB(t)
	repo := NewBlockRuleRepository(db)

	// 无条件的 wildcard 规则（启用）
	plainWildcard := model.BlockRule{
		PackageName: "pkg-plain-*",
		Version:     "*",
		MatchType:   model.BlockMatchWildcard,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := repo.Create(&plainWildcard); err != nil {
		t.Fatalf("create plain wildcard rule: %v", err)
	}

	// 带条件的 wildcard 规则（启用）
	condWildcard := model.BlockRule{
		PackageName:    "pkg-cond-*",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpBefore,
		ConditionValue: "2024-01-01",
	}
	if err := repo.Create(&condWildcard); err != nil {
		t.Fatalf("create conditional wildcard rule: %v", err)
	}

	got, err := repo.FindAllEnabledWildcardRules()
	if err != nil {
		t.Fatalf("FindAllEnabledWildcardRules failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d wildcard rules, want 1 (conditional rule should be excluded)", len(got))
	}

	if got[0].ID != plainWildcard.ID {
		t.Fatalf("got rule id=%d, want id=%d (plain wildcard rule)", got[0].ID, plainWildcard.ID)
	}

	if got[0].ConditionType != "" {
		t.Fatalf("returned rule id=%d has ConditionType=%q, should be empty",
			got[0].ID, got[0].ConditionType)
	}
}

func TestFindAllEnabledRangeRulesExcludesConditional(t *testing.T) {
	db := setupTestBlockRuleDB(t)
	repo := NewBlockRuleRepository(db)

	plainRange := model.BlockRule{
		PackageName: "lodash",
		Version:     ">=4.17.0 <5.0.0",
		MatchType:   model.BlockMatchRange,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := repo.Create(&plainRange); err != nil {
		t.Fatalf("create plain range rule: %v", err)
	}

	condRange := model.BlockRule{
		PackageName:    "express",
		Version:        "^4.18.0",
		MatchType:      model.BlockMatchRange,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := repo.Create(&condRange); err != nil {
		t.Fatalf("create conditional range rule: %v", err)
	}

	got, err := repo.FindAllEnabledRangeRules()
	if err != nil {
		t.Fatalf("FindAllEnabledRangeRules failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d range rules, want 1 (conditional rule should be excluded)", len(got))
	}
	if got[0].ID != plainRange.ID {
		t.Fatalf("got rule id=%d, want id=%d (plain range rule)", got[0].ID, plainRange.ID)
	}
	if got[0].ConditionType != "" {
		t.Fatalf("returned rule id=%d has ConditionType=%q, should be empty", got[0].ID, got[0].ConditionType)
	}
}

// TestFindAllEnabledConditionalRulesOnlyEnabled 验证 FindAllEnabledConditionalRules
// 不返回禁用的条件规则。
func TestFindAllEnabledConditionalRulesOnlyEnabled(t *testing.T) {
	db := setupTestBlockRuleDB(t)
	repo := NewBlockRuleRepository(db)

	// 启用的条件规则
	enabledCond := model.BlockRule{
		PackageName:    "pkg-enabled",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := repo.Create(&enabledCond); err != nil {
		t.Fatalf("create enabled conditional rule: %v", err)
	}

	// 禁用的条件规则
	// 注意：Enabled 字段有 gorm:"default:true"，直接传 false（零值）会被默认值覆盖，
	// 需要先创建再更新为禁用状态。
	disabledCond := model.BlockRule{
		PackageName:    "pkg-disabled",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "MIT",
	}
	if err := repo.Create(&disabledCond); err != nil {
		t.Fatalf("create disabled conditional rule: %v", err)
	}
	if err := db.Model(&model.BlockRule{}).Where("id = ?", disabledCond.ID).
		Update("enabled", false).Error; err != nil {
		t.Fatalf("disable rule id=%d: %v", disabledCond.ID, err)
	}

	got, err := repo.FindAllEnabledConditionalRules()
	if err != nil {
		t.Fatalf("FindAllEnabledConditionalRules failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d conditional rules, want 1 (disabled should be excluded)", len(got))
	}

	if got[0].ID != enabledCond.ID {
		t.Fatalf("got rule id=%d, want id=%d (enabled conditional rule)", got[0].ID, enabledCond.ID)
	}

	if !got[0].Enabled {
		t.Fatalf("returned rule id=%d is disabled, should be enabled", got[0].ID)
	}
}
