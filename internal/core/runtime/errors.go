package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrNotImplemented      = errors.New("not implemented")
	ErrReadOnly            = errors.New("read only")
	ErrNotMatched          = errors.New("not matched")
	ErrInvalidUpload       = errors.New("invalid upload session state")
	ErrBlocked             = errors.New("blocked by rule")
	ErrRemoteUnsupported   = errors.New("remote open unsupported")
	ErrUpstreamUnavailable = errors.New("upstream unavailable")
	// ErrUpstreamTimeout 表示回源请求超时。
	// 与 ErrUpstreamUnavailable 的区别：
	//   - ErrUpstreamUnavailable：上游返回错误响应（如 5xx）或网络不可达
	//   - ErrUpstreamTimeout：请求已发出但等待响应超时（context deadline / net timeout）
	ErrUpstreamTimeout = errors.New("upstream timeout")
	// ErrCircuitOpen 表示熔断器打开时拒绝请求。
	// 与 ErrUpstreamUnavailable 的区别：
	//   - ErrUpstreamUnavailable：本次回源失败（每次独立）
	//   - ErrCircuitOpen：熔断器已积累足够失败，整个上游被判定为不可用，期间所有请求被短路
	ErrCircuitOpen = errors.New("circuit open")
)

// IsUpstreamTimeout 判断错误是否为上游超时。
// 覆盖三种场景：ErrUpstreamTimeout 哨兵、net.Error.Timeout()、context.DeadlineExceeded。
func IsUpstreamTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUpstreamTimeout) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// BlockedError preserves a safe, user-facing rule reason while remaining
// compatible with callers that use errors.Is(err, ErrBlocked).
type BlockedError struct {
	Reason string
}

func (e *BlockedError) Error() string { return ErrBlocked.Error() }
func (e *BlockedError) Unwrap() error { return ErrBlocked }

func NewBlockedError(reason string) error {
	if reason == "" {
		return ErrBlocked
	}
	return &BlockedError{Reason: reason}
}

func BlockReason(err error) string {
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		return blocked.Reason
	}
	return ""
}

// CircuitOpenError 表示熔断器打开时拒绝请求，携带 RetryAfter（秒）信息
// 供 HTTP 层透传给客户端（503 + Retry-After 头）。
// 与 BlockedError 同样的模式：errors.Is(err, ErrCircuitOpen) 成立，
// 同时可通过 errors.As 提取 RetryAfter。
type CircuitOpenError struct {
	RetryAfter int // 建议客户端等待的秒数（0 表示立即重试可能成功，如即将转 half_open）
}

func (e *CircuitOpenError) Error() string { return ErrCircuitOpen.Error() }
func (e *CircuitOpenError) Unwrap() error { return ErrCircuitOpen }

// NewCircuitOpenError 构造一个熔断打开错误。
// retryAfter 为 0 时仍返回 *CircuitOpenError（区别于直接返回 ErrCircuitOpen），
// 因为 errors.As 检查的是类型而非值。
func NewCircuitOpenError(retryAfter int) error {
	return &CircuitOpenError{RetryAfter: retryAfter}
}

// CircuitRetryAfter 从错误中提取 RetryAfter 秒数；非 CircuitOpenError 返回 0。
func CircuitRetryAfter(err error) int {
	var coe *CircuitOpenError
	if errors.As(err, &coe) {
		return coe.RetryAfter
	}
	return 0
}

// UpstreamError 携带结构化上游错误信息，供 HTTP 层返回 JSON 响应体。
// 与 CircuitOpenError 同样的模式：errors.Is(err, ErrUpstreamTimeout/ErrUpstreamUnavailable) 成立，
// 同时可通过 errors.As 提取 RemoteURL、RetryAfter 等字段。
type UpstreamError struct {
	Cause      string // "timeout" / "unavailable"
	RemoteURL  string // 上游地址
	RetryAfter int    // 建议重试秒数（0 表示不建议重试）
	Err        error  // 原始错误（保留完整错误链）
}

func (e *UpstreamError) Error() string {
	if e.Cause == "timeout" {
		return ErrUpstreamTimeout.Error()
	}
	return ErrUpstreamUnavailable.Error()
}

func (e *UpstreamError) Unwrap() error { return e.Err }

// NewUpstreamTimeoutError 构造一个上游超时错误，携带上游地址和建议重试时间。
// 内部用 fmt.Errorf 包装原始错误与 ErrUpstreamTimeout 哨兵，保证 errors.Is 对两者都成立。
func NewUpstreamTimeoutError(remoteURL string, retryAfter int, err error) error {
	return &UpstreamError{
		Cause:      "timeout",
		RemoteURL:  remoteURL,
		RetryAfter: retryAfter,
		Err:        fmt.Errorf("%w: %w", ErrUpstreamTimeout, err),
	}
}

// NewUpstreamUnavailableError 构造一个上游不可达错误，携带上游地址。
// 内部用 fmt.Errorf 包装原始错误与 ErrUpstreamUnavailable 哨兵，保证 errors.Is 对两者都成立。
func NewUpstreamUnavailableError(remoteURL string, err error) error {
	return &UpstreamError{
		Cause:     "unavailable",
		RemoteURL: remoteURL,
		Err:       fmt.Errorf("%w: %w", ErrUpstreamUnavailable, err),
	}
}
