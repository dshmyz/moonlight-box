package proxy

import "encoding/json"

// FailureCacheRule 失败缓存规则
type FailureCacheRule struct {
	StatusCode      int   `json:"status_code,omitempty"`
	StatusCodeRange []int `json:"status_code_range,omitempty"`
	TTLSeconds      int   `json:"ttl_seconds"`
}

// FailureCacheRules 失败缓存规则列表
type FailureCacheRules []FailureCacheRule

// ParseFailureCacheRules 解析 JSON 字符串为规则列表
func ParseFailureCacheRules(jsonStr string) (FailureCacheRules, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var rules FailureCacheRules
	err := json.Unmarshal([]byte(jsonStr), &rules)
	return rules, err
}

// Match 匹配状态码，返回匹配的规则的 TTL，未匹配返回 0
func (rules FailureCacheRules) Match(statusCode int) int {
	for _, rule := range rules {
		if rule.StatusCode > 0 && rule.StatusCode == statusCode {
			return rule.TTLSeconds
		}
		if len(rule.StatusCodeRange) == 2 {
			if statusCode >= rule.StatusCodeRange[0] && statusCode <= rule.StatusCodeRange[1] {
				return rule.TTLSeconds
			}
		}
	}
	return 0
}

// ShouldCache 判断是否应该缓存该状态码
func (rules FailureCacheRules) ShouldCache(statusCode int) bool {
	return rules.Match(statusCode) > 0
}
