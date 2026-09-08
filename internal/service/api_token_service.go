package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

const (
	// apiTokenCacheTTL 有效 token 校验结果的缓存时长，与 basic auth 缓存对齐。
	apiTokenCacheTTL = time.Minute
	// apiTokenLastUsedThrottle last_used_at 落库节流间隔：CI/CD 每个请求都带 token，
	// 不节流的话每次请求多一条 UPDATE（纯写放大）。
	apiTokenLastUsedThrottle = 5 * time.Minute
)

// apiTokenCacheEntry 缓存的有效 token 及其完整摘要。
// 命中时仍需对摘要做恒定时间比较：系统按 Prefix 前缀定位（FindByPrefix 返回首条），
// 极端前缀碰撞下缓存直接返回会造成误通过，摘要校验保持与查库路径完全一致的语义。
type apiTokenCacheEntry struct {
	token  *model.APIToken
	digest []byte
}

type APITokenService struct {
	repo  *repository.APITokenRepository
	cache *cache.MemoryCache // 有效 token 校验结果缓存（key: token Prefix）
}

func NewAPITokenService(repo *repository.APITokenRepository) *APITokenService {
	return &APITokenService{repo: repo, cache: cache.NewMemoryCache()}
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

// ValidateToken 校验 token 是否有效，返回关联的 token 记录。
// 校验结果缓存于 MemoryCache（TTL apiTokenCacheTTL，与 basic auth 对齐），命中免查库；
// last_used_at 按节流间隔落库，避免 CI/CD 高频请求时的写放大。
// 撤销（DeleteToken）立即失效；直接改库的撤销最迟 apiTokenCacheTTL 后生效。
func (s *APITokenService) ValidateToken(rawToken string) (*model.APIToken, error) {
	digest := sha256Raw([]byte(rawToken))
	now := time.Now()

	// 通过前缀快速定位（与 FindByPrefix 同一语义）
	prefix := rawToken
	if len(rawToken) > 12 {
		prefix = rawToken[:12]
	}

	// 命中有效缓存：摘要恒定时间比较通过即免查库返回
	if v, ok := s.cache.Get(prefix); ok {
		if entry, ok := v.(*apiTokenCacheEntry); ok && hashesEqual(entry.digest, digest) {
			if entry.token.ExpiresAt == nil || now.Before(*entry.token.ExpiresAt) {
				return entry.token, nil
			}
		}
	}

	token, err := s.repo.FindByPrefix(prefix)
	if err != nil {
		return nil, fmt.Errorf("token 不存在")
	}

	// 完整哈希校验（恒定时间比较，避免时序侧信道）
	if !hashesEqual(token.TokenHash, digest) {
		return nil, fmt.Errorf("token 无效")
	}

	// 检查过期
	if token.ExpiresAt != nil && now.After(*token.ExpiresAt) {
		return nil, fmt.Errorf("token 已过期")
	}

	// 节流更新最后使用时间：距上次落库超过阈值才写
	if token.LastUsed == nil || now.Sub(*token.LastUsed) > apiTokenLastUsedThrottle {
		_ = s.repo.UpdateLastUsed(token.ID)
		token.LastUsed = &now
	}

	cached := *token
	s.cache.Set(prefix, &apiTokenCacheEntry{token: &cached, digest: digest}, apiTokenCacheTTL)
	return token, nil
}

// ListTokens 列出用户的所有 token
func (s *APITokenService) ListTokens(userID uint) ([]model.APIToken, error) {
	return s.repo.ListByUserID(userID)
}

// DeleteToken 撤销 token，并清除其校验缓存（按 Prefix 定位，与查库定位语义一致）。
func (s *APITokenService) DeleteToken(id, userID uint) error {
	if token, err := s.repo.FindByIDAndUserID(id, userID); err == nil {
		s.cache.Delete(token.Prefix)
	}
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
