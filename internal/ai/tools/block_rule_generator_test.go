package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newGeneratorTestDB 创建内存 SQLite 并 migrate 相关表。
func newGeneratorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.BlockRule{},
		&model.Artifact{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedArtifact 插入一条 artifact 记录用于影响分析测试。
func seedArtifact(t *testing.T, db *gorm.DB, name, version, format string) {
	t.Helper()
	art := model.Artifact{
		RepositoryID: 1,
		Format:       format,
		Name:         name,
		Version:      version,
		IdentityKey:  name + "/" + version,
	}
	if err := db.Create(&art).Error; err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
}

// seedVuln 插入一条 vulnerability 记录用于测试。
func seedVuln(t *testing.T, db *gorm.DB, scanResultID uint, cve, depName, fixedVer string, severity model.VulnerabilitySeverity) {
	t.Helper()
	vuln := model.Vulnerability{
		ScanResultID:   scanResultID,
		CVEID:          cve,
		Severity:       severity,
		DependencyName: depName,
		CurrentVersion: "1.0.0",
		FixedVersion:   fixedVer,
		Title:          "test vuln",
	}
	if err := db.Create(&vuln).Error; err != nil {
		t.Fatalf("seed vuln: %v", err)
	}
}

// === 元信息测试 ===

func TestBlockRuleGeneratorTool_Meta(t *testing.T) {
	scanRepo := repository.NewScanRepository(newGeneratorTestDB(t))
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(newGeneratorTestDB(t)), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	if tool.Name() != "block_rule_generator" {
		t.Errorf("Name = %q, want %q", tool.Name(), "block_rule_generator")
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
	requiredFields := []string{"operation", "source"}
	for _, f := range requiredFields {
		if _, ok := props[f]; !ok {
			t.Errorf("schema missing required property: %s", f)
		}
	}
}

// === source=vulnerability 路径测试 ===

// TestBlockRuleGenerator_PreviewFromVulnerabilityWithFixedVersion
// 验证从 vulnerability 表生成 preview：FixedVersion 存在时生成 range 规则。
func TestBlockRuleGenerator_PreviewFromVulnerabilityWithFixedVersion(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// 解析返回的 JSON
	var resp struct {
		Preview bool `json:"preview"`
		Rules   []struct {
			PackageName string `json:"package_name"`
			Version     string `json:"version"`
			MatchType   string `json:"match_type"`
			PackageType string `json:"package_type"`
			Reason      string `json:"reason"`
			Severity    string `json:"severity"`
		} `json:"rules"`
		ActionRequired string `json:"action_required"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result JSON: %v\nresult: %s", err, result)
	}

	if !resp.Preview {
		t.Error("Preview = false, want true")
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	r := resp.Rules[0]
	if r.PackageName != "log4j-core" {
		t.Errorf("PackageName = %q, want %q", r.PackageName, "log4j-core")
	}
	if r.MatchType != "range" {
		t.Errorf("MatchType = %q, want %q", r.MatchType, "range")
	}
	if r.Version != "<2.17.1" {
		t.Errorf("Version = %q, want %q", r.Version, "<2.17.1")
	}
	if r.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", r.Severity, "critical")
	}
	if !strings.Contains(resp.ActionRequired, "管理后台") && !strings.Contains(resp.ActionRequired, "Block Rules") {
		t.Errorf("ActionRequired should mention manual confirmation, got: %q", resp.ActionRequired)
	}

	// 验证没有写入 DB（preview 模式）
	blockRepo := repository.NewBlockRuleRepository(db)
	rules, err := blockRepo.List(nil)
	if err != nil {
		t.Fatalf("list block rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("preview should not write to DB, got %d rules", len(rules))
	}
}

// TestBlockRuleGenerator_PreviewFromVulnerabilityWithoutFixedVersion
// 验证 FixedVersion 为空时生成 wildcard 规则。
func TestBlockRuleGenerator_PreviewFromVulnerabilityWithoutFixedVersion(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2023-9999", "vulnerable-pkg", "", model.SeverityHigh)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2023-9999",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			PackageName string `json:"package_name"`
			MatchType   string `json:"match_type"`
			Version     string `json:"version"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	r := resp.Rules[0]
	if r.MatchType != "wildcard" {
		t.Errorf("MatchType = %q, want %q", r.MatchType, "wildcard")
	}
	if r.Version != "*" {
		t.Errorf("Version = %q, want %q", r.Version, "*")
	}
}

// TestBlockRuleGenerator_VulnerabilityMultiplePackagesDeduplicates
// 验证同一 CVE 影响多个包时按包名去重，每个包一条规则。
func TestBlockRuleGenerator_VulnerabilityMultiplePackagesDeduplicates(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2022-22965", "spring-core", "5.3.18", model.SeverityCritical)
	seedVuln(t, db, scanResult.ID, "CVE-2022-22965", "spring-webmvc", "5.3.18", model.SeverityCritical)
	// 重复的 spring-core，应被去重
	seedVuln(t, db, scanResult.ID, "CVE-2022-22965", "spring-core", "5.3.18", model.SeverityHigh)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2022-22965",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			PackageName string `json:"package_name"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 2 {
		t.Fatalf("expected 2 rules (deduplicated), got %d", len(resp.Rules))
	}
	names := map[string]bool{}
	for _, r := range resp.Rules {
		names[r.PackageName] = true
	}
	if !names["spring-core"] || !names["spring-webmvc"] {
		t.Errorf("expected spring-core and spring-webmvc, got %v", names)
	}
}

// === source=description 路径测试 ===

// TestBlockRuleGenerator_PreviewFromDescription
// 验证用户对话描述路径：AI 在对话中解析出包名/版本约束后传入结构化参数。
func TestBlockRuleGenerator_PreviewFromDescription(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation":       "preview",
		"source":          "description",
		"package_name":    "lodash",
		"version":         "<4.17.21",
		"match_type":      "range",
		"package_type":    "npm",
		"reason":          "CVE-2021-23337 原型污染",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Preview bool `json:"preview"`
		Rules   []struct {
			PackageName string `json:"package_name"`
			Version     string `json:"version"`
			MatchType   string `json:"match_type"`
			PackageType string `json:"package_type"`
			Reason      string `json:"reason"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if !resp.Preview {
		t.Error("Preview = false, want true")
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	r := resp.Rules[0]
	if r.PackageName != "lodash" {
		t.Errorf("PackageName = %q, want %q", r.PackageName, "lodash")
	}
	if r.Version != "<4.17.21" {
		t.Errorf("Version = %q, want %q", r.Version, "<4.17.21")
	}
	if r.MatchType != "range" {
		t.Errorf("MatchType = %q, want %q", r.MatchType, "range")
	}
	if r.PackageType != "npm" {
		t.Errorf("PackageType = %q, want %q", r.PackageType, "npm")
	}
}

// === 安全约束测试 ===

// TestBlockRuleGenerator_RejectsGlobalBlock
// 验证拒绝生成 PackageName="*" + Version="*" 的全局阻断规则。
func TestBlockRuleGenerator_RejectsGlobalBlock(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation":    "preview",
		"source":       "description",
		"package_name": "*",
		"version":      "*",
		"match_type":   "wildcard",
		"package_type": "*",
	})
	if err == nil {
		t.Fatal("expected error for global block rule, got nil")
	}
	if !strings.Contains(err.Error(), "global") && !strings.Contains(err.Error(), "全局") {
		t.Errorf("error should mention global block, got: %v", err)
	}
}

// TestBlockRuleGenerator_VulnerabilityNotFoundReturnsError
// 验证 CVE 不存在时返回明确错误。
func TestBlockRuleGenerator_VulnerabilityNotFoundReturnsError(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-9999-9999",
	})
	if err == nil {
		t.Fatal("expected error when CVE not found, got nil")
	}
}

// TestBlockRuleGenerator_MissingRequiredParams
// 验证缺少必需参数时返回错误。
func TestBlockRuleGenerator_MissingRequiredParams(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	// 缺少 source
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
	})
	if err == nil {
		t.Fatal("expected error when source missing, got nil")
	}

	// source=vulnerability 但缺 cve_id
	_, err = tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
	})
	if err == nil {
		t.Fatal("expected error when cve_id missing for vulnerability source, got nil")
	}

	// source=description 但缺 package_name
	_, err = tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "description",
	})
	if err == nil {
		t.Fatal("expected error when package_name missing for description source, got nil")
	}
}

// TestBlockRuleGenerator_InvalidOperation
// 验证不支持的操作类型返回错误。
func TestBlockRuleGenerator_InvalidOperation(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "create",  // 目前只支持 preview
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err == nil {
		t.Fatal("expected error for unsupported operation, got nil")
	}
}

// TestBlockRuleGenerator_DescriptionInvalidMatchType
// 验证 description 源传入无效 match_type 时被 ValidateRule 拒绝。
func TestBlockRuleGenerator_DescriptionInvalidMatchType(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation":    "preview",
		"source":       "description",
		"package_name": "test-pkg",
		"version":      "1.0.0",
		"match_type":   "invalid_type",
		"package_type": "npm",
	})
	if err == nil {
		t.Fatal("expected error for invalid match_type, got nil")
	}
}

// TestBlockRuleGenerator_DescriptionInvalidRangeConstraint
// 验证 description 源传入无效 range 约束时被 ValidateRule 拒绝。
func TestBlockRuleGenerator_DescriptionInvalidRangeConstraint(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation":    "preview",
		"source":       "description",
		"package_name": "test-pkg",
		"version":      "not-a-valid-range",
		"match_type":   "range",
		"package_type": "npm",
	})
	if err == nil {
		t.Fatal("expected error for invalid range constraint, got nil")
	}
}

// TestBlockRuleGenerator_PreviewDoesNotPersistRules
// 验证 preview 操作不会持久化任何规则到 DB。
func TestBlockRuleGenerator_PreviewDoesNotPersistRules(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockRepo := repository.NewBlockRuleRepository(db)
	blockSvc := service.NewBlockRuleService(blockRepo, nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	rules, err := blockRepo.List(nil)
	if err != nil {
		t.Fatalf("list block rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("preview should not persist rules, got %d rules in DB", len(rules))
	}
}

// === #1 规则影响分析测试 ===

// TestBlockRuleGenerator_ImpactAnalysisForRangeRule
// 验证 preview 时附带影响分析：查询 artifacts 表统计受影响的包版本。
// range 规则 <2.17.1 应匹配 2.14.0、2.14.1、2.15.0，不匹配 2.17.1、2.18.0。
func TestBlockRuleGenerator_ImpactAnalysisForRangeRule(t *testing.T) {
	db := newGeneratorTestDB(t)
	// 播种 artifacts：受影响版本 + 不受影响版本
	seedArtifact(t, db, "log4j-core", "2.14.0", "maven")
	seedArtifact(t, db, "log4j-core", "2.14.1", "maven")
	seedArtifact(t, db, "log4j-core", "2.15.0", "maven")
	seedArtifact(t, db, "log4j-core", "2.17.1", "maven") // 不受影响（修复版本）
	seedArtifact(t, db, "log4j-core", "2.18.0", "maven") // 不受影响

	// 播种漏洞数据
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			PackageName       string   `json:"package_name"`
			Version           string   `json:"version"`
			MatchType         string   `json:"match_type"`
			AffectedCount     int      `json:"affected_count"`
			AffectedVersions  []string `json:"affected_versions"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	r := resp.Rules[0]
	if r.AffectedCount != 3 {
		t.Errorf("AffectedCount = %d, want 3 (2.14.0, 2.14.1, 2.15.0)", r.AffectedCount)
	}
	if len(r.AffectedVersions) != 3 {
		t.Errorf("len(AffectedVersions) = %d, want 3", len(r.AffectedVersions))
	}
	// 验证不包含修复版本和更高版本
	for _, v := range r.AffectedVersions {
		if v == "2.17.1" || v == "2.18.0" {
			t.Errorf("AffectedVersions should not contain %s", v)
		}
	}
}

// TestBlockRuleGenerator_ImpactAnalysisNoArtifactsReturnsZero
// 验证当 artifacts 表中没有匹配的包时，AffectedCount=0 且 AffectedVersions 为空。
func TestBlockRuleGenerator_ImpactAnalysisNoArtifactsReturnsZero(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)
	// 不播种任何 artifact

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			AffectedCount    int      `json:"affected_count"`
			AffectedVersions []string `json:"affected_versions"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	if resp.Rules[0].AffectedCount != 0 {
		t.Errorf("AffectedCount = %d, want 0 (no artifacts)", resp.Rules[0].AffectedCount)
	}
	if len(resp.Rules[0].AffectedVersions) != 0 {
		t.Errorf("AffectedVersions = %v, want empty", resp.Rules[0].AffectedVersions)
	}
}

// TestBlockRuleGenerator_ImpactAnalysisForWildcardRule
// 验证 wildcard 规则（Version="*"）的影响分析：匹配该包所有版本。
func TestBlockRuleGenerator_ImpactAnalysisForWildcardRule(t *testing.T) {
	db := newGeneratorTestDB(t)
	seedArtifact(t, db, "vulnerable-pkg", "1.0.0", "npm")
	seedArtifact(t, db, "vulnerable-pkg", "2.0.0", "npm")
	seedArtifact(t, db, "vulnerable-pkg", "3.0.0", "npm")

	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2023-9999", "vulnerable-pkg", "", model.SeverityHigh)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2023-9999",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			AffectedCount    int      `json:"affected_count"`
			AffectedVersions []string `json:"affected_versions"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	if resp.Rules[0].AffectedCount != 3 {
		t.Errorf("AffectedCount = %d, want 3 (all versions)", resp.Rules[0].AffectedCount)
	}
}

// === #2 去重/冲突检测测试 ===

// TestBlockRuleGenerator_DetectsDuplicateRule
// 验证当 DB 中已有相同 PackageName + Version + MatchType 的启用规则时，
// 草案标记 duplicate_of_id 和 duplicate_of_desc。
func TestBlockRuleGenerator_DetectsDuplicateRule(t *testing.T) {
	db := newGeneratorTestDB(t)
	// 预先创建一条规则
	existingRule := model.BlockRule{
		PackageName: "log4j-core",
		Version:     "<2.17.1",
		MatchType:   model.BlockMatchRange,
		PackageType: "*",
		Reason:      "existing rule",
		Enabled:     true,
	}
	if err := db.Create(&existingRule).Error; err != nil {
		t.Fatalf("create existing rule: %v", err)
	}

	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			PackageName    string `json:"package_name"`
			DuplicateOfID  uint   `json:"duplicate_of_id"`
			DuplicateOfDesc string `json:"duplicate_of_desc"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	r := resp.Rules[0]
	if r.DuplicateOfID != existingRule.ID {
		t.Errorf("DuplicateOfID = %d, want %d", r.DuplicateOfID, existingRule.ID)
	}
	if r.DuplicateOfDesc == "" {
		t.Error("DuplicateOfDesc should not be empty when duplicate detected")
	}
}

// TestBlockRuleGenerator_NoDuplicateWhenDifferentVersion
// 验证当现有规则的 Version 不同时，不标记为重复。
func TestBlockRuleGenerator_NoDuplicateWhenDifferentVersion(t *testing.T) {
	db := newGeneratorTestDB(t)
	// 现有规则版本约束不同（<2.15.0 vs 草案的 <2.17.1）
	existingRule := model.BlockRule{
		PackageName: "log4j-core",
		Version:     "<2.15.0",
		MatchType:   model.BlockMatchRange,
		PackageType: "*",
		Reason:      "existing rule for older range",
		Enabled:     true,
	}
	if err := db.Create(&existingRule).Error; err != nil {
		t.Fatalf("create existing rule: %v", err)
	}

	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_id":    "CVE-2021-44228",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			DuplicateOfID uint `json:"duplicate_of_id"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Rules))
	}
	if resp.Rules[0].DuplicateOfID != 0 {
		t.Errorf("DuplicateOfID = %d, want 0 (different version constraint, not duplicate)", resp.Rules[0].DuplicateOfID)
	}
}

// === #3 批量 CVE 处理测试 ===

// TestBlockRuleGenerator_BatchCVEProcessing
// 验证 source=vulnerability 时支持 cve_ids 数组，一次性处理多个 CVE。
func TestBlockRuleGenerator_BatchCVEProcessing(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	// CVE-1 影响 log4j-core
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)
	// CVE-2 影响 spring-core 和 spring-webmvc
	seedVuln(t, db, scanResult.ID, "CVE-2022-22965", "spring-core", "5.3.18", model.SeverityCritical)
	seedVuln(t, db, scanResult.ID, "CVE-2022-22965", "spring-webmvc", "5.3.18", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_ids":   []interface{}{"CVE-2021-44228", "CVE-2022-22965"},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			PackageName string `json:"package_name"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	// 应生成 3 条规则：log4j-core + spring-core + spring-webmvc
	if len(resp.Rules) != 3 {
		t.Fatalf("expected 3 rules (2 CVEs, 3 packages total), got %d", len(resp.Rules))
	}
	names := map[string]bool{}
	for _, r := range resp.Rules {
		names[r.PackageName] = true
	}
	for _, expected := range []string{"log4j-core", "spring-core", "spring-webmvc"} {
		if !names[expected] {
			t.Errorf("expected rule for %s, not found in %v", expected, names)
		}
	}
}

// TestBlockRuleGenerator_BatchCVEWithOneMissingStillProcessesOthers
// 验证批量 CVE 中某个 CVE 不存在时，其他 CVE 仍正常处理。
func TestBlockRuleGenerator_BatchCVEWithOneMissingStillProcessesOthers(t *testing.T) {
	db := newGeneratorTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVuln(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.17.1", model.SeverityCritical)
	// CVE-9999-9999 不存在

	scanRepo := repository.NewScanRepository(db)
	blockSvc := service.NewBlockRuleService(repository.NewBlockRuleRepository(db), nil)
	tool := NewBlockRuleGeneratorTool(scanRepo, blockSvc)
	tool.SetContext(&ToolContext{DB: db})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "preview",
		"source":    "vulnerability",
		"cve_ids":   []interface{}{"CVE-2021-44228", "CVE-9999-9999"},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var resp struct {
		Rules []struct {
			PackageName string `json:"package_name"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	// CVE-2021-44228 的规则应正常生成，CVE-9999-9999 被跳过
	if len(resp.Rules) != 1 {
		t.Fatalf("expected 1 rule (from existing CVE), got %d", len(resp.Rules))
	}
	if resp.Rules[0].PackageName != "log4j-core" {
		t.Errorf("PackageName = %q, want log4j-core", resp.Rules[0].PackageName)
	}
}
