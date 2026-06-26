package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golllm/pkg/embedding"
)

// Client LLM 客户端接口
type Client interface {
	// Chat 发送对话请求
	Chat(ctx context.Context, messages []Message) (string, error)
	//    ChatStream 发送流式对话请求
	ChatStream(ctx context.Context, messages []Message, callback func(string)) error
	// GetModel 获取当前模型名称
	GetModel() string
}

// Message 对话消息
type Message struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // 消息内容
}

// OllamaClient Ollama 本地模型客户端
type OllamaClient struct {
	model   string
	baseURL string
	client  *http.Client
}

// NewOllamaClient 创建 Ollama 客户端
func NewOllamaClient(model string) *OllamaClient {
	return &OllamaClient{
		model:   model,
		baseURL: "http://localhost:11434",
		client: &http.Client{
			Timeout: 120 * time.Second, // 本地模型响应较慢
		},
	}
}

// Chat 发送对话请求
func (c *OllamaClient) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := map[string]interface{}{
		"model":    c.model,
		"messages": messages,
		"stream":   false,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewBuffer(reqBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama request failed: %s", string(body))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Message.Content, nil
}

// ChatStream 发送流式对话请求
func (c *OllamaClient) ChatStream(ctx context.Context, messages []Message, callback func(string)) error {
	reqBody := map[string]interface{}{
		"model":    c.model,
		"messages": messages,
		"stream":   true,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewBuffer(reqBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama request failed: %s", string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := decoder.Decode(&chunk); err != nil {
			break
		}
		if chunk.Message.Content != "" {
			callback(chunk.Message.Content)
		}
		if chunk.Done {
			break
		}
	}

	return nil
}

// GetModel 获取当前模型名称
func (c *OllamaClient) GetModel() string {
	return c.model
}

// OpenAIClient OpenAI 兼容 API 客户端
type OpenAIClient struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIClient 创建 OpenAI 客户端
func NewOpenAIClient(model, apiKey, baseURL string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIClient{
		model:   model,
		apiKey:  apiKey,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Chat 发送对话请求
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := map[string]interface{}{
		"model":    c.model,
		"messages": messages,
		"stream":   false,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(reqBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai request failed: %s", string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}

	return result.Choices[0].Message.Content, nil
}

// ChatStream 发送流式对话请求
func (c *OpenAIClient) ChatStream(ctx context.Context, messages []Message, callback func(string)) error {
	reqBody := map[string]interface{}{
		"model":    c.model,
		"messages": messages,
		"stream":   true,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(reqBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai request failed: %s", string(body))
	}

	reader := resp.Body
	buffer := make([]byte, 0, 1024)
	for {
		buf := make([]byte, 1024)
		n, err := reader.Read(buf)
		if n > 0 {
			buffer = append(buffer, buf[:n]...)
			// 处理 SSE 格式
			for {
				content, ok := parseSSELine(buffer)
				if ok {
					callback(content)
					buffer = buffer[len(buffer):]
				} else {
					break
				}
			}
		}
		if err != nil {
			break
		}
	}

	return nil
}

// GetModel 获取当前模型名称
func (c *OpenAIClient) GetModel() string {
	return c.model
}

// parseSSELine 解析 SSE 行
func parseSSELine(data []byte) (string, bool) {
	line := string(data)
	if len(line) < 6 {
		return "", false
	}
	// 查找 data: 开头
	for i := 0; i < len(line)-6; i++ {
		if line[i:i+6] == "data: " {
			content := line[i+6:]
			// 查找结束标记
			for j, ch := range content {
				if ch == '\n' || ch == '\r' {
					content = content[:j]
					break
				}
			}
			if content == "[DONE]" {
				return "", false
			}
			// 解析 JSON
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(content), &chunk) == nil && len(chunk.Choices) > 0 {
				return chunk.Choices[0].Delta.Content, true
			}
		}
	}
	return "", false
}

// NewClient 根据配置创建 LLM 客户端
func NewClient(provider, model, apiKey, baseURL string) (Client, error) {
	switch provider {
	case "ollama":
		return NewOllamaClient(model), nil
	case "openai", "deepseek":
		return NewOpenAIClient(model, apiKey, baseURL), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// GenerateContext 生成带上下文的 Prompt
func GenerateContext(query string, chunks []embedding.TextChunk) string {
	if len(chunks) == 0 {
		return query
	}

	context := "请根据以下参考资料回答问题：\n\n"
	for i, chunk := range chunks {
		context += fmt.Sprintf("【参考资料 %d】\n%s\n\n", i+1, chunk.Text)
	}
	context += fmt.Sprintf("\n问题：%s", query)

	return context
}
