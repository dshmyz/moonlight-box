package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// TransportManager 管理多个 Transport 实例，避免每次请求都创建
type TransportManager struct {
	secureTransport   *http.Transport
	insecureTransport *http.Transport
	connectTimeout    time.Duration
	dnsResolver       *DNSResolver
}

// NewTransportManager 创建 TransportManager
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
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	insecureTransport := baseTransport.Clone()
	insecureTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	}

	return &TransportManager{
		secureTransport:   baseTransport,
		insecureTransport: insecureTransport,
		connectTimeout:    connectTimeout,
		dnsResolver:       dnsResolver,
	}
}

// GetTransport 根据是否需要跳过证书校验返回对应的 Transport
func (m *TransportManager) GetTransport(insecure bool) *http.Transport {
	if insecure {
		return m.insecureTransport
	}
	return m.secureTransport
}

// RequestOptions 请求选项
type RequestOptions struct {
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	MaxRedirects       int // -1 表示不跟随重定向，0 表示使用默认值
	InsecureSkipVerify bool
}

// RemoteClient 远程 HTTP 客户端
type RemoteClient struct {
	transportManager    *TransportManager
	defaultMaxRedirects int
}

// NewRemoteClient 创建远程客户端
func NewRemoteClient(tm *TransportManager, defaultMaxRedirects int) *RemoteClient {
	return &RemoteClient{
		transportManager:    tm,
		defaultMaxRedirects: defaultMaxRedirects,
	}
}

// buildClient 根据选项构建 http.Client
func (c *RemoteClient) buildClient(opts RequestOptions) *http.Client {
	maxRedirects := opts.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = c.defaultMaxRedirects
	}

	transport := c.transportManager.GetTransport(opts.InsecureSkipVerify)

	return &http.Client{
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
		Timeout: opts.ReadTimeout,
	}
}

// Get 发起 GET 请求，返回原始 HTTP 响应
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

// GetBytes 发起 GET 请求并读取完整响应体
func (c *RemoteClient) GetBytes(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) ([]byte, string, error) {
	resp, err := c.Get(ctx, url, opts, auth)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取响应体失败: %w", err)
	}

	return body, contentType, nil
}

// GetStream 发起 GET 请求并返回流式响应，避免大文件全部加载到内存
func (c *RemoteClient) GetStream(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (*http.Response, error) {
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

// RemoteError 远程请求错误
type RemoteError struct {
	StatusCode int
	URL        string
}

// Error 实现 error 接口
func (e *RemoteError) Error() string {
	return fmt.Sprintf("远程请求失败: %d %s", e.StatusCode, e.URL)
}

// IsNotFound 判断是否为 404 未找到错误
func (e *RemoteError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}
