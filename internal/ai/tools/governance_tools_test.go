package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newGovernanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.BlockRule{},
		&model.Artifact{},
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.Repository{},
		&model.Package{},
		&model.DownloadLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func runTool(t *testing.T, tool interface {
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}, db *gorm.DB, params map[string]interface{}) string {
	t.Helper()
	ctx := context.Background()
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("tool execute: %v", err)
	}
	return result
}

// ===== dependency_impact =====

// TestDependencyImpact_BlastRadius 验证漏洞表反查 + 阻断状态 + npm metadata 反查。
func TestDependencyImpact_BlastRadius(t *testing.T) {
	db := newGovernanceTestDB(t)

	// 仓库
	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	db.Create(&repo)
	// 目标包被阻断
	db.Create(&model.BlockRule{
		PackageName: "log4j", Version: "<2.17.1", MatchType: model.BlockMatchRange,
		PackageType: "maven", Enabled: true, Reason: "CVE-2021-44228",
	})
	// 依赖方 artifact（含扫描漏洞）
	app := model.Artifact{RepositoryID: repo.ID, Format: "npm", Name: "my-app", Version: "1.0.0"}
	db.Create(&app)
	scan := model.ScanResult{ComponentID: app.ID, ScanStatus: model.ScanStatusCompleted, TotalVulnerabilities: 1}
	db.Create(&scan)
	db.Create(&model.Vulnerability{
		ScanResultID: scan.ID, CVEID: "CVE-2021-44228", Severity: model.SeverityCritical,
		DependencyName: "log4j", CurrentVersion: "2.14.1", FixedVersion: "2.17.1",
		Title: "Log4Shell", IsDirectDep: true,
	})
	// npm metadata 依赖方
	dep := model.Artifact{
		RepositoryID: repo.ID, Format: "npm", Name: "another-lib", Version: "2.0.0",
		Attributes: model.JSONB{"dependencies": `{"lodash":"^4.17.21","log4j":"2.14.1"}`},
	}
	db.Create(&dep)

	tool := NewDependencyImpactTool()
	tool.SetContext(&ToolContext{DB: db})
	result := runTool(t, tool, db, map[string]interface{}{"package_name": "log4j"})

	var resp struct {
		Blocked          bool     `json:"blocked"`
		BlockingRules    []string `json:"blocking_rules"`
		TotalDependents  int      `json:"total_dependents"`
		DirectDependents []struct {
			PackageName string `json:"package_name"`
		} `json:"direct_dependents"`
		DataSources []string `json:"data_sources"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("unmarshal result: %v, result=%s", err, result)
	}

	if !resp.Blocked {
		t.Error("target should be marked blocked")
	}
	if len(resp.BlockingRules) == 0 {
		t.Error("blocking rules should be reported")
	}
	if resp.TotalDependents != 2 {
		t.Errorf("total dependents = %d, want 2 (my-app via scan, another-lib via metadata)", resp.TotalDependents)
	}
	names := map[string]bool{}
	for _, d := range resp.DirectDependents {
		names[d.PackageName] = true
	}
	if !names["my-app"] || !names["another-lib"] {
		t.Errorf("dependents missing: %v", names)
	}
	if !strings.Contains(result, "CVE-2021-44228") {
		t.Error("result should mention the CVE rule")
	}
}

// TestDependencyImpact_NoDependents 验证无依赖方时的输出。
func TestDependencyImpact_NoDependents(t *testing.T) {
	db := newGovernanceTestDB(t)
	tool := NewDependencyImpactTool()
	tool.SetContext(&ToolContext{DB: db})
	result := runTool(t, tool, db, map[string]interface{}{"package_name": "ghost-pkg"})

	if !strings.Contains(result, `"total_dependents": 0`) {
		t.Errorf("expected zero dependents, got: %s", result)
	}
	if !strings.Contains(result, "未被阻断") {
		t.Error("expected unblocked recommendation note")
	}
}

// ===== download_anomaly =====

// TestDownloadAnomaly_OverviewAndSpike 验证概览与骤增检测。
func TestDownloadAnomaly_OverviewAndSpike(t *testing.T) {
	db := newGovernanceTestDB(t)
	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	db.Create(&repo)

	now := time.Now()
	// 基线期（[now-48h, now-24h) 内）：pkg-a 每天 10 次 → 30h 前 50 条
	for i := 0; i < 50; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusSuccess, CreatedAt: now.Add(-30 * time.Hour),
		})
	}
	// 观察期：pkg-a 骤增到 300 次
	for i := 0; i < 300; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusSuccess, CreatedAt: now.Add(-2 * time.Hour),
		})
	}
	// 新包：只在观察期出现
	for i := 0; i < 40; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "brand-new-pkg",
			Status: model.DownloadStatusSuccess, CreatedAt: now.Add(-1 * time.Hour),
		})
	}

	tool := NewDownloadAnomalyTool()
	tool.SetContext(&ToolContext{DB: db})

	overview := runTool(t, tool, db, map[string]interface{}{"analysis_type": "overview", "hours": float64(24)})
	if !strings.Contains(overview, "pkg-a") {
		t.Errorf("overview should include pkg-a: %s", overview)
	}
	if !strings.Contains(overview, "brand-new-pkg") {
		t.Errorf("overview should include brand-new-pkg: %s", overview)
	}

	spike := runTool(t, tool, db, map[string]interface{}{"analysis_type": "spike_detection", "hours": float64(24), "threshold": float64(3)})
	if !strings.Contains(spike, "pkg-a") {
		t.Errorf("spike detection should flag pkg-a: %s", spike)
	}

	newPkg := runTool(t, tool, db, map[string]interface{}{"analysis_type": "new_package", "hours": float64(24)})
	if !strings.Contains(newPkg, "brand-new-pkg") {
		t.Errorf("new package detection should flag brand-new-pkg: %s", newPkg)
	}
}

// TestDownloadAnomaly_IPFocus 验证 IP 集中度。
func TestDownloadAnomaly_IPFocus(t *testing.T) {
	db := newGovernanceTestDB(t)
	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	db.Create(&repo)

	for i := 0; i < 90; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusSuccess, IPAddress: "10.0.0.1",
			CreatedAt: time.Now().Add(-1 * time.Hour),
		})
	}
	for i := 0; i < 10; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusSuccess, IPAddress: "10.0.0.2",
			CreatedAt: time.Now().Add(-1 * time.Hour),
		})
	}

	tool := NewDownloadAnomalyTool()
	tool.SetContext(&ToolContext{DB: db})
	result := runTool(t, tool, db, map[string]interface{}{"analysis_type": "ip_focus", "hours": float64(24)})

	if !strings.Contains(result, "10.0.0.1") {
		t.Errorf("ip focus should list 10.0.0.1: %s", result)
	}
	if !strings.Contains(result, "90.0%") {
		t.Errorf("ip focus should show >50%% concentration: %s", result)
	}
}

// TestDownloadAnomaly_FailedSpikeZeroBaseline 验证零基线骤增被识别：
// 基线期失败率 0%（全成功）的包，观察期失败率 ≥50% 应被标记；
// 旧实现 baseRate>=0.05 的门槛会让 0% → 60% 这类骤增漏报。
func TestDownloadAnomaly_FailedSpikeZeroBaseline(t *testing.T) {
	db := newGovernanceTestDB(t)
	repo := model.Repository{Name: "npm-proxy", PackageType: "npm", Type: "proxy"}
	db.Create(&repo)

	now := time.Now()
	// 基线期（[now-48h, now-24h)）：pkg-a 全部成功 → baseRate=0
	for i := 0; i < 100; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusSuccess, CreatedAt: now.Add(-30 * time.Hour),
		})
	}
	// 观察期：pkg-a 失败率 15/25=60%
	for i := 0; i < 10; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusSuccess, CreatedAt: now.Add(-2 * time.Hour),
		})
	}
	for i := 0; i < 15; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-a",
			Status: model.DownloadStatusFailed, CreatedAt: now.Add(-1 * time.Hour),
		})
	}

	// pkg-b 同为零基线但观察期失败率仅 10% → 不应误报
	for i := 0; i < 18; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-b",
			Status: model.DownloadStatusSuccess, CreatedAt: now.Add(-2 * time.Hour),
		})
	}
	for i := 0; i < 2; i++ {
		db.Create(&model.DownloadLog{
			RepositoryID: repo.ID, PackageType: "npm", PackageName: "pkg-b",
			Status: model.DownloadStatusFailed, CreatedAt: now.Add(-1 * time.Hour),
		})
	}

	tool := NewDownloadAnomalyTool()
	tool.SetContext(&ToolContext{DB: db})
	result := runTool(t, tool, db, map[string]interface{}{
		"analysis_type": "failed_spike", "hours": float64(24), "threshold": float64(3),
	})

	if !strings.Contains(result, "pkg-a") {
		t.Errorf("zero-baseline spike should flag pkg-a (0%% -> 60%%): %s", result)
	}
	if strings.Contains(result, "pkg-b") {
		t.Errorf("pkg-b (0%% -> 10%%) should NOT be flagged: %s", result)
	}
}

// ===== license_analyzer =====

// TestLicenseAnalyzer_Classification 验证许可证分类与 risky 检测。
func TestLicenseAnalyzer_Classification(t *testing.T) {
	db := newGovernanceTestDB(t)
	repo := model.Repository{Name: "maven-hosted", PackageType: "maven", Type: "hosted"}
	db.Create(&repo)

	packages := []model.Package{
		{RepositoryID: repo.ID, Format: "npm", Name: "good-lib", License: "MIT"},
		{RepositoryID: repo.ID, Format: "npm", Name: "bsd-lib", License: "BSD-3-Clause"},
		{RepositoryID: repo.ID, Format: "maven", Name: "gpl-lib", License: "GPL-3.0"},
		{RepositoryID: repo.ID, Format: "npm", Name: "agpl-lib", License: "AGPL-3.0"},
		{RepositoryID: repo.ID, Format: "npm", Name: "unknown-lib", License: ""},
		{RepositoryID: repo.ID, Format: "pypi", Name: "proprietary-lib", License: "Proprietary"},
	}
	for _, p := range packages {
		db.Create(&p)
	}

	tool := NewLicenseAnalyzerTool()
	tool.SetContext(&ToolContext{DB: db})

	overview := runTool(t, tool, db, map[string]interface{}{"analysis_type": "overview"})
	for _, want := range []string{"宽松许可", "Copyleft", "未知", "限制性"} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview missing %q: %s", want, overview)
		}
	}

	risky := runTool(t, tool, db, map[string]interface{}{"analysis_type": "risky"})
	if !strings.Contains(risky, "gpl-lib") || !strings.Contains(risky, "agpl-lib") || !strings.Contains(risky, "proprietary-lib") {
		t.Errorf("risky should list copyleft/restricted packages: %s", risky)
	}
	if strings.Contains(risky, "good-lib") {
		t.Error("risky should not list MIT package")
	}

	unknown := runTool(t, tool, db, map[string]interface{}{"analysis_type": "unknown"})
	if !strings.Contains(unknown, "unknown-lib") {
		t.Errorf("unknown should list unknown-lib: %s", unknown)
	}

	byLicense := runTool(t, tool, db, map[string]interface{}{"analysis_type": "by_license"})
	if !strings.Contains(byLicense, "MIT") || !strings.Contains(byLicense, "GPL-3.0") {
		t.Errorf("by_license should include MIT and GPL-3.0: %s", byLicense)
	}
}

// TestLicenseAnalyzer_BlockedRules 验证 license 阻断规则查询。
func TestLicenseAnalyzer_BlockedRules(t *testing.T) {
	db := newGovernanceTestDB(t)
	db.Create(&model.BlockRule{
		PackageName: "*", Version: "*", MatchType: model.BlockMatchWildcard,
		PackageType: model.PackageTypeAll, Enabled: true, Reason: "禁止 AGPL",
		ConditionType: model.ConditionTypeLicense, ConditionOp: model.ConditionOpContains,
		ConditionValue: "AGPL",
	})

	tool := NewLicenseAnalyzerTool()
	tool.SetContext(&ToolContext{DB: db})
	result := runTool(t, tool, db, map[string]interface{}{"analysis_type": "blocked"})

	if !strings.Contains(result, "AGPL") {
		t.Errorf("blocked should list license rules with AGPL: %s", result)
	}
}
