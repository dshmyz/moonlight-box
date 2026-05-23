package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/types"
)

var ErrPackageNotFound = fmt.Errorf("package not found")

type ProxyDownloader struct {
	cache              *CacheService
	client             *RemoteClient
	healthCheckSvc     *HealthCheckService
	largeFileThreshold int64
}

func NewProxyDownloader(
	cache *CacheService,
	client *RemoteClient,
) *ProxyDownloader {
	return &ProxyDownloader{
		cache:  cache,
		client: client,
	}
}

func (r *ProxyDownloader) SetLargeFileThreshold(threshold int64) {
	r.largeFileThreshold = threshold
}

func (r *ProxyDownloader) SetHealthCheckService(svc *HealthCheckService) {
	r.healthCheckSvc = svc
}

type RouteResult = types.RouteResult

// FetchFromRemote 从远程代理仓库拉取包
// 这是 ProxyDownloader 的核心职责：给定仓库和远程URL，发起远程请求并缓存
func (r *ProxyDownloader) FetchFromRemote(ctx context.Context, repo *model.Repository, remoteURL string) (*RouteResult, error) {
	cacheKey := fmt.Sprintf("proxy:%s:%s", repo.Name, remoteURL)

	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil && cached.IsNegative {
		return nil, ErrPackageNotFound
	}

	if r.healthCheckSvc != nil && r.healthCheckSvc.ShouldSkipRequest(repo.ID) {
		retryAfter := r.healthCheckSvc.GetRetryAfter(repo.ID)
		if r.healthCheckSvc.BlockOnUnhealthy() {
			slog.Warn("circuit breaker open, blocking request", "repo", repo.Name, "retry_after", retryAfter)
			return nil, fmt.Errorf("circuit breaker open for repo %s, retry after %d seconds", repo.Name, retryAfter)
		}
		slog.Warn("circuit breaker open, request allowed (block_on_unhealthy=false)", "repo", repo.Name, "retry_after", retryAfter)
	}

	if remoteURL == "" {
		return nil, fmt.Errorf("remote URL cannot be empty")
	}

	authCfg, err := repo.GetAuthConfig()
	if err != nil {
		return nil, err
	}

	readTimeout := r.calcReadTimeout(repo, -1)
	failureRules, _ := ParseFailureCacheRules(repo.FailureCacheRules)

	opts := RequestOptions{
		ReadTimeout:        readTimeout,
		MaxRedirects:       repo.MaxRedirects,
		InsecureSkipVerify: repo.InsecureSkipVerify,
	}

	resp, err := r.client.GetStream(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
	if err != nil {
		if r.healthCheckSvc != nil {
			r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordFailure()
		}

		if remoteErr, ok := err.(*RemoteError); ok {
			if failureRules.ShouldCache(remoteErr.StatusCode) {
				ttl := failureRules.Match(remoteErr.StatusCode)
				r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
			} else if remoteErr.IsNotFound() {
				r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
			}
		}
		return nil, err
	}

	if r.healthCheckSvc != nil {
		r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordSuccess()
	}

	contentLength := resp.ContentLength
	isLargeFile := r.largeFileThreshold > 0 && contentLength > r.largeFileThreshold

	if isLargeFile {
		return &RouteResult{
			Source:     repo.Name,
			SourceType: "proxy",
			RepoID:     repo.ID,
			Content:    resp.Body,
			Size:       contentLength,
			FromCache:  false,
			CacheTTL:   repo.CacheTTLSeconds,
			IsLarge:    true,
		}, nil
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		if r.healthCheckSvc != nil {
			r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordFailure()
		}
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	size := int64(len(body))

	return &RouteResult{
		Source:     repo.Name,
		SourceType: "proxy",
		RepoID:     repo.ID,
		Content:    io.NopCloser(bytes.NewReader(body)),
		Size:       size,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
		IsLarge:    false,
	}, nil
}

func (r *ProxyDownloader) calcReadTimeout(repo *model.Repository, contentLength int64) time.Duration {
	var baseTimeout time.Duration

	if repo.TimeoutSeconds > 0 {
		baseTimeout = time.Duration(repo.TimeoutSeconds) * time.Second
	} else {
		cfg := config.Get()
		if cfg != nil {
			baseTimeout = cfg.Proxy.DefaultTimeout
		} else {
			baseTimeout = 30 * time.Second
		}
	}

	if contentLength > 0 {
		threshold := int64(50 * 1024 * 1024)
		cfg := config.Get()
		if cfg != nil && cfg.Proxy.LargeFileThreshold > 0 {
			threshold = cfg.Proxy.LargeFileThreshold
		}
		if contentLength > threshold {
			baseTimeout *= 2
		}
	}

	return baseTimeout
}

func toProxyAuthConfig(cfg *model.ProxyAuthConfig) *ProxyAuthConfig {
	if cfg == nil {
		return nil
	}
	result := &ProxyAuthConfig{Type: cfg.Type}
	if cfg.Basic != nil {
		result.Basic = &BasicAuth{
			Username: cfg.Basic.Username,
			Password: cfg.Basic.Password,
		}
	}
	if cfg.Bearer != nil {
		result.Bearer = &BearerAuth{Token: cfg.Bearer.Token}
	}
	if cfg.APIKey != nil {
		result.APIKey = &APIKeyAuth{
			HeaderName: cfg.APIKey.HeaderName,
			KeyValue:   cfg.APIKey.KeyValue,
			QueryParam: cfg.APIKey.QueryParam,
		}
	}
	return result
}
