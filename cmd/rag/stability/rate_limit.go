package stability

import (
	"sync"
	"time"
)

// RateLimitConfig 限流配置参数
// 支持多级限流：全局、用户级、IP级
type RateLimitConfig struct {
	GlobalMaxQPS        int // 全局限流 QPS
	GlobalMaxConcurrent int // 全局最大并发数
	PerUserMaxQPS       int // 单用户限流 QPS
	PerUserMaxDaily     int // 单用户每日最大调用次数
	PerIPMaxQPS         int // 单IP限流 QPS
	PerIPMaxConcurrent  int // 单IP最大并发数
}

// TokenBucket 令牌桶
// 基于令牌桶算法实现的限流器，用于控制请求速率
type TokenBucket struct {
	capacity int        // 令牌桶容量
	tokens   int        // 当前可用令牌数
	rate     int        // 令牌补充速率（每秒）
	lastTime time.Time  // 上次补充令牌的时间
	mu       sync.Mutex // 互斥锁，保证并发安全
}

// NewTokenBucket 创建令牌桶实例
// 参数：
//   - capacity: 令牌桶容量
//   - rate: 令牌补充速率（每秒生成的令牌数）
func NewTokenBucket(capacity, rate int) *TokenBucket {
	return &TokenBucket{
		capacity: capacity,
		tokens:   capacity,
		rate:     rate,
		lastTime: time.Now(),
	}
}

// Take 尝试获取一个令牌
// 如果令牌桶中有可用令牌，返回 true
// 如果没有可用令牌，返回 false
func (tb *TokenBucket) Take() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tokensToAdd := int(elapsed * float64(tb.rate))

	tb.tokens = min(tb.tokens+tokensToAdd, tb.capacity)
	tb.lastTime = now

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// TakeN 尝试获取 N 个令牌
// 参数：
//   - n: 需要获取的令牌数
//
// 返回：
//   - bool: 是否成功获取
func (tb *TokenBucket) TakeN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tokensToAdd := int(elapsed * float64(tb.rate))

	tb.tokens = min(tb.tokens+tokensToAdd, tb.capacity)
	tb.lastTime = now

	if tb.tokens >= n {
		tb.tokens -= n
		return true
	}

	return false
}

// GetAvailable 获取当前可用令牌数
func (tb *TokenBucket) GetAvailable() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tokensToAdd := int(elapsed * float64(tb.rate))

	tb.tokens = min(tb.tokens+tokensToAdd, tb.capacity)
	tb.lastTime = now

	return tb.tokens
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RateLimiter 多级限流管理器
// 支持全局、用户级、IP级的 QPS 限流和并发数限制
type RateLimiter struct {
	config RateLimitConfig // 配置参数

	globalBucket    *TokenBucket  // 全局限流令牌桶
	globalSemaphore chan struct{} // 全局并发数信号量

	userBuckets       sync.Map // 用户级限流令牌桶（key: userID）
	userDailyCounters sync.Map // 用户每日调用计数器（key: userID）

	ipBuckets    sync.Map // IP级限流令牌桶（key: IP）
	ipSemaphores sync.Map // IP级并发数信号量（key: IP）

	mu sync.RWMutex // 读写锁
}

// NewRateLimiter 创建限流管理器实例
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config: config,
	}

	if config.GlobalMaxQPS > 0 {
		rl.globalBucket = NewTokenBucket(config.GlobalMaxQPS, config.GlobalMaxQPS)
	}

	if config.GlobalMaxConcurrent > 0 {
		rl.globalSemaphore = make(chan struct{}, config.GlobalMaxConcurrent)
	}

	return rl
}

// Allow 检查请求是否允许通过
// 依次检查：全局限流、全局并发、用户限流、用户每日限制、IP限流、IP并发
// 参数：
//   - userID: 用户ID（可选）
//   - ip: 客户端IP（可选）
//
// 返回：
//   - bool: 是否允许通过
func (rl *RateLimiter) Allow(userID, ip string) bool {
	if rl.globalBucket != nil && !rl.globalBucket.Take() {
		return false
	}

	if rl.globalSemaphore != nil {
		select {
		case rl.globalSemaphore <- struct{}{}:
			defer func() { <-rl.globalSemaphore }()
		default:
			return false
		}
	}

	if rl.config.PerUserMaxQPS > 0 && userID != "" {
		bucket := rl.getUserBucket(userID)
		if !bucket.Take() {
			return false
		}
	}

	if rl.config.PerUserMaxDaily > 0 && userID != "" {
		counter := rl.getUserDailyCounter(userID)
		if !counter.Increment() {
			return false
		}
	}

	if rl.config.PerIPMaxQPS > 0 && ip != "" {
		bucket := rl.getIPBucket(ip)
		if !bucket.Take() {
			return false
		}
	}

	if rl.config.PerIPMaxConcurrent > 0 && ip != "" {
		sem := rl.getIPSemaphore(ip)
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			return false
		}
	}

	return true
}

// getUserBucket 获取或创建用户级限流令牌桶
func (rl *RateLimiter) getUserBucket(userID string) *TokenBucket {
	if v, ok := rl.userBuckets.Load(userID); ok {
		return v.(*TokenBucket)
	}

	bucket := NewTokenBucket(rl.config.PerUserMaxQPS, rl.config.PerUserMaxQPS)
	v, _ := rl.userBuckets.LoadOrStore(userID, bucket)
	return v.(*TokenBucket)
}

// getIPBucket 获取或创建IP级限流令牌桶
func (rl *RateLimiter) getIPBucket(ip string) *TokenBucket {
	if v, ok := rl.ipBuckets.Load(ip); ok {
		return v.(*TokenBucket)
	}

	bucket := NewTokenBucket(rl.config.PerIPMaxQPS, rl.config.PerIPMaxQPS)
	v, _ := rl.ipBuckets.LoadOrStore(ip, bucket)
	return v.(*TokenBucket)
}

// getIPSemaphore 获取或创建IP级并发数信号量
func (rl *RateLimiter) getIPSemaphore(ip string) chan struct{} {
	if v, ok := rl.ipSemaphores.Load(ip); ok {
		return v.(chan struct{})
	}

	sem := make(chan struct{}, rl.config.PerIPMaxConcurrent)
	v, _ := rl.ipSemaphores.LoadOrStore(ip, sem)
	return v.(chan struct{})
}

// DailyCounter 每日计数器
// 用于限制用户每日调用次数，自动在新的一天重置
type DailyCounter struct {
	maxCount int        // 每日最大计数
	count    int        // 当前计数
	lastDate string     // 上次计数日期
	mu       sync.Mutex // 互斥锁
}

// NewDailyCounter 创建每日计数器实例
func NewDailyCounter(maxCount int) *DailyCounter {
	return &DailyCounter{
		maxCount: maxCount,
		lastDate: time.Now().Format("2006-01-02"),
	}
}

// Increment 尝试增加计数
// 如果超过每日限制，返回 false
// 如果日期变更，自动重置计数
func (dc *DailyCounter) Increment() bool {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today != dc.lastDate {
		dc.count = 0
		dc.lastDate = today
	}

	if dc.count >= dc.maxCount {
		return false
	}

	dc.count++
	return true
}

// GetCount 获取当前计数
func (dc *DailyCounter) GetCount() int {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.count
}

// getUserDailyCounter 获取或创建用户每日调用计数器
func (rl *RateLimiter) getUserDailyCounter(userID string) *DailyCounter {
	if v, ok := rl.userDailyCounters.Load(userID); ok {
		return v.(*DailyCounter)
	}

	counter := NewDailyCounter(rl.config.PerUserMaxDaily)
	v, _ := rl.userDailyCounters.LoadOrStore(userID, counter)
	return v.(*DailyCounter)
}

// GetMetrics 获取限流统计指标
func (rl *RateLimiter) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	if rl.globalBucket != nil {
		metrics["global_available_tokens"] = rl.globalBucket.GetAvailable()
	}

	return metrics
}
