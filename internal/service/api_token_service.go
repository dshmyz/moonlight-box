package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type APITokenService struct {
	repo *repository.APITokenRepository
}

func NewAPITokenService(repo *repository.APITokenRepository) *APITokenService {
	return &APITokenService{repo: repo}
}

// CreateToken 签发新 token，返回明文 token（仅此一次）
func (s *APITokenService) CreateToken(userID uint, name string, expiresAt *time.Time) (string, *model.APIToken, error) {
	raw, err := generateToken()
	if err != nil {
		return "", nil, fmt.Errorf("生成 token 失败: %w", err)
	}

	hash := sha256Raw([]byte(raw))
	prefix := raw[:12]

	token := &model.APIToken{
		UserID:    userID,
		Name:      name,
		TokenHash: hash,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
	}

	if err := s.repo.Create(token); err != nil {
		return "", nil, fmt.Errorf("保存 token 失败: %w", err)
	}

	return raw, token, nil
}

// ValidateToken 校验 token 是否有效，返回关联的 token 记录
func (s *APITokenService) ValidateToken(rawToken string) (*model.APIToken, error) {
	// 通过前缀快速定位
	prefix := rawToken
	if len(rawToken) > 12 {
		prefix = rawToken[:12]
	}

	token, err := s.repo.FindByPrefix(prefix)
	if err != nil {
		return nil, fmt.Errorf("token 不存在")
	}

	// 完整哈希校验（恒定时间比较，避免时序侧信道）
	hash := sha256Raw([]byte(rawToken))
	if !hashesEqual(token.TokenHash, hash) {
		return nil, fmt.Errorf("token 无效")
	}

	// 检查过期
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, fmt.Errorf("token 已过期")
	}

	// 更新最后使用时间
	_ = s.repo.UpdateLastUsed(token.ID)

	return token, nil
}

// ListTokens 列出用户的所有 token
func (s *APITokenService) ListTokens(userID uint) ([]model.APIToken, error) {
	return s.repo.ListByUserID(userID)
}

// DeleteToken 撤销 token
func (s *APITokenService) DeleteToken(id, userID uint) error {
	return s.repo.Delete(id, userID)
}

func generateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mlb_" + hex.EncodeToString(b), nil
}

// sha256Raw 计算原始 SHA-256 摘要（32 字节），而非 hex 字符串。
// 原始字节可配合 subtle.ConstantTimeCompare 做恒定时间比较。
func sha256Raw(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// hashesEqual 恒定时间比较两个摘要，防御时序侧信道。
// 长度不同的输入直接返回 false（ConstantTimeCompare 长度不等即不相等，且不返回调用方长度信息）。
func hashesEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
