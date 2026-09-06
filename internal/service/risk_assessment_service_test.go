package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/plugins/maven"
	"github.com/dshmyz/moonlight-box/internal/plugins/npm"
	"github.com/dshmyz/moonlight-box/internal/repository"
	ver "github.com/dshmyz/moonlight-box/internal/version"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newRiskAssessmentTestService 注册真实协议插件的依赖反查能力，供依赖匹配测试使用。
func newRiskAssessmentTestService(t *testing.T, db *gorm.DB) *RiskAssessmentService {
	t.Helper()
	svc := NewRiskAssessmentService(db)
	svc.SetDependencyResolvers(map[string]runtime.DependencyResolver{
		"npm":   npm.NewNpmPlugin(&http.Client{}),
		"maven": maven.NewMavenPlugin(&http.Client{}),
	})
	return svc
}

func newRiskAssessmentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.RiskAssessment{},
		&model.RiskAssessmentItem{},
		&model.Artifact{},
		&model.Repository{},
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.VulnRule{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestVersionMatches(t *testing.T) {
	cases := []struct {
		risky, actual string
		want          bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"1.2.3", "1.2.3-SNAPSHOT", false},
		{"1.2.3-SNAPSHOT", "1.2.3-SNAPSHOT", true},
		{"1.2.x", "1.2.3", true},
		{"1.2.x", "1.2.0", true},
		{"1.2.x", "1.3.0", false},
		{"1.2*", "1.2.9", true},
		{"1.2", "1.2.3", true},
		{"1.2", "1.2", true},
		{"1.2", "1.20", false}, // 前缀须以点分隔，避免误匹配 1.20
		{"1.2", "1.2.3.4", true},
		{"2.0.0", "1.0.0", false},
		{"1.2.3", "", false},
		{"", "1.2.3", false},
		// 版本族命中基础版
		{"1.2.x", "1.2", true},
		{"1.2*", "1.2", true},
		// 裸通配匹配全部版本
		{"x", "1.0.0", true},
		{"X", "2.3.1", true},
		{"*", "9.9.9", true},
		// Go 模块 v 前缀归一化
		{"1.2.3", "v1.2.3", true},
		{"v1.2.3", "1.2.3", true},
		{"v1.2.x", "1.2.5", true},
		{"1.2.3", "v1.2.4", false},
	}
	for _, c := range cases {
		if got := ver.Matches(c.risky, c.actual); got != c.want {
			t.Errorf("ver.Matches(%q, %q) = %v, want %v", c.risky, c.actual, got, c.want)
		}
	}
}

func TestParseComponents_CSV(t *testing.T) {
	svc := NewRiskAssessmentService(nil)

	t.Run("中文表头别名", func(t *testing.T) {
		csv := "组件名,版本,风险等级,原因\nlog4j,2.14.1,高危,Log4Shell\nfastjson,1.2.83,中危,\n\n"
		components, err := svc.ParseComponents(strings.NewReader(csv), "risk.csv")
		if err != nil {
			t.Fatalf("parse csv: %v", err)
		}
		if len(components) != 2 {
			t.Fatalf("got %d components, want 2", len(components))
		}
		if components[0].Name != "log4j" || components[0].Version != "2.14.1" || components[0].Severity != "高危" {
			t.Errorf("unexpected first component: %+v", components[0])
		}
		if components[1].Reason != "" {
			t.Errorf("unexpected reason: %+v", components[1])
		}
	})

	t.Run("英文表头", func(t *testing.T) {
		csv := "name,version,cve\nlodash,4.17.21,CVE-2021-23337\n"
		components, err := svc.ParseComponents(strings.NewReader(csv), "risk.csv")
		if err != nil {
			t.Fatalf("parse csv: %v", err)
		}
		if len(components) != 1 || components[0].Name != "lodash" || components[0].CVE != "CVE-2021-23337" {
			t.Errorf("unexpected components: %+v", components)
		}
	})

	t.Run("缺少表头报错", func(t *testing.T) {
		_, err := svc.ParseComponents(strings.NewReader("foo,bar\n1,2\n"), "risk.csv")
		if err == nil || !strings.Contains(err.Error(), "未识别到表头") {
			t.Errorf("want header error, got %v", err)
		}
	})

	t.Run("缺少必填字段报错", func(t *testing.T) {
		csv := "组件名,版本\nlog4j,\n"
		_, err := svc.ParseComponents(strings.NewReader(csv), "risk.csv")
		if err == nil || !strings.Contains(err.Error(), "缺少组件名或版本") {
			t.Errorf("want missing-field error, got %v", err)
		}
	})
}

func TestParseComponents_Excel(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	_ = f.SetCellValue(sheet, "A1", "包名")
	_ = f.SetCellValue(sheet, "B1", "版本")
	_ = f.SetCellValue(sheet, "C1", "风险等级")
	_ = f.SetCellValue(sheet, "A2", "spring-core")
	_ = f.SetCellValue(sheet, "B2", "5.3.18")
	_ = f.SetCellValue(sheet, "C2", "严重")
	_ = f.SetCellValue(sheet, "A3", "commons-logging")
	_ = f.SetCellValue(sheet, "B3", "1.2")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	svc := NewRiskAssessmentService(nil)
	components, err := svc.ParseComponents(bytes.NewReader(buf.Bytes()), "risk.xlsx")
	if err != nil {
		t.Fatalf("parse excel: %v", err)
	}
	if len(components) != 2 {
		t.Fatalf("got %d components, want 2", len(components))
	}
	if components[0].Name != "spring-core" || components[0].Version != "5.3.18" || components[0].Severity != "严重" {
		t.Errorf("unexpected first component: %+v", components[0])
	}
}

func TestAnalyze_ArtifactMatch(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	proxy := model.Repository{Name: "npm-proxy", Type: "proxy"}
	db.Create(&proxy)

	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.17.1"})
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "fastjson", Version: "1.2.83"})
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "app-core", Version: "1.0.0-SNAPSHOT"})
	// npm 元数据（应被 kind 过滤，不参与制品命中）
	db.Create(&model.Artifact{RepositoryID: proxy.ID, Format: "npm", Name: "lodash", Version: "4.17.21", Kind: "metadata"})

	svc := newRiskAssessmentTestService(t, db)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1", Severity: "高危"},
		{Name: "fastjson", Version: "1.2.x"},
		{Name: "app-core", Version: "1.0.0-SNAPSHOT"},
		{Name: "not-exist", Version: "9.9.9"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	if assessment.TotalItems != 4 {
		t.Errorf("total_items = %d, want 4", assessment.TotalItems)
	}
	if assessment.MatchedItems != 3 || assessment.ArtifactHits != 3 || assessment.DependencyHits != 0 {
		t.Errorf("counts = matched %d artifact %d dep %d, want 3/3/0",
			assessment.MatchedItems, assessment.ArtifactHits, assessment.DependencyHits)
	}

	byName := map[string]*model.RiskAssessmentItem{}
	for i := range assessment.Items {
		byName[assessment.Items[i].Name] = &assessment.Items[i]
	}
	if len(byName["log4j"].ArtifactHit) != 1 {
		t.Errorf("log4j hits = %+v, want 1 (仅 2.14.1)", byName["log4j"].ArtifactHit)
	}
	if hit := byName["log4j"].ArtifactHit[0]; hit["repo"] != "central" || hit["format"] != "maven" {
		t.Errorf("log4j hit = %+v", hit)
	}
	// 版本族 1.2.x 命中 1.2.83
	if len(byName["fastjson"].ArtifactHit) != 1 || byName["fastjson"].ArtifactHit[0]["version"] != "1.2.83" {
		t.Errorf("fastjson hits = %+v", byName["fastjson"].ArtifactHit)
	}
	// SNAPSHOT 精确命中
	if len(byName["app-core"].ArtifactHit) != 1 || byName["app-core"].ArtifactHit[0]["version"] != "1.0.0-SNAPSHOT" {
		t.Errorf("app-core hits = %+v", byName["app-core"].ArtifactHit)
	}
	// 未命中
	if byName["not-exist"].Matched || len(byName["not-exist"].ArtifactHit) > 0 {
		t.Errorf("not-exist should not match: %+v", byName["not-exist"])
	}
}

func TestAnalyze_DependencyMatch(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "npm-proxy", Type: "proxy"}
	db.Create(&repo)

	// npm metadata 依赖方：声明了 lodash ^4.17.21 与 left-pad 1.3.0
	db.Create(&model.Artifact{
		RepositoryID: repo.ID, Format: "npm", Name: "my-app", Version: "1.0.0",
		Attributes: model.JSONB{"dependencies": `{"lodash":"^4.17.21","left-pad":"1.3.0"}`},
	})
	// 漏洞反查依赖方：my-backend 的扫描记录声明依赖 log4j 2.14.1（风险版本）
	app := model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "my-backend", Version: "2.0.0"}
	db.Create(&app)
	scan := model.ScanResult{ComponentID: app.ID, ScanStatus: model.ScanStatusCompleted}
	db.Create(&scan)
	db.Create(&model.Vulnerability{
		ScanResultID: scan.ID, CVEID: "CVE-2021-44228", Severity: model.SeverityCritical,
		DependencyName: "log4j", CurrentVersion: "2.14.1",
	})
	// my-safe-backend 使用安全的 log4j 2.17.1，不应因 2.14.1 风险被误报
	safeApp := model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "my-safe-backend", Version: "3.0.0"}
	db.Create(&safeApp)
	safeScan := model.ScanResult{ComponentID: safeApp.ID, ScanStatus: model.ScanStatusCompleted}
	db.Create(&safeScan)
	db.Create(&model.Vulnerability{
		ScanResultID: safeScan.ID, CVEID: "CVE-2021-44228", Severity: model.SeverityCritical,
		DependencyName: "log4j", CurrentVersion: "2.17.1",
	})

	svc := newRiskAssessmentTestService(t, db)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "lodash", Version: "4.17.21"},          // 约束 ^4.17.21 覆盖 → 命中
		{Name: "lodash", Version: "4.16.0"},           // 约束不覆盖 → 不命中
		{Name: "left-pad", Version: "1.3.0"},          // 约束 1.3.0 覆盖 → 命中
		{Name: "log4j", Version: "2.14.1"},            // 漏洞反查 current_version 匹配 → 命中
		{Name: "log4j", Version: "3.0.0"},             // 无任何依赖方当前版本为 3.0.0 → 不命中
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	byName := map[string][]*model.RiskAssessmentItem{}
	for i := range assessment.Items {
		key := assessment.Items[i].Name + "|" + assessment.Items[i].Version
		byName[key] = append(byName[key], &assessment.Items[i])
	}

	if got := len(byName["lodash|4.17.21"][0].DependencyHit); got != 1 {
		t.Errorf("lodash 4.17.21 dep hits = %d, want 1", got)
	}
	if got := len(byName["lodash|4.16.0"][0].DependencyHit); got != 0 {
		t.Errorf("lodash 4.16.0 dep hits = %d, want 0 (约束 ^4.17.21 不覆盖)", got)
	}
	if got := len(byName["left-pad|1.3.0"][0].DependencyHit); got != 1 {
		t.Errorf("left-pad dep hits = %d, want 1", got)
	}
	if hit := byName["left-pad|1.3.0"][0].DependencyHit[0]; hit["dependent_name"] != "my-app" || hit["constraint"] != "1.3.0" {
		t.Errorf("left-pad hit = %+v", hit)
	}
	if got := len(byName["log4j|2.14.1"][0].DependencyHit); got != 1 {
		t.Errorf("log4j dep hits = %d, want 1 (漏洞反查)", got)
	}
	if hit := byName["log4j|2.14.1"][0].DependencyHit[0]; hit["dependent_name"] != "my-backend" {
		t.Errorf("log4j hit = %+v", hit)
	}
	// 版本过滤：风险 3.0.0 无项目使用该版本，不命中；安全版本 2.17.1 不被 2.14.1 误报
	if got := len(byName["log4j|3.0.0"][0].DependencyHit); got != 0 {
		t.Errorf("log4j 3.0.0 dep hits = %d, want 0 (版本过滤)", got)
	}
}

func TestAnalyze_Persistence(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})

	svc := newRiskAssessmentTestService(t, db)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 7, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1"},
		{Name: "missing", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if assessment.ID == 0 {
		t.Fatal("assessment id should be set after persistence")
	}

	// GetAssessment 回读
	got, err := svc.GetAssessment(context.Background(), assessment.ID)
	if err != nil {
		t.Fatalf("get assessment: %v", err)
	}
	if got.FileName != "risk.xlsx" || got.CreatedBy != 7 || len(got.Items) != 2 {
		t.Errorf("got assessment = %+v", got)
	}
	if !got.Items[0].Matched || got.Items[1].Matched {
		t.Errorf("item matched flags wrong: %+v", got.Items)
	}

	// ListAssessments
	list, total, err := svc.ListAssessments(context.Background(), 1, 10)
	if err != nil || total != 1 || len(list) != 1 {
		t.Errorf("list = %d/%d, err %v", len(list), total, err)
	}

	// DeleteAssessment
	if err := svc.DeleteAssessment(context.Background(), assessment.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetAssessment(context.Background(), assessment.ID); err == nil {
		t.Error("assessment should be gone after delete")
	}
	var itemCount int64
	db.Model(&model.RiskAssessmentItem{}).Where("assessment_id = ?", assessment.ID).Count(&itemCount)
	if itemCount != 0 {
		t.Errorf("items should be cascade deleted, count = %d", itemCount)
	}
}

func TestExportAssessment(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})

	svc := newRiskAssessmentTestService(t, db)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	data, err := svc.ExportAssessment(context.Background(), assessment.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(data) == 0 || !bytes.Contains(data, []byte("PK")) {
		t.Error("export should produce a non-empty xlsx (PK zip header)")
	}
}

func TestTemplateBytes(t *testing.T) {
	svc := NewRiskAssessmentService(nil)
	data, err := svc.TemplateBytes()
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	if len(data) == 0 || !bytes.Contains(data, []byte("PK")) {
		t.Error("template should be a valid xlsx (PK zip header)")
	}
}

func TestEnrichSuggestions(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	db.Create(&model.VulnRule{
		PackagePattern: "log4j", PackageType: "maven", CVE: "CVE-2021-44228",
		Severity: model.SeverityCritical, MaxVersion: "2.17.0", FixedVersion: "2.17.1", Enabled: true,
	})
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})

	svc := newRiskAssessmentTestService(t, db)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1"},
		{Name: "missing", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	svc.EnrichSuggestions(context.Background(), assessment)

	hit := assessment.Items[0]
	if !hit.Matched || !strings.Contains(hit.Suggestion, "制品命中") || !strings.Contains(hit.Suggestion, "2.17.1") {
		t.Errorf("matched item suggestion unexpected: %q", hit.Suggestion)
	}
	if !strings.Contains(assessment.Items[1].Suggestion, "未命中") {
		t.Errorf("unmatched item suggestion unexpected: %q", assessment.Items[1].Suggestion)
	}
}

func TestCreateBlockRules(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatalf("migrate block rule: %v", err)
	}
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "npm", Name: "lodash", Version: "4.17.18"})

	svc := newRiskAssessmentTestService(t, db)
	svc.SetBlockRuleService(NewBlockRuleService(repository.NewBlockRuleRepository(db), nil))
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1"},
		{Name: "lodash", Version: "4.17.x"},
		{Name: "missing", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	count, err := svc.CreateBlockRules(context.Background(), assessment.ID, nil)
	if err != nil {
		t.Fatalf("create block rules: %v", err)
	}
	if count != 2 {
		t.Fatalf("created %d block rules, want 2", count)
	}

	var rules []model.BlockRule
	db.Find(&rules)
	if len(rules) != 2 {
		t.Fatalf("block rules in db = %d, want 2", len(rules))
	}
	for _, r := range rules {
		switch r.PackageName {
		case "log4j":
			if r.MatchType != model.BlockMatchExact || r.Version != "2.14.1" || r.PackageType != "maven" {
				t.Errorf("log4j rule unexpected: %+v", r)
			}
		case "lodash":
			if r.MatchType != model.BlockMatchWildcard || r.Version != "4.17.*" || r.PackageType != "npm" {
				t.Errorf("lodash rule unexpected: %+v", r)
			}
		default:
			t.Errorf("unexpected rule: %+v", r)
		}
	}

	// 指定 item_ids 时应只生成对应规则
	itemIDs := []uint{assessment.Items[0].ID}
	count, err = svc.CreateBlockRules(context.Background(), assessment.ID, itemIDs)
	if err != nil || count != 1 {
		t.Errorf("item-filtered create = %d, err %v; want 1", count, err)
	}
}

func TestDisposeItem(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatalf("migrate block rule: %v", err)
	}
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})

	// mock ArtifactRemover，记录删除调用
	removed := []string{}
	remover := &fakeArtifactRemover{onRemove: func(repoID uint, format, name, version string) error {
		removed = append(removed, fmt.Sprintf("%d|%s|%s|%s", repoID, format, name, version))
		return nil
	}}

	svc := newRiskAssessmentTestService(t, db)
	svc.SetBlockRuleService(NewBlockRuleService(repository.NewBlockRuleRepository(db), nil))
	svc.SetArtifactService(remover)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	item := assessment.Items[0]

	// remove：应删除制品并自动生成阻断规则
	disposed, err := svc.DisposeItem(context.Background(), DisposeInput{
		ItemID: item.ID, Action: "remove", Note: "上级要求", UserID: 1,
	})
	if err != nil {
		t.Fatalf("dispose remove: %v", err)
	}
	if disposed.Disposition != model.DispositionRemoved {
		t.Errorf("disposition = %q, want removed", disposed.Disposition)
	}
	if disposed.DisposedAt == nil || disposed.DisposedBy != 1 {
		t.Errorf("dispose trace missing: %+v", disposed)
	}
	if !strings.Contains(disposed.DispositionNote, "移除 1 个制品版本") || !strings.Contains(disposed.DispositionNote, "上级要求") {
		t.Errorf("note = %q", disposed.DispositionNote)
	}
	if len(removed) != 1 || removed[0] != fmt.Sprintf("%d|maven|log4j|2.14.1", repo.ID) {
		t.Errorf("removal calls = %v", removed)
	}
	var blockRules int64
	db.Model(&model.BlockRule{}).Where("package_name = ?", "log4j").Count(&blockRules)
	if blockRules != 1 {
		t.Errorf("auto block rules = %d, want 1", blockRules)
	}

	// ignore：留痕
	disposed, err = svc.DisposeItem(context.Background(), DisposeInput{ItemID: item.ID, Action: "ignore", Note: "误报"})
	if err != nil {
		t.Fatalf("dispose ignore: %v", err)
	}
	if disposed.Disposition != model.DispositionIgnored || disposed.DispositionNote != "误报" {
		t.Errorf("ignore result: %+v", disposed)
	}

	// reset：清空
	disposed, err = svc.DisposeItem(context.Background(), DisposeInput{ItemID: item.ID, Action: "reset"})
	if err != nil {
		t.Fatalf("dispose reset: %v", err)
	}
	if disposed.Disposition != model.DispositionNone || disposed.DisposedAt != nil {
		t.Errorf("reset result: %+v", disposed)
	}

	// 统计
	stats, err := svc.AssessmentDisposeStats(context.Background(), assessment.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Total != 1 || stats.Disposed != 0 {
		t.Errorf("stats = %+v", stats)
	}

	// remove 非法动作报错
	if _, err = svc.DisposeItem(context.Background(), DisposeInput{ItemID: item.ID, Action: "bogus"}); err == nil {
		t.Error("bogus action should fail")
	}
	// 无制品命中的明细不能 remove
	noHit, err := svc.Analyze(context.Background(), "r2.xlsx", 1, []RiskComponentInput{{Name: "ghost", Version: "1.0"}})
	if err != nil {
		t.Fatalf("analyze ghost: %v", err)
	}
	if _, err = svc.DisposeItem(context.Background(), DisposeInput{ItemID: noHit.Items[0].ID, Action: "remove"}); err == nil {
		t.Error("remove on non-matched item should fail")
	}
}

type fakeArtifactRemover struct {
	onRemove func(repoID uint, format, name, version string) error
}

func (f *fakeArtifactRemover) DeletePackageVersionByCoordinates(ctx context.Context, repoID uint, format, name, version string) error {
	return f.onRemove(repoID, format, name, version)
}

func TestAnalyze_FormatFilter(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repoNpm := model.Repository{Name: "central-npm", Type: "hosted"}
	repoPypi := model.Repository{Name: "central-pypi", Type: "hosted"}
	db.Create(&repoNpm)
	db.Create(&repoPypi)

	// 同名不同格式、不同仓库的制品
	db.Create(&model.Artifact{RepositoryID: repoNpm.ID, Format: "npm", Name: "fastjson", Version: "1.2.83"})
	db.Create(&model.Artifact{RepositoryID: repoPypi.ID, Format: "pypi", Name: "fastjson", Version: "1.2.83"})

	svc := newRiskAssessmentTestService(t, db)

	// 限定 npm：只命中 npm 仓库
	assessment, err := svc.Analyze(context.Background(), "r.xlsx", 1, []RiskComponentInput{
		{Name: "fastjson", Version: "1.2.83", Format: "npm"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	item := assessment.Items[0]
	if len(item.ArtifactHit) != 1 || item.ArtifactHit[0]["format"] != "npm" || item.ArtifactHit[0]["repo"] != "central-npm" {
		t.Errorf("npm-scoped hits = %+v, want only npm", item.ArtifactHit)
	}
	if item.Format != "npm" {
		t.Errorf("item.format = %q, want npm", item.Format)
	}

	// 不限格式：两个都命中
	assessment, err = svc.Analyze(context.Background(), "r.xlsx", 1, []RiskComponentInput{
		{Name: "fastjson", Version: "1.2.83"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(assessment.Items[0].ArtifactHit) != 2 {
		t.Errorf("unscoped hits = %+v, want 2", assessment.Items[0].ArtifactHit)
	}
}

func TestParseComponents_FormatColumn(t *testing.T) {
	svc := NewRiskAssessmentService(nil)
	csv := "组件名,版本,格式\nfastjson,1.2.83,maven\nlodash,4.17.18,\n"
	components, err := svc.ParseComponents(strings.NewReader(csv), "risk.csv")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(components) != 2 {
		t.Fatalf("got %d, want 2", len(components))
	}
	if components[0].Format != "maven" {
		t.Errorf("format = %q, want maven", components[0].Format)
	}
	if components[1].Format != "" {
		t.Errorf("empty format = %q, want ''", components[1].Format)
	}
}

func TestAnalyze_MavenDependencyMatch(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)

	// maven 制品声明 POM 依赖（attributes.dependencies 为 JSON 字符串，与插件写入一致）
	db.Create(&model.Artifact{
		RepositoryID: repo.ID, Format: "maven", Name: "my-app", Version: "1.0.0",
		Attributes: model.JSONB{"dependencies": `{"org.apache.logging.log4j:log4j-core":"2.14.1"}`},
	})
	// 依赖安全版本 2.17.1 的制品不应命中风险 2.14.1
	db.Create(&model.Artifact{
		RepositoryID: repo.ID, Format: "maven", Name: "my-app2", Version: "2.0.0",
		Attributes: model.JSONB{"dependencies": `{"org.apache.logging.log4j:log4j-core":"2.17.1"}`},
	})
	// npm 制品不应被 maven 反查误命中
	db.Create(&model.Artifact{
		RepositoryID: repo.ID, Format: "npm", Name: "js-log4j", Version: "1.0.0",
		Attributes: model.JSONB{"dependencies": `{"log4j-core":"2.14.1"}`},
	})

	svc := newRiskAssessmentTestService(t, db)
	assessment, err := svc.Analyze(context.Background(), "r.xlsx", 1, []RiskComponentInput{
		{Name: "log4j-core", Version: "2.14.1", Format: "maven"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	item := assessment.Items[0]
	if len(item.DependencyHit) != 1 {
		t.Fatalf("maven dep hits = %+v, want 1", item.DependencyHit)
	}
	h := item.DependencyHit[0]
	if h["dependent_name"] != "my-app" || h["format"] != "maven" || h["constraint"] != "2.14.1" {
		t.Errorf("hit = %+v, want my-app/maven/2.14.1", h)
	}
}

func TestDisposeItem_PartialRemoveFailurePersistsStatus(t *testing.T) {
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.2"})

	remover := &fakeArtifactRemover{onRemove: func(repoID uint, format, name, version string) error {
		if version == "2.14.2" {
			return fmt.Errorf("internal server error")
		}
		return nil
	}}

	svc := NewRiskAssessmentService(db)
	svc.SetArtifactService(remover)
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.x"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	item := assessment.Items[0]

	// 一个版本删除失败：仍应落处置状态并记录失败明细，避免"已删但未处置"的中间态
	disposed, err := svc.DisposeItem(context.Background(), DisposeInput{ItemID: item.ID, Action: "remove", UserID: 1})
	if err == nil {
		t.Fatal("want partial-failure error")
	}
	if disposed.Disposition != model.DispositionRemoved {
		t.Errorf("disposition = %q, want removed (部分失败仍应落状态)", disposed.Disposition)
	}
	if !strings.Contains(disposed.DispositionNote, "移除 1 个制品版本") || !strings.Contains(disposed.DispositionNote, "部分移除失败") {
		t.Errorf("note = %q, want 移除计数与失败明细", disposed.DispositionNote)
	}
}

func TestDisposeItem_BlockFailureKeepsPending(t *testing.T) {
	// 故意不迁移 block_rules 表：BatchCreate 会失败，block 动作应报错且不落"已阻断"
	db := newRiskAssessmentTestDB(t)
	repo := model.Repository{Name: "central", Type: "hosted"}
	db.Create(&repo)
	db.Create(&model.Artifact{RepositoryID: repo.ID, Format: "maven", Name: "log4j", Version: "2.14.1"})

	svc := NewRiskAssessmentService(db)
	svc.SetBlockRuleService(NewBlockRuleService(repository.NewBlockRuleRepository(db), nil))
	assessment, err := svc.Analyze(context.Background(), "risk.xlsx", 1, []RiskComponentInput{
		{Name: "log4j", Version: "2.14.1"},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	item := assessment.Items[0]

	_, err = svc.DisposeItem(context.Background(), DisposeInput{ItemID: item.ID, Action: "block"})
	if err == nil {
		t.Fatal("block 创建失败应返回错误")
	}
	var after model.RiskAssessmentItem
	db.First(&after, item.ID)
	if after.Disposition != model.DispositionNone {
		t.Errorf("disposition = %q, want 保持待处置 (block 失败不落已阻断)", after.Disposition)
	}
}
