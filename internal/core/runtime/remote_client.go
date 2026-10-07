package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
)

var (
	// streamTTFBTimeout 限制流式请求（FetchBlob/Open）从发起到收到响应头的最长时间。
	// 用变量以便测试缩短。
	streamTTFBTimeout = 30 * time.Second
	// streamIdleTimeout 是流式响应体相邻两次 Read 之间的最长空闲；
	// 上游断流超过该值判死，防止 goroutine 与连接被无限占用。
	streamIdleTimeout = 60 * time.Second
)

// HTTPRemoteClient 适配器：将通用 HTTP 能力适配为 runtime.RemoteClient 接口
// 用于 ProxyRuntime 从远程仓库获取元数据和 blob
type HTTPRemoteClient struct {
	// client 带 Client.Timeout，用于小体积的 metadata 请求（HEAD/GET 元数据）。
	client *http.Client
	// stream 无总超时，用于 blob 与不透明流的响应体消费（大文件）。
	// 超时控制改为 TTFB 定时器 + 响应体读空闲，见 doStream。
	stream *http.Client
}

// NewHTTPRemoteClient 创建 HTTPRemoteClient。
// 如果传入的 client 非 nil，使用它（应来自 proxy.TransportManager，含 DNS 映射和 TLS 配置）；
// 否则使用默认裸客户端。
func NewHTTPRemoteClient(client *http.Client) *HTTPRemoteClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	// 流式变体共享 Transport（连接池/DNS 映射/TLS 配置），但去掉总超时：
	// http.Client.Timeout 从发起请求一直管到响应体读完，大文件流式下载必然被腰斩。
	stream := &http.Client{Transport: client.Transport}
	return &HTTPRemoteClient{client: client, stream: stream}
}

// doStream 发起流式请求。超时控制分两层，替代会被大文件触发的 Client.Timeout：
//  1. TTFB：ctx 本身不带 deadline（ctx 级 deadline 会延伸到响应体读取，腰斩传输）；
//     改用 WithCancelCause + 定时器，streamTTFBTimeout 内未收到响应头才取消，
//     响应头到达后立即停表。
//  2. 响应体：idleTimeoutBody 施加读空闲超时，上游断流超过 streamIdleTimeout 判死。
func doStream(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(streamTTFBTimeout, func() {
		cancel(fmt.Errorf("upstream TTFB timeout after %s", streamTTFBTimeout))
	})
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		timer.Stop()
		cancel(nil)
		if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
			return nil, cause
		}
		return nil, err
	}
	// 响应头已到达：解除 TTFB 定时。cancel 不能 defer——ctx 关联着响应体生命周期，
	// 由 idleTimeoutBody 在读终止 / Close 时收尾。
	timer.Stop()
	resp.Body = newIdleTimeoutBody(resp.Body, cancel)
	return resp, nil
}

type readResult struct {
	data []byte
	err  error
}

// idleTimeoutBody 给流式响应体施加读空闲超时：客户端侧拿不到底层连接的
// deadline（http.ResponseController 仅限服务端），因此用常驻读 goroutine
// 把 body.Read 与定时器 select——上游断流超过 streamIdleTimeout 返回错误
// 并释放连接，慢速但持续的传输不受任何总时长限制。
type idleTimeoutBody struct {
	body    io.ReadCloser
	cancel  context.CancelCauseFunc
	reads   chan readResult
	closing chan struct{}
	pending []byte // 上次读出的未消费数据（内部缓冲可能大于调用方 p）
}

func newIdleTimeoutBody(body io.ReadCloser, cancel context.CancelCauseFunc) *idleTimeoutBody {
	b := &idleTimeoutBody{
		body:    body,
		cancel:  cancel,
		reads:   make(chan readResult, 1),
		closing: make(chan struct{}),
	}
	util.SafeGo("runtime.remote-client.read-loop", func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := body.Read(buf)
			out := make([]byte, n)
			copy(out, buf[:n])
			select {
			case b.reads <- readResult{data: out, err: err}:
			case <-b.closing:
				return
			}
			if err != nil {
				return
			}
		}
	})
	return b
}

// terminate 一次性关闭读循环并解除 ctx 关联。
func (b *idleTimeoutBody) terminate(cause error) {
	select {
	case <-b.closing:
	default:
		close(b.closing)
	}
	b.cancel(cause)
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	if len(b.pending) > 0 {
		n := copy(p, b.pending)
		b.pending = b.pending[n:]
		return n, nil
	}
	select {
	case res := <-b.reads:
		b.pending = res.data
		n := copy(p, b.pending)
		b.pending = b.pending[n:]
		if res.err != nil {
			// 读终止（EOF/断流）：解除 ctx 关联释放连接。
			b.terminate(res.err)
			return n, res.err
		}
		return n, nil
	case <-b.closing:
		b.cancel(nil)
		return 0, http.ErrBodyReadAfterClose
	case <-time.After(streamIdleTimeout):
		err := fmt.Errorf("upstream read idle timeout after %s", streamIdleTimeout)
		b.terminate(err)
		return 0, err
	}
}

func (b *idleTimeoutBody) Close() error {
	b.terminate(nil)
	return b.body.Close()
}

// Open sends a raw upstream request and preserves its response for the caller.
// HTTP status codes are returned as responses; only request construction and
// transport failures are returned as errors.
func (c *HTTPRemoteClient) Open(ctx context.Context, request RemoteRequest) (*RemoteResponse, error) {
	req, err := http.NewRequestWithContext(ctx, request.Method, request.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header = request.Headers.Clone()

	client := *c.stream
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := doStream(ctx, &client, req)
	if err != nil {
		return nil, fmt.Errorf("opening remote response: %w", err)
	}

	return &RemoteResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       resp.Body,
	}, nil
}

func (c *HTTPRemoteClient) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	remoteURL := key.RemoteURL
	if remoteURL == "" {
		return nil, fmt.Errorf("remote URL not configured for artifact key: %s", key.String())
	}

	meta, status, err := c.fetchMetadataWithMethod(ctx, http.MethodHead, remoteURL)
	if err == nil && status < 400 {
		return meta, nil
	}
	if status == http.StatusMethodNotAllowed || status == http.StatusForbidden || status == http.StatusUnauthorized || status >= 500 {
		getMeta, _, getErr := c.fetchMetadataWithMethod(ctx, http.MethodGet, remoteURL)
		if getErr == nil {
			return getMeta, nil
		}
		return nil, getErr
	}
	return meta, err
}

func (c *HTTPRemoteClient) fetchMetadataWithMethod(ctx context.Context, method, remoteURL string) (*RemoteMetadata, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, remoteURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

	// metadata 响应体很小，保留 Client.Timeout 的整体保护是正确行为。
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &RemoteMetadata{Exists: false}, resp.StatusCode, nil
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	meta := &RemoteMetadata{
		Exists: true,
		Size:   resp.ContentLength,
	}
	if digest := resp.Header.Get("ETag"); digest != "" {
		digest = strings.Trim(digest, `"`)
		meta.ETag = digest
		meta.Digest = digest
	}
	if modTime, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		meta.ModifiedAt = modTime
	}

	return meta, resp.StatusCode, nil
}

func (c *HTTPRemoteClient) FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	remoteURL := key.RemoteURL
	if remoteURL == "" {
		return nil, fmt.Errorf("remote URL not configured for artifact key: %s", key.String())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := doStream(ctx, c.stream, req)
	if err != nil {
		return nil, fmt.Errorf("fetching blob: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}
