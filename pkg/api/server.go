package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golllm/pkg/llm"
	"github.com/golllm/pkg/prompt"
)

// Server HTTP API 服务器
type Server struct {
	llmClient     *llm.Client
	promptManager *prompt.Manager
	addr          string
}

// NewServer 创建新的 API 服务器
func NewServer(llmClient *llm.Client) *Server {
	pm := prompt.NewManager()
	pm.RegisterDefaultTemplates()

	return &Server{
		llmClient:     llmClient,
		promptManager: pm,
		addr:          ":8080",
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 启用 CORS
	r.Use(corsMiddleware())

	// 注册路由
	r.GET("/health", s.healthHandler)
	r.POST("/chat", s.chatHandler)
	r.POST("/chat/stream", s.chatStreamHandler)
	r.POST("/chat/multi", s.multiChatHandler)
	r.POST("/chat/multi/stream", s.multiChatStreamHandler)
	r.POST("/tools/call", s.toolCallHandler)
	r.GET("/templates", s.listTemplatesHandler)
	r.POST("/templates/render", s.renderTemplateHandler)

	return r.Run(s.addr)
}

// SetAddress 设置服务器地址
func (s *Server) SetAddress(addr string) {
	s.addr = addr
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// healthHandler 健康检查
func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "ok",
		"service": "golllm-api",
	})
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model       string            `json:"model"`
	Message     string            `json:"message"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
	SystemPrompt string           `json:"system_prompt"`
}

// chatHandler 单轮对话
func (s *Server) chatHandler(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// 构建消息
	messages := []llm.Message{}
	if req.SystemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: req.Message,
	})

	// 调用 LLM
	resp, err := s.llmClient.Chat(ctx, llm.ChatRequest{
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"content":  resp.Content,
		"usage":    resp.Usage,
		"finish_reason": resp.FinishReason,
	})
}

// chatStreamHandler 流式对话
func (s *Server) chatStreamHandler(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	// 构建消息
	messages := []llm.Message{}
	if req.SystemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: req.Message,
	})

	// 创建流
	stream, err := s.llmClient.ChatStream(ctx, llm.ChatRequest{
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer stream.Close()

	// 设置 SSE Headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// 流式返回
	fullContent := ""
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(c.Writer, "data: [ERROR] %s\n\n", err.Error())
			c.Writer.Flush()
			break
		}

		fullContent += resp.Content
		fmt.Fprintf(c.Writer, "data: %s\n\n", resp.Content)
		c.Writer.Flush()
	}

	// 发送完成信号
	fmt.Fprintf(c.Writer, "data: [DONE] %s\n\n", fullContent)
	c.Writer.Flush()
}

// MultiChatRequest 多轮对话请求
type MultiChatRequest struct {
	Messages    []llm.Message `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Tools       []llm.ToolDefinition `json:"tools"`
}

// multiChatHandler 多轮对话
func (s *Server) multiChatHandler(c *gin.Context) {
	var req MultiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// 调用 LLM
	resp, err := s.llmClient.Chat(ctx, llm.ChatRequest{
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Tools:       req.Tools,
	})

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, resp)
}

// multiChatStreamHandler 多轮流式对话
func (s *Server) multiChatStreamHandler(c *gin.Context) {
	var req MultiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	// 创建流
	stream, err := s.llmClient.ChatStream(ctx, llm.ChatRequest{
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Tools:       req.Tools,
		Stream:      true,
	})

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer stream.Close()

	// 设置 SSE Headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 流式返回
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
			break
		}
		if err != nil {
			fmt.Fprintf(c.Writer, "data: [ERROR] %s\n\n", err.Error())
			c.Writer.Flush()
			break
		}

		data, _ := json.Marshal(resp)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}
}

// ToolCallRequest 工具调用请求
type ToolCallRequest struct {
	Messages    []llm.Message      `json:"messages"`
	Tools       []llm.ToolDefinition `json:"tools"`
	Temperature float64             `json:"temperature"`
}

// toolCallHandler 函数调用入口
func (s *Server) toolCallHandler(c *gin.Context) {
	var req ToolCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// 调用 LLM
	resp, err := s.llmClient.Chat(ctx, llm.ChatRequest{
		Messages:    req.Messages,
		Temperature: req.Temperature,
		Tools:       req.Tools,
	})

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// 返回响应（包含工具调用信息）
	c.JSON(200, gin.H{
		"content":     resp.Content,
		"tool_calls":  resp.ToolCalls,
		"finish_reason": resp.FinishReason,
		"usage":        resp.Usage,
	})
}

// listTemplatesHandler 列出所有模板
func (s *Server) listTemplatesHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"templates": []string{"assistant", "coder", "analyst", "translator", "question", "code_review", "summary"},
	})
}

// RenderTemplateRequest 渲染模板请求
type RenderTemplateRequest struct {
	Name string            `json:"name"`
	Vars map[string]string `json:"vars"`
}

// renderTemplateHandler 渲染模板
func (s *Server) renderTemplateHandler(c *gin.Context) {
	var req RenderTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	result, err := s.promptManager.Render(req.Name, req.Vars)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"result": result,
	})
}


