package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"
)

// ProviderType 定义支持的 LLM 提供商
type ProviderType string

const (
	ProviderOpenAI   ProviderType = "openai"
	ProviderTongyi   ProviderType = "tongyi"   // 通义千问
	ProviderDoubao   ProviderType = "doubao"   // 豆包
	ProviderWenxin   ProviderType = "wenxin"   // 文心一言
	ProviderDeepSeek ProviderType = "deepseek" // DeepSeek
)

// Config LLM 提供商配置
type Config struct {
	Provider    ProviderType // 提供商类型
	APIKey      string       // API Key
	BaseURL     string       // 自定义 Base URL（可选）
	Model       string       // 模型名称
	MaxTokens   int          // 最大 token 数
	Temperature float64      // 温度系数 (0.0-2.0)
}

// Client LLM 通用客户端
type Client struct {
	config       Config
	openaiClient *openai.Client
}

// NewClient 创建新的 LLM 客户端
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// 设置默认值
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}

	// 根据提供商设置默认配置
	cfg = setProviderDefaults(cfg)

	// 创建 OpenAI 客户端（兼容所有 OpenAI 协议的大模型）
	openaiCfg := openai.DefaultConfig(cfg.APIKey)

	if cfg.BaseURL != "" {
		openaiCfg.BaseURL = cfg.BaseURL
	}

	return &Client{
		config:       cfg,
		openaiClient: openai.NewClientWithConfig(openaiCfg),
	}, nil
}

// setProviderDefaults 设置提供商默认配置
func setProviderDefaults(cfg Config) Config {
	switch cfg.Provider {
	case ProviderTongyi:
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
		if cfg.Model == "" {
			cfg.Model = "qwen-plus"
		}
	case ProviderDoubao:
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://ark.cn-beijing.volces.com/api/v3"
		}
		if cfg.Model == "" {
			cfg.Model = "doubao-pro-32k"
		}
	case ProviderWenxin:
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://qianfan.baidubce.com/v2"
		}
		if cfg.Model == "" {
			cfg.Model = "ernie-4.0-8k-latest"
		}
	case ProviderDeepSeek:
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.deepseek.com/v1"
		}
		if cfg.Model == "" {
			cfg.Model = "deepseek-chat"
		}
	case ProviderOpenAI:
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
		if cfg.Model == "" {
			cfg.Model = "gpt-4o-mini"
		}
	}
	return cfg
}

// Message 对话消息
type Message struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // 消息内容
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Messages    []Message        `json:"messages"`    // 对话历史
	Temperature float64          `json:"temperature"` // 温度系数
	MaxTokens   int              `json:"max_tokens"`  // 最大 token 数
	Stream      bool             `json:"stream"`      // 是否流式输出
	Tools       []ToolDefinition `json:"tools"`       // 函数工具定义
	ToolChoice  string           `json:"tool_choice"` // 强制使用某个工具
}

// ToolDefinition 函数工具定义
type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

// ToolFunctionDefinition 函数定义
type ToolFunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema 格式
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Content      string     `json:"content"`    // 回答内容
	ToolCalls    []ToolCall `json:"tool_calls"` // 函数调用（如果有）
	FinishReason string     `json:"finish_reason"`
	Usage        TokenUsage `json:"usage"` // Token 使用统计
}

// ToolCall 函数调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON 格式的参数
	} `json:"function"`
}

// TokenUsage Token 使用统计
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat 单轮对话
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 转换消息格式
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 设置温度和最大 token
	if req.Temperature == 0 {
		req.Temperature = c.config.Temperature
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = c.config.MaxTokens
	}

	// 构建请求
	chatReq := openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: float32(req.Temperature),
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}

	// 处理函数调用
	if len(req.Tools) > 0 {
		tools := make([]openai.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			funcDef := openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			}
			tools[i] = openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &funcDef,
			}
		}
		chatReq.Tools = tools
		chatReq.ToolChoice = "auto"
	}

	// 发送请求
	resp, err := c.openaiClient.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion error: %w", err)
	}

	// 解析响应
	response := &ChatResponse{
		Usage: TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		response.Content = choice.Message.Content
		response.FinishReason = string(choice.FinishReason)

		// 处理函数调用
		if len(choice.Message.ToolCalls) > 0 {
			response.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))

			for i, tc := range choice.Message.ToolCalls {
				response.ToolCalls[i] = ToolCall{
					ID:   tc.ID,
					Type: string(tc.Type),
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}

	return response, nil
}

// ChatStream 流式对话
func (c *Client) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	// 转换消息格式
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 设置温度和最大 token
	if req.Temperature == 0 {
		req.Temperature = c.config.Temperature
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = c.config.MaxTokens
	}

	// 构建请求
	chatReq := openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: float32(req.Temperature),
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	}

	// 处理函数调用
	if len(req.Tools) > 0 {
		tools := make([]openai.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			funcDef := openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			}
			tools[i] = openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &funcDef,
			}
		}
		chatReq.Tools = tools
		chatReq.ToolChoice = "auto"
	}

	// 创建流
	stream, err := c.openaiClient.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("create stream error: %w", err)
	}

	return &StreamReader{stream: stream}, nil
}

// StreamReader 流式读取器
type StreamReader struct {
	stream *openai.ChatCompletionStream
}

// Recv 接收流式响应
func (sr *StreamReader) Recv() (*StreamResponse, error) {
	resp, err := sr.stream.Recv()
	if err != nil {
		return nil, err
	}

	result := &StreamResponse{}
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Delta.Content
		result.FinishReason = string(choice.FinishReason)
	}

	return result, nil
}

// Close 关闭流
func (sr *StreamReader) Close() error {
	return sr.stream.Close()
}

// StreamResponse 流式响应
type StreamResponse struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason"`
}

// MultiTurnChat 多轮对话管理器
type MultiTurnChat struct {
	client        *Client
	history       []Message
	SystemPrompts []Message // 系统提示词列表
}

// NewMultiTurnChat 创建多轮对话管理器
func NewMultiTurnChat(client *Client, systemPrompts ...string) *MultiTurnChat {
	mtc := &MultiTurnChat{
		client:        client,
		history:       make([]Message, 0),
		SystemPrompts: make([]Message, 0),
	}
	// 添加系统提示词
	for _, sp := range systemPrompts {
		mtc.SystemPrompts = append(mtc.SystemPrompts, Message{
			Role:    "system",
			Content: sp,
		})
	}
	return mtc
}

// Send 发送消息并获取回复
func (m *MultiTurnChat) Send(ctx context.Context, content string) (*ChatResponse, error) {
	// 添加用户消息
	m.history = append(m.history, Message{
		Role:    "user",
		Content: content,
	})

	// 构建完整消息列表
	messages := append(m.SystemPrompts, m.history...)

	// 调用 API
	resp, err := m.client.Chat(ctx, ChatRequest{
		Messages: messages,
	})

	if err != nil {
		return nil, err
	}

	// 添加助手回复到历史
	if resp.Content != "" {
		m.history = append(m.history, Message{
			Role:    "assistant",
			Content: resp.Content,
		})
	}

	return resp, nil
}

// SendStream 流式发送消息
func (m *MultiTurnChat) SendStream(ctx context.Context, content string) (*StreamReader, error) {
	// 添加用户消息
	m.history = append(m.history, Message{
		Role:    "user",
		Content: content,
	})

	// 构建完整消息列表
	messages := append(m.SystemPrompts, m.history...)

	// 调用流式 API
	stream, err := m.client.ChatStream(ctx, ChatRequest{
		Messages: messages,
	})

	if err != nil {
		return nil, err
	}

	return stream, nil
}

// GetHistory 获取对话历史
func (m *MultiTurnChat) GetHistory() []Message {
	result := make([]Message, len(m.history))
	copy(result, m.history)
	return result
}

// ClearHistory 清除对话历史
func (m *MultiTurnChat) ClearHistory() {
	m.history = make([]Message, 0)
}

// AddToolResult 添加工具执行结果到对话
func (m *MultiTurnChat) AddToolResult(toolCallID, result string) {
	m.history = append(m.history, Message{
		Role:    "tool",
		Content: result,
	})
}

// Getenv 获取环境变量（便捷方法）
func Getenv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
