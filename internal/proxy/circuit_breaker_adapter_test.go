package proxy

import (
	"testing"
)

// TestCircuitBreakerAdapterForwardsAllCalls 验证适配器把 runtime.CircuitBreaker 接口
// 的每个方法调用正确转发到内层 *proxy.CircuitBreaker。
// 适配器本身无业务逻辑，只做接口转换；测试通过观察内层 cb 的状态变化来验证转发。
func TestCircuitBreakerAdapterForwardsAllCalls(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	adapter := NewCircuitBreakerAdapter(cb)

	// 初始状态：closed，AllowRequest=true
	if !adapter.AllowRequest() {
		t.Fatal("AllowRequest should be true when circuit closed")
	}

	// 触发 MaxFailures 次失败，应该熔断打开
	for i := 0; i < DefaultCircuitBreakerConfig().MaxFailures; i++ {
		adapter.RecordFailure()
	}
	if adapter.AllowRequest() {
		t.Fatal("AllowRequest should be false after max failures reached")
	}

	// GetRetryAfter 应返回正数（resetTimeout 秒）
	if ra := adapter.GetRetryAfter(); ra <= 0 {
		t.Fatalf("GetRetryAfter = %d, want > 0 when circuit open", ra)
	}

	// RecordTimeout 也应被转发（不 panic，状态保持 open）
	adapter.RecordTimeout()
	if adapter.AllowRequest() {
		t.Fatal("AllowRequest should still be false after RecordTimeout in open state")
	}

	// RecordSuccess 应重置连续失败计数（但 open 状态需要等 resetTimeout 才转 half_open）
	// 这里只验证不 panic
	adapter.RecordSuccess()
}

// TestCircuitBreakerAdapterNilSafe 验证 cb 为 nil 时不 panic。
// 这与 runtime.CircuitBreakerDecorator 的 nil 安全语义对齐：
// 调用方可以无条件使用适配器，无需 nil 检查。
func TestCircuitBreakerAdapterNilSafe(t *testing.T) {
	adapter := NewCircuitBreakerAdapter(nil)

	// 不应 panic
	if !adapter.AllowRequest() {
		t.Fatal("nil adapter should allow all requests (fail-open)")
	}
	adapter.RecordSuccess()
	adapter.RecordFailure()
	adapter.RecordTimeout()
	if ra := adapter.GetRetryAfter(); ra != 0 {
		t.Fatalf("nil adapter GetRetryAfter = %d, want 0", ra)
	}
}
