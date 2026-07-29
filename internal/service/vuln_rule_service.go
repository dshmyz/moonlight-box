package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
)

type VulnRuleService struct {
	ruleRepo   *repository.VulnRuleRepository
	sourceRepo *repository.VulnDataSourceRepository
	logger     *logrus.Logger
	httpClient *http.Client
}

func NewVulnRuleService(ruleRepo *repository.VulnRuleRepository, sourceRepo *repository.VulnDataSourceRepository) *VulnRuleService {
	return &VulnRuleService{
		ruleRepo:   ruleRepo,
		sourceRepo: sourceRepo,
		logger:     logrus.New(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *VulnRuleService) CreateRule(rule *model.VulnRule) error {
	rule.Source = model.VulnRuleSourceCustom
	return s.ruleRepo.Create(rule)
}

func (s *VulnRuleService) UpdateRule(id uint, updates map[string]interface{}) error {
	_, err := s.ruleRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}
	return s.ruleRepo.Update(id, filterUpdates(updates, allowedVulnRuleFields))
}

func (s *VulnRuleService) DeleteRule(id uint) error {
	rule, err := s.ruleRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}
	if rule.Source == model.VulnRuleSourceBuiltin {
		return fmt.Errorf("cannot delete builtin rule")
	}
	return s.ruleRepo.Delete(id)
}

func (s *VulnRuleService) GetRule(id uint) (*model.VulnRule, error) {
	return s.ruleRepo.FindByID(id)
}

func (s *VulnRuleService) ListRules(page, pageSize int, source, severity, pkgType, keyword string) ([]model.VulnRule, int64, error) {
	return s.ruleRepo.List(page, pageSize, source, severity, pkgType, keyword)
}

func (s *VulnRuleService) GetAllScanRules() ([]ScanRule, error) {
	dbRules, err := s.ruleRepo.ListAllEnabled()
	if err != nil {
		return nil, err
	}

	var rules []ScanRule

	for _, r := range dbRules {
		pattern, err := regexp.Compile(r.PackagePattern)
		if err != nil {
			s.logger.Warnf("Invalid package pattern %s: %v", r.PackagePattern, err)
			continue
		}
		rules = append(rules, ScanRule{
			PackagePattern: pattern,
			MaxVersion:     r.MaxVersion,
			MinVersion:     r.MinVersion,
			CVE:            r.CVE,
			Severity:       r.Severity,
			CVSS:           r.CVSS,
			Title:          r.Title,
			Description:    r.Description,
			FixedVersion:   r.FixedVersion,
			References:     r.References,
		})
	}

	rules = append(rules, scanRules...)
	return rules, nil
}

func (s *VulnRuleService) ImportRules(rules []model.VulnRule) (int, error) {
	count := 0
	for i := range rules {
		rules[i].Source = model.VulnRuleSourceCustom
		if err := s.ruleRepo.Create(&rules[i]); err != nil {
			s.logger.Warnf("Failed to import rule %s: %v", rules[i].CVE, err)
			continue
		}
		count++
	}
	return count, nil
}

func (s *VulnRuleService) SyncAllDataSources(ctx context.Context) error {
	sources, err := s.sourceRepo.ListEnabled()
	if err != nil {
		return err
	}

	for _, ds := range sources {
		if err := s.syncDataSource(ctx, &ds); err != nil {
			s.logger.Errorf("Failed to sync data source %s: %v", ds.Name, err)
			s.sourceRepo.UpdateSyncStatus(ds.ID, "failed", err.Error())
		} else {
			s.sourceRepo.UpdateSyncStatus(ds.ID, "success", "")
		}
	}
	return nil
}

func (s *VulnRuleService) syncDataSource(ctx context.Context, ds *model.VulnDataSource) error {
	s.logger.Infof("Syncing data source: %s", ds.Name)

	req, err := http.NewRequestWithContext(ctx, "GET", ds.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if ds.AuthType == "bearer" && ds.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+ds.AuthToken)
	}

	client := s.httpClient
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var externalRules []struct {
		CVE            string  `json:"cve"`
		PackagePattern string  `json:"package_pattern"`
		PackageType    string  `json:"package_type"`
		MaxVersion     string  `json:"max_version"`
		MinVersion     string  `json:"min_version"`
		Severity       string  `json:"severity"`
		CVSS           float64 `json:"cvss"`
		Title          string  `json:"title"`
		Description    string  `json:"description"`
		FixedVersion   string  `json:"fixed_version"`
		References     string  `json:"references"`
		ExternalID     string  `json:"external_id"`
	}

	if err := json.Unmarshal(body, &externalRules); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	now := time.Now()
	var externalIDs []string
	for _, er := range externalRules {
		rule := model.VulnRule{
			CVE:            er.CVE,
			PackagePattern: er.PackagePattern,
			PackageType:    er.PackageType,
			MaxVersion:     er.MaxVersion,
			MinVersion:     er.MinVersion,
			Severity:       model.VulnerabilitySeverity(er.Severity),
			CVSS:           er.CVSS,
			Title:          er.Title,
			Description:    er.Description,
			FixedVersion:   er.FixedVersion,
			References:     er.References,
			Source:         model.VulnRuleSourceSynced,
			ExternalID:     er.ExternalID,
			Enabled:        true,
			SyncedAt:       &now,
		}
		if err := s.ruleRepo.UpsertByCVE(er.CVE, &rule); err != nil {
			s.logger.Warnf("Failed to upsert rule %s: %v", er.CVE, err)
		}
		externalIDs = append(externalIDs, er.ExternalID)
	}

	s.ruleRepo.DeleteBySourceAndExternalIDs(model.VulnRuleSourceSynced, externalIDs)

	return s.sourceRepo.Update(ds.ID, map[string]interface{}{
		"last_sync_at": now,
	})
}

func (s *VulnRuleService) CreateDataSource(ds *model.VulnDataSource) error {
	return s.sourceRepo.Create(ds)
}

func (s *VulnRuleService) UpdateDataSource(id uint, updates map[string]interface{}) error {
	_, err := s.sourceRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("data source not found: %w", err)
	}
	return s.sourceRepo.Update(id, filterUpdates(updates, allowedDataSourceFields))
}

func (s *VulnRuleService) DeleteDataSource(id uint) error {
	return s.sourceRepo.Delete(id)
}

func (s *VulnRuleService) ListDataSources() ([]model.VulnDataSource, error) {
	return s.sourceRepo.List()
}

func (s *VulnRuleService) SyncDataSource(ctx context.Context, id uint) error {
	ds, err := s.sourceRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("data source not found: %w", err)
	}
	return s.syncDataSource(ctx, ds)
}

// allowedVulnRuleFields 白名单：UpdateRule 允许修改的字段
var allowedVulnRuleFields = map[string]bool{
	"package_pattern": true, "package_type": true,
	"max_version": true, "min_version": true,
	"cve": true, "severity": true, "cvss": true,
	"title": true, "description": true, "fixed_version": true,
	"references": true, "enabled": true,
}

// allowedDataSourceFields 白名单：UpdateDataSource 允许修改的字段
var allowedDataSourceFields = map[string]bool{
	"name": true, "type": true, "url": true,
	"auth_type": true, "auth_token": true,
	"enabled": true, "sync_cron": true,
}

// filterUpdates 从 updates 中只保留白名单字段，防止 mass assignment
func filterUpdates(updates map[string]interface{}, allowed map[string]bool) map[string]interface{} {
	filtered := make(map[string]interface{}, len(updates))
	for k, v := range updates {
		if allowed[k] {
			filtered[k] = v
		}
	}
	return filtered
}
