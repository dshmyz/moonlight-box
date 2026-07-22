package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newOptimizerTestDB 创建内存 SQLite 并 migrate 相关表。
func newOptimizerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.BlockRule{},
		&model.Artifact{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedBlockRule 插入一条 block rule 记录。
func seedBlockRule(t *testing.T, db *gorm.DB, pkgName, version string, matchType model.BlockMatchType, pkgType string, enabled bool) *model.BlockRule {
	t.Helper()
	rule := model.BlockRule{
		PackageName: pkgName,
		Version:     version,
		MatchType:   matchType,
		PackageType: pkgType,
		Enabled:     enabled,
		Reason:      "test rule",
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("seed block rule: %v", err)
	}
	return &rule
}

// === 元信息测试 ===

func TestBlockRuleOptimizerTool_Meta(t *testing.T) {
	tool := NewBlockRuleOptimizerTool()
	if tool.Name() != "block_rule_optimizer" {
		t.Errorf("Name = %q, want %q", tool.Name(), "block_rule_optimizer")
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Fatal("Parameters should not be empty")
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("parse parameters schema: %v", err)
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema should have properties")
	}
	if _, ok := props["operation"]; !ok {
		t.Error("schema missing operation property")
	}
}

// === 过宽规则检测 ===

// TestBlockRuleOptimizer_OverBroadWildcardRule
// 验证 wildcard + Version="*" 的规则被标记为 over_broad。
func TestBlockRuleOptimizer_OverBroadWildcardRule(t *testing.T) {
	db := newOptimizerTestDB(t)
	seedBlockRule(t, db, "lodash", "*", model.BlockMatchWildcard, "*", true)

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Suggestions []struct {
			Type        string `json:"type"`
			RuleID      uint   `json:"rule_id"`
			PackageName string `json:"package_name"`
			Suggestion  string `json:"suggestion"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	found := false
	for _, s := range resp.Suggestions {
		if s.Type == "over_broad" && s.PackageName == "lodash" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected over_broad suggestion for lodash, got: %+v", resp.Suggestions)
	}
}

// === 过期规则检测 ===

// TestBlockRuleOptimizer_StaleRuleNoMatchingArtifacts
// 验证不匹配任何 artifact 的规则被标记为 stale。
func TestBlockRuleOptimizer_StaleRuleNoMatchingArtifacts(t *testing.T) {
	db := newOptimizerTestDB(t)
	// 规则阻断 lodash <4.17.21，但 artifacts 表里没有 lodash
	seedBlockRule(t, db, "lodash", "<4.17.21", model.BlockMatchRange, "npm", true)
	// 放一个无关的 artifact
	seedArtifact(t, db, "express", "4.18.0", "npm")

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Suggestions []struct {
			Type        string `json:"type"`
			RuleID      uint   `json:"rule_id"`
			PackageName string `json:"package_name"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	found := false
	for _, s := range resp.Suggestions {
		if s.Type == "stale" && s.PackageName == "lodash" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected stale suggestion for lodash, got: %+v", resp.Suggestions)
	}
}

// TestBlockRuleOptimizer_NotStaleWhenArtifactMatches
// 验证规则匹配到 artifact 时不标记为 stale。
func TestBlockRuleOptimizer_NotStaleWhenArtifactMatches(t *testing.T) {
	db := newOptimizerTestDB(t)
	seedBlockRule(t, db, "lodash", "<4.17.21", model.BlockMatchRange, "npm", true)
	seedArtifact(t, db, "lodash", "4.17.0", "npm")

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Suggestions []struct {
			Type        string `json:"type"`
			PackageName string `json:"package_name"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	for _, s := range resp.Suggestions {
		if s.Type == "stale" {
			t.Errorf("did not expect stale suggestion, got: %+v", s)
		}
	}
}

// === 冗余规则检测 ===

// TestBlockRuleOptimizer_RedundantRuleSubsumedByWildcard
// 验证同包的 wildcard "*" 规则覆盖了 range 规则时，range 规则被标记为 redundant。
func TestBlockRuleOptimizer_RedundantRuleSubsumedByWildcard(t *testing.T) {
	db := newOptimizerTestDB(t)
	// wildcard * 阻断 lodash 所有版本
	wildcardRule := seedBlockRule(t, db, "lodash", "*", model.BlockMatchWildcard, "npm", true)
	// range 阻断 lodash <4.17.21，被 wildcard 覆盖
	rangeRule := seedBlockRule(t, db, "lodash", "<4.17.21", model.BlockMatchRange, "npm", true)
	_ = wildcardRule

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Suggestions []struct {
			Type        string `json:"type"`
			RuleID      uint   `json:"rule_id"`
			PackageName string `json:"package_name"`
			Detail      string `json:"detail"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	found := false
	for _, s := range resp.Suggestions {
		if s.Type == "redundant" && s.RuleID == rangeRule.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected redundant suggestion for range rule (ID=%d), got: %+v", rangeRule.ID, resp.Suggestions)
	}
}

// === 干净规则集 ===

// TestBlockRuleOptimizer_NoSuggestionsForCleanRules
// 验证无问题的规则集不产生任何建议。
func TestBlockRuleOptimizer_NoSuggestionsForCleanRules(t *testing.T) {
	db := newOptimizerTestDB(t)
	// range 规则，匹配到 artifact，不过宽，不冗余
	seedBlockRule(t, db, "lodash", "<4.17.21", model.BlockMatchRange, "npm", true)
	seedArtifact(t, db, "lodash", "4.17.0", "npm")

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Suggestions []struct {
			Type string `json:"type"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions for clean rules, got %d: %+v", len(resp.Suggestions), resp.Suggestions)
	}
}

// === 参数校验 ===

// TestBlockRuleOptimizer_InvalidOperation
// 验证不支持的操作类型返回错误。
func TestBlockRuleOptimizer_InvalidOperation(t *testing.T) {
	db := newOptimizerTestDB(t)
	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "fix",
	})
	if err == nil {
		t.Fatal("expected error for unsupported operation, got nil")
	}
}

// TestBlockRuleOptimizer_MissingOperation
// 验证缺少 operation 参数返回错误。
func TestBlockRuleOptimizer_MissingOperation(t *testing.T) {
	db := newOptimizerTestDB(t)
	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error when operation missing, got nil")
	}
}

// === 禁用规则不分析 ===

// TestBlockRuleOptimizer_OnlyAnalyzesEnabledRules
// 验证只分析 enabled=true 的规则。
func TestBlockRuleOptimizer_OnlyAnalyzesEnabledRules(t *testing.T) {
	db := newOptimizerTestDB(t)
	// 禁用的过宽规则，不应被标记（GORM default:true 会把 false 覆盖，需显式更新）
	rule := seedBlockRule(t, db, "disabled-pkg", "*", model.BlockMatchWildcard, "*", true)
	if err := db.Model(rule).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable rule: %v", err)
	}

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		TotalRules  int `json:"total_rules"`
		Suggestions []struct {
			Type        string `json:"type"`
			PackageName string `json:"package_name"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if resp.TotalRules != 0 {
		t.Errorf("TotalRules = %d, want 0 (no enabled rules)", resp.TotalRules)
	}
	for _, s := range resp.Suggestions {
		if s.PackageName == "disabled-pkg" {
			t.Errorf("disabled rule should not be analyzed, got suggestion: %+v", s)
		}
	}
}

// === 综合场景 ===

// TestBlockRuleOptimizer_CombinedAnalysis
// 验证一次分析同时检测多种问题。
func TestBlockRuleOptimizer_CombinedAnalysis(t *testing.T) {
	db := newOptimizerTestDB(t)
	// 过宽：wildcard * 阻断所有版本（有 artifact，不是 stale）
	seedBlockRule(t, db, "left-pad", "*", model.BlockMatchWildcard, "npm", true)
	seedArtifact(t, db, "left-pad", "1.0.0", "npm")
	// 过期：不匹配任何 artifact
	seedBlockRule(t, db, "ghost-pkg", "<2.0.0", model.BlockMatchRange, "npm", true)
	// 冗余：被同包 wildcard 覆盖（有 artifact，不是 stale）
	seedBlockRule(t, db, "lodash", "*", model.BlockMatchWildcard, "npm", true)
	seedBlockRule(t, db, "lodash", "<4.17.21", model.BlockMatchRange, "npm", true)
	seedArtifact(t, db, "lodash", "4.17.0", "npm")

	tool := NewBlockRuleOptimizerTool()
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "analyze",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		TotalRules int `json:"total_rules"`
		Suggestions []struct {
			Type        string `json:"type"`
			PackageName string `json:"package_name"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if resp.TotalRules != 4 {
		t.Errorf("TotalRules = %d, want 4", resp.TotalRules)
	}
	types := map[string]int{}
	for _, s := range resp.Suggestions {
		types[s.Type]++
	}
	// over_broad: left-pad + lodash(wildcard *) = 2
	// stale: ghost-pkg = 1
	// redundant: lodash(range) = 1
	if types["over_broad"] != 2 {
		t.Errorf("over_broad count = %d, want 2", types["over_broad"])
	}
	if types["stale"] != 1 {
		t.Errorf("stale count = %d, want 1", types["stale"])
	}
	if types["redundant"] != 1 {
		t.Errorf("redundant count = %d, want 1", types["redundant"])
	}
}
