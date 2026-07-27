package npm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/golang-jwt/jwt/v5"
)

// TestWhoamiSuccess 测试成功获取用户名
func TestWhoamiSuccess(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	// 创建一个测试 JWT token
	token := createTestToken("admin")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/whoami",
	}

	err := p.handleWhoami(ctx)
	if err != nil {
		t.Fatalf("handleWhoami returned error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["username"] != "admin" {
		t.Errorf("expected username admin, got %s", resp["username"])
	}
}

// TestWhoamiNoAuth 测试无认证头
func TestWhoamiNoAuth(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/whoami", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/whoami",
	}

	err := p.handleWhoami(ctx)
	if err != nil {
		t.Fatalf("handleWhoami returned error: %v", err)
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

// TestWhoamiInvalidToken 测试无效 token
func TestWhoamiInvalidToken(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/whoami", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/whoami",
	}

	err := p.handleWhoami(ctx)
	if err != nil {
		t.Fatalf("handleWhoami returned error: %v", err)
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

// TestWhoamiMethodNotAllowed 测试非 GET 方法
func TestWhoamiMethodNotAllowed(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/-/whoami", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/whoami",
	}

	err := p.handleWhoami(ctx)
	if err == nil {
		t.Fatal("expected error for POST method")
	}
}

// TestExtractBearerToken 测试 token 提取
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		auth     string
		expected string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"lowercase bearer", "bearer abc123", "abc123"},
		{"no bearer prefix", "abc123", ""},
		{"empty", "", ""},
		{"basic auth", "Basic dXNlcjpwYXNz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tt.auth)
			result := extractBearerToken(req)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// ========== Helpers ==========

func createTestToken(username string) string {
	claims := jwt.MapClaims{
		"uid":   1,
		"uname": username,
		"roles": []string{"admin"},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}
