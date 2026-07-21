package ai

import (
	"net/http"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
)

func TestAIClientFallbackTimeoutWhenUnset(t *testing.T) {
	// ai.timeout 未配置（零值）时，NewAIClient 必须兜底，避免 http.Client 无超时导致 LLM 调用挂死。
	c := NewAIClient(&config.AIConfig{Timeout: 0})
	if c.httpClient.Timeout != defaultAITimeout {
		t.Fatalf("httpClient.Timeout = %v, want default %v", c.httpClient.Timeout, defaultAITimeout)
	}
}

func TestAIClientRespectsConfiguredTimeout(t *testing.T) {
	c := NewAIClient(&config.AIConfig{Timeout: 5 * time.Second})
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("httpClient.Timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestAIClientStreamClientHasHeaderTimeoutButNoOverallTimeout(t *testing.T) {
	// 流式 SSE 不能用整体 Timeout（会切断长 token 流），但 Transport 必须有
	// ResponseHeaderTimeout 兜底"上游 accept 后不发响应头"的挂死。
	c := NewAIClient(&config.AIConfig{Timeout: 0})
	if c.streamHTTPClient == nil {
		t.Fatal("streamHTTPClient must be initialized")
	}
	if c.streamHTTPClient.Timeout != 0 {
		t.Fatalf("streamHTTPClient.Timeout = %v, want 0 (no overall timeout on SSE body)", c.streamHTTPClient.Timeout)
	}
	tr, ok := c.streamHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("streamHTTPClient.Transport = %T, want *http.Transport", c.streamHTTPClient.Transport)
	}
	if tr.ResponseHeaderTimeout != defaultAITimeout {
		t.Fatalf("ResponseHeaderTimeout = %v, want default %v", tr.ResponseHeaderTimeout, defaultAITimeout)
	}
}
