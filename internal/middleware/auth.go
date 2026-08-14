package middleware

import (
	"crypto/sha256"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

// basicAuthCache caches successful Basic Auth lookups to avoid full DB login per request.
// Key = sha256(username:password), value = cached auth result.
// CI/CD tools use Basic Auth on every request; 1min TTL avoids stale credentials.
var (
	basicAuthCache   = make(map[string]*basicAuthEntry)
	basicAuthCacheMu sync.RWMutex
	basicAuthTTL     = 1 * time.Minute
	basicAuthMaxSize = 10000
)

type basicAuthEntry struct {
	userID   uint
	username string
	roles    []string
	expires  time.Time
}

// apiTokenPrefix 是 API token 的固定前缀（见 service.generateToken），
// 用于在 Bearer 处与 JWT 分流：以该前缀开头的 token 走 API token 校验，否则走 JWT。
const apiTokenPrefix = "mlb_"

// Auth 是统一的 HTTP 鉴权中间件。
// 支持三种凭据，按优先级依次尝试：
//  1. API token（Bearer mlb_...）：通过 apiTokenSvc 校验，挂到 token 关联的用户
//  2. 用户 JWT（Bearer xxx.yyy.zzz）：原有逻辑，原样保留
//  3. Basic Auth（username:password）：原有一分钟缓存登录逻辑，原样保留
//
// apiTokenSvc 为可变参数：不传或传 nil 时退化为仅 JWT + Basic，与旧行为完全一致。
func Auth(authService *service.AuthService, apiTokenSvc ...*service.APITokenService) gin.HandlerFunc {
	hasAPITokenSvc := len(apiTokenSvc) > 0 && apiTokenSvc[0] != nil
	return func(c *gin.Context) {
		token := extractToken(c)
		if token != "" {
			// API token 专用前缀分流 —— 纯增量，不影响现有 JWT 分支
			if hasAPITokenSvc && strings.HasPrefix(token, apiTokenPrefix) {
				apiToken, err := apiTokenSvc[0].ValidateToken(token)
				if err != nil {
					response.Unauthorized(c, "invalid or expired api token")
					c.Abort()
					return
				}
				// RBAC 仅依赖 userID（见 RequirePermission → HasPermission），
				// 补齐 username/roles 仅为避免下游对空值 panic。
				c.Set("userID", apiToken.UserID)
				c.Set("username", "api-token")
				c.Set("roles", []string{})
				c.Next()
				return
			}

			claims, err := authService.ValidateToken(token)
			if err != nil {
				response.Unauthorized(c, "invalid or expired token")
				c.Abort()
				return
			}

			c.Set("userID", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("roles", claims.Roles)
			c.Next()
			return
		}

		userID, username, roles, ok := extractBasicAuth(c, authService)
		if ok {
			c.Set("userID", userID)
			c.Set("username", username)
			c.Set("roles", roles)
			c.Next()
			return
		}

		response.Unauthorized(c, "missing authorization header")
		c.Abort()
	}
}

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}

	if strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}

	return ""
}

func basicAuthCacheKey(username, password string) string {
	h := sha256.Sum256([]byte(username + ":" + password))
	return string(h[:])
}

func getBasicAuthCache(key string) (*basicAuthEntry, bool) {
	basicAuthCacheMu.RLock()
	entry, ok := basicAuthCache[key]
	basicAuthCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry, true
}

func setBasicAuthCache(key string, entry *basicAuthEntry) {
	basicAuthCacheMu.Lock()
	defer basicAuthCacheMu.Unlock()
	if len(basicAuthCache) >= basicAuthMaxSize {
		now := time.Now()
		for k, v := range basicAuthCache {
			if now.After(v.expires) {
				delete(basicAuthCache, k)
			}
		}
	}
	entry.expires = time.Now().Add(basicAuthTTL)
	basicAuthCache[key] = entry
}

func extractBasicAuth(c *gin.Context, authService *service.AuthService) (uint, string, []string, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return 0, "", nil, false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return 0, "", nil, false
	}

	username, password, ok := parseBasicAuth(c, parts[1])
	if !ok {
		return 0, "", nil, false
	}

	// Check cache first
	cacheKey := basicAuthCacheKey(username, password)
	if entry, hit := getBasicAuthCache(cacheKey); hit {
		return entry.userID, entry.username, entry.roles, true
	}

	claims, err := authService.Login(&service.LoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return 0, "", nil, false
	}

	setBasicAuthCache(cacheKey, &basicAuthEntry{
		userID:   claims.User.ID,
		username: claims.User.Username,
		roles:    claims.User.Roles,
	})

	return claims.User.ID, claims.User.Username, claims.User.Roles, true
}

func parseBasicAuth(c *gin.Context, auth string) (username, password string, ok bool) {
	return c.Request.BasicAuth()
}
