package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
)

type cachedWildcardRule struct {
	rule            *model.BlockRule
	compiled        *regexp.Regexp // 包名正则
	versionCompiled *regexp.Regexp // 版本正则（Version=="*" 时为 nil）
}

// cachedConditionalRule 条件规则缓存，带预编译的包名正则，用于第二层匹配前先检查包名+版本
type cachedConditionalRule struct {
	rule            *model.BlockRule
	pkgCompiled     *regexp.Regexp // 包名正则（exact 时为精确匹配，wildcard 时为通配符）
	versionCompiled *regexp.Regexp // 版本正则（Version=="*" 时为 nil）
}

type BlockRuleService struct {
	repo     *repository.BlockRuleRepository
	auditSvc *AuditService

	cacheMu          sync.RWMutex
	cachedAt         time.Time
	cacheTTL         time.Duration
	exactRulesCache  map[string][]*model.BlockRule
	wildcardRules    map[string][]cachedWildcardRule
	conditionalRules map[string][]cachedConditionalRule // 按 PackageType 分组的条件规则（第二层匹配）
}

func NewBlockRuleService(repo *repository.BlockRuleRepository, auditSvc *AuditService) *BlockRuleService {
	svc := &BlockRuleService{
		repo:             repo,
		auditSvc:         auditSvc,
		cacheTTL:         1 * time.Minute,
		exactRulesCache:  make(map[string][]*model.BlockRule),
		wildcardRules:    make(map[string][]cachedWildcardRule),
		conditionalRules: make(map[string][]cachedConditionalRule),
	}
	return svc
}

type BlockResult struct {
	Blocked bool
	Rule    *model.BlockRule
}

// ConditionalRuleRequirement identifies a conditional rule that may apply to a
// package key and the semantic artifact attribute needed to evaluate it.
type ConditionalRuleRequirement struct {
	RuleID    uint
	Attribute string
}

// RequiredAttributes returns the attributes needed by conditional rules whose
// package name and version constraints can match. It only reads the local rule
// cache and never performs remote I/O.
func (s *BlockRuleService) RequiredAttributes(pkgType, pkgName, version string) []ConditionalRuleRequirement {
	s.cacheMu.RLock()
	cacheValid := time.Since(s.cachedAt) < s.cacheTTL
	s.cacheMu.RUnlock()
	if !cacheValid {
		if err := s.refreshCache(); err != nil {
			logrus.WithFields(logrus.Fields{
				"pkg_type": pkgType,
				"pkg_name": pkgName,
				"version":  version,
			}).Warn("刷新条件规则缓存失败，本次请求将跳过条件拦截")
			return nil
		}
	}

	s.cacheMu.RLock()
	rules := append([]cachedConditionalRule(nil), s.conditionalRules[pkgType]...)
	allRules := append([]cachedConditionalRule(nil), s.conditionalRules[model.PackageTypeAll]...)
	s.cacheMu.RUnlock()

	requirements := make([]ConditionalRuleRequirement, 0)
	seen := make(map[string]struct{})
	for _, cached := range append(rules, allRules...) {
		if !s.matchPkgNameVersion(&cached, pkgName, version) {
			continue
		}
		attribute := conditionalRuleAttribute(cached.rule.ConditionType)
		if attribute == "" {
			continue
		}
		key := strconv.FormatUint(uint64(cached.rule.ID), 10) + ":" + attribute
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		requirements = append(requirements, ConditionalRuleRequirement{
			RuleID:    cached.rule.ID,
			Attribute: attribute,
		})
	}
	return requirements
}

func conditionalRuleAttribute(conditionType model.ConditionType) string {
	switch conditionType {
	case model.ConditionTypeLicense:
		return "license"
	case model.ConditionTypePublishTime:
		return "published_at"
	default:
		return ""
	}
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

// IsBlockedWithArtifact 两层规则匹配：
// 第一层调用 IsBlocked（包名+版本）；未命中时进入第二层条件匹配，
// 遍历条件规则，按 ConditionType 从 attrs 取值并按 ConditionOp 匹配。
// attrs 中缺少对应 key 时放行（元数据缺失不阻断）。
func (s *BlockRuleService) IsBlockedWithArtifact(pkgType, pkgName, version string, attrs map[string]interface{}) (*BlockResult, error) {
	// 第一层：包名+版本匹配
	firstResult, err := s.IsBlocked(pkgType, pkgName, version)
	if err != nil {
		return nil, err
	}
	if firstResult.Blocked {
		return firstResult, nil
	}

	// 第二层：条件匹配（先检查包名+版本是否匹配规则，再检查条件）
	// 同时检查当前包类型和 "all"（跨包类型）的规则
	s.cacheMu.RLock()
	rules := s.conditionalRules[pkgType]
	allRules := s.conditionalRules[model.PackageTypeAll]
	s.cacheMu.RUnlock()

	if result := s.matchConditionalRules(rules, pkgName, version, attrs); result != nil {
		return result, nil
	}
	if result := s.matchConditionalRules(allRules, pkgName, version, attrs); result != nil {
		return result, nil
	}

	return &BlockResult{Blocked: false}, nil
}

// matchConditionalRules 遍历条件规则，先匹配包名+版本，再匹配条件。
// 命中时返回 *BlockResult，未命中返回 nil。
func (s *BlockRuleService) matchConditionalRules(rules []cachedConditionalRule, pkgName, version string, attrs map[string]interface{}) *BlockResult {
	for i := range rules {
		cached := &rules[i]
		if !s.matchPkgNameVersion(cached, pkgName, version) {
			continue
		}
		if s.matchCondition(cached.rule, attrs) {
			return &BlockResult{Blocked: true, Rule: cached.rule}
		}
	}
	return nil
}

// matchCondition 根据规则的 ConditionType/ConditionOp 对 attrs 进行条件匹配。
// attrs 中缺少对应 key 时返回 false（元数据缺失放行）。
func (s *BlockRuleService) matchCondition(rule *model.BlockRule, attrs map[string]interface{}) bool {
	switch rule.ConditionType {
	case model.ConditionTypeLicense:
		raw, ok := attrs["license"]
		if !ok {
			return false
		}
		value, ok := raw.(string)
		if !ok {
			return false
		}
		switch rule.ConditionOp {
		case model.ConditionOpEquals:
			return value == rule.ConditionValue
		case model.ConditionOpContains:
			return strings.Contains(value, rule.ConditionValue)
		}
	case model.ConditionTypePublishTime:
		raw, ok := attrs["published_at"]
		if !ok {
			return false
		}
		value, ok := raw.(string)
		if !ok {
			return false
		}
		// 解析 attrs 中的时间（RFC3339）
		actualTime, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return false
		}
		switch rule.ConditionOp {
		case model.ConditionOpBefore:
			thresholdTime, err := time.Parse(time.RFC3339, rule.ConditionValue)
			if err != nil {
				return false
			}
			return actualTime.Before(thresholdTime)
		case model.ConditionOpAfter:
			thresholdTime, err := time.Parse(time.RFC3339, rule.ConditionValue)
			if err != nil {
				return false
			}
			return actualTime.After(thresholdTime)
		case model.ConditionOpWithinLast:
			// ConditionValue 为天数，计算最近 N 天的阈值时间
			days, err := strconv.Atoi(rule.ConditionValue)
			if err != nil || days < 0 {
				return false
			}
			threshold := time.Now().AddDate(0, 0, -days)
			return actualTime.After(threshold)
		}
	}
	return false
}

// compileVersionRegex 预编译版本通配符正则。Version=="*" 时返回 nil（匹配所有版本，走快速路径）。
func compileVersionRegex(version string) *regexp.Regexp {
	if version == "*" {
		return nil
	}
	pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(version), `\*`, ".*") + "$"
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return compiled
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

	// 加载第二层条件规则
	conditionalRules, err := s.repo.FindAllEnabledConditionalRules()
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
		versionCompiled := compileVersionRegex(rule.Version)
		newWildcardCache[rule.PackageType] = append(newWildcardCache[rule.PackageType], cachedWildcardRule{
			rule:            rule,
			compiled:        compiled,
			versionCompiled: versionCompiled,
		})
	}

	// 按 PackageType 分组构建条件规则缓存，预编译包名正则
	newConditionalCache := make(map[string][]cachedConditionalRule)
	for i := range conditionalRules {
		rule := &conditionalRules[i]

		// 为 wildcard 规则预编译包名正则；PackageName=* 或 exact 规则不需要正则
		var compiled *regexp.Regexp
		if rule.MatchType == model.BlockMatchWildcard && rule.PackageName != "*" {
			pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(rule.PackageName), `\*`, ".*") + "$"
			compiled, err = regexp.Compile(pattern)
			if err != nil {
				continue
			}
		}
		versionCompiled := compileVersionRegex(rule.Version)
		newConditionalCache[rule.PackageType] = append(newConditionalCache[rule.PackageType], cachedConditionalRule{
			rule:            rule,
			pkgCompiled:     compiled,
			versionCompiled: versionCompiled,
		})
	}

	s.exactRulesCache = newExactCache
	s.wildcardRules = newWildcardCache
	s.conditionalRules = newConditionalCache
	s.cachedAt = time.Now()

	return nil
}

// matchPkgNameVersion 检查请求的包名和版本是否匹配条件规则的包名和版本约束。
// 快速路径：PackageName=* + Version=* 时直接返回 true，不编译/使用正则。
// exact 规则精确匹配包名；wildcard 规则用预编译正则匹配包名。
func (s *BlockRuleService) matchPkgNameVersion(cached *cachedConditionalRule, pkgName, version string) bool {
	// 快速路径：PackageName=* + Version=* 直接放行
	if cached.rule.PackageName == "*" && cached.rule.Version == "*" {
		return true
	}

	// 包名匹配
	if cached.rule.MatchType == model.BlockMatchExact {
		if cached.rule.PackageName != pkgName {
			return false
		}
	} else {
		// wildcard：用预编译正则匹配（PackageName=* 时 pkgCompiled 为 nil，但上面已快速路径返回）
		if cached.pkgCompiled != nil && !cached.pkgCompiled.MatchString(pkgName) {
			return false
		}
	}

	// 版本匹配
	if cached.rule.Version == "*" || cached.versionCompiled == nil {
		return true
	}
	return cached.versionCompiled.MatchString(version)
}

func (s *BlockRuleService) matchCachedWildcard(cached *cachedWildcardRule, pkgName, version string) bool {
	if !cached.compiled.MatchString(pkgName) {
		return false
	}
	if cached.rule.Version == "*" || cached.versionCompiled == nil {
		return true
	}
	return cached.versionCompiled.MatchString(version)
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

// LogConditionUnverified records an allowed download whose applicable
// conditional rule could not be evaluated because metadata was unavailable.
func (s *BlockRuleService) LogConditionUnverified(ctx context.Context, repositoryID, format, pkgName, version, remotePath string, ruleIDs []uint, missing []string, reason string) error {
	if s.auditSvc == nil {
		return nil
	}
	details, _ := json.Marshal(map[string]interface{}{
		"repository_id":      repositoryID,
		"format":             format,
		"remote_path":        remotePath,
		"rule_ids":           ruleIDs,
		"missing_attributes": missing,
		"reason":             reason,
	})
	return s.auditSvc.LogWithRequestAndStatus(ctx, nil, model.ActionConditionUnverified, "package", nil,
		fmt.Sprintf("%s@%s", pkgName, version), string(details), "", "", 200, 0)
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
		// 条件规则字段一致性校验：设置了 ConditionType 时必须同时有 ConditionOp 和 ConditionValue
		if rule.ConditionType != "" && (rule.ConditionOp == "" || rule.ConditionValue == "") {
			failed++
			continue
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
