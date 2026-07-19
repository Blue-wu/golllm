package stability

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitState 熔断器状态枚举
// 熔断器有三种状态：Closed(闭合)、Open(打开)、HalfOpen(半开)
type CircuitState int

const (
	// StateClosed 闭合状态，所有请求正常通过
	// 在该状态下，熔断器收集请求成功率数据
	StateClosed CircuitState = iota

	// StateOpen 打开状态，所有请求直接被拒绝
	// 当错误率超过阈值时，熔断器进入此状态，保护下游服务
	StateOpen

	// StateHalfOpen 半开状态，允许少量请求探测
	// 打开状态持续一段时间后，熔断器进入半开状态，探测下游服务是否恢复
	StateHalfOpen
)

// String 返回熔断器状态的字符串表示
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig 熔断器配置参数
type CircuitBreakerConfig struct {
	Enabled             bool          // 是否启用熔断器
	FailureThreshold    int           // 错误率阈值，超过此值触发熔断（百分比）
	WindowDuration      time.Duration // 滑动窗口时长，用于计算错误率
	MinRequests         int           // 窗口内最小请求数，低于此值不计算错误率
	SleepWindow         time.Duration // 打开状态持续时间，超时后进入半开状态
	HalfOpenMaxRequests int           // 半开状态下允许的最大请求数
	MaxWindowSize       int           // 滑动窗口最大容量，防止内存无限增长
}

// requestRecord 请求记录
// 记录每次请求的时间戳和是否失败，用于滑动窗口计算
type requestRecord struct {
	timestamp time.Time // 请求时间戳
	failed    bool      // 是否失败
}

// CircuitBreaker 熔断器
// 基于滑动窗口实现的熔断器，用于保护下游大模型服务
type CircuitBreaker struct {
	config           CircuitBreakerConfig // 配置参数
	state            CircuitState         // 当前状态
	lastStateChange  time.Time            // 上次状态变更时间
	mu               sync.RWMutex         // 读写锁，保证并发安全
	window           []requestRecord      // 滑动窗口，存储请求记录
	windowHead       int                  // 窗口头部索引（已废弃，保留兼容性）
	halfOpenRequests int                  // 半开状态下已处理的请求数
}

// NewCircuitBreaker 创建熔断器实例
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxWindowSize == 0 {
		config.MaxWindowSize = 1000
	}
	return &CircuitBreaker{
		config:          config,
		state:           StateClosed,
		lastStateChange: time.Now(),
		window:          make([]requestRecord, 0, config.MaxWindowSize),
	}
}

// Execute 执行受熔断器保护的函数
// 根据当前熔断器状态决定是否允许请求通过
// 参数：
//   - ctx: 上下文
//   - fn: 受保护的函数
//
// 返回：
//   - interface{}: 函数执行结果
//   - error: 如果熔断器打开，返回 ErrCircuitOpen
func (cb *CircuitBreaker) Execute(ctx context.Context, fn RetryableFunc) (interface{}, error) {
	if !cb.config.Enabled {
		return fn(ctx)
	}

	cb.mu.Lock()
	state := cb.getState()
	if state == StateOpen {
		cb.mu.Unlock()
		return nil, ErrCircuitOpen
	}

	if state == StateHalfOpen {
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			cb.mu.Unlock()
			return nil, ErrCircuitOpen
		}
		cb.halfOpenRequests++
	}
	cb.mu.Unlock()

	result, err := fn(ctx)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.pruneWindow()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return result, err
}

// getState 获取当前熔断器状态
// 如果处于打开状态且已超过休眠时间，自动切换到半开状态
func (cb *CircuitBreaker) getState() CircuitState {
	if cb.state == StateOpen {
		if time.Since(cb.lastStateChange) >= cb.config.SleepWindow {
			cb.state = StateHalfOpen
			cb.lastStateChange = time.Now()
			cb.halfOpenRequests = 0
			cb.window = make([]requestRecord, 0, cb.config.MaxWindowSize)
		}
	}
	return cb.state
}

// pruneWindow 清理过期的请求记录
// 删除滑动窗口中超过窗口时长的记录
func (cb *CircuitBreaker) pruneWindow() {
	now := time.Now()
	cutoff := now.Add(-cb.config.WindowDuration)

	for len(cb.window) > 0 && cb.window[0].timestamp.Before(cutoff) {
		cb.window = cb.window[1:]
	}
}

// recordFailure 记录失败请求
// 添加失败记录到滑动窗口，并检查是否需要触发熔断
func (cb *CircuitBreaker) recordFailure() {
	if len(cb.window) >= cb.config.MaxWindowSize {
		cb.window = cb.window[1:]
	}
	cb.window = append(cb.window, requestRecord{
		timestamp: time.Now(),
		failed:    true,
	})

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
		return
	}

	cb.checkThreshold()
}

// recordSuccess 记录成功请求
// 添加成功记录到滑动窗口，如果处于半开状态则切换到闭合状态
func (cb *CircuitBreaker) recordSuccess() {
	if len(cb.window) >= cb.config.MaxWindowSize {
		cb.window = cb.window[1:]
	}
	cb.window = append(cb.window, requestRecord{
		timestamp: time.Now(),
		failed:    false,
	})

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.lastStateChange = time.Now()
		cb.halfOpenRequests = 0
		return
	}

	cb.checkThreshold()
}

// checkThreshold 检查是否需要触发熔断
// 计算当前窗口内的错误率，如果超过阈值则切换到打开状态
func (cb *CircuitBreaker) checkThreshold() {
	requestCount := len(cb.window)
	if requestCount < cb.config.MinRequests {
		return
	}

	failureCount := 0
	for _, record := range cb.window {
		if record.failed {
			failureCount++
		}
	}

	failureRate := (failureCount * 100) / requestCount
	if failureRate >= cb.config.FailureThreshold {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
	}
}

// GetState 获取当前熔断器状态（线程安全）
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.getState()
}

// GetMetrics 获取熔断器统计指标
// 返回：请求总数、失败数、当前状态
func (cb *CircuitBreaker) GetMetrics() (int, int, CircuitState) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	cb.pruneWindow()

	requestCount := len(cb.window)
	failureCount := 0
	for _, record := range cb.window {
		if record.failed {
			failureCount++
		}
	}

	return requestCount, failureCount, cb.getState()
}

// ErrCircuitOpen 熔断器打开错误
var ErrCircuitOpen = errors.New("circuit breaker is open")
