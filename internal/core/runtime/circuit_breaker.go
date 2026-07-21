package runtime

import (
	"context"
	"errors"
	"io"
	"net"
)

// CircuitBreaker 是熔断器的抽象接口（端口）。
// 由 internal/proxy 包提供具体实现并通过适配器注入到 runtime 层，
// 避免 internal/core/runtime 反向依赖 internal/proxy（Hexagonal Architecture）。
//
// 方法语义与 internal/proxy.CircuitBreaker 一致：
//   - AllowRequest: 闭→true；开→检查 resetTimeout 后转 half_open 放一个探测；half_open→只允许一个探测
//   - RecordSuccess: 重置连续失败计数，half_open→closed
//   - RecordFailure: 连续达 MaxFailures 转 open
//   - RecordTimeout: 同 RecordFailure，但额外计 totalTimeouts
//   - GetRetryAfter: 返回建议客户端等待的秒数（open 状态才有意义，其他返回 0）
type CircuitBreaker interface {
	AllowRequest() bool
	RecordSuccess()
	RecordFailure()
	RecordTimeout()
	GetRetryAfter() int
}

// CircuitBreakerDecorator 包装 RemoteClient，在回源边界应用熔断策略。
//
// 设计权衡：
//   - 错误分类在 RemoteClient 边界（最接近上游的位置），保证 ProxyRuntime 业务逻辑不受污染
//   - 上游正常应答 404（ErrNotFound / RemoteMetadata.Exists=false）不计入失败——上游在工作
//   - 网络错误、5xx、timeout 计入失败——上游故障
//   - Open 是不透明流：只按传输层 err 判定，HTTP 5xx 透传给 Plugin（不视为传输失败）
//   - nil CircuitBreaker 时装饰器完全透传，便于测试和无熔断器场景向后兼容
type CircuitBreakerDecorator struct {
	inner RemoteClient
	cb    CircuitBreaker
}

// NewCircuitBreakerDecorator 创建装饰器。cb 为 nil 时返回的装饰器完全透传 inner，
// 不进行任何熔断检查——调用方可以无条件调用，无需 nil 检查。
func NewCircuitBreakerDecorator(inner RemoteClient, cb CircuitBreaker) *CircuitBreakerDecorator {
	return &CircuitBreakerDecorator{inner: inner, cb: cb}
}

// FetchMetadata 在熔断关闭时透传 inner，根据结果记录熔断状态。
//
// 错误分类：
//   - err == nil && meta.Exists: RecordSuccess（上游正常响应）
//   - err == nil && !meta.Exists: 不记录（上游正常应答 404，不算失败也不算成功）
//   - err != nil 且是 timeout: RecordTimeout
//   - err != nil 其他: RecordFailure
func (d *CircuitBreakerDecorator) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	if d.cb == nil {
		return d.inner.FetchMetadata(ctx, key)
	}
	if !d.cb.AllowRequest() {
		return nil, NewCircuitOpenError(d.cb.GetRetryAfter())
	}
	meta, err := d.inner.FetchMetadata(ctx, key)
	if err != nil {
		d.recordFailure(err)
	} else if meta != nil && meta.Exists {
		d.cb.RecordSuccess()
	}
	// err == nil && meta.Exists == false：上游正常应答 404，不记录
	return meta, err
}

// FetchBlob 在熔断关闭时透传 inner，根据结果记录熔断状态。
//
// 错误分类：
//   - err == nil: RecordSuccess
//   - errors.Is(err, ErrNotFound): 不记录（上游正常应答 404）
//   - err 是 timeout: RecordTimeout
//   - 其他 err: RecordFailure
func (d *CircuitBreakerDecorator) FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	if d.cb == nil {
		return d.inner.FetchBlob(ctx, key)
	}
	if !d.cb.AllowRequest() {
		return nil, NewCircuitOpenError(d.cb.GetRetryAfter())
	}
	blob, err := d.inner.FetchBlob(ctx, key)
	if err == nil {
		d.cb.RecordSuccess()
		return blob, nil
	}
	// ErrNotFound 是上游正常应答 404，不计入熔断失败
	if errors.Is(err, ErrNotFound) {
		return blob, err
	}
	d.recordFailure(err)
	return blob, err
}

// Open 在熔断关闭时透传 inner，根据传输层错误记录熔断状态。
//
// 设计权衡：Open 返回的是不透明响应流，HTTP 状态码（含 5xx）透传给 Plugin 自行处理。
// 因此只按传输层 err 判定（err != nil 视为传输失败），不检查响应状态码。
// 这与 FetchMetadata/FetchBlob 的语义不同——后两者 5xx 是 error。
func (d *CircuitBreakerDecorator) Open(ctx context.Context, request RemoteRequest) (*RemoteResponse, error) {
	if d.cb == nil {
		return d.inner.Open(ctx, request)
	}
	if !d.cb.AllowRequest() {
		return nil, NewCircuitOpenError(d.cb.GetRetryAfter())
	}
	resp, err := d.inner.Open(ctx, request)
	if err == nil {
		d.cb.RecordSuccess()
		return resp, nil
	}
	d.recordFailure(err)
	return resp, err
}

// recordFailure 统一处理 err != nil 的失败记录：区分 timeout 和普通失败。
// 一次失败只调一次 Record*，避免重复计数。
func (d *CircuitBreakerDecorator) recordFailure(err error) {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		d.cb.RecordTimeout()
		return
	}
	d.cb.RecordFailure()
}
