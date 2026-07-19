package stability

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/golllm/cmd/rag/llm"
)

// LLMClientWrapperConfig LLM客户端包装器配置
// 集成重试、熔断、降级三级保障机制
type LLMClientWrapperConfig struct {
	RetryConfig          RetryConfig          // 重试配置
	CircuitBreakerConfig CircuitBreakerConfig // 熔断器配置
	FallbackConfig       FallbackConfig       // 降级配置
}

// LLMClientWrapper LLM客户端包装器
// 将重试、熔断、降级机制集成到LLM客户端调用中
type LLMClientWrapper struct {
	client         llm.Client       // 原始LLM客户端
	retryer        *Retryer         // 重试器
	circuitBreaker *CircuitBreaker  // 熔断器
	fallback       *FallbackHandler // 降级处理器
	logger         *Logger          // 日志记录器
	metrics        *Metrics         // 指标记录器
}

// NewLLMClientWrapper 创建LLM客户端包装器实例
// 参数：
//   - client: 原始LLM客户端，需实现 llm.Client 接口
//   - retryer: 重试器实例
//   - circuitBreaker: 熔断器实例
//   - fallback: 降级处理器实例
//   - logger: 日志记录器（可选）
//   - metrics: 指标记录器（可选）
func NewLLMClientWrapper(client llm.Client, retryer *Retryer, circuitBreaker *CircuitBreaker, fallback *FallbackHandler, logger *Logger, metrics *Metrics) *LLMClientWrapper {
	wrapper := &LLMClientWrapper{
		client:         client,
		retryer:        retryer,
		circuitBreaker: circuitBreaker,
		fallback:       fallback,
		logger:         logger,
		metrics:        metrics,
	}

	if wrapper.logger == nil {
		wrapper.logger = NewLogger(LogLevelInfo)
	}
	if wrapper.metrics == nil {
		wrapper.metrics = NewMetrics()
	}

	return wrapper
}

// Chat 调用LLM进行对话
// 集成了重试、熔断、降级三级保障机制
// 参数：
//   - ctx: 上下文，用于传递超时和取消信号
//   - messages: 对话消息列表
//
// 返回：
//   - string: 模型响应内容
//   - error: 错误信息
func (w *LLMClientWrapper) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	startTime := time.Now()

	var result string
	var err error

	_, err = w.circuitBreaker.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		innerResult, innerErr := w.retryer.Execute(ctx, func(ctx context.Context) (interface{}, error) {
			return w.client.Chat(ctx, messages)
		})
		if innerErr != nil {
			return nil, innerErr
		}
		result = innerResult.(string)
		return result, nil
	})

	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			w.logger.Warn(ctx, "circuit breaker is open, attempting fallback")
			w.metrics.RecordError(ErrCodeCircuitOpen)
			result, err = w.fallback.Execute(ctx, messages, func(ctx context.Context) (string, error) {
				return w.client.Chat(ctx, messages)
			})
		} else {
			result, err = w.fallback.Execute(ctx, messages, func(ctx context.Context) (string, error) {
				return w.client.Chat(ctx, messages)
			})
		}
	}

	latency := time.Since(startTime).Milliseconds()

	if err != nil {
		w.logger.Error(ctx, ErrCodeModelError, err)
		w.metrics.RecordError(ErrCodeModelError)
	} else {
		w.logger.Debug(ctx, "LLM call successful")
		w.metrics.IncrementCounter("llm.call.success")
	}

	w.metrics.RecordLatency("llm.call", float64(latency))

	return result, err
}

// GetCircuitBreakerState 获取熔断器当前状态
func (w *LLMClientWrapper) GetCircuitBreakerState() CircuitState {
	if w.circuitBreaker != nil {
		return w.circuitBreaker.GetState()
	}
	return StateClosed
}

// IsInFallback 判断当前是否处于降级模式
func (w *LLMClientWrapper) IsInFallback() bool {
	if w.fallback != nil {
		return w.fallback.IsInFallback()
	}
	return false
}

// ChatStream 发送流式对话请求
// 集成了重试、熔断、降级三级保障机制
func (w *LLMClientWrapper) ChatStream(ctx context.Context, messages []llm.Message, callback func(string)) error {
	return w.client.ChatStream(ctx, messages, callback)
}

// GetModel 获取当前模型名称
func (w *LLMClientWrapper) GetModel() string {
	return w.client.GetModel()
}

// GetMetrics 获取包装器统计指标
func (w *LLMClientWrapper) GetMetrics() map[string]interface{} {
	metrics := w.metrics.GetMetrics()

	if w.circuitBreaker != nil {
		reqCount, failCount, state := w.circuitBreaker.GetMetrics()
		metrics["circuit.requests"] = reqCount
		metrics["circuit.failures"] = failCount
		metrics["circuit.state"] = state.String()
	}

	if w.fallback != nil {
		metrics["fallback.active"] = w.fallback.IsInFallback()
	}

	return metrics
}

// HealthCheck 健康检查
// 检查底层LLM客户端是否可用
func (w *LLMClientWrapper) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := w.client.Chat(ctx, []llm.Message{
		{Role: "user", Content: "health check"},
	})
	if err != nil {
		log.Printf("[LLM Wrapper] Health check failed: %v", err)
		return err
	}

	return nil
}
