package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type SecurityScanner struct {
	scanRepo        *repository.ScanRepository
	db              *gorm.DB
	blockRepo       *repository.BlockRuleRepository
	blockRuleSvc    *BlockRuleService
	vulnRuleService *VulnRuleService
	logger          *logrus.Logger
	scanSem         chan struct{}
	// scanSlots 扫描投递的总量上限（排队 + 在途）：非阻塞投递满时丢弃并告警，
	// 防止一次大批量写入瞬间创建海量等锁 goroutine。nil 表示退化为无界投递（直接构造的测试场景）。
	scanSlots   chan struct{}
	scanPackage func(ctx context.Context, versionID uint, pkgType, name, version string) *model.ScanResult

	// scanLocks 按组件串行化并发扫描（versionID -> *sync.Mutex），
	// 避免上传自动扫描与定时全量扫描同时扫同一组件时产生重复 scan_result。
	scanLocks sync.Map

	// 安全扫描自动化配置（读取 system_configs，回退到 YAML 默认值）
	configSvc     *SystemConfigService
	cfgMu         sync.RWMutex
	scanOnUpload  bool
	blockCritical bool
	blockHigh     bool
	blockMedium   bool

	// autoBlockMu 串行化自动阻断规则的"查重+创建"，避免并发扫描下的重复规则
	autoBlockMu sync.Mutex
}

const (
	// defaultMaxConcurrentScans 同时并发扫描数。扫描每次约 5 次 DB 往返，
	// 并发过高会在回源/上传高峰挤占数据库连接（尤其 SQLite 单写锁）。
	defaultMaxConcurrentScans = 4
	scanAllPackagesBatchSize  = 500
	// maxQueuedScans 扫描投递队列上限（排队 + 在途）。超出时非阻塞投递直接丢弃并告警，
	// 防止一次大批量上传瞬间创建海量等锁 goroutine 挤爆内存。
	maxQueuedScans = 1024
	// scanRescanInterval 增量全量扫描的重扫间隔：scanned_at 在此之内的组件跳过。
	// 定时全量扫描只补"从未扫过 + 超过间隔未扫"的组件，避免每晚对全库逐行重扫。
	scanRescanInterval = 7 * 24 * time.Hour
)

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

func NewSecurityScanner(scanRepo *repository.ScanRepository, db *gorm.DB, blockRepo *repository.BlockRuleRepository) *SecurityScanner {
	scanner := &SecurityScanner{
		scanRepo:  scanRepo,
		db:        db,
		blockRepo: blockRepo,
		logger:    logrus.New(),
		scanSem:   make(chan struct{}, defaultMaxConcurrentScans),
		scanSlots: make(chan struct{}, maxQueuedScans),
	}
	scanner.scanPackage = scanner.ScanPackage
	return scanner
}

func (s *SecurityScanner) SetVulnRuleService(vulnRuleService *VulnRuleService) {
	s.vulnRuleService = vulnRuleService
}

// SetBlockRuleService 注入阻断规则服务，用于扫描命中后自动生成阻断规则（会失效运行时缓存）。
func (s *SecurityScanner) SetBlockRuleService(blockRuleSvc *BlockRuleService) {
	s.blockRuleSvc = blockRuleSvc
}

// SetSecurityDefaults 设置 YAML 提供的自动化扫描默认值，作为 system_configs 缺失时的回退。
func (s *SecurityScanner) SetSecurityDefaults(scanOnUpload, blockCritical, blockHigh bool) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.scanOnUpload = scanOnUpload
	s.blockCritical = blockCritical
	s.blockHigh = blockHigh
}

// SetConfigService 注入系统配置服务，启用热更新。必须在 LoadSecurityConfig 之前调用。
func (s *SecurityScanner) SetConfigService(configSvc *SystemConfigService) {
	s.configSvc = configSvc
}

// LoadSecurityConfig 从 system_configs 加载安全扫描配置，失败时回退到 YAML 默认值。
func (s *SecurityScanner) LoadSecurityConfig() {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if s.configSvc == nil {
		return
	}
	if v, err := s.configSvc.Get("security.scan_on_upload"); err == nil {
		s.scanOnUpload = v.Value == "true" || v.Value == "1"
	}
	if v, err := s.configSvc.Get("security.block_critical"); err == nil {
		s.blockCritical = v.Value == "true" || v.Value == "1"
	}
	if v, err := s.configSvc.Get("security.block_high"); err == nil {
		s.blockHigh = v.Value == "true" || v.Value == "1"
	}
	if v, err := s.configSvc.Get("security.block_medium"); err == nil {
		s.blockMedium = v.Value == "true" || v.Value == "1"
	}
}

// ShouldScanOnUpload 是否启用上传时自动扫描。
func (s *SecurityScanner) ShouldScanOnUpload() bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.scanOnUpload
}

// Reload 实现 ScheduledTask.Reload，热更新扫描配置。
func (s *SecurityScanner) Reload() {
	s.LoadSecurityConfig()
}

// Name 实现 ScheduledTask：任务标识，由 TaskScheduler 统一调度（每日全量扫描）。
func (s *SecurityScanner) Name() string { return "security_scan" }

// Run 实现 ScheduledTask：执行一次全量扫描。
func (s *SecurityScanner) Run(ctx context.Context) (int, error) {
	s.ScanAllPackages(ctx)
	return 0, nil
}

// Stop 实现 ScheduledTask：扫描为无状态短任务，无需释放资源。
func (s *SecurityScanner) Stop() {}

// maybeAutoBlock 扫描命中后按配置自动生成阻断规则（严重/高危/中危可选）。
// 通过 blockRuleSvc 创建以失效运行时缓存；同名同版本精确规则已存在时跳过。
func (s *SecurityScanner) maybeAutoBlock(pkgType, name, version string, vulns []model.Vulnerability) {
	if s.blockRuleSvc == nil {
		return
	}
	s.cfgMu.RLock()
	blockCritical, blockHigh, blockMedium := s.blockCritical, s.blockHigh, s.blockMedium
	s.cfgMu.RUnlock()

	var rules []*model.BlockRule
	for i := range vulns {
		v := &vulns[i]
		shouldBlock := (v.Severity == model.SeverityCritical && blockCritical) ||
			(v.Severity == model.SeverityHigh && blockHigh) ||
			(v.Severity == model.SeverityMedium && blockMedium)
		if !shouldBlock {
			continue
		}
		rules = append(rules, &model.BlockRule{
			PackageName: name,
			PackageType: pkgType,
			Version:     version,
			MatchType:   model.BlockMatchExact,
			Reason:      fmt.Sprintf("安全扫描自动阻断：%s (%s)", v.CVEID, v.Title),
			Enabled:     true,
		})
	}
	if len(rules) == 0 {
		return
	}

	// 串行化查重+创建，避免 8 路并发扫描同时判断"规则不存在"导致重复创建
	s.autoBlockMu.Lock()
	defer s.autoBlockMu.Unlock()
	for _, rule := range rules {
		var count int64
		if err := s.db.Model(&model.BlockRule{}).
			Where("package_name = ? AND package_type = ? AND version = ? AND match_type = ?",
				rule.PackageName, rule.PackageType, rule.Version, rule.MatchType).
			Count(&count).Error; err == nil && count > 0 {
			continue
		}
		if err := s.blockRuleSvc.Create(rule); err != nil {
			s.logger.Warnf("自动阻断 %s@%s 失败: %v", name, version, err)
		} else {
			s.logger.Infof("安全扫描自动阻断: %s@%s (%s)", name, version, rule.Reason)
		}
	}
}

func (s *SecurityScanner) ScanPackage(ctx context.Context, versionID uint, pkgType, name, version string) *model.ScanResult {
	// 按组件串行化：上传自动扫描与定时全量扫描可能同时扫同一组件，
	// 并发执行会产生两条 scan_result（FindCreate 与 BulkCreate 均非原子）。
	lockAny, _ := s.scanLocks.LoadOrStore(versionID, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	s.logger.Infof("Scanning %s@%s (type: %s, versionID: %d)", name, version, pkgType, versionID)

	// 幂等：已存在扫描结果则就地重扫，避免定时全量扫描造成表膨胀
	scanResult, err := s.scanRepo.FindScanResultByComponentID(versionID)
	if err != nil {
		s.logger.Errorf("Failed to find scan result: %v", err)
		scanResult = &model.ScanResult{}
	}
	if scanResult == nil || scanResult.ID == 0 {
		scanResult = &model.ScanResult{
			ComponentID:    versionID,
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
	} else {
		scanResult.ScanStatus = model.ScanStatusScanning
		scanResult.ScannedAt = time.Now()
		s.scanRepo.UpdateScanResult(scanResult.ID, map[string]interface{}{
			"scan_status": scanResult.ScanStatus,
			"scanned_at":  scanResult.ScannedAt,
		})
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

	// 重扫时替换旧的漏洞明细
	if err := s.db.WithContext(ctx).Where("scan_result_id = ?", scanResult.ID).Delete(&model.Vulnerability{}).Error; err != nil {
		s.logger.Warnf("清理旧漏洞记录失败: %v", err)
	}
	for i := range vulnerabilities {
		vulnerabilities[i].ScanResultID = scanResult.ID
	}
	s.scanRepo.BulkCreateVulnerabilities(vulnerabilities)

	// 按配置自动生成阻断规则
	s.maybeAutoBlock(pkgType, name, version, vulnerabilities)

	s.logger.Infof("Scan completed for %s@%s: %d vulnerabilities found", name, version, len(vulnerabilities))
	return scanResult
}

// TriggerScan 异步触发一次扫描（非阻塞）：队列满时丢弃并告警，适用于上传自动扫描
// ——丢一次扫描可接受（下次定时全量扫描会补上），但不能拖慢上传请求。
func (s *SecurityScanner) TriggerScan(ctx context.Context, versionID uint, pkgType, name, version string) {
	s.dispatchScan(ctx, versionID, pkgType, name, version, false)
}

// TriggerScanWait 等待扫描进入队列后返回（阻塞），供全量扫描做背压：
// 队列满时阻塞到有空位，避免瞬间投递十万级任务撑爆内存。ctx 取消时放弃投递。
func (s *SecurityScanner) TriggerScanWait(ctx context.Context, versionID uint, pkgType, name, version string) {
	s.dispatchScan(ctx, versionID, pkgType, name, version, true)
}

// dispatchScan 投递扫描任务。scanSlots 限制排队+在途总量；scanSem 限制实际并发数。
// scanSlots 为 nil（测试直接构造 scanner）时退化为旧的"无界 goroutine + 信号量"行为。
func (s *SecurityScanner) dispatchScan(ctx context.Context, versionID uint, pkgType, name, version string, wait bool) {
	if s.scanSlots != nil {
		if wait {
			select {
			case s.scanSlots <- struct{}{}:
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case s.scanSlots <- struct{}{}:
			default:
				s.logger.Warnf("scan queue full, dropping scan for %s@%s", name, version)
				return
			}
		}
	}
	scanPackage := s.scanPackage
	if scanPackage == nil {
		scanPackage = s.ScanPackage
	}
	go func() {
		if s.scanSlots != nil {
			// ctx 在等 scanSem 时取消也要释放队列槽位
			defer func() { <-s.scanSlots }()
		}
		if s.scanSem != nil {
			select {
			case s.scanSem <- struct{}{}:
				defer func() { <-s.scanSem }()
			case <-ctx.Done():
				return
			}
		}
		scanPackage(ctx, versionID, pkgType, name, version)
	}()
}

func (s *SecurityScanner) GetScanResult(versionID uint) (*model.ScanResult, error) {
	return s.scanRepo.FindScanResultByComponentID(versionID)
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

// ListVulnerabilitiesByScanResultIDs 批量查询多个 scan result 的 vulnerabilities，避免 N+1 查询。
func (s *SecurityScanner) ListVulnerabilitiesByScanResultIDs(scanResultIDs []uint) ([]model.Vulnerability, error) {
	return s.scanRepo.ListVulnerabilitiesByScanResultIDs(scanResultIDs)
}

func (s *SecurityScanner) ListVulnerabilitiesPaginated(page, pageSize int, severity, pkgType string) ([]model.Vulnerability, int64, error) {
	return s.scanRepo.ListVulnerabilitiesPaginated(page, pageSize, severity, pkgType)
}

func (s *SecurityScanner) detectVulnerabilities(pkgType, name, version string) ([]model.Vulnerability, error) {
	var vulns []model.Vulnerability

	rules := scanRules
	if s.vulnRuleService != nil {
		if allRules, err := s.vulnRuleService.GetAllScanRules(); err == nil {
			rules = allRules
		}
	}

	for _, rule := range rules {
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
	for _, pkgType := range []string{"npm", "maven", "pypi", "go"} {
		var total int64
		query := s.db.WithContext(ctx).Model(&model.Artifact{}).
			Where("format = ?", pkgType).
			Where("name != ''").
			Where("version != ''").
			// metadata/checksum/directory 不是可扫组件，只按 artifact/version 行扫描，
			// 否则同一版本的 checksum 行会重复扫描并污染组件扫描计数
			Where("kind NOT IN ?", []string{runtime.KindMetadata, runtime.KindChecksum, runtime.KindDirectory})
		if err := query.Count(&total).Error; err != nil {
			s.logger.Errorf("Failed to count %s packages for scan: %v", pkgType, err)
			continue
		}

		s.logger.Infof("Scanning %d %s packages", total, pkgType)
		var artifacts []model.Artifact
		if err := query.Order("id ASC").FindInBatches(&artifacts, scanAllPackagesBatchSize, func(tx *gorm.DB, batch int) error {
			// 增量：跳过最近已扫描的组件，只扫"从未扫过 + 超过重扫间隔"的，
			// 避免大库每晚全量重扫造成 DB 风暴（每组件约 5 次 DB 往返）。
			ids := make([]uint, 0, len(artifacts))
			for i := range artifacts {
				ids = append(ids, artifacts[i].ID)
			}
			var recentIDs []uint
			if err := s.db.WithContext(ctx).Model(&model.ScanResult{}).
				Where("component_id IN ? AND scanned_at > ?", ids, time.Now().Add(-scanRescanInterval)).
				Pluck("component_id", &recentIDs).Error; err != nil {
				s.logger.Warnf("query recent scan results failed (fallback to full scan): %v", err)
				recentIDs = nil
			}
			recent := make(map[uint]bool, len(recentIDs))
			for _, id := range recentIDs {
				recent[id] = true
			}
			for _, a := range artifacts {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if recent[a.ID] {
					continue
				}
				// 阻塞式投递：队列满时等待空位，全量扫描对投递量做背压而非无界堆积
				s.TriggerScanWait(ctx, a.ID, pkgType, a.Name, a.Version)
			}
			return nil
		}).Error; err != nil {
			s.logger.Errorf("Failed to scan %s packages: %v", pkgType, err)
		}
	}
}

// BlockByVulnerability 根据 CVE ID 查询 vulnerability 表，为每个受影响的包
// 生成精确的阻断规则：
//   - FixedVersion 非空：生成 range 规则，阻断所有低于 FixedVersion 的版本
//   - FixedVersion 为空：生成 wildcard 规则，阻断该包所有版本
//
// 按 DependencyName 去重（同一个包在同一 CVE 下只创建一条规则）。
// PackageType 设为 "*"，因为 vulnerability 表不存储包类型，且阻断规则应覆盖所有协议。
func (s *SecurityScanner) BlockByVulnerability(ctx context.Context, cveID string) error {
	vulns, err := s.scanRepo.FindVulnerabilitiesByCVE(cveID)
	if err != nil {
		return fmt.Errorf("query vulnerabilities for CVE %s: %w", cveID, err)
	}
	if len(vulns) == 0 {
		return fmt.Errorf("no vulnerability data found for CVE %s", cveID)
	}

	// 按 DependencyName 去重：FindVulnerabilitiesByCVE 已按 cvss_score DESC 排序，
	// 同名包只取第一条（CVSS 分数最高）
	seen := make(map[string]bool)
	var rules []*model.BlockRule
	for i := range vulns {
		v := &vulns[i]
		depName := strings.TrimSpace(v.DependencyName)
		if depName == "" || seen[depName] {
			continue
		}
		seen[depName] = true

		rule := &model.BlockRule{
			PackageName: depName,
			PackageType: "*",
			Reason:      fmt.Sprintf("Auto-blocked for %s: %s", cveID, v.Title),
			Enabled:     true,
		}

		if fixed := strings.TrimSpace(v.FixedVersion); fixed != "" {
			// FixedVersion 存在：用 range 规则阻断所有低于修复版本的版本
			rule.MatchType = model.BlockMatchRange
			rule.Version = fmt.Sprintf("<%s", fixed)
		} else {
			// FixedVersion 为空：阻断该包所有版本
			rule.MatchType = model.BlockMatchWildcard
			rule.Version = "*"
		}

		rules = append(rules, rule)
	}

	if len(rules) == 0 {
		return fmt.Errorf("no valid dependency names found for CVE %s", cveID)
	}

	// 批量创建规则
	for _, rule := range rules {
		if err := s.blockRepo.Create(rule); err != nil {
			return fmt.Errorf("failed to create block rule for %s: %w", rule.PackageName, err)
		}
	}

	s.logger.Infof("Created %d block rule(s) for CVE: %s", len(rules), cveID)
	return nil
}
