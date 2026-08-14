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

// CircuitBreakerFetcherDecorator 包装 RemoteFetcher，在 metadata 回源边界应用熔断策略。
//
// 它与 CircuitBreakerDecorator 对称：后者守护 RemoteClient（GetArtifact 的下载路径），
// 本装饰器守护 RemoteFetcher（QueryArtifacts 的 metadata 回源路径）——两条回源路径由此获得
// 一致的熔断保护，否则 QueryArtifacts 的 FetchRemote 会绕过熔断器，慢上游上任何失败的请求
// 都不会触发熔断，新请求持续堆积。
//
// 错误分类与 CircuitBreakerDecorator 保持一致：
//   - err == nil: RecordSuccess（上游正常响应）
//   - IsUpstreamTimeout(err): RecordTimeout（上游慢/挂起）
//   - 其他 err != nil: RecordFailure（上游故障）
//   - nil CircuitBreaker 时装饰器完全透传，便于测试和无熔断器场景向后兼容
type CircuitBreakerFetcherDecorator struct {
	inner RemoteFetcher
	cb    CircuitBreaker
}

// NewCircuitBreakerFetcherDecorator 创建装饰器。inner 或 cb 为 nil 时返回的装饰器
// 直接透传 inner，不进行任何熔断检查——调用方可以无条件调用，无需 nil 检查。
func NewCircuitBreakerFetcherDecorator(inner RemoteFetcher, cb CircuitBreaker) *CircuitBreakerFetcherDecorator {
	return &CircuitBreakerFetcherDecorator{inner: inner, cb: cb}
}

// FetchRemote 在熔断关闭时透传 inner，根据结果记录熔断状态。
// 只在错误层面做分类，不感知协议内容，符合架构红线。
func (d *CircuitBreakerFetcherDecorator) FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error) {
	if d.inner == nil || d.cb == nil {
		return d.inner.FetchRemote(ctx, remoteURL, path)
	}
	if !d.cb.AllowRequest() {
		return nil, NewCircuitOpenError(d.cb.GetRetryAfter())
	}
	artifacts, err := d.inner.FetchRemote(ctx, remoteURL, path)
	if err == nil {
		d.cb.RecordSuccess()
		return artifacts, nil
	}
	if IsUpstreamTimeout(err) {
		d.cb.RecordTimeout()
		return artifacts, err
	}
	d.cb.RecordFailure()
	return artifacts, err
}

// FetchArtifactMetadata 转发内层 Fetcher 的可选能力 ArtifactMetadataFetcher。
//
// 装饰器不能剥离被包裹对象的可选接口：maven/pypi/npm 插件实现了
// ArtifactMetadataFetcher，若包装后类型断言 n.Fetcher.(ArtifactMetadataFetcher) 失败，
// 基于 license/发布时间等的条件阻断规则会静默失效。因此这里恒实现该接口，
// 内层不支持时返回 ErrMetadataUnsupported（语义与插件直接不支持一致）。
func (d *CircuitBreakerFetcherDecorator) FetchArtifactMetadata(ctx context.Context, remoteURL string, key ArtifactKey) (*ArtifactMetadata, error) {
	inner, ok := d.inner.(ArtifactMetadataFetcher)
	if !ok {
		return nil, ErrMetadataUnsupported
	}
	if d.cb == nil {
		return inner.FetchArtifactMetadata(ctx, remoteURL, key)
	}
	if !d.cb.AllowRequest() {
		return nil, NewCircuitOpenError(d.cb.GetRetryAfter())
	}
	meta, err := inner.FetchArtifactMetadata(ctx, remoteURL, key)
	if err == nil {
		d.cb.RecordSuccess()
		return meta, nil
	}
	if IsUpstreamTimeout(err) {
		d.cb.RecordTimeout()
	} else {
		d.cb.RecordFailure()
	}
	return meta, err
}
