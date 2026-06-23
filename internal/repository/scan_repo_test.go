package repository

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestScanDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.ScanResult{}, &model.Vulnerability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestListVulnerabilitiesByScanResultIDsReturnsAllMatching(t *testing.T) {
	db := setupTestScanDB(t)
	repo := NewScanRepository(db)
	now := time.Now()

	// 创建 3 个 scan result
	results := []model.ScanResult{
		{ComponentID: 1, ScanStatus: model.ScanStatusCompleted, ScannedAt: now},
		{ComponentID: 2, ScanStatus: model.ScanStatusCompleted, ScannedAt: now},
		{ComponentID: 3, ScanStatus: model.ScanStatusCompleted, ScannedAt: now},
	}
	for i := range results {
		if err := db.Create(&results[i]).Error; err != nil {
			t.Fatalf("create scan result %d: %v", i, err)
		}
	}

	// 每个 scan result 对应 2 个 vulnerability
	vulns := []model.Vulnerability{
		{ScanResultID: results[0].ID, CVEID: "CVE-2024-0001", Severity: model.SeverityHigh, CVSSScore: 8.0, DependencyName: "pkg-a"},
		{ScanResultID: results[0].ID, CVEID: "CVE-2024-0002", Severity: model.SeverityMedium, CVSSScore: 5.0, DependencyName: "pkg-a"},
		{ScanResultID: results[1].ID, CVEID: "CVE-2024-0003", Severity: model.SeverityCritical, CVSSScore: 9.5, DependencyName: "pkg-b"},
		{ScanResultID: results[1].ID, CVEID: "CVE-2024-0004", Severity: model.SeverityLow, CVSSScore: 2.0, DependencyName: "pkg-b"},
		{ScanResultID: results[2].ID, CVEID: "CVE-2024-0005", Severity: model.SeverityHigh, CVSSScore: 7.5, DependencyName: "pkg-c"},
		{ScanResultID: results[2].ID, CVEID: "CVE-2024-0006", Severity: model.SeverityMedium, CVSSScore: 4.0, DependencyName: "pkg-c"},
	}
	for i := range vulns {
		if err := db.Create(&vulns[i]).Error; err != nil {
			t.Fatalf("create vulnerability %d: %v", i, err)
		}
	}

	// 批量查询前两个 scan result 的 vulnerabilities
	got, err := repo.ListVulnerabilitiesByScanResultIDs([]uint{results[0].ID, results[1].ID})
	if err != nil {
		t.Fatalf("ListVulnerabilitiesByScanResultIDs failed: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("got %d vulnerabilities, want 4", len(got))
	}

	// 验证返回的 vulnerabilities 都属于查询的 scan result IDs
	validIDs := map[uint]bool{results[0].ID: true, results[1].ID: true}
	for _, v := range got {
		if !validIDs[v.ScanResultID] {
			t.Fatalf("vulnerability %q has scan_result_id %d not in query set", v.CVEID, v.ScanResultID)
		}
	}
}

func TestListVulnerabilitiesByScanResultIDsEmptyInputReturnsEmpty(t *testing.T) {
	db := setupTestScanDB(t)
	repo := NewScanRepository(db)

	got, err := repo.ListVulnerabilitiesByScanResultIDs([]uint{})
	if err != nil {
		t.Fatalf("ListVulnerabilitiesByScanResultIDs failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d vulnerabilities, want 0 for empty input", len(got))
	}
}

func TestListVulnerabilitiesByScanResultIDsNoMatchReturnsEmpty(t *testing.T) {
	db := setupTestScanDB(t)
	repo := NewScanRepository(db)

	got, err := repo.ListVulnerabilitiesByScanResultIDs([]uint{999, 1000})
	if err != nil {
		t.Fatalf("ListVulnerabilitiesByScanResultIDs failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d vulnerabilities, want 0 for no match", len(got))
	}
}

func TestListVulnerabilitiesByScanResultIDsOrdersByCVSSDesc(t *testing.T) {
	db := setupTestScanDB(t)
	repo := NewScanRepository(db)
	now := time.Now()

	result := model.ScanResult{ComponentID: 1, ScanStatus: model.ScanStatusCompleted, ScannedAt: now}
	if err := db.Create(&result).Error; err != nil {
		t.Fatalf("create scan result: %v", err)
	}

	vulns := []model.Vulnerability{
		{ScanResultID: result.ID, CVEID: "CVE-LOW", Severity: model.SeverityLow, CVSSScore: 2.0},
		{ScanResultID: result.ID, CVEID: "CVE-CRIT", Severity: model.SeverityCritical, CVSSScore: 9.5},
		{ScanResultID: result.ID, CVEID: "CVE-MED", Severity: model.SeverityMedium, CVSSScore: 5.0},
	}
	for i := range vulns {
		if err := db.Create(&vulns[i]).Error; err != nil {
			t.Fatalf("create vulnerability %d: %v", i, err)
		}
	}

	got, err := repo.ListVulnerabilitiesByScanResultIDs([]uint{result.ID})
	if err != nil {
		t.Fatalf("ListVulnerabilitiesByScanResultIDs failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d vulnerabilities, want 3", len(got))
	}

	// 验证按 CVSS 降序排列
	if got[0].CVSSScore < got[1].CVSSScore || got[1].CVSSScore < got[2].CVSSScore {
		t.Fatalf("vulnerabilities not ordered by CVSS DESC: %f, %f, %f",
			got[0].CVSSScore, got[1].CVSSScore, got[2].CVSSScore)
	}
}
