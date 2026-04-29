package repository

import (
	"github.com/moonlight-box/registry/internal/model"
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

func (r *ScanRepository) FindScanResultByVersionID(versionID uint) (*model.ScanResult, error) {
	var result model.ScanResult
	err := r.db.Preload("Vulnerabilities").Where("version_id = ?", versionID).First(&result).Error
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

func (r *ScanRepository) ListScanResults(page, pageSize int, status string, pkgType string) ([]model.ScanResult, int64, error) {
	var results []model.ScanResult
	var total int64

	query := r.db.Model(&model.ScanResult{}).
		Joins("JOIN package_versions ON package_versions.id = scan_results.version_id").
		Joins("JOIN packages ON packages.id = package_versions.package_id")

	if status != "" {
		query = query.Where("scan_results.scan_status = ?", status)
	}
	if pkgType != "" {
		query = query.Where("packages.type = ?", pkgType)
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
