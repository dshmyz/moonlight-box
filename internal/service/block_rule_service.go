package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type BlockRuleService struct {
	repo     *repository.BlockRuleRepository
	auditSvc *AuditService
}

func NewBlockRuleService(repo *repository.BlockRuleRepository, auditSvc *AuditService) *BlockRuleService {
	return &BlockRuleService{
		repo:     repo,
		auditSvc: auditSvc,
	}
}

type BlockResult struct {
	Blocked bool
	Rule    *model.BlockRule
}

func (s *BlockRuleService) IsBlocked(pkgType, pkgName, version string) (*BlockResult, error) {
	exactRules, err := s.repo.FindEnabledExactRules(pkgType, pkgName, version)
	if err != nil {
		return nil, err
	}
	if len(exactRules) > 0 {
		return &BlockResult{Blocked: true, Rule: &exactRules[0]}, nil
	}

	wildcardRules, err := s.repo.FindEnabledWildcardRules(pkgType)
	if err != nil {
		return nil, err
	}
	for i := range wildcardRules {
		if s.matchWildcard(&wildcardRules[i], pkgName, version) {
			return &BlockResult{Blocked: true, Rule: &wildcardRules[i]}, nil
		}
	}

	return &BlockResult{Blocked: false}, nil
}

func (s *BlockRuleService) matchWildcard(rule *model.BlockRule, pkgName, version string) bool {
	if !s.wildcardMatch(rule.PackageName, pkgName) {
		return false
	}
	if rule.Version == "*" {
		return true
	}
	return s.wildcardMatch(rule.Version, version)
}

func (s *BlockRuleService) wildcardMatch(pattern, text string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == text
	}
	regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	matched, err := regexp.MatchString(regexPattern, text)
	if err != nil {
		return false
	}
	return matched
}

func (s *BlockRuleService) LogBlock(ctx context.Context, pkgName, version string, rule *model.BlockRule, ipAddress, userAgent string) error {
	details, _ := json.Marshal(map[string]interface{}{
		"rule_id":    rule.ID,
		"reason":     rule.Reason,
		"match_type": rule.MatchType,
		"version":    version,
	})

	return s.auditSvc.LogWithRequest(ctx, nil, model.ActionBlock, "package", nil,
		fmt.Sprintf("%s@%s", pkgName, version),
		string(details), ipAddress, userAgent)
}

func (s *BlockRuleService) Create(rule *model.BlockRule) error {
	return s.repo.Create(rule)
}

func (s *BlockRuleService) BatchCreate(rules []*model.BlockRule) (int, int, error) {
	success := 0
	failed := 0
	for _, rule := range rules {
		if rule.PackageName == "" || rule.Version == "" || rule.PackageType == "" {
			failed++
			continue
		}
		if rule.MatchType == "" {
			rule.MatchType = model.BlockMatchExact
		}
		if err := s.repo.Create(rule); err != nil {
			failed++
		} else {
			success++
		}
	}
	return success, failed, nil
}

func (s *BlockRuleService) Update(id uint, updates map[string]interface{}) error {
	return s.repo.Update(id, updates)
}

func (s *BlockRuleService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *BlockRuleService) GetByID(id uint) (*model.BlockRule, error) {
	return s.repo.GetByID(id)
}

func (s *BlockRuleService) List(filter map[string]interface{}) ([]model.BlockRule, error) {
	return s.repo.List(filter)
}
