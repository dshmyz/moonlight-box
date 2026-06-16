package service

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
)

func TestAuthServiceValidateTokenPrunesExpiredBlacklistedToken(t *testing.T) {
	svc := NewAuthService(nil, nil, &config.AuthConfig{
		JWTSecret:     "test-secret",
		TokenExpiry:   time.Minute,
		RefreshExpiry: time.Hour,
	}, nil)
	token, err := svc.generateToken(1, "alice", nil, -time.Hour)
	if err != nil {
		t.Fatalf("generate expired token: %v", err)
	}
	if err := svc.Logout(token); err != nil {
		t.Fatalf("logout token: %v", err)
	}

	if _, err := svc.ValidateToken(token); err == nil {
		t.Fatal("expected expired token to be invalid")
	}
	if len(svc.tokenBlacklist) != 0 {
		t.Fatalf("expected expired blacklisted token to be pruned, got %d entries", len(svc.tokenBlacklist))
	}
}
