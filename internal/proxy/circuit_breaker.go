package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// CircuitState 断路器状态
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"    // 正常状态，允许请求通过
	CircuitOpen     CircuitState = "open"      // 熔断状态，拒绝请求
	CircuitHalfOpen CircuitState = "half_open" // 半开状态，允许探测请求
)

// CircuitBreaker 断路器实现
type CircuitBreaker struct {
	mu sync.RWMutex

	// 状态配置
	maxFailures   int           // 最大失败次数，超过后熔断
	probeInterval time.Duration // 熔断后探测间隔
	resetTimeout  time.Duration // 熔断后多久进入半开状态

	// 运行时状态
	state                 CircuitState
	failureCount          int
	successCount          int
	lastFailureTime       time.Time
	lastSuccessTime       time.Time
	lastStateChange       time.Time
	halfOpenProbeInFlight bool
	consecutiveFailures   int // 连续失败次数

	// 统计信息
	totalRequests  int64
	totalFailures  int64
	totalSuccesses int64
	totalTimeouts  int64
}

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig struct {
	MaxFailures   int           `json:"max_failures"`   // 触发熔断的最大失败次数
	ResetTimeout  time.Duration `json:"reset_timeout"`  // 熔断后多久进入半开状态
	ProbeInterval time.Duration `json:"probe_interval"` // 半开状态下的探测间隔
}

// DefaultCircuitBreakerConfig 默认断路器配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:   5,                // 连续5次失败后熔断
		ResetTimeout:  60 * time.Second, // 熔断60秒后进入半开状态
		ProbeInterval: 10 * time.Second, // 半开状态下每10秒探测一次
	}
}

// NewCircuitBreaker 创建断路器
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:           CircuitClosed,
		maxFailures:     config.MaxFailures,
		resetTimeout:    config.ResetTimeout,
		probeInterval:   config.ProbeInterval,
		lastStateChange: time.Now(),
	}
}

// AllowRequest 检查是否允许请求通过
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		// 正常状态，允许请求通过
		return true

	case CircuitOpen:
		// 熔断状态，检查是否已过重置超时
		if time.Since(cb.lastStateChange) > cb.resetTimeout {
			cb.transitionTo(CircuitHalfOpen)
			cb.halfOpenProbeInFlight = true
			return true
		}
		return false

	case CircuitHalfOpen:
		// 半开状态一次只允许一个探测请求通过，避免探测风暴。
		if cb.halfOpenProbeInFlight {
			return false
		}
		if time.Since(cb.lastStateChange) >= cb.probeInterval {
			cb.halfOpenProbeInFlight = true
			cb.lastStateChange = time.Now()
			return true
		}
		return false

	default:
		return false
	}
}

// RecordSuccess 记录成功请求
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.totalSuccesses++
	cb.successCount++
	cb.lastSuccessTime = time.Now()
	cb.consecutiveFailures = 0 // 重置连续失败计数
	cb.halfOpenProbeInFlight = false

	// 如果在半开状态，成功则恢复到关闭状态
	if cb.state == CircuitHalfOpen {
		cb.transitionTo(CircuitClosed)
	}
}

// RecordFailure 记录失败请求
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.totalFailures++
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	cb.consecutiveFailures++
	cb.halfOpenProbeInFlight = false

	// 在半开状态下，任何失败都立即回到熔断状态
	if cb.state == CircuitHalfOpen {
		cb.transitionTo(CircuitOpen)
		return
	}

	// 检查是否需要熔断
	if cb.consecutiveFailures >= cb.maxFailures && cb.state == CircuitClosed {
		cb.transitionTo(CircuitOpen)
	}
}

// RecordTimeout 记录超时请求（特殊类型的失败）
func (cb *CircuitBreaker) RecordTimeout() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.totalFailures++
	cb.totalTimeouts++
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	cb.consecutiveFailures++
	cb.halfOpenProbeInFlight = false

	if cb.state == CircuitHalfOpen {
		cb.transitionTo(CircuitOpen)
		return
	}

	// 超时也计入失败次数
	if cb.consecutiveFailures >= cb.maxFailures && cb.state == CircuitClosed {
		cb.transitionTo(CircuitOpen)
	}
}

// GetState 获取断路器当前状态
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// 检查是否应该从 Open 转换到 HalfOpen
	if cb.state == CircuitOpen && time.Since(cb.lastStateChange) > cb.resetTimeout {
		return CircuitHalfOpen
	}
	return cb.state
}

// GetStats 获取断路器统计信息
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:               cb.state,
		TotalRequests:       cb.totalRequests,
		TotalSuccesses:      cb.totalSuccesses,
		TotalFailures:       cb.totalFailures,
		TotalTimeouts:       cb.totalTimeouts,
		ConsecutiveFailures: cb.consecutiveFailures,
		LastFailureTime:     cb.lastFailureTime,
		LastSuccessTime:     cb.lastSuccessTime,
		LastStateChange:     cb.lastStateChange,
	}
}

// Reset 重置断路器状态
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionTo(CircuitClosed)
	cb.failureCount = 0
	cb.successCount = 0
	cb.consecutiveFailures = 0
	cb.halfOpenProbeInFlight = false
}

// transitionTo 内部状态转换方法（必须在持有锁的情况下调用）
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	if cb.state != newState {
		oldState := cb.state
		cb.state = newState
		cb.lastStateChange = time.Now()
		if newState != CircuitHalfOpen {
			cb.halfOpenProbeInFlight = false
		}
		logrus.WithFields(logrus.Fields{
			"module":    "circuit_breaker",
			"old_state": string(oldState),
			"new_state": string(newState),
		}).Info("Circuit breaker state changed")

		if newState == CircuitOpen {
			logrus.WithFields(logrus.Fields{
				"module":               "circuit_breaker",
				"consecutive_failures": cb.consecutiveFailures,
				"reset_timeout":        cb.resetTimeout.Seconds(),
			}).Warn("Circuit breaker opened, requests will be rejected")
		} else if newState == CircuitClosed {
			logrus.WithFields(logrus.Fields{
				"module": "circuit_breaker",
			}).Info("Circuit breaker closed, requests allowed")
		} else if newState == CircuitHalfOpen {
			logrus.WithFields(logrus.Fields{
				"module": "circuit_breaker",
			}).Info("Circuit breaker half-open, allowing probe request")
		}
	}
}

// CircuitBreakerStats 断路器统计信息
type CircuitBreakerStats struct {
	State               CircuitState `json:"state"`
	TotalRequests       int64        `json:"total_requests"`
	TotalSuccesses      int64        `json:"total_successes"`
	TotalFailures       int64        `json:"total_failures"`
	TotalTimeouts       int64        `json:"total_timeouts"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	LastFailureTime     time.Time    `json:"last_failure_time"`
	LastSuccessTime     time.Time    `json:"last_success_time"`
	LastStateChange     time.Time    `json:"last_state_change"`
}

// String 实现 Stringer 接口
func (s CircuitState) String() string {
	return string(s)
}

// GetRemainingFailures 获取距离熔断还有多少次失败
func (cb *CircuitBreaker) GetRemainingFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	remaining := cb.maxFailures - cb.consecutiveFailures
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetRetryAfter 获取熔断后多久可以重试（秒）
func (cb *CircuitBreaker) GetRetryAfter() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state != CircuitOpen {
		return 0
	}

	elapsed := time.Since(cb.lastStateChange)
	remaining := cb.resetTimeout - elapsed
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Seconds())
}

// String 返回人类可读的状态描述
func (s CircuitBreakerStats) String() string {
	return fmt.Sprintf("CircuitBreaker{state=%s, failures=%d/%d, requests=%d}",
		s.State, s.ConsecutiveFailures, s.ConsecutiveFailures+1, s.TotalRequests)
}
