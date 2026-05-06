package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_NormalFlow(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  1 * time.Second,
		ProbeInterval: 100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 初始状态应该是关闭的
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.True(t, cb.AllowRequest())

	// 记录成功请求
	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.GetState())

	// 记录失败请求
	cb.RecordFailure()
	assert.Equal(t, CircuitClosed, cb.GetState())

	// 再次记录成功请求
	cb.RecordSuccess()
	stats := cb.GetStats()
	assert.Equal(t, int64(2), stats.TotalSuccesses)
	assert.Equal(t, int64(1), stats.TotalFailures)
}

func TestCircuitBreaker_TransitionsToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  1 * time.Second,
		ProbeInterval: 100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 连续3次失败
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// 断路器应该打开
	assert.Equal(t, CircuitOpen, cb.GetState())
	assert.False(t, cb.AllowRequest())

	// 验证统计
	stats := cb.GetStats()
	assert.Equal(t, int64(3), stats.TotalFailures)
	assert.Equal(t, 3, stats.ConsecutiveFailures)
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  100 * time.Millisecond,
		ProbeInterval: 50 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	// 等待重置超时
	time.Sleep(150 * time.Millisecond)

	// 断路器应该允许探测请求（进入半开状态）
	assert.True(t, cb.AllowRequest())
	assert.Equal(t, CircuitHalfOpen, cb.GetState())
}

func TestCircuitBreaker_RecoveryFromHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  100 * time.Millisecond,
		ProbeInterval: 50 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// 等待重置超时
	time.Sleep(150 * time.Millisecond)

	// 允许探测请求
	assert.True(t, cb.AllowRequest())
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// 记录成功，恢复到关闭状态
	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestCircuitBreaker_FailureFromHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  100 * time.Millisecond,
		ProbeInterval: 50 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// 等待重置超时
	time.Sleep(150 * time.Millisecond)

	// 允许探测请求
	assert.True(t, cb.AllowRequest())

	// 记录失败，回到熔断状态
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  1 * time.Second,
		ProbeInterval: 100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	// 重置断路器
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.True(t, cb.AllowRequest())
}

func TestCircuitBreaker_Timeout(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  1 * time.Second,
		ProbeInterval: 100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 记录超时
	cb.RecordTimeout()
	cb.RecordTimeout()
	cb.RecordTimeout()

	// 超时应该导致熔断
	assert.Equal(t, CircuitOpen, cb.GetState())

	// 验证统计
	stats := cb.GetStats()
	assert.Equal(t, int64(3), stats.TotalTimeouts)
}

func TestCircuitBreaker_GetRetryAfter(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   3,
		ResetTimeout:  2 * time.Second,
		ProbeInterval: 100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 初始状态应该返回0
	assert.Equal(t, 0, cb.GetRetryAfter())

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// 应该返回剩余等待时间
	retryAfter := cb.GetRetryAfter()
	assert.True(t, retryAfter > 0 && retryAfter <= 2)

	// 等待重置超时
	time.Sleep(3 * time.Second)

	// 应该返回0
	assert.Equal(t, 0, cb.GetRetryAfter())
}

func TestCircuitBreaker_GetRemainingFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:   5,
		ResetTimeout:  1 * time.Second,
		ProbeInterval: 100 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)

	// 初始应该还有5次失败机会
	assert.Equal(t, 5, cb.GetRemainingFailures())

	// 记录2次失败
	cb.RecordFailure()
	cb.RecordFailure()

	assert.Equal(t, 3, cb.GetRemainingFailures())
}
