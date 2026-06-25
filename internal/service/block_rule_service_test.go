package service

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBlockRuleTestDB 构建内存 SQLite 数据库并迁移 BlockRule 表
func setupBlockRuleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// setupBlockRuleService 构建 service 与内存 DB，auditSvc 传 nil（测试不涉及审计）
func setupBlockRuleService(t *testing.T) (*BlockRuleService, *gorm.DB) {
	t.Helper()
	db := setupBlockRuleTestDB(t)
	repo := repository.NewBlockRuleRepository(db)
	svc := NewBlockRuleService(repo, nil)
	return svc, db
}

// TestIsBlockedWithArtifact_LicenseEquals 验证 license equals 条件规则：
// attrs 中 license 等于规则值时阻断。
func TestIsBlockedWithArtifact_LicenseEquals(t *testing.T) {
	svc, db := setupBlockRuleService(t)
	_ = db

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"license": "GPL-3.0",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（license equals GPL-3.0 命中），实际 blocked=false")
	}
}

// TestIsBlockedWithArtifact_LicenseContains 验证 license contains 条件规则：
// attrs 中 license 包含规则子串时阻断；不包含时放行。
func TestIsBlockedWithArtifact_LicenseContains(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpContains,
		ConditionValue: "GPL",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 命中：GPL-3.0 包含 GPL
	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"license": "GPL-3.0",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（license GPL-3.0 包含 GPL），实际 blocked=false")
	}

	// 放行：MIT 不包含 GPL
	result, err = svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"license": "MIT",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if result.Blocked {
		t.Fatalf("期望 blocked=false（license MIT 不包含 GPL），实际 blocked=true")
	}
}

// TestIsBlockedWithArtifact_PublishTimeBefore 验证 publish_time before 条件规则：
// attrs 中 published_at 早于规则阈值时阻断。
func TestIsBlockedWithArtifact_PublishTimeBefore(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpBefore,
		ConditionValue: "2020-01-01T00:00:00Z",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"published_at": "2019-06-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（2019 早于 2020 阈值），实际 blocked=false")
	}
}

// TestIsBlockedWithArtifact_PublishTimeAfter 验证 publish_time after 条件规则：
// attrs 中 published_at 晚于规则阈值时阻断；早于时放行。
func TestIsBlockedWithArtifact_PublishTimeAfter(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpAfter,
		ConditionValue: "2024-01-01T00:00:00Z",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 命中：2024-06 晚于 2024-01
	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"published_at": "2024-06-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（2024-06 晚于 2024-01 阈值），实际 blocked=false")
	}

	// 放行：2023-01 早于 2024-01
	result, err = svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"published_at": "2023-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if result.Blocked {
		t.Fatalf("期望 blocked=false（2023-01 早于 2024-01 阈值），实际 blocked=true")
	}
}

// TestIsBlockedWithArtifact_MetadataMissing 验证元数据缺失时放行：
// 规则要求 license，但 attrs 中无 license 字段，应 blocked=false。
func TestIsBlockedWithArtifact_MetadataMissing(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if result.Blocked {
		t.Fatalf("期望 blocked=false（attrs 无 license 字段，元数据缺失放行），实际 blocked=true")
	}
}

// TestIsBlockedWithArtifact_BothLayers 验证两层规则优先级：
// 同时存在第一层规则（阻断 lodash@4.17.20）和第二层条件规则，
// 第一层命中时应直接返回 blocked=true，不进入条件匹配。
func TestIsBlockedWithArtifact_BothLayers(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	// 第一层：无条件 exact 规则阻断 lodash@4.17.20
	firstLayer := model.BlockRule{
		PackageName: "lodash",
		Version:     "4.17.20",
		MatchType:   model.BlockMatchExact,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := svc.Create(&firstLayer); err != nil {
		t.Fatalf("create first layer rule: %v", err)
	}

	// 第二层：条件规则 license equals GPL-3.0
	secondLayer := model.BlockRule{
		PackageName:    "lodash",
		Version:        "4.17.20",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := svc.Create(&secondLayer); err != nil {
		t.Fatalf("create second layer rule: %v", err)
	}

	result, err := svc.IsBlockedWithArtifact("npm", "lodash", "4.17.20", map[string]interface{}{
		"license": "MIT",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（第一层 exact 规则命中），实际 blocked=false")
	}
}

// TestIsBlockedWithArtifact_ConditionCacheInvalidation 验证条件规则缓存失效：
// 创建条件规则后调用应命中；删除规则后缓存失效，再次调用应放行。
func TestIsBlockedWithArtifact_ConditionCacheInvalidation(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 首次调用应命中条件规则
	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"license": "GPL-3.0",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact first call: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("首次调用期望 blocked=true，实际 blocked=false")
	}

	// 删除规则，缓存应失效
	if err := svc.Delete(rule.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}

	// 再次调用应放行（规则已被删除且缓存已失效）
	result, err = svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"license": "GPL-3.0",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact second call: %v", err)
	}
	if result.Blocked {
		t.Fatalf("删除规则后期望 blocked=false（缓存失效），实际 blocked=true")
	}
}

// TestIsBlockedWithArtifact_PublishTimeWithinLast 验证 publish_time within_last 条件规则：
// ConditionValue 为天数，发布时间在最近 N 天内时阻断；超过 N 天时放行。
func TestIsBlockedWithArtifact_PublishTimeWithinLast(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "some-pkg",
		Version:        "1.0.0",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpWithinLast,
		ConditionValue: "15", // 15 天内
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 命中：3 天前发布，在 15 天内
	recentTime := time.Now().AddDate(0, 0, -3).Format(time.RFC3339)
	result, err := svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"published_at": recentTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（3 天前在 15 天内），实际 blocked=false")
	}

	// 放行：30 天前发布，超过 15 天
	oldTime := time.Now().AddDate(0, 0, -30).Format(time.RFC3339)
	result, err = svc.IsBlockedWithArtifact("npm", "some-pkg", "1.0.0", map[string]interface{}{
		"published_at": oldTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if result.Blocked {
		t.Fatalf("期望 blocked=false（30 天前超过 15 天），实际 blocked=true")
	}
}

// TestIsBlockedWithArtifact_WildcardPkgNameMatchesAll 验证通配符包名（*）的条件规则
// 能匹配所有包名，实现"所有包发布时间 15 天内阻断"的场景。
func TestIsBlockedWithArtifact_WildcardPkgNameMatchesAll(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	// 创建通配符规则：所有包、所有版本、15 天内发布阻断
	rule := model.BlockRule{
		PackageName:    "*",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpWithinLast,
		ConditionValue: "15",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 任意包名 + 3 天前发布 → 阻断
	recentTime := time.Now().AddDate(0, 0, -3).Format(time.RFC3339)
	result, err := svc.IsBlockedWithArtifact("npm", "any-package-name", "2.5.1", map[string]interface{}{
		"published_at": recentTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（通配符 * 匹配任意包，3 天前在 15 天内），实际 blocked=false")
	}

	// 任意包名 + 30 天前发布 → 放行
	oldTime := time.Now().AddDate(0, 0, -30).Format(time.RFC3339)
	result, err = svc.IsBlockedWithArtifact("npm", "another-pkg", "0.1.0", map[string]interface{}{
		"published_at": oldTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if result.Blocked {
		t.Fatalf("期望 blocked=false（30 天前超过 15 天），实际 blocked=true")
	}
}

// TestIsBlockedWithArtifact_ConditionRuleNotMatchOtherPkg 验证条件规则不会误伤
// 不匹配包名的请求。给 express 配的 license 规则，不应阻断 lodash 的请求。
func TestIsBlockedWithArtifact_ConditionRuleNotMatchOtherPkg(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	// 给 express 配一条 license 阻断规则
	rule := model.BlockRule{
		PackageName:    "express",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "MIT",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// lodash 的请求，即使 attrs 里 license=MIT，也不应被 express 的规则阻断
	result, err := svc.IsBlockedWithArtifact("npm", "lodash", "4.17.20", map[string]interface{}{
		"license": "MIT",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if result.Blocked {
		t.Fatalf("期望 blocked=false（express 的规则不应匹配 lodash），实际 blocked=true")
	}

	// express 的请求，license=MIT → 命中阻断
	result, err = svc.IsBlockedWithArtifact("npm", "express", "4.18.0", map[string]interface{}{
		"license": "MIT",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（express + license=MIT 命中规则），实际 blocked=false")
	}
}

// TestIsBlockedWithArtifact_MatchAllNoRegex 验证 PackageName=* + Version=* 的条件规则
// 走快速路径（不编译正则），且能匹配任意包名和版本。
func TestIsBlockedWithArtifact_MatchAllNoRegex(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "*",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// 刷新缓存使规则生效
	if err := svc.refreshCache(); err != nil {
		t.Fatalf("refreshCache: %v", err)
	}

	// 验证 matchAll 条件规则没有预编译正则（pkgCompiled 应为 nil）
	svc.cacheMu.RLock()
	rules := svc.conditionalRules["npm"]
	svc.cacheMu.RUnlock()
	if len(rules) != 1 {
		t.Fatalf("期望条件规则缓存 1 条，实际 %d 条", len(rules))
	}
	if rules[0].pkgCompiled != nil {
		t.Fatalf("期望 PackageName=* 时 pkgCompiled 为 nil（快速路径），实际已编译正则")
	}

	// 任意包名 + 任意版本 + license=GPL-3.0 → 阻断
	result, err := svc.IsBlockedWithArtifact("npm", "any-pkg", "9.9.9", map[string]interface{}{
		"license": "GPL-3.0",
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 blocked=true（* 匹配所有包，GPL-3.0 命中），实际 blocked=false")
	}
}

// TestIsBlockedWithArtifact_AllPackageTypes 验证 PackageType="all" 的条件规则
// 能跨包类型匹配（npm/maven/pypi 等都命中）。
func TestIsBlockedWithArtifact_AllPackageTypes(t *testing.T) {
	svc, _ := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "*",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    model.PackageTypeAll,
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpWithinLast,
		ConditionValue: "15",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	recentTime := time.Now().AddDate(0, 0, -3).Format(time.RFC3339)

	// npm 包命中
	result, err := svc.IsBlockedWithArtifact("npm", "some-npm-pkg", "1.0.0", map[string]interface{}{
		"published_at": recentTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact npm: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 npm 包 blocked=true，实际 blocked=false")
	}

	// maven 包命中
	result, err = svc.IsBlockedWithArtifact("maven", "com.example:lib", "2.0", map[string]interface{}{
		"published_at": recentTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact maven: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 maven 包 blocked=true，实际 blocked=false")
	}

	// pypi 包命中
	result, err = svc.IsBlockedWithArtifact("pypi", "django", "4.0", map[string]interface{}{
		"published_at": recentTime,
	})
	if err != nil {
		t.Fatalf("IsBlockedWithArtifact pypi: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("期望 pypi 包 blocked=true，实际 blocked=false")
	}
}

func TestCreateRejectsInvalidConditionCombination(t *testing.T) {
	svc, db := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "lodash",
		Version:        "4.17.21",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpBefore,
		ConditionValue: "2020-01-01T00:00:00Z",
	}

	if err := svc.Create(&rule); err == nil {
		t.Fatalf("期望 license + before 被拒绝，实际创建成功")
	}

	var count int64
	if err := db.Model(&model.BlockRule{}).Count(&count).Error; err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if count != 0 {
		t.Fatalf("非法规则不应落库，实际落库 %d 条", count)
	}
}

func TestBatchCreateCountsInvalidConditionalRulesAsFailed(t *testing.T) {
	svc, db := setupBlockRuleService(t)

	valid := &model.BlockRule{
		PackageName:    "lodash",
		Version:        "4.17.21",
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpContains,
		ConditionValue: "GPL",
	}
	invalid := &model.BlockRule{
		PackageName:    "fresh-pkg",
		Version:        "*",
		MatchType:      model.BlockMatchWildcard,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypePublishTime,
		ConditionOp:    model.ConditionOpWithinLast,
		ConditionValue: "0",
	}

	success, failed, err := svc.BatchCreate([]*model.BlockRule{valid, invalid})
	if err != nil {
		t.Fatalf("BatchCreate: %v", err)
	}
	if success != 1 || failed != 1 {
		t.Fatalf("success=%d failed=%d, want success=1 failed=1", success, failed)
	}

	var count int64
	if err := db.Model(&model.BlockRule{}).Count(&count).Error; err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if count != 1 {
		t.Fatalf("只应落库有效规则 1 条，实际 %d 条", count)
	}
}

func TestUpdateRejectsInvalidConditionalTransition(t *testing.T) {
	svc, db := setupBlockRuleService(t)

	rule := model.BlockRule{
		PackageName:    "lodash",
		Version:        "4.17.21",
		MatchType:      model.BlockMatchExact,
		PackageType:    "npm",
		Enabled:        true,
		ConditionType:  model.ConditionTypeLicense,
		ConditionOp:    model.ConditionOpEquals,
		ConditionValue: "GPL-3.0",
	}
	if err := svc.Create(&rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	err := svc.Update(rule.ID, map[string]interface{}{
		"condition_op":    string(model.ConditionOpBefore),
		"condition_value": "2020-01-01T00:00:00Z",
	})
	if err == nil {
		t.Fatalf("期望 update 把 license 改成 before 时被拒绝，实际成功")
	}

	var stored model.BlockRule
	if err := db.First(&stored, rule.ID).Error; err != nil {
		t.Fatalf("load stored rule: %v", err)
	}
	if stored.ConditionOp != model.ConditionOpEquals || stored.ConditionValue != "GPL-3.0" {
		t.Fatalf("非法更新不应落库，实际 condition_op=%q condition_value=%q", stored.ConditionOp, stored.ConditionValue)
	}
}
