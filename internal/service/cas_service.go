package service

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CAS 端点默认路径，由 cas_service、config_initializer seed 与前端 CASSettings 共用。
const (
	DefaultCASLoginPath    = "/cas/login"
	DefaultCASValidatePath = "/cas/serviceValidate"
	// defaultServicePath 是前端 SPA 登录页路由。service_url 留空时按请求 Host 推导回跳地址，
	// CAS 登录后浏览器会落回该页面并携带 ticket。
	defaultServicePath = "/login"
)

// casHTTPClient 为 CAS 出站请求统一加超时，避免 CAS 不可达时登录/测试连接请求无限挂起。
var casHTTPClient = &http.Client{Timeout: 10 * time.Second}

type CASService struct {
	cfg       *config.CASConfig
	authCfg   *config.AuthConfig
	configSvc *SystemConfigService
	userRepo  *repository.UserRepository
	roleRepo  *repository.RoleRepository
	authSvc   *AuthService
}

type CASValidationResponse struct {
	XMLName               xml.Name `xml:"cas:serviceResponse"`
	AuthenticationSuccess *struct {
		XMLName    xml.Name `xml:"cas:authenticationSuccess"`
		User       string   `xml:"cas:user"`
		Attributes *struct {
			XMLName     xml.Name `xml:"cas:attributes"`
			DisplayName string   `xml:"cas:displayName,omitempty"`
			Email       string   `xml:"cas:email,omitempty"`
		} `xml:"cas:attributes,omitempty"`
	} `xml:"cas:authenticationSuccess,omitempty"`
	AuthenticationFailure *struct {
		XMLName xml.Name `xml:"cas:authenticationFailure"`
		Code    string   `xml:"code,attr"`
		Message string   `xml:",chardata"`
	} `xml:"cas:authenticationFailure,omitempty"`
}

func NewCASService(
	cfg *config.AuthConfig,
	userRepo *repository.UserRepository,
	roleRepo *repository.RoleRepository,
	authSvc *AuthService,
	configSvc *SystemConfigService,
) *CASService {
	return &CASService{
		cfg:       &cfg.CAS,
		authCfg:   cfg,
		userRepo:  userRepo,
		roleRepo:  roleRepo,
		authSvc:   authSvc,
		configSvc: configSvc,
	}
}

func (s *CASService) getEffectiveConfig() *model.CASConfig {
	if s.configSvc != nil {
		enabled, _ := s.configSvc.Get("cas.enabled")
		if enabled != nil && enabled.Value == "true" {
			serverURL, _ := s.configSvc.Get("cas.server_url")
			serviceURL, _ := s.configSvc.Get("cas.service_url")
			loginPath, _ := s.configSvc.Get("cas.login_path")
			validatePath, _ := s.configSvc.Get("cas.validate_path")
			allowedHosts, _ := s.configSvc.Get("cas.allowed_hosts")

			casConfig := &model.CASConfig{
				Enabled: true,
			}
			if serverURL != nil {
				casConfig.ServerURL = serverURL.Value
			}
			if serviceURL != nil {
				casConfig.ServiceURL = serviceURL.Value
			}
			if loginPath != nil {
				casConfig.LoginPath = loginPath.Value
			}
			if validatePath != nil {
				casConfig.ValidatePath = validatePath.Value
			}
			if allowedHosts != nil {
				casConfig.AllowedHosts = splitHostList(allowedHosts.Value)
			}
			return casConfig
		}
	}

	if s.cfg.Enabled {
		return &model.CASConfig{
			Enabled:      s.cfg.Enabled,
			ServerURL:    s.cfg.ServerURL,
			ServiceURL:   s.cfg.ServiceURL,
			LoginPath:    s.cfg.LoginPath,
			ValidatePath: s.cfg.ValidatePath,
			AllowedHosts: s.cfg.AllowedHosts,
		}
	}

	return &model.CASConfig{}
}

// splitHostList 将逗号分隔的域名白名单拆成切片，空项与空白忽略。
func splitHostList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// GetAdminConfig 返回当前 CAS 配置（供管理端使用，始终返回完整字段）
func (s *CASService) GetAdminConfig() *model.CASConfig {
	return s.getEffectiveConfig()
}

// UpdateAdminConfig 更新 CAS 配置到 system_configs 表
func (s *CASService) UpdateAdminConfig(cfg *model.CASConfig, userID uint) error {
	if s.configSvc == nil {
		return fmt.Errorf("system config service unavailable")
	}

	enabledVal := "false"
	if cfg.Enabled {
		enabledVal = "true"
	}

	keys := []struct {
		key, value, valueType, category, description string
	}{
		{"cas.enabled", enabledVal, "bool", "login", "Enable CAS SSO"},
		{"cas.server_url", cfg.ServerURL, "string", "login", "CAS server base URL"},
		{"cas.service_url", cfg.ServiceURL, "string", "login", "CAS service URL (callback)"},
		{"cas.login_path", cfg.LoginPath, "string", "login", "CAS login path"},
		{"cas.validate_path", cfg.ValidatePath, "string", "login", "CAS ticket validation path"},
		{"cas.allowed_hosts", strings.Join(cfg.AllowedHosts, ","), "string", "login", "Allowed hosts for dynamic service URL (comma separated)"},
	}

	for _, k := range keys {
		if err := s.configSvc.Set(k.key, k.value, k.valueType, k.category, k.description, false, userID); err != nil {
			return fmt.Errorf("failed to set %s: %w", k.key, err)
		}
	}
	return nil
}

// TestConnection 测试 CAS 服务器可达性
func (s *CASService) TestConnection() error {
	cfg := s.getEffectiveConfig()
	if cfg.ServerURL == "" {
		return fmt.Errorf("CAS server URL is not configured")
	}
	if cfg.LoginPath == "" {
		cfg.LoginPath = DefaultCASLoginPath
	}

	testURL := strings.TrimRight(cfg.ServerURL, "/") + cfg.LoginPath
	resp, err := casHTTPClient.Head(testURL)
	if err != nil {
		return fmt.Errorf("failed to connect to CAS server: %w", err)
	}
	defer resp.Body.Close()

	// CAS 登录端点通常返回 302 重定向，200/302 都算可达
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("CAS server returned status %d", resp.StatusCode)
}

func (s *CASService) IsEnabled() bool {
	cfg := s.getEffectiveConfig()
	// enabled ⟹ service 可解析：要么配置了静态 service_url，要么配置了 allowed_hosts
	// 供动态推导。二者皆空时 CAS 无法完成登录，视为未启用，避免登录/回调可预见地失败。
	return cfg.Enabled && cfg.ServerURL != "" && (cfg.ServiceURL != "" || len(cfg.AllowedHosts) > 0)
}

// resolveServiceURL 决定 CAS service 参数：优先使用静态配置 service_url（单域名/兼容旧行为）；
// 留空时按当前请求的 Host 动态推导（多域名场景），并要求域名命中 allowed_hosts 白名单，
// 防止 Host 头注入把用户弹到任意站点。cfg 由调用方传入，全链路复用一次 getEffectiveConfig。
func (s *CASService) resolveServiceURL(c *gin.Context, cfg *model.CASConfig) (string, error) {
	if cfg.ServiceURL != "" {
		return cfg.ServiceURL, nil
	}

	host := firstForwardedValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return "", fmt.Errorf("service_url 未配置且无法从请求解析 Host")
	}
	if !s.isHostAllowed(cfg, host) {
		return "", fmt.Errorf("service_url 未配置且域名 %s 不在允许列表中，请在 CAS 设置中填写回调地址或允许的域名", host)
	}

	scheme := firstForwardedValue(c.GetHeader("X-Forwarded-Proto"))
	// 仅接受 http/https，其它 scheme（代理透传异常值/攻击性注入）一律回退到 TLS 探测结果
	if scheme == "" || (scheme != "http" && scheme != "https") {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, defaultServicePath), nil
}

// firstForwardedValue 取 X-Forwarded-* 头首个值（RFC 7239 允许逗号分隔多值，取最近一跳代理的设置）。
func firstForwardedValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return strings.TrimSpace(v)
}

// isHostAllowed 校验 host 是否命中白名单：支持精确匹配与 *.example.com 通配（含裸后缀）。
// host 与 pattern 均忽略端口与结尾点，比较不区分大小写。
func (s *CASService) isHostAllowed(cfg *model.CASConfig, host string) bool {
	clean := normalizeHostForMatch(host)
	for _, pattern := range cfg.AllowedHosts {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		p := normalizeHostForMatch(pattern)
		if strings.HasPrefix(p, "*.") {
			suffix := strings.ToLower(strings.TrimPrefix(p, "*."))
			low := strings.ToLower(clean)
			if strings.EqualFold(clean, suffix) || strings.HasSuffix(low, "."+suffix) {
				return true
			}
		} else if strings.EqualFold(clean, p) {
			return true
		}
	}
	return false
}

// normalizeHostForMatch 去掉端口与结尾点，得到可比较的主机名（域名域名比较一般仅 ASCII）。
func normalizeHostForMatch(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

func (s *CASService) GetLoginURL(c *gin.Context, redirect string) (string, error) {
	cfg := s.getEffectiveConfig()
	serviceURL, err := s.resolveServiceURL(c, cfg)
	if err != nil {
		return "", err
	}
	if redirect != "" {
		serviceURL = fmt.Sprintf("%s?redirect=%s", serviceURL, url.QueryEscape(redirect))
	}
	return fmt.Sprintf("%s%s?service=%s",
		cfg.ServerURL,
		cfg.LoginPath,
		url.QueryEscape(serviceURL),
	), nil
}

func (s *CASService) ValidateTicket(c *gin.Context, ticket string) (username string, displayName string, email string, err error) {
	cfg := s.getEffectiveConfig()
	serviceURL, err := s.resolveServiceURL(c, cfg)
	if err != nil {
		return "", "", "", err
	}
	// CAS 校验时 service 必须与登录时完全一致；redirect 参数随 CAS 回跳原样带回，需一并拼回。
	if redirect := c.Query("redirect"); redirect != "" {
		serviceURL = fmt.Sprintf("%s?redirect=%s", serviceURL, url.QueryEscape(redirect))
	}
	validateURL := fmt.Sprintf("%s%s?service=%s&ticket=%s",
		cfg.ServerURL,
		cfg.ValidatePath,
		url.QueryEscape(serviceURL),
		ticket,
	)

	resp, err := casHTTPClient.Get(validateURL)
	if err != nil {
		return "", "", "", fmt.Errorf("CAS validation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read CAS response: %w", err)
	}

	var casResp CASValidationResponse
	if err := xml.Unmarshal(body, &casResp); err != nil {
		return "", "", "", fmt.Errorf("failed to parse CAS response: %w", err)
	}

	if casResp.AuthenticationFailure != nil {
		return "", "", "", fmt.Errorf("CAS authentication failed: %s - %s",
			casResp.AuthenticationFailure.Code,
			casResp.AuthenticationFailure.Message)
	}

	if casResp.AuthenticationSuccess == nil {
		return "", "", "", fmt.Errorf("invalid CAS response")
	}

	success := casResp.AuthenticationSuccess
	username = success.User

	if success.Attributes != nil {
		displayName = success.Attributes.DisplayName
		email = success.Attributes.Email
	}

	if username == "" {
		return "", "", "", fmt.Errorf("CAS returned empty username")
	}

	return username, displayName, email, nil
}

func (s *CASService) LoginByTicket(c *gin.Context, ticket string) (*AuthResponse, error) {
	casUsername, casDisplayName, casEmail, err := s.ValidateTicket(c, ticket)
	if err != nil {
		return nil, fmt.Errorf("ticket validation failed: %w", err)
	}

	user, err := s.userRepo.FindOrCreateCASUser(casUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create CAS user: %w", err)
	}

	if casDisplayName != "" {
		user.DisplayName = casDisplayName
		if err := s.userRepo.Update(user); err != nil {
			logrus.WithError(err).WithField("user_id", user.ID).Warn("CAS: failed to update display name")
		}
	}
	if casEmail != "" && user.Email == casUsername+"@cas.local" {
		user.Email = casEmail
		if err := s.userRepo.Update(user); err != nil {
			logrus.WithError(err).WithField("user_id", user.ID).Warn("CAS: failed to update email")
		}
	}

	roles, _ := s.roleRepo.GetUserRoles(user.ID)
	if len(roles) == 0 {
		s.assignDefaultRole(user.ID)
		roles, _ = s.roleRepo.GetUserRoles(user.ID)
	}

	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	accessToken, err := s.authSvc.generateToken(user.ID, user.Username, roleNames, s.authCfg.TokenExpiry)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.authSvc.generateToken(user.ID, user.Username, roleNames, s.authCfg.RefreshExpiry)
	if err != nil {
		return nil, err
	}

	if err := s.userRepo.UpdateLastLogin(user.ID); err != nil {
		logrus.WithError(err).WithField("user_id", user.ID).Warn("CAS: failed to update last login")
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.authCfg.TokenExpiry.Seconds(),
		User:         user.ToDTO(),
	}, nil
}

func (s *CASService) assignDefaultRole(userID uint) {
	roles, _ := s.roleRepo.List()
	for _, role := range roles {
		if role.Name == "readonly" {
			_ = s.roleRepo.AssignRole(userID, role.ID, 0)
			break
		}
	}
}
