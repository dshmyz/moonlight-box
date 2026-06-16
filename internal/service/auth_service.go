package service

import (
	"errors"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/util"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

type AuthService struct {
	userRepo       *repository.UserRepository
	roleRepo       *repository.RoleRepository
	config         *config.AuthConfig
	tokenBlacklist map[string]time.Time // token -> expiry，生产环境可用 Redis
	blacklistMu    sync.RWMutex
	auditSvc       *AuditService
}

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=5"`
}

type AuthResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresIn    float64       `json:"expires_in"`
	User         model.UserDTO `json:"user"`
}

type TokenClaims struct {
	UserID   uint     `json:"uid"`
	Username string   `json:"uname"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

func NewAuthService(
	userRepo *repository.UserRepository,
	roleRepo *repository.RoleRepository,
	cfg *config.AuthConfig,
	auditSvc *AuditService,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		roleRepo:       roleRepo,
		config:         cfg,
		tokenBlacklist: make(map[string]time.Time),
		auditSvc:       auditSvc,
	}
}

func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		if errors.Is(err, util.ErrUserNotFound) {
			logrus.WithFields(logrus.Fields{
				"module":   "auth",
				"username": req.Username,
			}).Warn("Login failed: user not found")
			return nil, util.ErrInvalidCredentials
		}
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"username": req.Username,
			"error":    err,
		}).Error("Login failed: database error")
		return nil, err
	}

	if !util.CheckPasswordHash(req.Password, user.PasswordHash) {
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"user_id":  user.ID,
			"username": req.Username,
		}).Warn("Login failed: invalid password")
		return nil, util.ErrInvalidCredentials
	}

	if !user.IsActive {
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"user_id":  user.ID,
			"username": req.Username,
		}).Warn("Login failed: account disabled")
		return nil, errors.New("account is disabled")
	}

	roles, err := s.roleRepo.GetUserRoles(user.ID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": user.ID,
			"error":   err,
		}).Warn("Failed to get user roles during login, proceeding with empty roles")
		roles = nil
	}
	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	accessToken, err := s.generateToken(user.ID, user.Username, roleNames, s.config.TokenExpiry)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": user.ID,
			"error":   err,
		}).Error("Failed to generate access token")
		return nil, err
	}

	refreshToken, err := s.generateToken(user.ID, user.Username, roleNames, s.config.RefreshExpiry)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": user.ID,
			"error":   err,
		}).Error("Failed to generate refresh token")
		return nil, err
	}
	s.userRepo.UpdateLastLogin(user.ID)

	logrus.WithFields(logrus.Fields{
		"module":   "auth",
		"user_id":  user.ID,
		"username": req.Username,
	}).Info("User logged in successfully")

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.config.TokenExpiry.Seconds(),
		User:         user.ToDTO(),
	}, nil
}

func (s *AuthService) generateToken(userID uint, username string, roles []string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := TokenClaims{
		UserID:   userID,
		Username: username,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "moonlight-registry",
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
	s.pruneTokenBlacklist(time.Now())

	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		return nil, util.ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		// 检查黑名单
		if s.isTokenBlacklisted(tokenString, time.Now()) {
			return nil, errors.New("token has been revoked")
		}
		return claims, nil
	}

	return nil, util.ErrTokenInvalid
}

func (s *AuthService) Logout(tokenString string) error {
	s.blacklistToken(tokenString)
	logrus.WithFields(logrus.Fields{
		"module": "auth",
	}).Info("User logged out")
	return nil
}

func (s *AuthService) RefreshToken(refreshTokenString string) (*AuthResponse, error) {
	claims, err := s.ValidateToken(refreshTokenString)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "auth",
			"error":  err,
		}).Warn("Token refresh failed: invalid token")
		return nil, err
	}

	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": claims.UserID,
			"error":   err,
		}).Error("Token refresh failed: user not found")
		return nil, err
	}

	s.blacklistToken(refreshTokenString)

	accessToken, err := s.generateToken(user.ID, user.Username, claims.Roles, s.config.TokenExpiry)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": user.ID,
			"error":   err,
		}).Error("Failed to generate new access token during refresh")
		return nil, err
	}

	newRefreshToken, err := s.generateToken(user.ID, user.Username, claims.Roles, s.config.RefreshExpiry)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": user.ID,
			"error":   err,
		}).Error("Failed to generate new refresh token during refresh")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"module":   "auth",
		"user_id":  user.ID,
		"username": user.Username,
	}).Info("Token refreshed successfully")

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    s.config.TokenExpiry.Seconds(),
		User:         user.ToDTO(),
	}, nil
}

func (s *AuthService) blacklistToken(tokenString string) {
	if tokenString == "" {
		return
	}
	now := time.Now()
	expiresAt := now.Add(s.config.RefreshExpiry)
	var claims TokenClaims
	_, _, _ = jwt.NewParser().ParseUnverified(tokenString, &claims)
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	if !expiresAt.After(now) {
		s.pruneTokenBlacklist(now)
		return
	}

	s.blacklistMu.Lock()
	s.tokenBlacklist[tokenString] = expiresAt
	s.blacklistMu.Unlock()
	s.pruneTokenBlacklist(now)
}

func (s *AuthService) isTokenBlacklisted(tokenString string, now time.Time) bool {
	s.blacklistMu.RLock()
	expiresAt, ok := s.tokenBlacklist[tokenString]
	s.blacklistMu.RUnlock()
	if !ok {
		return false
	}
	if now.Before(expiresAt) {
		return true
	}
	s.blacklistMu.Lock()
	if current, ok := s.tokenBlacklist[tokenString]; ok && !now.Before(current) {
		delete(s.tokenBlacklist, tokenString)
	}
	s.blacklistMu.Unlock()
	return false
}

func (s *AuthService) pruneTokenBlacklist(now time.Time) {
	s.blacklistMu.Lock()
	for token, expiresAt := range s.tokenBlacklist {
		if !now.Before(expiresAt) {
			delete(s.tokenBlacklist, token)
		}
	}
	s.blacklistMu.Unlock()
}

func (s *AuthService) CreateUser(username, password, email string) (*model.UserDTO, error) {
	existing, err := s.userRepo.FindByUsername(username)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"username": username,
			"error":    err,
		}).Error("Create user failed: database error while checking username")
		return nil, err
	}
	if existing != nil {
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"username": username,
		}).Warn("Create user failed: username already exists")
		return nil, util.ErrUserAlreadyExists
	}

	hashedPassword, err := util.HashPassword(password)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"username": username,
			"error":    err,
		}).Error("Create user failed: password hashing error")
		return nil, err
	}

	user := &model.User{
		Username:     username,
		PasswordHash: hashedPassword,
		Email:        email,
		DisplayName:  username,
		IsActive:     true,
	}

	if err := s.userRepo.Create(user); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":   "auth",
			"username": username,
			"error":    err,
		}).Error("Create user failed: database error")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"module":   "auth",
		"user_id":  user.ID,
		"username": username,
	}).Info("User created successfully")

	dto := user.ToDTO()
	return &dto, nil
}

func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": userID,
			"error":   err,
		}).Error("Change password failed: user not found")
		return err
	}

	if !util.CheckPasswordHash(oldPassword, user.PasswordHash) {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": userID,
		}).Warn("Change password failed: invalid old password")
		return util.ErrInvalidCredentials
	}

	hashedPassword, err := util.HashPassword(newPassword)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": userID,
			"error":   err,
		}).Error("Change password failed: password hashing error")
		return err
	}

	user.PasswordHash = hashedPassword
	if err := s.userRepo.Update(user); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "auth",
			"user_id": userID,
			"error":   err,
		}).Error("Change password failed: database error")
		return err
	}

	logrus.WithFields(logrus.Fields{
		"module":  "auth",
		"user_id": userID,
	}).Info("Password changed successfully")

	return nil
}

func (s *AuthService) GetUserByID(userID uint) (*model.User, error) {
	return s.userRepo.FindByID(userID)
}

func (s *AuthService) UpdateUser(user *model.User) error {
	return s.userRepo.Update(user)
}
