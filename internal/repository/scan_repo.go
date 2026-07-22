package repository

import (
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type ScanRepository struct {
	db *gorm.DB
}

func NewScanRepository(db *gorm.DB) *ScanRepository {
	return &ScanRepository{db: db}
}

func (r *ScanRepository) CreateScanResult(scanResult *model.ScanResult) error {
	return r.db.Create(scanResult).Error
}

func (r *ScanRepository) UpdateScanResult(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.ScanResult{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ScanRepository) FindScanResultByComponentID(componentID uint) (*model.ScanResult, error) {
	var result model.ScanResult
	err := r.db.Preload("Vulnerabilities").Where("component_id = ?", componentID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *ScanRepository) FindScanResult(id uint) (*model.ScanResult, error) {
	var result model.ScanResult
	err := r.db.Preload("Vulnerabilities").First(&result, id).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *ScanRepository) CreateVulnerability(vuln *model.Vulnerability) error {
	return r.db.Create(vuln).Error
}

func (r *ScanRepository) BulkCreateVulnerabilities(vulns []model.Vulnerability) error {
	if len(vulns) == 0 {
		return nil
	}
	return r.db.CreateInBatches(vulns, 50).Error
}

func (r *ScanRepository) ListVulnerabilities(scanResultID uint) ([]model.Vulnerability, error) {
	var vulns []model.Vulnerability
	err := r.db.Where("scan_result_id = ?", scanResultID).Order("cvss_score DESC").Find(&vulns).Error
	return vulns, err
}

// FindVulnerabilitiesByCVE 按 CVE ID 查询所有相关的 vulnerability 记录。
// 用于 BlockByVulnerability 生成精确阻断规则时获取 DependencyName 和 FixedVersion。
// 结果按 dependency_name 去重（同一个包在同一 CVE 下只保留 CVSS 分数最高的一条）。
func (r *ScanRepository) FindVulnerabilitiesByCVE(cveID string) ([]model.Vulnerability, error) {
	var vulns []model.Vulnerability
	err := r.db.Where("cve_id = ?", cveID).
		Order("cvss_score DESC, dependency_name").
		Find(&vulns).Error
	return vulns, err
}

// ListVulnerabilitiesByScanResultIDs 批量查询多个 scan result 的 vulnerabilities，避免 N+1 查询。
// 结果按 cvss_score DESC 排序，与 ListVulnerabilities 保持一致。
func (r *ScanRepository) ListVulnerabilitiesByScanResultIDs(scanResultIDs []uint) ([]model.Vulnerability, error) {
	if len(scanResultIDs) == 0 {
		return nil, nil
	}
	var vulns []model.Vulnerability
	err := r.db.Where("scan_result_id IN ?", scanResultIDs).
		Order("cvss_score DESC").
		Find(&vulns).Error
	return vulns, err
}

func (r *ScanRepository) ListVulnerabilitiesPaginated(page, pageSize int, severity, pkgType string) ([]model.Vulnerability, int64, error) {
	var vulns []model.Vulnerability
	var total int64

	query := r.db.Model(&model.Vulnerability{}).
		Joins("JOIN scan_results ON scan_results.id = vulnerabilities.scan_result_id").
		Joins("JOIN artifacts ON artifacts.id = scan_results.component_id")

	if severity != "" {
		query = query.Where("vulnerabilities.severity = ?", severity)
	}
	if pkgType != "" {
		query = query.Where("artifacts.format = ?", pkgType)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset((page - 1) * pageSize).Limit(pageSize).
		Order("vulnerabilities.cvss_score DESC").
		Find(&vulns).Error

	return vulns, total, err
}

func (r *ScanRepository) ListScanResults(page, pageSize int, status string, pkgType string) ([]model.ScanResult, int64, error) {
	var results []model.ScanResult
	var total int64

	query := r.db.Model(&model.ScanResult{}).
		Joins("JOIN artifacts ON artifacts.id = scan_results.component_id")

	if status != "" {
		query = query.Where("scan_results.scan_status = ?", status)
	}
	if pkgType != "" {
		query = query.Where("artifacts.format = ?", pkgType)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset((page - 1) * pageSize).Limit(pageSize).
		Order("scan_results.created_at DESC").
		Find(&results).Error

	return results, total, err
}

func (r *ScanRepository) GetSecurityStats() (total, critical, high, medium, low int64, err error) {
	err = r.db.Model(&model.ScanResult{}).
		Select(
			"COUNT(*) as total",
			"COALESCE(SUM(critical_count), 0) as critical",
			"COALESCE(SUM(high_count), 0) as high",
			"COALESCE(SUM(medium_count), 0) as medium",
			"COALESCE(SUM(low_count), 0) as low",
		).
		Row().Scan(&total, &critical, &high, &medium, &low)

	return
}
