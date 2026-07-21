package proxy

import (
	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

// CircuitBreakerAdapter 把 *proxy.CircuitBreaker 适配为 runtime.CircuitBreaker 接口。
//
// 存在原因：internal/core/runtime 不能反向依赖 internal/proxy（架构红线），
// 因此 runtime 包定义端口接口 runtime.CircuitBreaker，由 proxy 包提供具体实现并适配。
// 这是 Hexagonal Architecture 的标准做法，与 cmd/registry/runtime_init.go:335-405
// 的 blockRuleBlocker 适配器同构。
//
// nil 安全：cb 为 nil 时适配器 fail-open（AllowRequest 恒返回 true，所有 Record* 空操作），
// 与 runtime.CircuitBreakerDecorator 的 nil 语义对齐——调用方无需 nil 检查。
type CircuitBreakerAdapter struct {
	cb *CircuitBreaker
}

// NewCircuitBreakerAdapter 创建适配器。cb 可为 nil。
func NewCircuitBreakerAdapter(cb *CircuitBreaker) *CircuitBreakerAdapter {
	return &CircuitBreakerAdapter{cb: cb}
}

// Compile-time assertion: *CircuitBreakerAdapter 实现 runtime.CircuitBreaker。
var _ runtime.CircuitBreaker = (*CircuitBreakerAdapter)(nil)

func (a *CircuitBreakerAdapter) AllowRequest() bool {
	if a.cb == nil {
		// fail-open：无熔断器时放行所有请求
		return true
	}
	return a.cb.AllowRequest()
}

func (a *CircuitBreakerAdapter) RecordSuccess() {
	if a.cb == nil {
		return
	}
	a.cb.RecordSuccess()
}

func (a *CircuitBreakerAdapter) RecordFailure() {
	if a.cb == nil {
		return
	}
	a.cb.RecordFailure()
}

func (a *CircuitBreakerAdapter) RecordTimeout() {
	if a.cb == nil {
		return
	}
	a.cb.RecordTimeout()
}

func (a *CircuitBreakerAdapter) GetRetryAfter() int {
	if a.cb == nil {
		return 0
	}
	return a.cb.GetRetryAfter()
}

// CircuitBreakerLookup 提供 per-repo 的熔断器查找能力。
// HealthCheckService 实现此接口，runtime_init.go 用它为每个 ProxyRuntime 装配熔断器。
type CircuitBreakerLookup interface {
	GetOrCreateCircuitBreaker(repoID uint) runtime.CircuitBreaker
}

// Assert HealthCheckService satisfies CircuitBreakerLookup at compile time.
// 实际方法在 health_check.go：GetOrCreateCircuitBreaker 返回 *CircuitBreaker，
// 需要包装为适配器——由调用方（runtime_init.go）用 NewCircuitBreakerAdapter 包装。
// 这里不直接做接口断言，因为 HealthCheckService.GetOrCreateCircuitBreaker 返回具体类型。
