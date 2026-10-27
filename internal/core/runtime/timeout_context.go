package runtime

import (
	"context"
	"time"
)

// connectTimeoutKey 是"本次请求 TCP 建连超时"的 ctx key。
//
// 必须走 ctx 而不是 http.Client：全站共用一份 Transport（连接池），插件也
// 共用同一个 http.Client，只有按请求传递才能做到仓库级隔离且不分裂连接池。
type connectTimeoutKey struct{}

// WithConnectTimeout 给本次上游请求设置仓库级建连超时；timeout <= 0 表示沿用全局。
func WithConnectTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if ctx == nil || timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, connectTimeoutKey{}, timeout)
}

// ConnectTimeoutFromContext 读取仓库级建连超时，未设置时返回 0。
func ConnectTimeoutFromContext(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	timeout, _ := ctx.Value(connectTimeoutKey{}).(time.Duration)
	return timeout
}
