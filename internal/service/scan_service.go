package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type SecurityScanner struct {
	scanRepo  *repository.ScanRepository
	pkgRepo   *repository.PackageRepository
	blockRepo *repository.BlockRuleRepository
	logger    *logrus.Logger
}

type ScanRule struct {
	PackagePattern *regexp.Regexp
	MinVersion     string
	MaxVersion     string
	CVE            string
	Severity       model.VulnerabilitySeverity
	CVSS           float64
	Title          string
	Description    string
	FixedVersion   string
	References     string
}

var scanRules = []ScanRule{
	{
		PackagePattern: regexp.MustCompile(`(?i)log4j`),
		MaxVersion:     "2.17.0",
		CVE:            "CVE-2021-44228",
		Severity:       model.SeverityCritical,
		CVSS:           10.0,
		Title:          "Apache Log4j Remote Code Execution (Log4Shell)",
		Description:    "Apache Log4j2 JNDI features do not protect against attacker controlled LDAP and other JNDI related endpoints.",
		FixedVersion:   "2.17.0",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2021-44228",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)lodash`),
		MaxVersion:     "4.17.21",
		CVE:            "CVE-2021-23337",
		Severity:       model.SeverityHigh,
		CVSS:           7.2,
		Title:          "Lodash Command Injection",
		Description:    "Lodash versions prior to 4.17.21 are vulnerable to Command Injection via the template function.",
		FixedVersion:   "4.17.21",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2021-23337",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)^express$`),
		MaxVersion:     "4.17.3",
		CVE:            "CVE-2022-24999",
		Severity:       model.SeverityMedium,
		CVSS:           5.3,
		Title:          "Express.js qs Prototype Pollution",
		Description:    "Express.js prior to 4.17.3 allows qs prototype pollution via the query string.",
		FixedVersion:   "4.17.3",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2022-24999",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)^django$`),
		MaxVersion:     "3.2.14",
		CVE:            "CVE-2022-28346",
		Severity:       model.SeverityHigh,
		CVSS:           7.5,
		Title:          "Django SQL Injection",
		Description:    "Django before 3.2.14 allows SQL injection via the QuerySet.order_by() method.",
		FixedVersion:   "3.2.14",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2022-28346",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)^flask$`),
		MaxVersion:     "2.2.5",
		CVE:            "CVE-2023-30861",
		Severity:       model.SeverityMedium,
		CVSS:           5.5,
		Title:          "Flask Cookie Vulnerability",
		Description:    "Flask before 2.2.5 allows unauthorized access to session cookies.",
		FixedVersion:   "2.2.5",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2023-30861",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)requests$`),
		MaxVersion:     "2.31.0",
		CVE:            "CVE-2023-32681",
		Severity:       model.SeverityMedium,
		CVSS:           5.6,
		Title:          "Requests Proxy Authorization Leak",
		Description:    "Requests prior to 2.31.0 leaks Proxy-Authorization header to destination servers.",
		FixedVersion:   "2.31.0",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2023-32681",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)spring-core`),
		MaxVersion:     "5.3.18",
		CVE:            "CVE-2022-22965",
		Severity:       model.SeverityCritical,
		CVSS:           9.8,
		Title:          "Spring4Shell RCE Vulnerability",
		Description:    "Spring Framework RCE via Data Binding on JDK 9+.",
		FixedVersion:   "5.3.18",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2022-22965",
	},
	{
		PackagePattern: regexp.MustCompile(`(?i)^jsonwebtoken$`),
		MaxVersion:     "9.0.0",
		CVE:            "CVE-2022-23529",
		Severity:       model.SeverityHigh,
		CVSS:           7.5,
		Title:          "jsonwebtoken Insecure Key Handling",
		Description:    "jsonwebtoken prior to 9.0.0 allows insecure key handling.",
		FixedVersion:   "9.0.0",
		References:     "https://nvd.nist.gov/vuln/detail/CVE-2022-23529",
	},
}

func NewSecurityScanner(scanRepo *repository.ScanRepository, pkgRepo *repository.PackageRepository, blockRepo *repository.BlockRuleRepository) *SecurityScanner {
	return &SecurityScanner{
		scanRepo:  scanRepo,
		pkgRepo:   pkgRepo,
		blockRepo: blockRepo,
		logger:    logrus.New(),
	}
}

func (s *SecurityScanner) ScanPackage(ctx context.Context, versionID uint, pkgType, name, version string) *model.ScanResult {
	s.logger.Infof("Scanning %s@%s (type: %s, versionID: %d)", name, version, pkgType, versionID)

	scanResult := &model.ScanResult{
		VersionID:      versionID,
		ScanStatus:     model.ScanStatusScanning,
		ScannerVersion: "1.0.0",
		ScannedAt:      time.Now(),
	}

	if err := s.scanRepo.CreateScanResult(scanResult); err != nil {
		s.logger.Errorf("Failed to create scan result: %v", err)
		scanResult.ScanStatus = model.ScanStatusFailed
		scanResult.ErrorMessage = err.Error()
		return scanResult
	}

	vulnerabilities, err := s.detectVulnerabilities(pkgType, name, version)
	if err != nil {
		scanResult.ScanStatus = model.ScanStatusFailed
		scanResult.ErrorMessage = err.Error()
		s.scanRepo.UpdateScanResult(scanResult.ID, map[string]interface{}{
			"scan_status":   scanResult.ScanStatus,
			"error_message": scanResult.ErrorMessage,
		})
		return scanResult
	}

	var critical, high, medium, low int
	for _, v := range vulnerabilities {
		switch v.Severity {
		case model.SeverityCritical:
			critical++
		case model.SeverityHigh:
			high++
		case model.SeverityMedium:
			medium++
		case model.SeverityLow:
			low++
		}
	}

	scanResult.ScanStatus = model.ScanStatusCompleted
	scanResult.TotalVulnerabilities = len(vulnerabilities)
	scanResult.CriticalCount = critical
	scanResult.HighCount = high
	scanResult.MediumCount = medium
	scanResult.LowCount = low

	s.scanRepo.UpdateScanResult(scanResult.ID, map[string]interface{}{
		"scan_status":           scanResult.ScanStatus,
		"total_vulnerabilities": scanResult.TotalVulnerabilities,
		"critical_count":        scanResult.CriticalCount,
		"high_count":            scanResult.HighCount,
		"medium_count":          scanResult.MediumCount,
		"low_count":             scanResult.LowCount,
	})

	for i := range vulnerabilities {
		vulnerabilities[i].ScanResultID = scanResult.ID
	}
	s.scanRepo.BulkCreateVulnerabilities(vulnerabilities)

	s.logger.Infof("Scan completed for %s@%s: %d vulnerabilities found", name, version, len(vulnerabilities))
	return scanResult
}

func (s *SecurityScanner) TriggerScan(ctx context.Context, versionID uint, pkgType, name, version string) {
	go s.ScanPackage(ctx, versionID, pkgType, name, version)
}

func (s *SecurityScanner) GetScanResult(versionID uint) (*model.ScanResult, error) {
	return s.scanRepo.FindScanResultByVersionID(versionID)
}

func (s *SecurityScanner) ListScanResults(page, pageSize int, status, pkgType string) ([]model.ScanResult, int64, error) {
	return s.scanRepo.ListScanResults(page, pageSize, status, pkgType)
}

func (s *SecurityScanner) GetSecurityStats() (total, critical, high, medium, low int64, err error) {
	return s.scanRepo.GetSecurityStats()
}

func (s *SecurityScanner) ListVulnerabilities(scanResultID uint) ([]model.Vulnerability, error) {
	return s.scanRepo.ListVulnerabilities(scanResultID)
}

func (s *SecurityScanner) detectVulnerabilities(pkgType, name, version string) ([]model.Vulnerability, error) {
	var vulns []model.Vulnerability

	for _, rule := range scanRules {
		if rule.PackagePattern.MatchString(name) {
			if rule.MaxVersion == "" || isVersionLessThan(version, rule.MaxVersion) {
				vulns = append(vulns, model.Vulnerability{
					CVEID:          rule.CVE,
					Severity:       rule.Severity,
					CVSSScore:      rule.CVSS,
					DependencyName: name,
					CurrentVersion: version,
					FixedVersion:   rule.FixedVersion,
					Title:          rule.Title,
					Description:    rule.Description,
					References:     rule.References,
				})
			}
		}
	}

	return vulns, nil
}

func isVersionLessThan(a, b string) bool {
	va := parseVersion(a)
	vb := parseVersion(b)

	for i := 0; i < len(va) && i < len(vb); i++ {
		if va[i] < vb[i] {
			return true
		}
		if va[i] > vb[i] {
			return false
		}
	}
	return len(va) < len(vb)
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.Split(p, "-")[0]
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		result = append(result, n)
	}
	return result
}

func (s *SecurityScanner) ScanAllPackages(ctx context.Context) {
	for _, pkgType := range []string{"npm", "maven2", "pypi", "go"} {
		packages, total, err := s.pkgRepo.List(1, 10000, pkgType, "")
		if err != nil {
			s.logger.Errorf("Failed to list %s packages: %v", pkgType, err)
			continue
		}
		s.logger.Infof("Scanning %d %s packages", total, pkgType)
		for _, pkg := range packages {
			for _, ver := range pkg.Versions {
				s.TriggerScan(ctx, ver.ID, pkgType, pkg.Name, ver.Version)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

func (s *SecurityScanner) BlockByVulnerability(ctx context.Context, cveID string) error {
	ruleName := fmt.Sprintf("auto-block-%s", strings.ToLower(cveID))

	err := s.blockRepo.Create(&model.BlockRule{
		PackageName: ruleName,
		Version:     "*",
		MatchType:   model.BlockMatchWildcard,
		PackageType: "*",
		Reason:      fmt.Sprintf("Auto-blocked for %s", cveID),
		Enabled:     true,
	})

	if err != nil {
		return fmt.Errorf("failed to create block rule: %w", err)
	}

	s.logger.Infof("Created block rule for CVE: %s", cveID)
	return nil
}
