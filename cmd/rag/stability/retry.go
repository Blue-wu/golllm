package stability

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"
)

// RetryConfig 重试配置参数
// 控制指数退避重试的行为，适用于大模型接口调用的容错处理
type RetryConfig struct {
	MaxRetries   int           // 最大重试次数，0表示不重试
	InitialDelay time.Duration // 初始重试间隔，单位毫秒
	MaxDelay     time.Duration // 最大重试间隔，防止间隔无限增长
	Multiplier   int           // 退避乘数，每次重试间隔乘以该值
	Timeout      time.Duration // 单次请求超时时间
}

// Retryer 重试器
// 基于指数退避策略实现的重试机制，区分可重试错误与不可重试错误
type Retryer struct {
	config RetryConfig
}

// NewRetryer 创建重试器实例
func NewRetryer(config RetryConfig) *Retryer {
	return &Retryer{
		config: config,
	}
}

// RetryableFunc 可重试函数类型
// 定义了重试器可以执行的函数签名
type RetryableFunc func(ctx context.Context) (interface{}, error)

// Execute 执行可重试函数
// 如果函数执行失败且错误是可重试的，则按照指数退避策略进行重试
// 参数：
//   - ctx: 上下文，用于传递超时和取消信号
//   - fn: 要执行的可重试函数
//
// 返回：
//   - interface{}: 函数执行成功时的返回值
//   - error: 所有重试都失败时返回最后一次的错误
func (r *Retryer) Execute(ctx context.Context, fn RetryableFunc) (interface{}, error) {
	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if !r.isRetryable(err) {
			return nil, err
		}

		if attempt >= r.config.MaxRetries {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		delay = time.Duration(math.Min(float64(delay)*float64(r.config.Multiplier), float64(r.config.MaxDelay)))
	}

	return nil, lastErr
}

// isRetryable 判断错误是否可重试
// 根据错误信息中是否包含特定关键字来判断
// 可重试错误包括：超时、连接拒绝、5xx服务器错误、429限流错误
func (r *Retryer) isRetryable(err error) bool {
	errStr := err.Error()

	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") {
		return true
	}

	return false
}

// 预定义错误类型，用于错误分类和判断
var ErrTimeout = errors.New("request timeout")
var ErrRateLimit = errors.New("rate limit exceeded")
var ErrServerError = errors.New("server error")
var ErrNetworkError = errors.New("network error")

// IsTimeoutError 判断是否为超时错误
func IsTimeoutError(err error) bool {
	return errors.Is(err, ErrTimeout) || strings.Contains(err.Error(), "timeout")
}

// IsRetryableHTTPStatus 判断HTTP状态码是否可重试
// 可重试的状态码包括：请求超时(408)、限流(429)、服务器错误(500-504)
func IsRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
