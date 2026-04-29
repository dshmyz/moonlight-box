package proxy

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// ProxyAuthConfig 代理认证配置
// 支持 basic、bearer、api_key 三种认证方式
type ProxyAuthConfig struct {
	Type   string      `json:"type"`
	Basic  *BasicAuth  `json:"basic,omitempty"`
	Bearer *BearerAuth `json:"bearer,omitempty"`
	APIKey *APIKeyAuth `json:"api_key,omitempty"`
}

// BasicAuth Basic 认证配置
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BearerAuth Bearer Token 认证配置
type BearerAuth struct {
	Token string `json:"token"`
}

// APIKeyAuth API Key 认证配置
type APIKeyAuth struct {
	HeaderName string `json:"header_name"`
	KeyValue   string `json:"key_value"`
	QueryParam string `json:"query_param,omitempty"`
}

// Apply 将认证信息应用到 HTTP 请求上
func (c *ProxyAuthConfig) Apply(req *http.Request) error {
	if c == nil || c.Type == "none" {
		return nil
	}

	switch c.Type {
	case "basic":
		if c.Basic == nil {
			return fmt.Errorf("basic auth config is missing")
		}
		password := resolveEnv(c.Basic.Password)
		req.SetBasicAuth(c.Basic.Username, password)

	case "bearer":
		if c.Bearer == nil {
			return fmt.Errorf("bearer auth config is missing")
		}
		token := resolveEnv(c.Bearer.Token)
		req.Header.Set("Authorization", "Bearer "+token)

	case "api_key":
		if c.APIKey == nil {
			return fmt.Errorf("api key auth config is missing")
		}
		keyValue := resolveEnv(c.APIKey.KeyValue)
		req.Header.Set(c.APIKey.HeaderName, keyValue)
		if c.APIKey.QueryParam != "" {
			q := req.URL.Query()
			q.Set(c.APIKey.QueryParam, keyValue)
			req.URL.RawQuery = q.Encode()
		}

	default:
		return fmt.Errorf("unsupported auth type: %s", c.Type)
	}

	return nil
}

// resolveEnv 解析环境变量占位符
// 如果字符串格式为 ${ENV_VAR}，则从环境变量中获取对应值
func resolveEnv(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envKey := s[2 : len(s)-1]
		return os.Getenv(envKey)
	}
	return s
}
