package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type cachedWildcardRule struct {
	rule     *model.BlockRule
	compiled *regexp.Regexp
}

type BlockRuleService struct {
	repo     *repository.BlockRuleRepository
	auditSvc *AuditService

	cacheMu         sync.RWMutex
	cachedAt        time.Time
	cacheTTL        time.Duration
	exactRulesCache map[string][]*model.BlockRule
	wildcardRules   map[string][]cachedWildcardRule
}

func NewBlockRuleService(repo *repository.BlockRuleRepository, auditSvc *AuditService) *BlockRuleService {
	svc := &BlockRuleService{
		repo:            repo,
		auditSvc:        auditSvc,
		cacheTTL:        1 * time.Minute,
		exactRulesCache: make(map[string][]*model.BlockRule),
		wildcardRules:   make(map[string][]cachedWildcardRule),
	}
	return svc
}

type BlockResult struct {
	Blocked bool
	Rule    *model.BlockRule
}

func (s *BlockRuleService) IsBlocked(pkgType, pkgName, version string) (*BlockResult, error) {
	s.cacheMu.RLock()
	cacheValid := time.Since(s.cachedAt) < s.cacheTTL
	s.cacheMu.RUnlock()

	if !cacheValid {
		if err := s.refreshCache(); err != nil {
			return nil, err
		}
	}

	s.cacheMu.RLock()
	exactKey := pkgType + ":" + pkgName + ":" + version
	exactRules, ok := s.exactRulesCache[exactKey]
	wildcardRules, ok2 := s.wildcardRules[pkgType]
	s.cacheMu.RUnlock()

	if ok && len(exactRules) > 0 {
		return &BlockResult{Blocked: true, Rule: exactRules[0]}, nil
	}

	// Fallback: version="*" 匹配所有版本
	if version != "*" {
		wildKey := pkgType + ":" + pkgName + ":*"
		if rules, ok3 := s.exactRulesCache[wildKey]; ok3 && len(rules) > 0 {
			return &BlockResult{Blocked: true, Rule: rules[0]}, nil
		}
	}

	if ok2 {
		for _, cached := range wildcardRules {
			if s.matchCachedWildcard(&cached, pkgName, version) {
				return &BlockResult{Blocked: true, Rule: cached.rule}, nil
			}
		}
	}

	return &BlockResult{Blocked: false}, nil
}

func (s *BlockRuleService) refreshCache() error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if time.Since(s.cachedAt) < s.cacheTTL {
		return nil
	}

	exactRules, err := s.repo.FindAllEnabledExactRules()
	if err != nil {
		return err
	}

	wildcardRules, err := s.repo.FindAllEnabledWildcardRules()
	if err != nil {
		return err
	}

	newExactCache := make(map[string][]*model.BlockRule)
	for i := range exactRules {
		key := string(exactRules[i].PackageType) + ":" + exactRules[i].PackageName + ":" + exactRules[i].Version
		newExactCache[key] = append(newExactCache[key], &exactRules[i])
	}

	newWildcardCache := make(map[string][]cachedWildcardRule)
	for i := range wildcardRules {
		rule := &wildcardRules[i]
		regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(rule.PackageName), `\*`, ".*") + "$"
		compiled, err := regexp.Compile(regexPattern)
		if err != nil {
			continue
		}
		pkgType := string(rule.PackageType)
		newWildcardCache[pkgType] = append(newWildcardCache[pkgType], cachedWildcardRule{
			rule:     rule,
			compiled: compiled,
		})
	}

	s.exactRulesCache = newExactCache
	s.wildcardRules = newWildcardCache
	s.cachedAt = time.Now()

	return nil
}

func (s *BlockRuleService) matchCachedWildcard(cached *cachedWildcardRule, pkgName, version string) bool {
	if !cached.compiled.MatchString(pkgName) {
		return false
	}
	if cached.rule.Version == "*" {
		return true
	}
	versionPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(cached.rule.Version), `\*`, ".*") + "$"
	matched, _ := regexp.MatchString(versionPattern, version)
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
	err := s.repo.Create(rule)
	if err == nil {
		s.invalidateCache()
	}
	return err
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
	if success > 0 {
		s.invalidateCache()
	}
	return success, failed, nil
}

func (s *BlockRuleService) Update(id uint, updates map[string]interface{}) error {
	err := s.repo.Update(id, updates)
	if err == nil {
		s.invalidateCache()
	}
	return err
}

func (s *BlockRuleService) Delete(id uint) error {
	err := s.repo.Delete(id)
	if err == nil {
		s.invalidateCache()
	}
	return err
}

func (s *BlockRuleService) invalidateCache() {
	s.cacheMu.Lock()
	s.cachedAt = time.Time{}
	s.cacheMu.Unlock()
}

func (s *BlockRuleService) GetByID(id uint) (*model.BlockRule, error) {
	return s.repo.GetByID(id)
}

func (s *BlockRuleService) List(filter map[string]interface{}) ([]model.BlockRule, error) {
	return s.repo.List(filter)
}

func (s *BlockRuleService) ListWithPage(page, pageSize int, filter map[string]interface{}) ([]model.BlockRule, int64, error) {
	return s.repo.ListWithPage(page, pageSize, filter)
}
