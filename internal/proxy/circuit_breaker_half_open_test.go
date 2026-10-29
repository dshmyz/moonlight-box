package proxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

type stubFetcher struct{ err error }

func (s stubFetcher) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	return nil, s.err
}

// 半开状态下唯一放行的探测若返回 404，必须记为成功并关闭熔断。
// 若不落账，halfOpenProbeInFlight 永远为 true，AllowRequest 之后一直返回 false，
// 整条上游会被永久拒绝直到进程重启。
func TestCircuitBreakerHalfOpenProbe404Recovers(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:   1,
		ResetTimeout:  20 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	})
	ctx := context.Background()

	failing := runtime.NewCircuitBreakerFetcherDecorator(stubFetcher{err: errors.New("boom")}, cb)
	if _, err := failing.FetchRemote(ctx, "https://up.test", "pkg"); err == nil {
		t.Fatal("expected failure to trip the breaker")
	}
	if state := cb.GetState(); state != CircuitOpen {
		t.Fatalf("state = %s, want open", state)
	}

	time.Sleep(30 * time.Millisecond)

	notFound := runtime.NewCircuitBreakerFetcherDecorator(stubFetcher{err: runtime.ErrNotFound}, cb)
	if _, err := notFound.FetchRemote(ctx, "https://up.test", "pkg"); !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("FetchRemote err = %v, want ErrNotFound", err)
	}

	if state := cb.GetState(); state != CircuitClosed {
		t.Fatalf("state after 404 probe = %s, want closed", state)
	}
	if !cb.AllowRequest() {
		t.Fatal("upstream still rejected after 404 probe")
	}
}
