package runtime

import "context"

// upstreamRepositoryKey 是"本次回源所属仓库名"的 ctx key。
// 只用于可观测性展示（conntrack），不参与路由或缓存决策。
type upstreamRepositoryKey struct{}

// WithUpstreamRepository 给回源请求打上仓库名；空名表示不设置。
func WithUpstreamRepository(ctx context.Context, name string) context.Context {
	if ctx == nil || name == "" {
		return ctx
	}
	return context.WithValue(ctx, upstreamRepositoryKey{}, name)
}

// UpstreamRepositoryFromContext 读取回源仓库名，未设置时返回空串。
func UpstreamRepositoryFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	name, _ := ctx.Value(upstreamRepositoryKey{}).(string)
	return name
}
