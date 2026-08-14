package service

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestHashesEqualConstantTime 锁定哈希比较使用恒定时间比较（ciphers/subtle），
// 避免 TokenHash 走字符串 != 短路比较带来的时序侧信道。
// 比较函数须对任意长度、任意字节差异返回正确布尔，且内部应为恒定时间。
func TestHashesEqualConstantTime(t *testing.T) {
	a := sha256Raw([]byte("token-a-1234567890abcdef"))
	b := sha256Raw([]byte("token-a-1234567890abcdef")) // same
	c := sha256Raw([]byte("token-b-1234567890abcdef")) // differs in first bytes
	d := append([]byte{1, 2, 3}, sha256Raw([]byte("x"))...) // length differs

	if !hashesEqual(a, b) {
		t.Fatal("hashesEqual(equal) = false, want true")
	}
	if hashesEqual(a, c) {
		t.Fatal("hashesEqual(different) = true, want false")
	}
	if hashesEqual(a, d) {
		t.Fatal("hashesEqual(diff-length) = true, want false")
	}
}

func TestSHA256RawIsRawBytesNotHex(t *testing.T) {
	raw := sha256Raw([]byte("anything"))
	// 必须是 32 字节原始摘要，而非 64 字符 hex 文本。
	if len(raw) != 32 {
		t.Fatalf("sha256Raw len = %d, want 32 (raw digest, not 64-char hex)", len(raw))
	}
}

// newAPITokenTestService 构造内存 DB + APITokenService，并向 userID 签发一个 token。
func newAPITokenTestService(t *testing.T) (*APITokenService, *model.APIToken, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.APIToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewAPITokenService(repository.NewAPITokenRepository(db))
	raw, info, err := svc.CreateToken(1, "ci", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return svc, info, raw
}

// TestValidateToken_Valid 验证签发的明文 token 能通过 ValidateToken，且带 mlb_ 前缀、归属正确的 UserID。
func TestValidateToken_Valid(t *testing.T) {
	svc, info, raw := newAPITokenTestService(t)

	got, err := svc.ValidateToken(raw)
	if err != nil {
		t.Fatalf("ValidateToken(valid) error: %v", err)
	}
	if got.UserID != 1 {
		t.Errorf("UserID = %d, want 1", got.UserID)
	}
	if got.ID != info.ID {
		t.Errorf("token ID = %d, want %d (prefix定位到同一行)", got.ID, info.ID)
	}
	if len(raw) <= 4 || raw[:4] != "mlb_" {
		t.Errorf("token 缺少 mlb_ 前缀: %q", raw)
	}
}

// TestValidateToken_WrongToken 验证任意未签发 token 均返回错误。
func TestValidateToken_WrongToken(t *testing.T) {
	svc, _, _ := newAPITokenTestService(t)

	if _, err := svc.ValidateToken("mlb_deadbeefdeadbeefdeadbeef"); err == nil {
		t.Fatal("ValidateToken(unknown) = nil error, want error")
	}
}

// TestValidateToken_Expired 验证过期 token 被拒绝。
func TestValidateToken_Expired(t *testing.T) {
	svc, _, _ := newAPITokenTestService(t)

	past := time.Now().Add(-time.Hour)
	raw, _, err := svc.CreateToken(1, "expired", &past)
	if err != nil {
		t.Fatalf("create expired token: %v", err)
	}

	if _, err := svc.ValidateToken(raw); err == nil {
		t.Fatal("ValidateToken(expired) = nil error, want error")
	}
}

// TestValidateToken_CaseDiffers 验证 ValidateToken 严格校验完整哈希（非仅前缀），
// 同前缀不同后缀应被拒绝（恒定时间比较完整 hash）。
func TestValidateToken_CaseDiffers(t *testing.T) {
	svc, _, raw := newAPITokenTestService(t)

	// 同前缀、篡改剩余字符："mlb_" + 8 位前缀 + 48 位 hex
	forged := raw[:12] + "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	if _, err := svc.ValidateToken(forged); err == nil {
		t.Fatal("ValidateToken(forged) = nil error, want error (必须完整哈希校验，不能只对前缀)")
	}
}