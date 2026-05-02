package ai

import (
	"regexp"
	"strings"
	"sync"
)

// Sanitizer 数据脱敏器
type Sanitizer struct {
	rules []sanitizationRule
	mu    sync.Mutex
}

// sanitizationRule 脱敏规则
type sanitizationRule struct {
	name        string
	pattern     *regexp.Regexp
	replacement string
}

// SanitizerConfig 脱敏器配置
type SanitizerConfig struct {
	// 是否启用各种脱敏规则
	EnablePassword bool
	EnableAPIKey   bool
	EnableToken    bool
	EnableIP       bool
	EnableEmail    bool
	EnablePhone    bool
	EnableIDCard   bool
	EnableBankCard bool
	// 自定义规则
	CustomRules []CustomSanitizationRule
}

// CustomSanitizationRule 自定义脱敏规则
type CustomSanitizationRule struct {
	Name        string
	Pattern     string
	Replacement string
}

// DefaultSanitizerConfig 返回默认配置
func DefaultSanitizerConfig() *SanitizerConfig {
	return &SanitizerConfig{
		EnablePassword: true,
		EnableAPIKey:   true,
		EnableToken:    true,
		EnableIP:       true,
		EnableEmail:    true,
		EnablePhone:    true,
		EnableIDCard:   true,
		EnableBankCard: true,
	}
}

// NewSanitizer 创建一个新的脱敏器
func NewSanitizer(cfg *SanitizerConfig) *Sanitizer {
	if cfg == nil {
		cfg = DefaultSanitizerConfig()
	}

	s := &Sanitizer{
		rules: make([]sanitizationRule, 0),
	}

	// 添加内置规则
	if cfg.EnablePassword {
		s.addPasswordRules()
	}
	if cfg.EnableAPIKey {
		s.addAPIKeyRules()
	}
	if cfg.EnableToken {
		s.addTokenRules()
	}
	if cfg.EnableIP {
		s.addIPRules()
	}
	if cfg.EnableEmail {
		s.addEmailRules()
	}
	if cfg.EnablePhone {
		s.addPhoneRules()
	}
	if cfg.EnableIDCard {
		s.addIDCardRules()
	}
	if cfg.EnableBankCard {
		s.addBankCardRules()
	}

	// 添加自定义规则
	for _, custom := range cfg.CustomRules {
		if compiled, err := regexp.Compile(custom.Pattern); err == nil {
			s.rules = append(s.rules, sanitizationRule{
				name:        custom.Name,
				pattern:     compiled,
				replacement: custom.Replacement,
			})
		}
	}

	return s
}

// addPasswordRules 添加密码相关规则
func (s *Sanitizer) addPasswordRules() {
	// password=xxx, password: xxx, "password": "xxx" 等格式
	s.rules = append(s.rules, sanitizationRule{
		name:        "password_assignment",
		pattern:     regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*['"]?([^\s'"<>]{4,})['"]?`),
		replacement: `${1}=${2}****`,
	})
	// JSON 格式的密码字段
	s.rules = append(s.rules, sanitizationRule{
		name:        "password_json",
		pattern:     regexp.MustCompile(`(?i)"(password|passwd|pwd)"\s*:\s*"([^"]+)"`),
		replacement: `"${1}":"****"`,
	})
}

// addAPIKeyRules 添加API密钥相关规则
func (s *Sanitizer) addAPIKeyRules() {
	// api_key=xxx, apikey=xxx 等格式
	s.rules = append(s.rules, sanitizationRule{
		name:        "apikey_assignment",
		pattern:     regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[=:]\s*['"]?([a-zA-Z0-9_-]{20,})['"]?`),
		replacement: `${1}=${2}****`,
	})
	// JSON 格式的API密钥
	s.rules = append(s.rules, sanitizationRule{
		name:        "apikey_json",
		pattern:     regexp.MustCompile(`(?i)"(api[_-]?key|apikey)"\s*:\s*"([^"]+)"`),
		replacement: `"${1}":"****"`,
	})
	// Bearer token
	s.rules = append(s.rules, sanitizationRule{
		name:        "bearer_token",
		pattern:     regexp.MustCompile(`(?i)Bearer\s+[a-zA-Z0-9_-]{20,}`),
		replacement: `Bearer ****`,
	})
	// Authorization header
	s.rules = append(s.rules, sanitizationRule{
		name:        "auth_header",
		pattern:     regexp.MustCompile(`(?i)(Authorization|Auth)\s*[=:]\s*['"]?[^'"<>\s]+['"]?`),
		replacement: `${1}=****`,
	})
}

// addTokenRules 添加Token相关规则
func (s *Sanitizer) addTokenRules() {
	// token=xxx 格式
	s.rules = append(s.rules, sanitizationRule{
		name:        "token_assignment",
		pattern:     regexp.MustCompile(`(?i)(access[_-]?token|auth[_-]?token|token)\s*[=:]\s*['"]?([a-zA-Z0-9_.-]{20,})['"]?`),
		replacement: `${1}=${2}****`,
	})
	// JSON 格式的token
	s.rules = append(s.rules, sanitizationRule{
		name:        "token_json",
		pattern:     regexp.MustCompile(`(?i)"(access[_-]?token|auth[_-]?token|token)"\s*:\s*"([^"]+)"`),
		replacement: `"${1}":"****"`,
	})
	// JWT token (三段式)
	s.rules = append(s.rules, sanitizationRule{
		name:        "jwt_token",
		pattern:     regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`),
		replacement: `eyJ****.eyJ****.****`,
	})
}

// addIPRules 添加IP地址相关规则
func (s *Sanitizer) addIPRules() {
	// IPv4 地址
	s.rules = append(s.rules, sanitizationRule{
		name:        "ipv4",
		pattern:     regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`),
		replacement: `${1}.${2}.***.***`,
	})
	// IPv6 地址（简化匹配）
	s.rules = append(s.rules, sanitizationRule{
		name:        "ipv6",
		pattern:     regexp.MustCompile(`\b([0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b`),
		replacement: `****:****:****:****`,
	})
}

// addEmailRules 添加邮箱相关规则
func (s *Sanitizer) addEmailRules() {
	// 邮箱脱敏需要特殊处理，使用 ReplaceAllStringFunc
	// 这里只添加规则标记，实际处理在 Sanitize 方法中
	s.rules = append(s.rules, sanitizationRule{
		name:        "email",
		pattern:     regexp.MustCompile(`\b([a-zA-Z0-9._%+-]+)@([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})\b`),
		replacement: `EMAIL_PLACEHOLDER`,
	})
}

// addPhoneRules 添加手机号相关规则
func (s *Sanitizer) addPhoneRules() {
	// 中国手机号
	s.rules = append(s.rules, sanitizationRule{
		name:        "phone_cn",
		pattern:     regexp.MustCompile(`\b(1[3-9])\d{9}\b`),
		replacement: `${1}****1234`,
	})
	// 国际格式手机号
	s.rules = append(s.rules, sanitizationRule{
		name:        "phone_intl",
		pattern:     regexp.MustCompile(`\+(\d{1,3})\s*\d{7,14}\b`),
		replacement: `+${1}****`,
	})
}

// addIDCardRules 添加身份证号相关规则
func (s *Sanitizer) addIDCardRules() {
	// 中国身份证号（18位）
	s.rules = append(s.rules, sanitizationRule{
		name:        "idcard_cn",
		pattern:     regexp.MustCompile(`\b\d{6}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`),
		replacement: `******************`,
	})
}

// addBankCardRules 添加银行卡号相关规则
func (s *Sanitizer) addBankCardRules() {
	// 银行卡号（16-19位）
	s.rules = append(s.rules, sanitizationRule{
		name:        "bankcard",
		pattern:     regexp.MustCompile(`\b\d{4}\d{8,12}\d{4}\b`),
		replacement: `**** **** **** 1234`,
	})
}

// Sanitize 对输入文本进行脱敏处理
func (s *Sanitizer) Sanitize(input string) string {
	result := input
	for _, rule := range s.rules {
		// 邮箱需要特殊处理
		if rule.name == "email" {
			result = rule.pattern.ReplaceAllStringFunc(result, func(match string) string {
				parts := strings.Split(match, "@")
				if len(parts) != 2 {
					return match
				}
				local := parts[0]
				domain := parts[1]
				if len(local) <= 2 {
					return "***@" + domain
				}
				return local[:2] + "***@" + domain
			})
		} else {
			result = rule.pattern.ReplaceAllString(result, rule.replacement)
		}
	}
	return result
}

// SanitizeToolResult 对工具执行结果进行脱敏处理
// 根据工具名称应用不同的脱敏策略
func (s *Sanitizer) SanitizeToolResult(toolName, result string) string {
	// 先应用通用脱敏规则
	sanitized := s.Sanitize(result)

	// 针对特定工具的额外处理
	switch strings.ToLower(toolName) {
	case "file_read", "read_file":
		// 文件读取结果可能包含敏感配置
		return s.sanitizeFileContent(sanitized)
	case "http_request", "fetch":
		// HTTP请求结果可能包含敏感头信息
		return s.sanitizeHTTPResponse(sanitized)
	case "database_query", "sql_query":
		// 数据库查询结果可能包含敏感数据
		return s.sanitizeDatabaseResult(sanitized)
	default:
		return sanitized
	}
}

// sanitizeFileContent 对文件内容进行额外脱敏
func (s *Sanitizer) sanitizeFileContent(content string) string {
	// 额外处理环境变量文件
	envPattern := regexp.MustCompile(`(?im)^([a-zA-Z_][a-zA-Z0-9_]*(?:KEY|SECRET|PASSWORD|TOKEN|PASS))\s*=\s*(.+)$`)
	return envPattern.ReplaceAllString(content, `${1}=****`)
}

// sanitizeHTTPResponse 对HTTP响应进行额外脱敏
func (s *Sanitizer) sanitizeHTTPResponse(response string) string {
	// 处理Set-Cookie头
	cookiePattern := regexp.MustCompile(`(?i)(Set-Cookie|Cookie)\s*:\s*[^\n]+`)
	return cookiePattern.ReplaceAllString(response, `${1}: ****`)
}

// sanitizeDatabaseResult 对数据库结果进行额外脱敏
func (s *Sanitizer) sanitizeDatabaseResult(result string) string {
	// 处理常见的敏感字段
	sensitiveFields := []string{"password", "secret", "token", "key", "salt"}
	for _, field := range sensitiveFields {
		pattern := regexp.MustCompile(`(?i)"?` + field + `"?\s*[=:]\s*["']?[^"',}\s]+["']?`)
		result = pattern.ReplaceAllString(result, `"${field}": "****"`)
	}
	return result
}

// AddCustomRule 添加自定义脱敏规则
func (s *Sanitizer) AddCustomRule(name, pattern, replacement string) error {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.rules = append(s.rules, sanitizationRule{
		name:        name,
		pattern:     compiled,
		replacement: replacement,
	})
	return nil
}
