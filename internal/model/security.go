package model

import (
	"time"
)

type ScanStatus string

const (
	ScanStatusPending   ScanStatus = "pending"
	ScanStatusScanning  ScanStatus = "scanning"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
)

type VulnerabilitySeverity string

const (
	SeverityCritical VulnerabilitySeverity = "critical"
	SeverityHigh     VulnerabilitySeverity = "high"
	SeverityMedium   VulnerabilitySeverity = "medium"
	SeverityLow      VulnerabilitySeverity = "low"
	SeverityNone     VulnerabilitySeverity = "none"
)

type ScanResult struct {
	ID                   uint            `gorm:"primaryKey" json:"id"`
	VersionID            uint            `gorm:"not null;uniqueIndex" json:"version_id"`
	ScanStatus           ScanStatus      `gorm:"not null;index" json:"scan_status"`
	ScannerVersion       string          `gorm:"size:50" json:"scanner_version"`
	TotalVulnerabilities int             `gorm:"default:0" json:"total_vulnerabilities"`
	CriticalCount        int             `gorm:"default:0" json:"critical_count"`
	HighCount            int             `gorm:"default:0" json:"high_count"`
	MediumCount          int             `gorm:"default:0" json:"medium_count"`
	LowCount             int             `gorm:"default:0" json:"low_count"`
	ScannedAt            time.Time       `json:"scanned_at"`
	ReportPath           string          `gorm:"size:500" json:"report_path,omitempty"`
	ErrorMessage         string          `gorm:"type:text" json:"error_message,omitempty"`
	Vulnerabilities      []Vulnerability `gorm:"foreignKey:ScanResultID" json:"vulnerabilities,omitempty"`
	CreatedAt            time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

type Vulnerability struct {
	ID             uint                  `gorm:"primaryKey" json:"id"`
	ScanResultID   uint                  `gorm:"not null;index" json:"scan_result_id"`
	CVEID          string                `gorm:"size:30;not null;index" json:"cve_id"`
	Severity       VulnerabilitySeverity `gorm:"not null;index" json:"severity"`
	CVSSScore      float64               `json:"cvss_score"`
	DependencyName string                `gorm:"size:200" json:"dependency_name"`
	CurrentVersion string                `gorm:"size:50" json:"current_version"`
	FixedVersion   string                `gorm:"size:50" json:"fixed_version,omitempty"`
	IsDirectDep    bool                  `gorm:"default:true" json:"is_direct_dep"`
	Title          string                `gorm:"size:500" json:"title"`
	Description    string                `gorm:"type:text" json:"description"`
	References     string                `gorm:"type:text" json:"references,omitempty"`
	CreatedAt      time.Time             `gorm:"autoCreateTime" json:"created_at"`
}
