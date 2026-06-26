package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// Config Embedding 客户端配置
type Config struct {
	APIKey   string // API 密钥
	BaseURL  string // API 基础 URL（用于自定义提供商）
	Model    string // 模型名称
	Provider string // 提供商：openai, deepseek, ollama 等
}

// Client Embedding 客户端
type Client struct {
	config Config         // 客户端配置
	openai *openai.Client // OpenAI SDK 客户端（兼容 OpenAI 协议）
}

// NewClient 创建新的 Embedding 客户端
// 支持的提供商：
//   - openai: OpenAI 官方 API
//   - deepseek: DeepSeek API（兼容 OpenAI 协议）
//   - ollama: Ollama 本地模型（无需 API Key）
//   - 自定义: 通过 BaseURL 指定
func NewClient(cfg Config) *Client {
	var client *openai.Client

	switch cfg.Provider {
	case "deepseek":
		// DeepSeek 使用兼容 OpenAI 协议的 API
		config := openai.DefaultConfig(cfg.APIKey)
		config.BaseURL = "https://api.deepseek.com/v1"
		client = openai.NewClientWithConfig(config)
	case "ollama":
		// Ollama 本地服务，默认地址 http://localhost:11434
		// 使用 OpenAI 兼容接口
		config := openai.DefaultConfig("ollama")
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		config.BaseURL = cfg.BaseURL
		client = openai.NewClientWithConfig(config)
	case "openai":
		// OpenAI 官方 API
		client = openai.NewClient(cfg.APIKey)
	default:
		// 自定义提供商（兼容 OpenAI 协议）
		config := openai.DefaultConfig(cfg.APIKey)
		config.BaseURL = cfg.BaseURL
		client = openai.NewClientWithConfig(config)
	}

	return &Client{
		config: cfg,
		openai: client,
	}
}

// Embeddings 批量生成文本向量
// 参数:
//   - ctx: 上下文
//   - texts: 文本列表
//
// 返回: 向量列表（每个文本对应一个向量）
func (c *Client) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	// Ollama 使用原生 API（更稳定）
	if c.config.Provider == "ollama" {
		return c.ollamaEmbeddings(ctx, texts)
	}

	// 其他提供商使用 OpenAI SDK
	req := openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(c.config.Model),
		Input: texts,
	}

	resp, err := c.openai.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	vectors := make([][]float32, 0, len(resp.Data))
	for _, item := range resp.Data {
		vectors = append(vectors, item.Embedding)
	}

	return vectors, nil
}

// ollamaEmbeddings 使用 Ollama 原生 API 生成向量
func (c *Client) ollamaEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	baseURL := "http://localhost:11434"
	if c.config.BaseURL != "" {
		baseURL = strings.TrimSuffix(c.config.BaseURL, "/v1")
	}

	vectors := make([][]float32, 0, len(texts))

	for _, text := range texts {
		// Ollama embed API (新版)
		reqBody := map[string]interface{}{
			"model": c.config.Model,
			"input": text,
		}

		reqBytes, _ := json.Marshal(reqBody)

		url := baseURL + "/api/embed"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("ollama error: %s", string(body))
		}

		var result struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		err = json.Unmarshal(body, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		if len(result.Embeddings) > 0 {
			vectors = append(vectors, result.Embeddings[0])
		}
	}

	return vectors, nil
}

// Embedding 生成单个文本的向量
// 参数:
//   - ctx: 上下文
//   - text: 文本
//
// 返回: 向量
func (c *Client) Embedding(ctx context.Context, text string) ([]float32, error) {
	vectors, err := c.Embeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) > 0 {
		return vectors[0], nil
	}
	return nil, fmt.Errorf("no embedding returned")
}

// GetDimension 获取当前模型的向量维度
// 不同模型的维度：
//   - text-embedding-3-small: 1536
//   - text-embedding-3-large: 3072
//   - text-embedding-ada-002: 1536
//   - deepseek-chat: 1024
//   - deepseek-embedding: 1024
//   - nomic-embed-text: 768
//   - bge-m3: 1024
func (c *Client) GetDimension() int {
	switch c.config.Model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	case "deepseek-chat":
		return 1024
	case "deepseek-embedding":
		return 1024
	case "nomic-embed-text":
		return 768
	case "bge-m3":
		return 1024
	default:
		return 768 // Ollama 默认
	}
}

// TextChunk 文本片段（用于 RAG 召回）
type TextChunk struct {
	Text   string  `json:"text"`   // 文本内容
	Score  float32 `json:"score"`  // 相似度分数
	Source string  `json:"source"` // 来源文档
	Index  int     `json:"index"`  // 在文档中的位置
}
