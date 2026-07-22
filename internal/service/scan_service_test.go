package service

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSecurityScannerTriggerScanLimitsConcurrency(t *testing.T) {
	scanner := &SecurityScanner{
		scanSem: make(chan struct{}, 2),
	}

	var active int64
	var maxActive int64
	var wg sync.WaitGroup
	scanner.scanPackage = func(ctx context.Context, versionID uint, pkgType, name, version string) *model.ScanResult {
		defer wg.Done()
		current := atomic.AddInt64(&active, 1)
		for {
			previous := atomic.LoadInt64(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt64(&maxActive, previous, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&active, -1)
		return nil
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		scanner.TriggerScan(context.Background(), uint(i+1), "npm", "pkg", "1.0.0")
	}
	wg.Wait()

	if got := atomic.LoadInt64(&maxActive); got > 2 {
		t.Fatalf("max concurrent scans = %d, want <= 2", got)
	}
}

func TestSecurityScannerScanAllPackagesUsesBatches(t *testing.T) {
	source, err := os.ReadFile("scan_service.go")
	if err != nil {
		t.Fatalf("read scan service source: %v", err)
	}
	body := extractScanServiceFunctionBodyForTest(string(source), "func (s *SecurityScanner) ScanAllPackages")
	if body == "" {
		t.Fatal("SecurityScanner.ScanAllPackages source not found")
	}
	if !strings.Contains(body, "FindInBatches") {
		t.Fatal("ScanAllPackages should use batched artifact queries")
	}
	if strings.Contains(body, "Find(&artifacts)") {
		t.Fatal("ScanAllPackages should not load all artifacts into memory")
	}
}

func extractScanServiceFunctionBodyForTest(source, signature string) string {
	start := strings.Index(source, signature)
	if start < 0 {
		return ""
	}
	open := strings.Index(source[start:], "{")
	if open < 0 {
		return ""
	}
	pos := start + open
	depth := 0
	for i := pos; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[pos : i+1]
			}
		}
	}
	return ""
}

// === BlockByVulnerability 修复测试（TDD）===

// newScanTestDB 创建内存 SQLite 并 migrate 安全/阻断相关表。
func newScanTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.BlockRule{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedVulnerabilityForCVE 在 DB 中插入一条 vulnerability 记录。
func seedVulnerabilityForCVE(t *testing.T, db *gorm.DB, scanResultID uint, cve, depName, currentVer, fixedVer string, severity model.VulnerabilitySeverity) {
	t.Helper()
	vuln := model.Vulnerability{
		ScanResultID:   scanResultID,
		CVEID:          cve,
		Severity:       severity,
		DependencyName: depName,
		CurrentVersion: currentVer,
		FixedVersion:   fixedVer,
		Title:          "test vulnerability",
	}
	if err := db.Create(&vuln).Error; err != nil {
		t.Fatalf("seed vulnerability: %v", err)
	}
}

// TestBlockByVulnerabilityWithFixedVersionGeneratesRangeRule
// 验证当 Vulnerability 表中存在 FixedVersion 时，BlockByVulnerability 生成
// 基于真实 DependencyName + range 版本约束的阻断规则，而非用规则名当包名。
func TestBlockByVulnerabilityWithFixedVersionGeneratesRangeRule(t *testing.T) {
	db := newScanTestDB(t)
	// 插入 scan result 和 vulnerability
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	seedVulnerabilityForCVE(t, db, scanResult.ID, "CVE-2021-44228", "log4j-core", "2.14.1", "2.17.1", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockRepo := repository.NewBlockRuleRepository(db)
	scanner := &SecurityScanner{
		scanRepo:  scanRepo,
		db:        db,
		blockRepo: blockRepo,
		logger:    logrus.New(),
		scanSem:   make(chan struct{}, 1),
	}

	err := scanner.BlockByVulnerability(context.Background(), "CVE-2021-44228")
	if err != nil {
		t.Fatalf("BlockByVulnerability error: %v", err)
	}

	// 查询创建的规则
	rules, err := blockRepo.List(nil)
	if err != nil {
		t.Fatalf("list block rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 block rule, got %d", len(rules))
	}

	rule := rules[0]
	// 关键断言：PackageName 必须是真实包名，而非 "auto-block-cve-xxx"
	if rule.PackageName != "log4j-core" {
		t.Errorf("PackageName = %q, want %q (real dependency name, not rule name)", rule.PackageName, "log4j-core")
	}
	// FixedVersion 存在时必须用 range 规则阻断所有低于修复版本的版本
	if rule.MatchType != model.BlockMatchRange {
		t.Errorf("MatchType = %q, want %q (range when FixedVersion exists)", rule.MatchType, model.BlockMatchRange)
	}
	if rule.Version != "<2.17.1" {
		t.Errorf("Version = %q, want %q (range constraint based on FixedVersion)", rule.Version, "<2.17.1")
	}
	if !rule.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if !strings.Contains(rule.Reason, "CVE-2021-44228") {
		t.Errorf("Reason = %q, want contains CVE ID", rule.Reason)
	}
}

// TestBlockByVulnerabilityWithoutFixedVersionGeneratesWildcardRule
// 验证当 FixedVersion 为空时，生成 wildcard 规则阻断该包所有版本。
func TestBlockByVulnerabilityWithoutFixedVersionGeneratesWildcardRule(t *testing.T) {
	db := newScanTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	// FixedVersion 为空
	seedVulnerabilityForCVE(t, db, scanResult.ID, "CVE-2023-9999", "vulnerable-pkg", "1.0.0", "", model.SeverityHigh)

	scanRepo := repository.NewScanRepository(db)
	blockRepo := repository.NewBlockRuleRepository(db)
	scanner := &SecurityScanner{
		scanRepo:  scanRepo,
		db:        db,
		blockRepo: blockRepo,
		logger:    logrus.New(),
		scanSem:   make(chan struct{}, 1),
	}

	err := scanner.BlockByVulnerability(context.Background(), "CVE-2023-9999")
	if err != nil {
		t.Fatalf("BlockByVulnerability error: %v", err)
	}

	rules, err := blockRepo.List(nil)
	if err != nil {
		t.Fatalf("list block rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 block rule, got %d", len(rules))
	}

	rule := rules[0]
	if rule.PackageName != "vulnerable-pkg" {
		t.Errorf("PackageName = %q, want %q", rule.PackageName, "vulnerable-pkg")
	}
	if rule.MatchType != model.BlockMatchWildcard {
		t.Errorf("MatchType = %q, want %q (wildcard when FixedVersion empty)", rule.MatchType, model.BlockMatchWildcard)
	}
	if rule.Version != "*" {
		t.Errorf("Version = %q, want %q", rule.Version, "*")
	}
}

// TestBlockByVulnerabilityMultiplePackagesCreatesMultipleRules
// 验证同一个 CVE 影响多个包时，按包名去重后为每个包创建一条规则。
func TestBlockByVulnerabilityMultiplePackagesCreatesMultipleRules(t *testing.T) {
	db := newScanTestDB(t)
	scanResult := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted}
	if err := db.Create(&scanResult).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}
	// 同一 CVE 影响两个包
	seedVulnerabilityForCVE(t, db, scanResult.ID, "CVE-2022-22965", "spring-core", "5.3.17", "5.3.18", model.SeverityCritical)
	seedVulnerabilityForCVE(t, db, scanResult.ID, "CVE-2022-22965", "spring-webmvc", "5.3.17", "5.3.18", model.SeverityCritical)

	scanRepo := repository.NewScanRepository(db)
	blockRepo := repository.NewBlockRuleRepository(db)
	scanner := &SecurityScanner{
		scanRepo:  scanRepo,
		db:        db,
		blockRepo: blockRepo,
		logger:    logrus.New(),
		scanSem:   make(chan struct{}, 1),
	}

	err := scanner.BlockByVulnerability(context.Background(), "CVE-2022-22965")
	if err != nil {
		t.Fatalf("BlockByVulnerability error: %v", err)
	}

	rules, err := blockRepo.List(nil)
	if err != nil {
		t.Fatalf("list block rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 block rules (one per package), got %d", len(rules))
	}

	// 收集包名，验证两个真实包名都被覆盖
	names := map[string]bool{}
	for _, r := range rules {
		names[r.PackageName] = true
	}
	if !names["spring-core"] || !names["spring-webmvc"] {
		t.Errorf("expected rules for spring-core and spring-webmvc, got %v", names)
	}
}

// TestBlockByVulnerabilityNoVulnerabilityDataReturnsError
// 验证当 DB 中没有该 CVE 的 vulnerability 记录时，返回明确错误而非创建无效规则。
func TestBlockByVulnerabilityNoVulnerabilityDataReturnsError(t *testing.T) {
	db := newScanTestDB(t)
	scanRepo := repository.NewScanRepository(db)
	blockRepo := repository.NewBlockRuleRepository(db)
	scanner := &SecurityScanner{
		scanRepo:  scanRepo,
		db:        db,
		blockRepo: blockRepo,
		logger:    logrus.New(),
		scanSem:   make(chan struct{}, 1),
	}

	err := scanner.BlockByVulnerability(context.Background(), "CVE-9999-9999")
	if err == nil {
		t.Fatal("expected error when no vulnerability data found, got nil")
	}

	// 确保没有创建任何规则
	rules, err := blockRepo.List(nil)
	if err != nil {
		t.Fatalf("list block rules: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 block rules, got %d", len(rules))
	}
}
