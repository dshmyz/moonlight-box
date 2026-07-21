package runtime

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrNotImplemented      = errors.New("not implemented")
	ErrReadOnly            = errors.New("read only")
	ErrNotMatched          = errors.New("not matched")
	ErrInvalidUpload       = errors.New("invalid upload session state")
	ErrBlocked             = errors.New("blocked by rule")
	ErrRemoteUnsupported   = errors.New("remote open unsupported")
	ErrUpstreamUnavailable = errors.New("upstream unavailable")
	// ErrCircuitOpen 表示熔断器打开时拒绝请求。
	// 与 ErrUpstreamUnavailable 的区别：
	//   - ErrUpstreamUnavailable：本次回源失败（每次独立）
	//   - ErrCircuitOpen：熔断器已积累足够失败，整个上游被判定为不可用，期间所有请求被短路
	ErrCircuitOpen = errors.New("circuit open")
)

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
