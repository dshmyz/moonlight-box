package runtime

import (
	"testing"
)

// mockBlockerWithAttrs 是一个实现了 IsBlocked + BlockReason + IsBlockedWithAttrs 的 mock，
// 用于验证 PackageBlocker 接口契约。
type mockBlockerWithAttrs struct {
	blocked bool
	reason  string
}

func (m *mockBlockerWithAttrs) IsBlocked(packageType, packageName, version string) bool {
	return m.blocked
}

func (m *mockBlockerWithAttrs) BlockReason(packageType, packageName, version string) string {
	return m.reason
}

// IsBlockedWithAttrs 带元数据的第二层阻断检查，返回是否阻断及原因。
func (m *mockBlockerWithAttrs) IsBlockedWithAttrs(packageType, packageName, version string, attrs map[string]interface{}) (bool, string) {
	return m.blocked, m.reason
}

func (m *mockBlockerWithAttrs) IsBlockedByPath(string, string) bool     { return false }
func (m *mockBlockerWithAttrs) BlockReasonByPath(string, string) string { return "" }

// TestPackageBlockerInterface_HasIsBlockedWithAttrs 验证：
// 实现了 IsBlocked + BlockReason + IsBlockedWithAttrs 三个方法的对象，
// 必须能被赋值给 PackageBlocker 接口变量。
func TestPackageBlockerInterface_HasIsBlockedWithAttrs(t *testing.T) {
	var blocker PackageBlocker = &mockBlockerWithAttrs{blocked: true, reason: "license violation"}

	// 验证 IsBlockedWithAttrs 可通过接口调用
	blocked, reason := blocker.IsBlockedWithAttrs("npm", "lodash", "4.17.20", map[string]interface{}{
		"license": "GPL-3.0",
	})
	if !blocked {
		t.Error("期望 blocked=true，实际 false")
	}
	if reason != "license violation" {
		t.Errorf("期望 reason=license violation，实际 %q", reason)
	}
}

// TestPackageBlockerInterface_IsBlockedWithAttrsSignature 验证方法签名：
// IsBlockedWithAttrs(packageType, packageName, version string, attrs map[string]interface{}) (blocked bool, reason string)
func TestPackageBlockerInterface_IsBlockedWithAttrsSignature(t *testing.T) {
	var blocker PackageBlocker = &mockBlockerWithAttrs{}

	// 调用时传入 string, string, string, map[string]interface{}
	// 返回 (bool, string)
	blocked, reason := blocker.IsBlockedWithAttrs("maven", "org.foo:bar", "1.0.0", map[string]interface{}{})

	// 仅验证类型契约（值本身由 mock 决定，这里都是零值）
	_ = blocked
	_ = reason
}
