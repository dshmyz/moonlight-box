package npm

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/golang-jwt/jwt/v5"
)

// handleWhoami 处理 GET /-/whoami
// 返回当前认证用户的用户名。
//
// API 参考: https://github.com/npm/registry/blob/main/docs/REGISTRY-API.md
func (p *NpmPlugin) handleWhoami(ctx *runtime.RequestContext) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	// 从 Authorization 头提取 token
	token := extractBearerToken(ctx.Request)
	if token == "" {
		http.Error(ctx.Writer, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	// 解析 JWT token 获取用户名
	username, err := extractUsernameFromToken(token)
	if err != nil {
		http.Error(ctx.Writer, "unauthorized", http.StatusUnauthorized)
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	// npm CLI 期望响应格式为 {"username": "xxx"}
	json.NewEncoder(ctx.Writer).Encode(map[string]string{"username": username})
	return nil
}

// extractBearerToken 从 Authorization 头中提取 Bearer token。
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// extractUsernameFromToken 从 JWT token 中提取用户名。
// Moonlight Box 的 JWT token claims 中包含 "uname" 字段。
func extractUsernameFromToken(tokenString string) (string, error) {
	// 使用无验证的解析来提取 claims（认证已在 middleware 中完成）
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &jwt.MapClaims{})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(*jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	// Moonlight Box JWT claims: {"uid":1,"uname":"admin","roles":["admin"],...}
	if uname, ok := (*claims)["uname"].(string); ok && uname != "" {
		return uname, nil
	}

	return "", errors.New("username not found in token")
}
