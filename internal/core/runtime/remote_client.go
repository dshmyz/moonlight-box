package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPRemoteClient 适配器：将通用 HTTP 能力适配为 runtime.RemoteClient 接口
// 用于 ProxyRuntime 从远程仓库获取元数据和 blob
type HTTPRemoteClient struct {
	client *http.Client
}

// NewHTTPRemoteClient 创建 HTTPRemoteClient。
// 如果传入的 client 非 nil，使用它（应来自 proxy.TransportManager，含 DNS 映射和 TLS 配置）；
// 否则使用默认裸客户端。
func NewHTTPRemoteClient(client *http.Client) *HTTPRemoteClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPRemoteClient{client: client}
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
	}
	return meta, err
}

func (c *HTTPRemoteClient) fetchMetadataWithMethod(ctx context.Context, method, remoteURL string) (*RemoteMetadata, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, remoteURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

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

	resp, err := c.client.Do(req)
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
