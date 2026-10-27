package runtime

import (
	"context"
	"testing"
	"time"
)

func TestConnectTimeoutContextRoundTrip(t *testing.T) {
	if got := ConnectTimeoutFromContext(context.Background()); got != 0 {
		t.Fatalf("ConnectTimeoutFromContext(empty) = %s, want 0", got)
	}
	if got := ConnectTimeoutFromContext(nil); got != 0 {
		t.Fatalf("ConnectTimeoutFromContext(nil) = %s, want 0", got)
	}

	ctx := WithConnectTimeout(context.Background(), 5*time.Second)
	if got := ConnectTimeoutFromContext(ctx); got != 5*time.Second {
		t.Fatalf("ConnectTimeoutFromContext = %s, want 5s", got)
	}

	// <=0 是"沿用全局"，不能覆盖已有值
	if got := ConnectTimeoutFromContext(WithConnectTimeout(ctx, 0)); got != 5*time.Second {
		t.Fatalf("zero timeout overrode existing value: %s", got)
	}

	// blob 下载用 WithoutCancel 剥离 deadline，值必须保留
	if got := ConnectTimeoutFromContext(context.WithoutCancel(ctx)); got != 5*time.Second {
		t.Fatalf("WithoutCancel dropped connect timeout: %s", got)
	}
}
