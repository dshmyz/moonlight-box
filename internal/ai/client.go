package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/ai/models"
	"github.com/dshmyz/moonlight-box/internal/config"
)

// defaultAITimeout 在配置未指定 ai.timeout 时的兜底超时。
const defaultAITimeout = 60 * time.Second

// AIClient 是 AI 服务的客户端
type AIClient struct {
	config     *config.AIConfig
	httpClient *http.Client
	// streamHTTPClient 用于 SSE 流式响应：不设整体 Timeout（避免切断长 token 流），
	// 但 Transport 的 ResponseHeaderTimeout 保证上游 accept 后不发响应头时不会无限挂起。
	streamHTTPClient *http.Client
}

// NewAIClient 创建一个新的 AI 客户端
func NewAIClient(cfg *config.AIConfig) *AIClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultAITimeout
	}
	streamTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	return &AIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		streamHTTPClient: &http.Client{
			Transport: streamTransport,
		},
	}
}

// ErrorResponse 表示 OpenAI 标准错误响应
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Call 执行普通的 AI 请求（非流式）
func (c *AIClient) Call(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	// 确保请求是非流式的
	req.Stream = false

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[AI-DEBUG] 请求体: %s", string(reqBody))

	// 创建 HTTP 请求
	url := strings.TrimSuffix(c.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	// 发送请求
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 检查错误响应
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s (type: %s, code: %s)",
				errResp.Error.Message, errResp.Error.Type, errResp.Error.Code)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析成功响应
	var chatResp models.ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// 打印完整的响应结构用于调试
	respJSON, _ := json.Marshal(chatResp)
	log.Printf("[AI-DEBUG] 完整响应: %s", string(respJSON))

	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		log.Printf("[AI-DEBUG] 模型返回了 %d 个工具调用", len(chatResp.Choices[0].Message.ToolCalls))
		for _, tc := range chatResp.Choices[0].Message.ToolCalls {
			log.Printf("[AI-DEBUG] 工具调用: %s, 参数: %s", tc.Function.Name, string(tc.Function.Arguments))
		}
	} else if len(chatResp.Choices) > 0 {
		log.Printf("[AI-DEBUG] 模型没有触发工具调用，finish_reason: %s, 内容: %s", chatResp.Choices[0].FinishReason, chatResp.Choices[0].Message.Content[:min(200, len(chatResp.Choices[0].Message.Content))])
	}

	return &chatResp, nil
}

// Stream 执行流式 AI 请求，返回一个channel持续推送响应块
func (c *AIClient) Stream(ctx context.Context, req *models.ChatRequest) (<-chan *models.StreamChunk, error) {
	// 创建channel，缓冲大小为100
	ch := make(chan *models.StreamChunk, 100)

	// 确保请求是流式的
	req.Stream = true

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建 HTTP 请求
	url := strings.TrimSuffix(c.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	// 发送请求
	resp, err := c.streamHTTPClient.Do(httpReq)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// 检查错误响应
	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		close(ch)
		if err != nil {
			return nil, fmt.Errorf("API returned status %d, failed to read error response: %w", resp.StatusCode, err)
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s (type: %s, code: %s)",
				errResp.Error.Message, errResp.Error.Type, errResp.Error.Code)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// 启动goroutine读取流式响应
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// 跳过空行
			if line == "" {
				continue
			}

			// 检查是否是数据行
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			// 提取数据部分
			data := strings.TrimPrefix(line, "data: ")

			// 检查是否是结束标记
			if data == "[DONE]" {
				return
			}

			// 解析流式响应块
			var chunk models.StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// JSON解析错误，发送错误chunk通知调用者
				select {
				case ch <- &models.StreamChunk{Error: err}:
				case <-ctx.Done():
				}
				return
			}

			// 发送到channel或检查context是否已取消
			select {
			case ch <- &chunk:
			case <-ctx.Done():
				return
			}
		}

		// 检查scanner错误（网络错误、读取超时等）
		if err := scanner.Err(); err != nil {
			select {
			case ch <- &models.StreamChunk{Error: err}:
			case <-ctx.Done():
			}
		}
	}()

	return ch, nil
}

// Close 关闭客户端（当前不需要清理资源，但保留接口以便将来扩展）
func (c *AIClient) Close() error {
	// 当前 http.Client 不需要显式关闭
	// 保留此方法以便将来可能需要清理资源
	return nil
}
