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

	"github.com/sirupsen/logrus"
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
		DisableCompression:    true,
	}
	secureTransport := baseTransport.Clone()
	insecureTransport := baseTransport.Clone()
	insecureTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	return &TransportManager{
		secureTransport:   secureTransport,
		insecureTransport: insecureTransport,
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
	// defaultReadTimeout 在调用方未指定 ReadTimeout 时的兜底响应超时，
	// 防止上游 accept 后不发响应导致请求无限挂起。
	defaultReadTimeout time.Duration

	clientCache sync.Map // 缓存 http.Client，key 为配置参数
}

const maxGetBytesResponseBytes = 64 * 1024 * 1024

func NewRemoteClient(tm *TransportManager, defaultMaxRedirects int) *RemoteClient {
	return &RemoteClient{
		transportManager:    tm,
		defaultMaxRedirects: defaultMaxRedirects,
		defaultMaxRetries:   3,
		defaultRetryDelay:   1 * time.Second,
		defaultReadTimeout:  30 * time.Second,
	}
}

func NewRemoteClientWithRetry(tm *TransportManager, defaultMaxRedirects, defaultMaxRetries int, defaultRetryDelay time.Duration) *RemoteClient {
	return &RemoteClient{
		transportManager:    tm,
		defaultMaxRedirects: defaultMaxRedirects,
		defaultMaxRetries:   defaultMaxRetries,
		defaultRetryDelay:   defaultRetryDelay,
		defaultReadTimeout:  30 * time.Second,
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
	if timeout <= 0 {
		timeout = c.defaultReadTimeout
	}
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
	start := time.Now()
	client := c.buildClient(opts)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logrus.WithError(err).WithField("url", url).Error("proxy: create request failed")
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Moonlight-Registry/1.0")

	if auth != nil {
		if err := auth.Apply(req); err != nil {
			logrus.WithError(err).WithField("url", url).Error("proxy: apply auth failed")
			return nil, fmt.Errorf("应用认证信息失败: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"url":      url,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("proxy: HTTP request failed")
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		logrus.WithFields(logrus.Fields{
			"url":        url,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Warn("proxy: HTTP request returned non-200 status")
		return nil, &RemoteError{
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	logrus.WithFields(logrus.Fields{
		"url":        url,
		"statusCode": resp.StatusCode,
		"duration":   time.Since(start).Seconds(),
	}).Debug("proxy: HTTP request success")
	return resp, nil
}

func (c *RemoteClient) GetBytes(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) ([]byte, string, error) {
	start := time.Now()
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = c.defaultMaxRetries
	}
	retryDelay := opts.RetryDelay
	if retryDelay == 0 {
		retryDelay = c.defaultRetryDelay
	}

	logrus.WithFields(logrus.Fields{
		"url":        url,
		"maxRetries": maxRetries,
	}).Debug("proxy: GetBytes request started")

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := c.Get(ctx, url, opts, auth)
		if err != nil {
			lastErr = err

			if c.shouldRetry(err, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				logrus.WithFields(logrus.Fields{
					"url":     url,
					"attempt": i + 1,
					"delay":   delay.Seconds(),
					"error":   err.Error(),
				}).Warn("proxy: GetBytes request failed, retrying")
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			logrus.WithFields(logrus.Fields{
				"url":      url,
				"attempts": i + 1,
				"duration": time.Since(start).Seconds(),
				"error":    err.Error(),
			}).Error("proxy: GetBytes request failed after all retries")
			return nil, "", err
		}

		contentType := resp.Header.Get("Content-Type")
		body, err := readLimitedResponseBody(resp)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("读取响应体失败: %w", err)
			if c.shouldRetry(lastErr, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				logrus.WithFields(logrus.Fields{
					"url":     url,
					"attempt": i + 1,
					"delay":   delay.Seconds(),
					"error":   lastErr.Error(),
				}).Warn("proxy: GetBytes read body failed, retrying")
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			logrus.WithFields(logrus.Fields{
				"url":      url,
				"attempts": i + 1,
				"duration": time.Since(start).Seconds(),
				"error":    lastErr.Error(),
			}).Error("proxy: GetBytes read body failed after all retries")
			return nil, "", lastErr
		}

		logrus.WithFields(logrus.Fields{
			"url":         url,
			"size":        len(body),
			"contentType": contentType,
			"duration":    time.Since(start).Seconds(),
		}).Debug("proxy: GetBytes request success")
		return body, contentType, nil
	}

	logrus.WithFields(logrus.Fields{
		"url":      url,
		"duration": time.Since(start).Seconds(),
		"error":    lastErr.Error(),
	}).Error("proxy: GetBytes request failed after all attempts")
	return nil, "", lastErr
}

func readLimitedResponseBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxGetBytesResponseBytes {
		return nil, fmt.Errorf("response body too large: %d > %d", resp.ContentLength, maxGetBytesResponseBytes)
	}
	var buf []byte
	limited := io.LimitReader(resp.Body, maxGetBytesResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxGetBytesResponseBytes {
		return nil, fmt.Errorf("response body too large: exceeds %d bytes", maxGetBytesResponseBytes)
	}
	buf = body
	return buf, nil
}

func (c *RemoteClient) GetStream(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (*http.Response, error) {
	start := time.Now()
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = c.defaultMaxRetries
	}
	retryDelay := opts.RetryDelay
	if retryDelay == 0 {
		retryDelay = c.defaultRetryDelay
	}

	logrus.WithFields(logrus.Fields{
		"url":        url,
		"maxRetries": maxRetries,
	}).Debug("proxy: GetStream request started")

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		client := c.buildClient(opts)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			logrus.WithError(err).WithField("url", url).Error("proxy: GetStream create request failed")
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		req.Header.Set("User-Agent", "Moonlight-Registry/1.0")

		if auth != nil {
			if err := auth.Apply(req); err != nil {
				logrus.WithError(err).WithField("url", url).Error("proxy: GetStream apply auth failed")
				return nil, fmt.Errorf("应用认证信息失败: %w", err)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if c.shouldRetry(err, i, maxRetries) {
				delay := time.Duration(math.Pow(2, float64(i))) * retryDelay
				logrus.WithFields(logrus.Fields{
					"url":     url,
					"attempt": i + 1,
					"delay":   delay.Seconds(),
					"error":   err.Error(),
				}).Warn("proxy: GetStream request failed, retrying")
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			logrus.WithFields(logrus.Fields{
				"url":      url,
				"attempts": i + 1,
				"duration": time.Since(start).Seconds(),
				"error":    err.Error(),
			}).Error("proxy: GetStream request failed after all retries")
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
				logrus.WithFields(logrus.Fields{
					"url":        url,
					"attempt":    i + 1,
					"delay":      delay.Seconds(),
					"statusCode": resp.StatusCode,
				}).Warn("proxy: GetStream returned non-200 status, retrying")
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			logrus.WithFields(logrus.Fields{
				"url":        url,
				"attempts":   i + 1,
				"statusCode": resp.StatusCode,
				"duration":   time.Since(start).Seconds(),
			}).Error("proxy: GetStream returned non-200 status after all retries")
			return nil, lastErr
		}

		logrus.WithFields(logrus.Fields{
			"url":        url,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Debug("proxy: GetStream request success")
		return resp, nil
	}

	logrus.WithFields(logrus.Fields{
		"url":      url,
		"duration": time.Since(start).Seconds(),
		"error":    lastErr.Error(),
	}).Error("proxy: GetStream request failed after all attempts")
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
