package stability

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/golllm/cmd/rag/llm"
)

// FallbackConfig 降级配置参数
type FallbackConfig struct {
	Enabled          bool   // 是否启用降级
	FallbackProvider string // 降级模型提供商（如 ollama、openai）
	FallbackModel    string // 降级模型名称
}

// FallbackHandler 降级处理器
// 当主模型调用失败时，自动切换到备用模型处理请求
type FallbackHandler struct {
	config         FallbackConfig // 配置参数
	fallbackClient llm.Client     // 降级模型客户端
	mu             sync.RWMutex   // 读写锁，保证并发安全
	inFallback     bool           // 当前是否处于降级模式
}

// NewFallbackHandler 创建降级处理器实例
// 参数：
//   - config: 降级配置
//   - fallbackClient: 降级模型客户端，需实现 llm.Client 接口
func NewFallbackHandler(config FallbackConfig, fallbackClient llm.Client) *FallbackHandler {
	return &FallbackHandler{
		config:         config,
		fallbackClient: fallbackClient,
	}
}

// Execute 执行主模型调用，失败时自动降级
// 流程：
//  1. 执行主模型调用
//  2. 如果成功，直接返回结果
//  3. 如果失败且错误是可降级的，切换到降级模型
//
// 参数：
//   - ctx: 上下文
//   - messages: 对话消息
//   - primaryFn: 主模型调用函数
//
// 返回：
//   - string: 模型响应内容
//   - error: 错误信息
func (fh *FallbackHandler) Execute(ctx context.Context, messages []llm.Message, primaryFn func(ctx context.Context) (string, error)) (string, error) {
	if !fh.config.Enabled {
		return primaryFn(ctx)
	}

	result, err := primaryFn(ctx)
	if err == nil {
		fh.mu.Lock()
		fh.inFallback = false
		fh.mu.Unlock()
		return result, nil
	}

	if !fh.isFallbackRequired(err) {
		return "", err
	}

	if fh.fallbackClient == nil {
		return "", err
	}

	fh.mu.Lock()
	if fh.inFallback {
		log.Printf("[降级] 已在降级模式，跳过主模型调用")
	} else {
		log.Printf("[降级] 主模型调用失败: %v，切换到降级模型 %s", err, fh.config.FallbackModel)
		fh.inFallback = true
	}
	fh.mu.Unlock()

	return fh.executeFallback(ctx, messages)
}

// isFallbackRequired 判断是否需要触发降级
// 根据错误信息中是否包含特定关键字来判断
// 需要降级的错误包括：超时、5xx错误、连接拒绝、熔断器打开、限流
func (fh *FallbackHandler) isFallbackRequired(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "circuit breaker") ||
		strings.Contains(errStr, "rate limit")
}

// executeFallback 执行降级调用
// 使用降级模型客户端处理请求
func (fh *FallbackHandler) executeFallback(ctx context.Context, messages []llm.Message) (string, error) {
	log.Printf("[降级] 使用降级模型 %s 处理请求", fh.config.FallbackModel)
	return fh.fallbackClient.Chat(ctx, messages)
}

// IsInFallback 判断当前是否处于降级模式
func (fh *FallbackHandler) IsInFallback() bool {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	return fh.inFallback
}
