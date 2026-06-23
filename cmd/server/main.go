package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golllm/pkg/api"
	"github.com/golllm/pkg/llm"
)

func main() {
	fmt.Println("=== Go LLM HTTP API Server ===")
	fmt.Println()

	// 配置
	apiKey := llm.Getenv("OPENAI_API_KEY", "sk-79a7e98d06644504935eec600a7474e1")
	providerStr := llm.Getenv("LLM_PROVIDER", "openai")
	addr := llm.Getenv("SERVER_ADDR", ":8080")
	model := llm.Getenv("LLM_MODEL", "")
	enableMock := llm.Getenv("ENABLE_MOCK", "") != ""

	// 检查 API Key
	if apiKey == "" && !enableMock {
		fmt.Println("警告: 未设置 OPENAI_API_KEY，启用 Mock 模式")
		fmt.Println()
		fmt.Println("支持的提供商:")
		fmt.Println("  - openai: OpenAI GPT 系列")
		fmt.Println("  - tongyi: 通义千问")
		fmt.Println("  - doubao: 豆包")
		fmt.Println("  - wenxin: 文心一言")
		fmt.Println("  - deepseek: DeepSeek")
		fmt.Println()
		fmt.Println("环境变量说明:")
		fmt.Println("  OPENAI_API_KEY - API Key（必填，除非启用 Mock）")
		fmt.Println("  LLM_PROVIDER   - 提供商 (默认: openai)")
		fmt.Println("  LLM_MODEL      - 模型名称（可选）")
		fmt.Println("  SERVER_ADDR    - 服务地址 (默认: :8080)")
		fmt.Println("  ENABLE_MOCK    - 启用 Mock 模式（无需 API Key）")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  export OPENAI_API_KEY=sk-xxxxxx")
		fmt.Println("  go run cmd/server/main.go")
		enableMock = true
	}

	// Mock 模式下使用测试 Key
	if enableMock && apiKey == "" {
		apiKey = "mock-key-for-testing"
	}

	// 解析提供商
	var provider llm.ProviderType
	switch providerStr {
	case "openai":
		provider = llm.ProviderOpenAI
	case "tongyi", "qwen":
		provider = llm.ProviderTongyi
	case "doubao", "volcengine":
		provider = llm.ProviderDoubao
	case "wenxin", "ernie":
		provider = llm.ProviderWenxin
	case "deepseek":
		provider = llm.ProviderDeepSeek
	default:
		provider = llm.ProviderOpenAI
	}

	// 创建 LLM 客户端
	cfg := llm.Config{
		Provider:    provider,
		APIKey:      apiKey,
		MaxTokens:   2048,
		Temperature: 0.7,
	}
	if model != "" {
		cfg.Model = model
	}

	client, err := llm.NewClient(cfg)
	if err != nil {
		log.Fatalf("创建 LLM 客户端失败: %v", err)
	}

	fmt.Printf("配置信息:\n")
	fmt.Printf("  提供商: %s\n", provider)
	fmt.Printf("  模型: %s\n", cfg.Model)
	fmt.Printf("  服务地址: %s\n", addr)
	if enableMock {
		fmt.Printf("  模式: Mock（演示用）\n")
	}
	fmt.Println()

	// 创建并启动服务器
	server := api.NewServer(client)
	server.SetAddress(addr)

	fmt.Println("启动 HTTP API 服务器...")
	fmt.Println("API 端点:")
	fmt.Println("  GET  /health              - 健康检查")
	fmt.Println("  POST /chat                - 单轮对话")
	fmt.Println("  POST /chat/stream         - 流式对话 (SSE)")
	fmt.Println("  POST /chat/multi          - 多轮对话")
	fmt.Println("  POST /chat/multi/stream   - 多轮流式对话")
	fmt.Println("  POST /tools/call          - 函数调用")
	fmt.Println("  GET  /templates           - 列出模板")
	fmt.Println("  POST /templates/render    - 渲染模板")
	fmt.Println()
	fmt.Printf("服务器监听: %s\n", addr)
	fmt.Println("按 Ctrl+C 停止服务器")
	fmt.Println()

	if err := server.Start(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

var _ = os.Getenv