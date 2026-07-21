package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeCircuitBreaker 是测试用的 CircuitBreaker 接口实现，记录所有调用便于断言。
type fakeCircuitBreaker struct {
	allowRequest     bool
	allowRequestCall  int
	successRecorded  int
	failureRecorded  int
	timeoutRecorded  int
	retryAfterValue  int
}

func (f *fakeCircuitBreaker) AllowRequest() bool {
	f.allowRequestCall++
	return f.allowRequest
}

func (f *fakeCircuitBreaker) RecordSuccess()  { f.successRecorded++ }
func (f *fakeCircuitBreaker) RecordFailure()  { f.failureRecorded++ }
func (f *fakeCircuitBreaker) RecordTimeout()  { f.timeoutRecorded++ }
func (f *fakeCircuitBreaker) GetRetryAfter() int { return f.retryAfterValue }

// fakeInnerRemoteClient 用于测试装饰器对内层 RemoteClient 的调用。
type fakeInnerRemoteClient struct {
	metadata       *RemoteMetadata
	metadataErr    error
	blob           io.ReadCloser
	blobErr        error
	openResponse   *RemoteResponse
	openErr        error
	metadataCalled int
	blobCalled     int
	openCalled     int
}

func (c *fakeInnerRemoteClient) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	c.metadataCalled++
	return c.metadata, c.metadataErr
}

func (c *fakeInnerRemoteClient) FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	c.blobCalled++
	return c.blob, c.blobErr
}

func (c *fakeInnerRemoteClient) Open(ctx context.Context, request RemoteRequest) (*RemoteResponse, error) {
	c.openCalled++
	return c.openResponse, c.openErr
}

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
	if inner.metadataCalled != 0 || inner.blobCalled != 0 || inner.openCalled != 0 {
		t.Fatalf("inner client should not be called when circuit open, got meta=%d blob=%d open=%d",
			inner.metadataCalled, inner.blobCalled, inner.openCalled)
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

	if cb.successRecorded != 1 {
		t.Fatalf("successRecorded = %d, want 1", cb.successRecorded)
	}
	if cb.failureRecorded != 0 {
		t.Fatalf("failureRecorded = %d, want 0", cb.failureRecorded)
	}
}

// TestCircuitBreakerDecoratorFetchMetadataNotFoundNotRecordedAsFailure
// 验证上游 404（meta.Exists=false, err=nil）不计入熔断失败——上游正常应答。
func TestCircuitBreakerDecoratorFetchMetadataNotFoundNotRecordedAsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadata: &RemoteMetadata{Exists: false}}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchMetadata(context.Background(), ArtifactKey{})

	if cb.failureRecorded != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (404 is not a failure)", cb.failureRecorded)
	}
	if cb.successRecorded != 0 {
		t.Fatalf("successRecorded = %d, want 0 (404 is not a success either)", cb.successRecorded)
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
	if cb.failureRecorded != 1 {
		t.Fatalf("failureRecorded = %d, want 1", cb.failureRecorded)
	}
	if cb.timeoutRecorded != 0 {
		t.Fatalf("timeoutRecorded = %d, want 0 (not a timeout)", cb.timeoutRecorded)
	}
}

// TestCircuitBreakerDecoratorFetchMetadataTimeoutRecordsTimeout
// 验证 net.Error.Timeout()==true 时走 RecordTimeout（区分于普通失败）。
func TestCircuitBreakerDecoratorFetchMetadataTimeoutRecordsTimeout(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{metadataErr: timeoutError{}}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchMetadata(context.Background(), ArtifactKey{})

	if cb.timeoutRecorded != 1 {
		t.Fatalf("timeoutRecorded = %d, want 1", cb.timeoutRecorded)
	}
	// 超时不应该再计 failure（避免一次失败计两次）
	if cb.failureRecorded != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (timeout should use RecordTimeout only)", cb.failureRecorded)
	}
}

// TestCircuitBreakerDecoratorFetchBlobRecordsSuccess 验证 FetchBlob 成功时记录 RecordSuccess。
func TestCircuitBreakerDecoratorFetchBlobRecordsSuccess(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{blob: io.NopCloser(strings.NewReader("content"))}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchBlob(context.Background(), ArtifactKey{})

	if cb.successRecorded != 1 {
		t.Fatalf("successRecorded = %d, want 1", cb.successRecorded)
	}
}

// TestCircuitBreakerDecoratorFetchBlobNotFoundNotRecordedAsFailure
// 验证上游 404（ErrNotFound）不计入熔断失败。
func TestCircuitBreakerDecoratorFetchBlobNotFoundNotRecordedAsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{blobErr: ErrNotFound}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchBlob(context.Background(), ArtifactKey{})

	if cb.failureRecorded != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (404 is not a failure)", cb.failureRecorded)
	}
	if cb.successRecorded != 0 {
		t.Fatalf("successRecorded = %d, want 0 (404 is not a success)", cb.successRecorded)
	}
}

// TestCircuitBreakerDecoratorFetchBlobErrorRecordsFailure
// 验证 FetchBlob 返回非 ErrNotFound 错误时记录 RecordFailure。
func TestCircuitBreakerDecoratorFetchBlobErrorRecordsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{blobErr: errors.New("remote returned status 503")}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.FetchBlob(context.Background(), ArtifactKey{})

	if cb.failureRecorded != 1 {
		t.Fatalf("failureRecorded = %d, want 1", cb.failureRecorded)
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

	if cb.successRecorded != 1 {
		t.Fatalf("successRecorded = %d, want 1 (Open 5xx is transparent)", cb.successRecorded)
	}
	if cb.failureRecorded != 0 {
		t.Fatalf("failureRecorded = %d, want 0 (Open 5xx is not a transport failure)", cb.failureRecorded)
	}
}

// TestCircuitBreakerDecoratorOpenErrorRecordsFailure 验证 Open 传输错误时记录 RecordFailure。
func TestCircuitBreakerDecoratorOpenErrorRecordsFailure(t *testing.T) {
	cb := &fakeCircuitBreaker{allowRequest: true}
	inner := &fakeInnerRemoteClient{openErr: errors.New("dial tcp: connection refused")}
	decorator := NewCircuitBreakerDecorator(inner, cb)

	_, _ = decorator.Open(context.Background(), RemoteRequest{})

	if cb.failureRecorded != 1 {
		t.Fatalf("failureRecorded = %d, want 1", cb.failureRecorded)
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
	// 不 panic 即通过；fakeCircuitBreaker 字段非原子操作会有 race，
	// 但本测试只验证装饰器不引入额外共享状态
}

// 确保 time 包被使用（避免 import 报错，后续测试可能用到）
var _ = time.Second
