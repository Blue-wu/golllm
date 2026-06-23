package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/golllm/pkg/llm"
	"github.com/golllm/pkg/prompt"
)

// 演示如何在代码中使用 llm 包
func main() {
	fmt.Println("=== Go LLM 大模型学习 Demo ===\n")

	// 从环境变量或直接设置 API Key
	apiKey := llm.Getenv("OPENAI_API_KEY", "sk-79a7e98d06644504935eec600a7474e1")
	provider := llm.ProviderType(llm.Getenv("LLM_PROVIDER", "deepseek"))

	// 创建 LLM 客户端
	client, err := llm.NewClient(llm.Config{
		Provider:    provider,
		APIKey:      apiKey,
		MaxTokens:   1024,
		Temperature: 0.7,
	})

	if err != nil {
		log.Fatalf("创建 LLM 客户端失败: %v", err)
	}

	// 运行演示
	demoSimpleChat(client)
	demoStreamingChat(client)
	demoMultiTurnChat(client)
	demoFunctionCalling(client)
	demoPromptTemplate()
}

// demoSimpleChat 单轮对话演示
func demoSimpleChat(client *llm.Client) {
	fmt.Println("【Demo 1】单轮对话")
	fmt.Println("----------------")

	ctx := context.Background()

	resp, err := client.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "你好，请用一句话介绍 Go 语言"},
		},
	})
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("回答: %s\n", resp.Content)
	fmt.Printf("Token 使用: %d\n\n", resp.Usage.TotalTokens)
}

// demoStreamingChat 流式对话演示
func demoStreamingChat(client *llm.Client) {
	fmt.Println("【Demo 2】流式对话")
	fmt.Println("----------------")

	ctx := context.Background()

	stream, err := client.ChatStream(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "用三句话介绍一下 Rust 语言的特点"},
		},
	})
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	defer stream.Close()

	fmt.Print("回答: ")
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		fmt.Print(resp.Content)

	}
	fmt.Println("\n")
}

// demoMultiTurnChat 多轮对话演示
func demoMultiTurnChat(client *llm.Client) {
	fmt.Println("【Demo 3】多轮对话")
	fmt.Println("----------------")

	ctx := context.Background()

	// 创建多轮对话管理器，设置系统提示词
	chat := llm.NewMultiTurnChat(client,
		"你是一个专业的编程助手，擅长解释技术概念。",
	)

	// 第一轮
	fmt.Println("用户: 什么是 Goroutine?")
	resp, err := chat.Send(ctx, "什么是 Goroutine?")
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	fmt.Printf("助手: %s\n\n", resp.Content)

	// 第二轮（带上下文）
	fmt.Println("用户: 它和线程有什么区别?")
	resp, err = chat.Send(ctx, "它和线程有什么区别?")
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	fmt.Printf("助手: %s\n\n", resp.Content)

	// 查看对话历史
	fmt.Println("对话历史长度:", len(chat.GetHistory()))

	// 清除历史
	chat.ClearHistory()
	fmt.Println("已清除对话历史\n")
	fmt.Printf("Token 使用: %d\n\n", resp.Usage.TotalTokens)
}

// demoFunctionCalling 函数调用演示
func demoFunctionCalling(client *llm.Client) {
	fmt.Println("【Demo 4】函数调用（Function Call）")
	fmt.Println("----------------")

	ctx := context.Background()

	// 定义函数工具
	tools := []llm.ToolDefinition{
		{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        "get_weather",
				Description: "获取指定城市的天气信息",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type":        "string",
							"description": "城市名称",
						},
						"unit": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"celsius", "fahrenheit"},
							"description": "温度单位",
						},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	// 发送需要调用函数的请求
	resp, err := client.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "北京今天天气怎么样？适合出门吗？"},
		},
		Tools: tools,
	})

	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	// 检查是否有函数调用
	if len(resp.ToolCalls) > 0 {
		fmt.Println("助手触发函数调用:")
		for _, tc := range resp.ToolCalls {
			fmt.Printf("  函数名: %s\n", tc.Function.Name)
			fmt.Printf("  参数: %s\n", tc.Function.Arguments)
		}
		fmt.Println()
		fmt.Printf("Token 使用: %d\n\n", resp.Usage.TotalTokens)
		// 模拟执行函数
		fmt.Println("模拟执行函数:")
		for _, tc := range resp.ToolCalls {
			if tc.Function.Name == "get_weather" {
				//fmt.Println("  执行结果: 北京今天晴，温度 25°C，适合出门！")
				fmt.Printf("助手: %s\n", resp.Content)
			}
		}
	} else {
		fmt.Printf("助手: %s\n", resp.Content)
	}
	fmt.Println()
}

// demoPromptTemplate Prompt 模板演示
func demoPromptTemplate() {
	fmt.Println("【Demo 5】Prompt 模板管理")
	fmt.Println("----------------")

	pm := prompt.NewManager()
	pm.RegisterDefaultTemplates()

	// 渲染代码助手模板
	result, err := pm.Render("coder", map[string]string{
		"language": "Go",
	})
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Println("渲染后的代码助手提示词:")
	fmt.Println(result)
	fmt.Println()

	// 渲染翻译模板
	result, err = pm.Render("translator", map[string]string{
		"from_lang": "English",
		"to_lang":   "中文",
	})
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Println("渲染后的翻译提示词:")
	fmt.Println(result)
	fmt.Println()
}

// 辅助函数：读取用户输入
func readUserInput(promptText string) string {
	fmt.Print(promptText)
	var input string
	fmt.Scanln(&input)
	return input
}

// 环境变量检查
func checkEnvVars() {
	fmt.Println("=== 环境变量检查 ===")
	fmt.Printf("OPENAI_API_KEY: %s\n", maskString(llm.Getenv("OPENAI_API_KEY", "")))
	fmt.Printf("LLM_PROVIDER: %s\n", llm.Getenv("LLM_PROVIDER", "openai"))
	fmt.Println()
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// 修改后的 main 函数，包含交互式演示
func mainInteractive() {
	fmt.Println("=== Go LLM 大模型交互式 Demo ===")
	fmt.Println("请确保设置了环境变量 OPENAI_API_KEY")
	fmt.Println()

	// 创建客户端
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("错误: 请设置 OPENAI_API_KEY 环境变量")
		fmt.Println("示例: export OPENAI_API_KEY=sk-xxxxxx")
		return
	}

	client, err := llm.NewClient(llm.Config{
		Provider:    llm.ProviderOpenAI,
		APIKey:      apiKey,
		MaxTokens:   2048,
		Temperature: 0.7,
	})
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	ctx := context.Background()

	// 交互式问答循环
	for {
		fmt.Print("\n请输入问题（输入 q 退出）: ")
		var question string
		fmt.Scanln(&question)

		if question == "q" || question == "quit" {
			break
		}

		if question == "" {
			continue
		}

		// 单轮对话
		resp, err := client.Chat(ctx, llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "user", Content: question},
			},
		})
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			continue
		}

		fmt.Printf("\n回答: %s\n", resp.Content)
		fmt.Printf("Token 使用: %d\n", resp.Usage.TotalTokens)
	}
}
