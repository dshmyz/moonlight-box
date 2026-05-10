package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sync"
	"time"
)

type TransportManager struct {
	secureTransport   *http.Transport
	insecureTransport *http.Transport
	connectTimeout    time.Duration
	dnsResolver       *DNSResolver
}

func NewTransportManager(connectTimeout time.Duration, dnsResolver *DNSResolver) *TransportManager {
	baseTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			resolvedIP, err := dnsResolver.Resolve(ctx, host)
			if err != nil {
				return nil, err
			}

			resolvedHost, resolvedPort, err := net.SplitHostPort(resolvedIP)
			if err != nil {
				resolvedHost = resolvedIP
				resolvedPort = port
			}

			dialer := &net.Dialer{
				Timeout:   connectTimeout,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedHost, resolvedPort))
		},
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		MaxConnsPerHost:       100,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	return &TransportManager{
		secureTransport:   baseTransport,
		insecureTransport: baseTransport,
		connectTimeout:    connectTimeout,
		dnsResolver:       dnsResolver,
	}
}

func (m *TransportManager) GetTransport(insecure bool) *http.Transport {
	if insecure {
		return m.insecureTransport
	}
	return m.secureTransport
}

type RequestOptions struct {
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	MaxRedirects       int
	InsecureSkipVerify bool
	MaxRetries         int
	RetryDelay         time.Duration
}

type RemoteClient struct {
	transportManager    *TransportManager
	defaultMaxRedirects int
	defaultMaxRetries   int
	defaultRetryDelay   time.Duration

	clientCache sync.Map // 缓存 http.Client，key 为配置参数
}

func NewRemoteClient(tm *TransportManager, defaultMaxRedirects int) *RemoteClient {
	return &RemoteClient{
		transportManager:    tm,
		defaultMaxRedirects: defaultMaxRedirects,
		defaultMaxRetries:   3,
		defaultRetryDelay:   1 * time.Second,
	}
}

func NewRemoteClientWithRetry(tm *TransportManager, defaultMaxRedirects, defaultMaxRetries int, defaultRetryDelay time.Duration) *RemoteClient {
	return &RemoteClient{
		transportManager:    tm,
		defaultMaxRedirects: defaultMaxRedirects,
		defaultMaxRetries:   defaultMaxRetries,
		defaultRetryDelay:   defaultRetryDelay,
	}
}

type clientCacheKey struct {
	maxRedirects int
	insecure     bool
	timeout      time.Duration
}

func (c *RemoteClient) buildClient(opts RequestOptions) *http.Client {
	maxRedirects := opts.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = c.defaultMaxRedirects
	}

	timeout := opts.ReadTimeout
	key := clientCacheKey{
		maxRedirects: maxRedirects,
		insecure:     opts.InsecureSkipVerify,
		timeout:      timeout,
	}

	if cached, ok := c.clientCache.Load(key); ok {
		return cached.(*http.Client)
	}

	transport := c.transportManager.GetTransport(opts.InsecureSkipVerify)

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if maxRedirects == -1 {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return fmt.Errorf("重定向次数超过限制: %d", maxRedirects)
			}
			return nil
		},
		Timeout: timeout,
	}

	c.clientCache.Store(key, client)
	return client
}

func (c *RemoteClient) Get(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (*http.Response, error) {
	client := c.buildClient(opts)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Moonlight-Registry/1.0")

	if auth != nil {
		if err := auth.Apply(req); err != nil {
			return nil, fmt.Errorf("应用认证信息失败: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &RemoteError{
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	return resp, nil
}

func (c *RemoteClient) GetBytes(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) ([]byte, string, error) {
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = c.defaultMaxRetries
	}
	retryDelay := opts.RetryDelay
	if retryDelay == 0 {
		retryDelay = c.defaultRetryDelay
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := c.Get(ctx, url, opts, auth)
		if err != nil {
			lastErr = err

			if c.shouldRetry(err, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			return nil, "", err
		}

		contentType := resp.Header.Get("Content-Type")
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("读取响应体失败: %w", err)
			if c.shouldRetry(lastErr, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			return nil, "", lastErr
		}

		return body, contentType, nil
	}

	return nil, "", lastErr
}

func (c *RemoteClient) GetStream(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (*http.Response, error) {
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = c.defaultMaxRetries
	}
	retryDelay := opts.RetryDelay
	if retryDelay == 0 {
		retryDelay = c.defaultRetryDelay
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		client := c.buildClient(opts)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		req.Header.Set("User-Agent", "Moonlight-Registry/1.0")

		if auth != nil {
			if err := auth.Apply(req); err != nil {
				return nil, fmt.Errorf("应用认证信息失败: %w", err)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if c.shouldRetry(err, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = &RemoteError{
				StatusCode: resp.StatusCode,
				URL:        url,
			}
			if c.shouldRetry(lastErr, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			return nil, lastErr
		}

		return resp, nil
	}

	return nil, lastErr
}

func (c *RemoteClient) shouldRetry(err error, attempt, maxRetries int) bool {
	if attempt >= maxRetries-1 {
		return false
	}

	switch err {
	case context.DeadlineExceeded:
		return true
	case context.Canceled:
		return false
	}

	if remoteErr, ok := err.(*RemoteError); ok {
		switch remoteErr.StatusCode {
		case 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}

	if _, ok := err.(net.Error); ok {
		return true
	}

	return false
}

type RemoteError struct {
	StatusCode int
	URL        string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("远程请求失败: %d %s", e.StatusCode, e.URL)
}

func (e *RemoteError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}
