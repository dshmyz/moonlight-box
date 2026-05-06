package service

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

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
		}
	}

	return &model.CASConfig{}
}

func (s *CASService) IsEnabled() bool {
	cfg := s.getEffectiveConfig()
	return cfg.Enabled && cfg.ServerURL != "" && cfg.ServiceURL != ""
}

func (s *CASService) GetLoginURL(redirect string) string {
	cfg := s.getEffectiveConfig()
	serviceURL := cfg.ServiceURL
	if redirect != "" {
		serviceURL = fmt.Sprintf("%s?redirect=%s", cfg.ServiceURL, url.QueryEscape(redirect))
	}
	return fmt.Sprintf("%s%s?service=%s",
		cfg.ServerURL,
		cfg.LoginPath,
		url.QueryEscape(serviceURL),
	)
}

func (s *CASService) ValidateTicket(ticket string) (username string, displayName string, email string, err error) {
	cfg := s.getEffectiveConfig()
	validateURL := fmt.Sprintf("%s%s?service=%s&ticket=%s",
		cfg.ServerURL,
		cfg.ValidatePath,
		url.QueryEscape(cfg.ServiceURL),
		ticket,
	)

	resp, err := http.Get(validateURL)
	if err != nil {
		return "", "", "", fmt.Errorf("CAS validation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
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

func (s *CASService) LoginByTicket(ticket string) (*AuthResponse, error) {
	casUsername, casDisplayName, casEmail, err := s.ValidateTicket(ticket)
	if err != nil {
		return nil, fmt.Errorf("ticket validation failed: %w", err)
	}

	user, err := s.userRepo.FindOrCreateCASUser(casUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create CAS user: %w", err)
	}

	if casDisplayName != "" {
		user.DisplayName = casDisplayName
		_ = s.userRepo.Update(user)
	}
	if casEmail != "" && user.Email == casUsername+"@cas.local" {
		user.Email = casEmail
		_ = s.userRepo.Update(user)
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

	_ = s.userRepo.UpdateLastLogin(user.ID)

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
