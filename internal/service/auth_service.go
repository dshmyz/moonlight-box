package service

import (
	"errors"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/golang-jwt/jwt/v5"
)

type AuthService struct {
	userRepo     *repository.UserRepository
	roleRepo     *repository.RoleRepository
	config       *config.AuthConfig
	tokenBlacklist map[string]bool // 简单内存黑名单，生产环境可用 Redis
}

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
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
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		roleRepo:       roleRepo,
		config:         cfg,
		tokenBlacklist: make(map[string]bool),
	}
}

func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	// 查找用户
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		if errors.Is(err, util.ErrUserNotFound) {
			return nil, util.ErrInvalidCredentials
		}
		return nil, err
	}

	// 验证密码
	if !util.CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, util.ErrInvalidCredentials
	}

	// 检查用户状态
	if !user.IsActive {
		return nil, errors.New("account is disabled")
	}

	// 获取角色列表
	roles, _ := s.roleRepo.GetUserRoles(user.ID)
	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	// 生成 Token
	accessToken, err := s.generateToken(user.ID, user.Username, roleNames, s.config.TokenExpiry)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateToken(user.ID, user.Username, roleNames, s.config.RefreshExpiry)
	if err != nil {
		return nil, err
	}

	// 更新最后登录时间
	s.userRepo.UpdateLastLogin(user.ID)

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
		if s.tokenBlacklist[tokenString] {
			return nil, errors.New("token has been revoked")
		}
		return claims, nil
	}

	return nil, util.ErrTokenInvalid
}

func (s *AuthService) Logout(tokenString string) error {
	s.tokenBlacklist[tokenString] = true
	return nil
}

func (s *AuthService) RefreshToken(refreshTokenString string) (*AuthResponse, error) {
	claims, err := s.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, err
	}

	// 将旧 Refresh Token 加入黑名单
	s.tokenBlacklist[refreshTokenString] = true

	// 生成新 Token 对
	accessToken, err := s.generateToken(user.ID, user.Username, claims.Roles, s.config.TokenExpiry)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.generateToken(user.ID, user.Username, claims.Roles, s.config.RefreshExpiry)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    s.config.TokenExpiry.Seconds(),
		User:         user.ToDTO(),
	}, nil
}

func (s *AuthService) CreateUser(username, password, email string) (*model.UserDTO, error) {
	// 检查是否已存在
	existing, _ := s.userRepo.FindByUsername(username)
	if existing != nil {
		return nil, util.ErrUserAlreadyExists
	}

	// 哈希密码
	hashedPassword, err := util.HashPassword(password)
	if err != nil {
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
		return nil, err
	}

	dto := user.ToDTO()
	return &dto, nil
}

func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}

	if !util.CheckPasswordHash(oldPassword, user.PasswordHash) {
		return util.ErrInvalidCredentials
	}

	hashedPassword, err := util.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hashedPassword
	return s.userRepo.Update(user)
}

func (s *AuthService) GetUserByID(userID uint) (*model.User, error) {
	return s.userRepo.FindByID(userID)
}
