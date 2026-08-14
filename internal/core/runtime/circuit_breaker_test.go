package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeCircuitBreaker 是测试用的 CircuitBreaker 接口实现，记录所有调用便于断言。
// 计数字段用 atomic 以支持 -race 下的并发测试；allowRequest/retryAfterValue 在测试
// 构造后只读，无需同步。
type fakeCircuitBreaker struct {
	allowRequest    bool
	retryAfterValue int

	allowRequestCall int64
	successRecorded  int64
	failureRecorded  int64
	timeoutRecorded  int64
}

func (f *fakeCircuitBreaker) AllowRequest() bool {
	atomic.AddInt64(&f.allowRequestCall, 1)
	return f.allowRequest
}

func (f *fakeCircuitBreaker) RecordSuccess() { atomic.AddInt64(&f.successRecorded, 1) }
func (f *fakeCircuitBreaker) RecordFailure() { atomic.AddInt64(&f.failureRecorded, 1) }
func (f *fakeCircuitBreaker) RecordTimeout() { atomic.AddInt64(&f.timeoutRecorded, 1) }
func (f *fakeCircuitBreaker) GetRetryAfter() int {
	return f.retryAfterValue
}

// 以下 getter 用 atomic.LoadInt64 读取计数器，保证 -race 下的安全读取。
func (f *fakeCircuitBreaker) AllowRequestCalls() int64 {
	return atomic.LoadInt64(&f.allowRequestCall)
}
func (f *fakeCircuitBreaker) SuccessCount() int64 {
	return atomic.LoadInt64(&f.successRecorded)
}
func (f *fakeCircuitBreaker) FailureCount() int64 {
	return atomic.LoadInt64(&f.failureRecorded)
}
func (f *fakeCircuitBreaker) TimeoutCount() int64 {
	return atomic.LoadInt64(&f.timeoutRecorded)
}

// fakeInnerRemoteClient 用于测试装饰器对内层 RemoteClient 的调用。
// 返回值字段在构造后只读；调用计数用 atomic 以支持 -race 下的并发测试。
type fakeInnerRemoteClient struct {
	metadata     *RemoteMetadata
	metadataErr  error
	blob         io.ReadCloser
	blobErr      error
	openResponse *RemoteResponse
	openErr      error

	metadataCalled int64
	blobCalled     int64
	openCalled     int64
}

func (c *fakeInnerRemoteClient) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	atomic.AddInt64(&c.metadataCalled, 1)
	return c.metadata, c.metadataErr
}

func (c *fakeInnerRemoteClient) FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	atomic.AddInt64(&c.blobCalled, 1)
	return c.blob, c.blobErr
}

func (c *fakeInnerRemoteClient) Open(ctx context.Context, request RemoteRequest) (*RemoteResponse, error) {
	atomic.AddInt64(&c.openCalled, 1)
	return c.openResponse, c.openErr
}

// MetadataCalls / BlobCalls / OpenCalls 用 atomic 读取调用计数，保证 -race 安全。
func (c *fakeInnerRemoteClient) MetadataCalls() int64 { return atomic.LoadInt64(&c.metadataCalled) }
func (c *fakeInnerRemoteClient) BlobCalls() int64     { return atomic.LoadInt64(&c.blobCalled) }
func (c *fakeInnerRemoteClient) OpenCalls() int64     { return atomic.LoadInt64(&c.openCalled) }

// timeoutError 实现 net.Error 接口，用于测试超时判定。
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

// TestCircuitBreakerDecoratorBlocksWhenOpen 验证熔断打开时（AllowRequest=false）
// 装饰器直接返回 ErrCircuitOpen，不调用内层 RemoteClient。
func TestCircuitBreakerDecoratorBlocksWhenOpen(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: false, retryAfterValue: 42}
	inner := &fakeInnerRemoteClient{
		metadata: &RemoteMetadata{Exists: true},
	}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	ctx := context.Background()
	key := ArtifactKey{Format: "npm", Name: "pkg"}

	// FetchMetadata
	_, err := decorator.FetchMetadata(ctx, key)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("FetchMetadata error = %v, want ErrCircuitOpen", err)
	}
	var coe *CircuitOpenError
	if !errors.As(err, &coe) {
		t.Fatalf("FetchMetadata error should wrap *CircuitOpenError, got %T", err)
	}
	if coe.RetryAfter != 42 {
		t.Fatalf("RetryAfter = %d, want 42", coe.RetryAfter)
	}

	// FetchBlob
	_, err = decorator.FetchBlob(ctx, key)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("FetchBlob error = %v, want ErrCircuitOpen", err)
	}

	// Open
	_, err = decorator.Open(ctx, RemoteRequest{URL: "https://example.test/pkg"})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Open error = %v, want ErrCircuitOpen", err)
	}

	// 熔断打开时不应该调用内层
	if inner.MetadataCalls() != 0 || inner.BlobCalls() != 0 || inner.OpenCalls() != 0 {
		t.Fatalf("inner client should not be called when circuit open, got meta=%d blob=%d open=%d",
			inner.MetadataCalls(), inner.BlobCalls(), inner.OpenCalls())
	}
}

// TestCircuitBreakerDecoratorAllowsWhenClosed 验证熔断关闭时（AllowRequest=true）
// 装饰器透传调用内层 RemoteClient。
func TestCircuitBreakerDecoratorAllowsWhenClosed(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{
		metadata: &RemoteMetadata{Exists: true, ETag: "etag"},
		blob:     io.NopCloser(strings.NewReader("blob-content")),
		openResponse: &RemoteResponse{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Test": {"yes"}},
			Body:       io.NopCloser(strings.NewReader("body")),
		},
	}
	decorator := NewCircuitBreakerDecorator(inner, cb)
	ctx := context.Background()
	key := ArtifactKey{Format: "npm", Name: "pkg"}

	// FetchMetadata 透传
	meta, err := decorator.FetchMetadata(ctx, key)
	if err != nil {
		t.Fatalf("FetchMetadata error = %v", err)
	}
	if meta != inner.metadata {
		t.Fatal("FetchMetadata should return inner metadata")
	}

	// FetchBlob 透传
	blob, err := decorator.FetchBlob(ctx, key)
	if err != nil {
		t.Fatalf("FetchBlob error = %v", err)
	}
	if blob != inner.blob {
		t.Fatal("FetchBlob should return inner blob reader")
	}

	// Open 透传
	resp, err := decorator.Open(ctx, RemoteRequest{URL: "https://example.test"})
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	if resp != inner.openResponse {
		t.Fatal("Open should return inner response")
	}
}

// TestCircuitBreakerDecoratorFetchMetadataRecordsSuccess 验证 FetchMetadata 成功时记录 RecordSuccess。
func TestCircuitBreakerDecoratorFetchMetadataRecordsSuccess(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadata: &RemoteMetadata{Exists: true}}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchMetadata(context.Background(), ArtifactKey{})

	if cb.SuccessCount() != 1 {
		t.Fatalf("successRecorded = %d, want 1", cb.SuccessCount())
	}
	if cb.FailureCount() != 0 {
		t.Fatalf("failureRecorded = %d, want 0", cb.FailureCount())
	}
}

// TestCircuitBreakerDecoratorFetchMetadataNotFoundNotRecordedAsFailure
// 验证上游 404（meta.Exists=false, err=nil）不计入熔断失败——上游正常应答。
func TestCircuitBreakerDecoratorFetchMetadataNotFoundNotRecordedAsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadata: &RemoteMetadata{Exists: false}}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchMetadata(context.Background(), ArtifactKey{})

	if cb.FailureCount() != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (404 is not a failure)", cb.FailureCount())
	}
	if cb.SuccessCount() != 0 {
		t.Fatalf("successRecorded = %d, want 0 (404 is not a success either)", cb.SuccessCount())
	}
}

// TestCircuitBreakerDecoratorFetchMetadataErrorRecordsFailure
// 验证 FetchMetadata 返回错误时（网络故障、5xx）记录 RecordFailure。
func TestCircuitBreakerDecoratorFetchMetadataErrorRecordsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadataErr: errors.New("dial upstream: connection refused")}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, err := decorator.FetchMetadata(context.Background(), ArtifactKey{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if cb.FailureCount() != 1 {
		t.Fatalf("failureRecorded = %d, want 1", cb.FailureCount())
	}
	if cb.TimeoutCount() != 0 {
		t.Fatalf("timeoutRecorded = %d, want 0 (not a timeout)", cb.TimeoutCount())
	}
}

// TestCircuitBreakerDecoratorFetchMetadataTimeoutRecordsTimeout
// 验证 net.Error.Timeout()==true 时走 RecordTimeout（区分于普通失败）。
func TestCircuitBreakerDecoratorFetchMetadataTimeoutRecordsTimeout(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadataErr: timeoutError{}}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchMetadata(context.Background(), ArtifactKey{})

	if cb.TimeoutCount() != 1 {
		t.Fatalf("timeoutRecorded = %d, want 1", cb.TimeoutCount())
	}
	// 超时不应该再计 failure（避免一次失败计两次）
	if cb.FailureCount() != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (timeout should use RecordTimeout only)", cb.FailureCount())
	}
}

// TestCircuitBreakerDecoratorFetchBlobRecordsSuccess 验证 FetchBlob 成功时记录 RecordSuccess。
func TestCircuitBreakerDecoratorFetchBlobRecordsSuccess(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{blob: io.NopCloser(strings.NewReader("content"))}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchBlob(context.Background(), ArtifactKey{})

	if cb.SuccessCount() != 1 {
		t.Fatalf("successRecorded = %d, want 1", cb.SuccessCount())
	}
}

// TestCircuitBreakerDecoratorFetchBlobNotFoundNotRecordedAsFailure
// 验证上游 404（ErrNotFound）不计入熔断失败。
func TestCircuitBreakerDecoratorFetchBlobNotFoundNotRecordedAsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{blobErr: ErrNotFound}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchBlob(context.Background(), ArtifactKey{})

	if cb.FailureCount() != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (404 is not a failure)", cb.FailureCount())
	}
	if cb.SuccessCount() != 0 {
		t.Fatalf("successRecorded = %d, want 0 (404 is not a success)", cb.SuccessCount())
	}
}

// TestCircuitBreakerDecoratorFetchBlobErrorRecordsFailure
// 验证 FetchBlob 返回非 ErrNotFound 错误时记录 RecordFailure。
func TestCircuitBreakerDecoratorFetchBlobErrorRecordsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{blobErr: errors.New("remote returned status 503")}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchBlob(context.Background(), ArtifactKey{})

	if cb.FailureCount() != 1 {
		t.Fatalf("failureRecorded = %d, want 1", cb.FailureCount())
	}
}

// TestCircuitBreakerDecoratorOpenRecordsSuccess 验证 Open 成功时（err==nil）记录 RecordSuccess。
// Open 是不透明流，HTTP 5xx 不视为 error（透传给 Plugin），因此不计入失败。
func TestCircuitBreakerDecoratorOpenRecordsSuccess(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{
		openResponse: &RemoteResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("")),
		},
	}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.Open(context.Background(), RemoteRequest{})

	if cb.SuccessCount() != 1 {
		t.Fatalf("successRecorded = %d, want 1 (Open 5xx is transparent)", cb.SuccessCount())
	}
	if cb.FailureCount() != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (Open 5xx is not a transport failure)", cb.FailureCount())
	}
}

// TestCircuitBreakerDecoratorOpenErrorRecordsFailure 验证 Open 传输错误时记录 RecordFailure。
func TestCircuitBreakerDecoratorOpenErrorRecordsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{openErr: errors.New("dial tcp: connection refused")}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.Open(context.Background(), RemoteRequest{})

	if cb.FailureCount() != 1 {
		t.Fatalf("failureRecorded = %d, want 1", cb.FailureCount())
	}
}

// TestCircuitBreakerDecoratorNilBreakerIsTransparent
// 验证 cb 为 nil 时装饰器完全透传——便于 ProxyRuntime 测试无熔断器场景向后兼容。
func TestCircuitBreakerDecoratorNilBreakerIsTransparent(t *testing.T) {
	inner := &fakeInnerRemoteClient{
		metadata: &RemoteMetadata{Exists: true},
		blob:     io.NopCloser(strings.NewReader("")),
		openResponse: &RemoteResponse{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		},
	}
	decorator := NewCircuitBreakerDecorator(inner, nil)
	ctx := context.Background()

	// 不应 panic，不检查 AllowRequest，直接透传
	meta, err := decorator.FetchMetadata(ctx, ArtifactKey{})
	if err != nil || meta != inner.metadata {
		t.Fatalf("FetchMetadata with nil cb should be transparent, err=%v", err)
	}
	blob, err := decorator.FetchBlob(ctx, ArtifactKey{})
	if err != nil || blob != inner.blob {
		t.Fatalf("FetchBlob with nil cb should be transparent, err=%v", err)
	}
	resp, err := decorator.Open(ctx, RemoteRequest{})
	if err != nil || resp != inner.openResponse {
		t.Fatalf("Open with nil cb should be transparent, err=%v", err)
	}
}

// TestCircuitOpenErrorContract 验证 CircuitOpenError 的 errors.Is/As 契约。
func TestCircuitOpenErrorContract(t *testing.T) {
	err := NewCircuitOpenError(30)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatal("errors.Is(err, ErrCircuitOpen) should be true")
	}
	var coe *CircuitOpenError
	if !errors.As(err, &coe) {
		t.Fatal("errors.As should succeed for *CircuitOpenError")
	}
	if coe.RetryAfter != 30 {
		t.Fatalf("RetryAfter = %d, want 30", coe.RetryAfter)
	}
	// RetryAfter=0 时也应工作（例如熔断即将转 half_open 的边界场景）
	zeroErr := NewCircuitOpenError(0)
	if !errors.Is(zeroErr, ErrCircuitOpen) {
		t.Fatal("errors.Is should be true even when RetryAfter=0")
	}
}

// TestCircuitBreakerDecoratorPreservesInnerErrors
// 验证装饰器透传内层返回的原始错误（不包装、不掩盖），仅副作用是记录熔断状态。
func TestCircuitBreakerDecoratorPreservesInnerErrors(t *testing.T) {
	originalErr := errors.New("remote returned status 500")
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadataErr: originalErr}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, err := decorator.FetchMetadata(context.Background(), ArtifactKey{})
	if !errors.Is(err, originalErr) {
		t.Fatalf("decorator should preserve inner error, got %v", err)
	}
}

// 以下测试用例确保装饰器在并发场景下的安全性（CircuitBreaker 实现本身是线程安全的，
// 装饰器本身无共享状态，此处主要验证不会因并发调用 fakeCircuitBreaker 产生 data race）。
func TestCircuitBreakerDecoratorConcurrentSafe(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{
		metadata: &RemoteMetadata{Exists: true},
		blob:     io.NopCloser(strings.NewReader("")),
		openResponse: &RemoteResponse{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		},
	}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	const concurrency = 20
	done := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			ctx := context.Background()
			_, _ = decorator.FetchMetadata(ctx, ArtifactKey{})
			_, _ = decorator.FetchBlob(ctx, ArtifactKey{})
			_, _ = decorator.Open(ctx, RemoteRequest{})
		}()
	}
	for i := 0; i < concurrency; i++ {
		<-done
	}
	// 通过条件：不 panic 且 -race 下无 data race 报告。
	// fakeCircuitBreaker 的计数字段用 atomic 保护，装饰器本身无共享状态。
	// 额外断言所有调用都被记录，确保并发下计数不丢。
	totalCalls := cb.SuccessCount() + cb.FailureCount() + cb.TimeoutCount()
	if totalCalls != int64(concurrency)*3 {
		t.Fatalf("total recorded = %d, want %d (concurrency=%d, 3 ops per goroutine)",
			totalCalls, int64(concurrency)*3, concurrency)
	}
}

// --- CircuitBreakerFetcherDecorator 测试 ---

// fakeRemoteFetcher 是测试用的 RemoteFetcher，记录调用次数与指定返回值。
type fakeRemoteFetcher struct {
	artifacts []*Artifact
	err       error

	remoteCalled int64
}

func (f *fakeRemoteFetcher) FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error) {
	atomic.AddInt64(&f.remoteCalled, 1)
	return f.artifacts, f.err
}

func (f *fakeRemoteFetcher) RemoteCalls() int64 { return atomic.LoadInt64(&f.remoteCalled) }

// fakeMetadataFetcher 同时实现 RemoteFetcher 与 ArtifactMetadataFetcher，
// 用于验证装饰器不剥离可选能力。
type fakeMetadataFetcher struct {
	fakeRemoteFetcher
	metadata *ArtifactMetadata
	metaErr  error
}

func (f *fakeMetadataFetcher) FetchArtifactMetadata(ctx context.Context, remoteURL string, key ArtifactKey) (*ArtifactMetadata, error) {
	return f.metadata, f.metaErr
}

// TestCircuitBreakerFetcherDecoratorBlocksWhenOpen 验证熔断打开时直接返回 ErrCircuitOpen，
// 不调用内层 RemoteFetcher。
func TestCircuitBreakerFetcherDecoratorBlocksWhenOpen(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: false, retryAfterValue: 7}
	inner := &fakeRemoteFetcher{artifacts: []*Artifact{{Name: "pkg"}}}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	_, err := decorator.FetchRemote(context.Background(), "https://example.test", "pkg")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("FetchRemote error = %v, want ErrCircuitOpen", err)
	}
	var coe *CircuitOpenError
	if !errors.As(err, &coe) || coe.RetryAfter != 7 {
		t.Fatalf("FetchRemote error should wrap *CircuitOpenError with RetryAfter=7, got %v", err)
	}
	if inner.RemoteCalls() != 0 {
		t.Fatalf("inner calls = %d, want 0 (must not reach inner when open)", inner.RemoteCalls())
	}
}

// TestCircuitBreakerFetcherDecoratorAllowsWhenClosed 验证熔断关闭时透传内层并记录成功。
func TestCircuitBreakerFetcherDecoratorAllowsWhenClosed(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	want := []*Artifact{{Name: "pkg"}}
	inner := &fakeRemoteFetcher{artifacts: want}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	got, err := decorator.FetchRemote(context.Background(), "https://example.test", "pkg")
	if err != nil {
		t.Fatalf("FetchRemote error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Name != "pkg" {
		t.Fatalf("FetchRemote = %v, want [pkg]", got)
	}
	if cb.SuccessCount() != 1 {
		t.Fatalf("success recorded = %d, want 1", cb.SuccessCount())
	}
}

// TestCircuitBreakerFetcherDecoratorTimeoutRecordsTimeout 验证超时错误记入 RecordTimeout。
func TestCircuitBreakerFetcherDecoratorTimeoutRecordsTimeout(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeRemoteFetcher{err: timeoutError{}}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	if _, err := decorator.FetchRemote(context.Background(), "https://example.test", "pkg"); err == nil {
		t.Fatal("FetchRemote error = nil, want timeout error")
	}
	if cb.TimeoutCount() != 1 {
		t.Fatalf("timeout recorded = %d, want 1", cb.TimeoutCount())
	}
	if cb.FailureCount() != 0 {
		t.Fatalf("failure recorded = %d, want 0", cb.FailureCount())
	}
}

// TestCircuitBreakerFetcherDecoratorFailureRecordsFailure 验证普通失败记入 RecordFailure。
func TestCircuitBreakerFetcherDecoratorFailureRecordsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeRemoteFetcher{err: errors.New("connection refused")}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	if _, err := decorator.FetchRemote(context.Background(), "https://example.test", "pkg"); err == nil {
		t.Fatal("FetchRemote error = nil, want error")
	}
	if cb.FailureCount() != 1 {
		t.Fatalf("failure recorded = %d, want 1", cb.FailureCount())
	}
	if cb.TimeoutCount() != 0 {
		t.Fatalf("timeout recorded = %d, want 0", cb.TimeoutCount())
	}
}

// TestCircuitBreakerFetcherDecoratorNilBreakerIsTransparent 验证 cb 为 nil 时装饰器完全透传，
// 不调用任何熔断方法，也不报熔断错误。
func TestCircuitBreakerFetcherDecoratorNilBreakerIsTransparent(t *testing.T) {
	inner := &fakeRemoteFetcher{artifacts: []*Artifact{{Name: "pkg"}}}
	decorator := NewCircuitBreakerFetcherDecorator(inner, nil)

	got, err := decorator.FetchRemote(context.Background(), "https://example.test", "pkg")
	if err != nil || len(got) != 1 {
		t.Fatalf("FetchRemote = (%v, %v), want (1 artifact, nil)", got, err)
	}
}

// TestCircuitBreakerFetcherDecoratorForwardsMetadataCapability 验证装饰器不剥离
// ArtifactMetadataFetcher 可选能力：类型断言必须成功，且透传内层实现。
func TestCircuitBreakerFetcherDecoratorForwardsMetadataCapability(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeMetadataFetcher{
		metadata: &ArtifactMetadata{Attributes: map[string]string{"license": "MIT"}},
	}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	// 类型断言必须成功（这正是包装后条件阻断规则依赖的接口）。
	fetcher, ok := interface{}(decorator).(ArtifactMetadataFetcher)
	if !ok {
		t.Fatal("decorator does not implement ArtifactMetadataFetcher, capability lost after wrapping")
	}
	meta, err := fetcher.FetchArtifactMetadata(context.Background(), "https://example.test", ArtifactKey{Name: "pkg"})
	if err != nil || meta.Attributes["license"] != "MIT" {
		t.Fatalf("FetchArtifactMetadata = (%v, %v), want (license=MIT, nil)", meta, err)
	}
	if cb.SuccessCount() != 1 {
		t.Fatalf("success recorded = %d, want 1", cb.SuccessCount())
	}
}

// TestCircuitBreakerFetcherDecoratorUnsupportedMetadata 验证被包裹对象不支持
// ArtifactMetadataFetcher 时返回 ErrMetadataUnsupported（语义与插件不支持一致）。
func TestCircuitBreakerFetcherDecoratorUnsupportedMetadata(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeRemoteFetcher{}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	_, err := decorator.FetchArtifactMetadata(context.Background(), "https://example.test", ArtifactKey{Name: "pkg"})
	if !errors.Is(err, ErrMetadataUnsupported) {
		t.Fatalf("FetchArtifactMetadata error = %v, want ErrMetadataUnsupported", err)
	}
}

// TestCircuitBreakerFetcherDecoratorConcurrentSafe 验证并发安全（-race 下无 data race）。
func TestCircuitBreakerFetcherDecoratorConcurrentSafe(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeMetadataFetcher{fakeRemoteFetcher: fakeRemoteFetcher{artifacts: []*Artifact{{Name: "pkg"}}}}
	decorator := NewCircuitBreakerFetcherDecorator(inner, cb)

	const concurrency = 20
	done := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			ctx := context.Background()
			_, _ = decorator.FetchRemote(ctx, "https://example.test", "pkg")
			_, _ = decorator.FetchArtifactMetadata(ctx, "https://example.test", ArtifactKey{Name: "pkg"})
		}()
	}
	for i := 0; i < concurrency; i++ {
		<-done
	}
	if total := cb.SuccessCount(); total != int64(concurrency)*2 {
		t.Fatalf("success recorded = %d, want %d (2 ops per goroutine)", total, int64(concurrency)*2)
	}
}
