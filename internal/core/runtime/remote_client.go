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

func NewHTTPRemoteClient() *HTTPRemoteClient {
	return &HTTPRemoteClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *HTTPRemoteClient) FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error) {
	remoteURL := key.RemoteURL
	if remoteURL == "" {
		return nil, fmt.Errorf("remote URL not configured for artifact key: %s", key.String())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, remoteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &RemoteMetadata{Exists: false}, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	meta := &RemoteMetadata{
		Exists: true,
		Size:   resp.ContentLength,
	}
	if digest := resp.Header.Get("ETag"); digest != "" {
		digest = strings.Trim(digest, `"`)
		meta.Digest = digest
	}
	if modTime, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		meta.ModifiedAt = modTime
	}

	return meta, nil
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
