package stability

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// LogLevel 日志级别枚举
type LogLevel string

const (
	// LogLevelDebug 调试级别，用于详细的调试信息
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo 信息级别，用于一般运行时信息
	LogLevelInfo LogLevel = "info"
	// LogLevelWarn 警告级别，用于警告信息
	LogLevelWarn LogLevel = "warn"
	// LogLevelError 错误级别，用于错误信息
	LogLevelError LogLevel = "error"
)

// ErrorCode 错误码类型
type ErrorCode string

// 客户端错误（400-499）
const (
	ErrCodeInvalidRequest   ErrorCode = "BAD_REQUEST"
	ErrCodeUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden        ErrorCode = "FORBIDDEN"
	ErrCodeNotFound         ErrorCode = "NOT_FOUND"
	ErrCodeRateLimited      ErrorCode = "RATE_LIMITED"
	ErrCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
)

// 模型错误
const (
	ErrCodeModelTimeout      ErrorCode = "MODEL_TIMEOUT"
	ErrCodeModelError        ErrorCode = "MODEL_ERROR"
	ErrCodeModelRateLimit    ErrorCode = "MODEL_RATE_LIMIT"
	ErrCodeModelQuotaExceed  ErrorCode = "MODEL_QUOTA_EXCEED"
	ErrCodeModelContentBlock ErrorCode = "MODEL_CONTENT_BLOCK"
)

// 系统内部错误（500-599）
const (
	ErrCodeInternalError      ErrorCode = "INTERNAL_ERROR"
	ErrCodeCircuitOpen        ErrorCode = "CIRCUIT_OPEN"
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrCodeDatabaseError      ErrorCode = "DATABASE_ERROR"
	ErrCodeNetworkError       ErrorCode = "NETWORK_ERROR"
)

// ErrorCategory 错误分类
type ErrorCategory string

const (
	// CategoryClient 客户端错误
	CategoryClient ErrorCategory = "client"
	// CategoryModel 模型错误
	CategoryModel ErrorCategory = "model"
	// CategorySystem 系统错误
	CategorySystem ErrorCategory = "system"
)

// GetErrorCategory 获取错误分类
func GetErrorCategory(code ErrorCode) ErrorCategory {
	switch code {
	case ErrCodeInvalidRequest, ErrCodeUnauthorized, ErrCodeForbidden,
		ErrCodeNotFound, ErrCodeRateLimited, ErrCodeValidationFailed:
		return CategoryClient
	case ErrCodeModelTimeout, ErrCodeModelError, ErrCodeModelRateLimit,
		ErrCodeModelQuotaExceed, ErrCodeModelContentBlock:
		return CategoryModel
	default:
		return CategorySystem
	}
}

// GetHTTPStatus 获取错误对应的HTTP状态码
func GetHTTPStatus(code ErrorCode) int {
	switch code {
	case ErrCodeInvalidRequest, ErrCodeValidationFailed:
		return 400
	case ErrCodeUnauthorized:
		return 401
	case ErrCodeForbidden:
		return 403
	case ErrCodeNotFound:
		return 404
	case ErrCodeRateLimited:
		return 429
	case ErrCodeModelTimeout, ErrCodeServiceUnavailable:
		return 504
	case ErrCodeCircuitOpen:
		return 503
	default:
		return 500
	}
}

// StructuredLog 结构化日志
type StructuredLog struct {
	Timestamp  string                 `json:"timestamp"`
	Level      LogLevel               `json:"level"`
	RequestID  string                 `json:"request_id"`
	UserID     string                 `json:"user_id"`
	ErrorCode  ErrorCode              `json:"error_code,omitempty"`
	ErrorMsg   string                 `json:"error_msg,omitempty"`
	Latency    float64                `json:"latency_ms"`
	TokenUsage *TokenUsage            `json:"token_usage,omitempty"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// TokenUsage Token 使用情况
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Logger 结构化日志记录器
type Logger struct {
	level LogLevel
}

// NewLogger 创建日志记录器实例
func NewLogger(level LogLevel) *Logger {
	return &Logger{level: level}
}

// Debug 记录调试日志
func (l *Logger) Debug(ctx context.Context, message string, details ...map[string]interface{}) {
	l.log(ctx, LogLevelDebug, message, details...)
}

// Info 记录信息日志
func (l *Logger) Info(ctx context.Context, message string, details ...map[string]interface{}) {
	l.log(ctx, LogLevelInfo, message, details...)
}

// Warn 记录警告日志
func (l *Logger) Warn(ctx context.Context, message string, details ...map[string]interface{}) {
	l.log(ctx, LogLevelWarn, message, details...)
}

// Error 记录错误日志
func (l *Logger) Error(ctx context.Context, code ErrorCode, err error, details ...map[string]interface{}) {
	logData := StructuredLog{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     LogLevelError,
		ErrorCode: code,
		ErrorMsg:  err.Error(),
		Message:   err.Error(),
	}

	if len(details) > 0 {
		logData.Details = details[0]
	}

	if requestID := ctx.Value("request_id"); requestID != nil {
		logData.RequestID = requestID.(string)
	}
	if userID := ctx.Value("user_id"); userID != nil {
		logData.UserID = userID.(string)
	}

	l.output(logData)
}

// RequestLog 记录请求日志
func (l *Logger) RequestLog(ctx context.Context, message string, latency float64, tokenUsage *TokenUsage, err error) {
	logData := StructuredLog{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      LogLevelInfo,
		Message:    message,
		Latency:    latency,
		TokenUsage: tokenUsage,
	}

	if requestID := ctx.Value("request_id"); requestID != nil {
		logData.RequestID = requestID.(string)
	}
	if userID := ctx.Value("user_id"); userID != nil {
		logData.UserID = userID.(string)
	}

	if err != nil {
		logData.Level = LogLevelError
		logData.ErrorMsg = err.Error()
		logData.ErrorCode = ErrCodeInternalError
	}

	l.output(logData)
}

// log 通用日志记录方法
func (l *Logger) log(ctx context.Context, level LogLevel, message string, details ...map[string]interface{}) {
	if level < l.level {
		return
	}

	logData := StructuredLog{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   message,
	}

	if len(details) > 0 {
		logData.Details = details[0]
	}

	if requestID := ctx.Value("request_id"); requestID != nil {
		logData.RequestID = requestID.(string)
	}
	if userID := ctx.Value("user_id"); userID != nil {
		logData.UserID = userID.(string)
	}

	l.output(logData)
}

// output 输出日志
func (l *Logger) output(logData StructuredLog) {
	data, err := json.Marshal(logData)
	if err != nil {
		log.Printf("failed to marshal log: %v", err)
		return
	}

	switch logData.Level {
	case LogLevelDebug:
		log.Printf("[DEBUG] %s", string(data))
	case LogLevelInfo:
		log.Printf("[INFO] %s", string(data))
	case LogLevelWarn:
		log.Printf("[WARN] %s", string(data))
	case LogLevelError:
		log.Printf("[ERROR] %s", string(data))
	}
}

// Metrics 关键指标记录器
type Metrics struct {
	counter      map[string]int64
	timer        map[string][]float64
	errorCounter map[ErrorCode]int64
	mu           struct {
		counter      sync.RWMutex
		timer        sync.RWMutex
		errorCounter sync.RWMutex
	}
}

// NewMetrics 创建指标记录器实例
func NewMetrics() *Metrics {
	return &Metrics{
		counter:      make(map[string]int64),
		timer:        make(map[string][]float64),
		errorCounter: make(map[ErrorCode]int64),
	}
}

// IncrementCounter 增加计数器
func (m *Metrics) IncrementCounter(name string) {
	m.mu.counter.Lock()
	defer m.mu.counter.Unlock()
	m.counter[name]++
}

// RecordLatency 记录延迟
func (m *Metrics) RecordLatency(name string, latencyMs float64) {
	m.mu.timer.Lock()
	defer m.mu.timer.Unlock()
	m.timer[name] = append(m.timer[name], latencyMs)
}

// RecordError 记录错误
func (m *Metrics) RecordError(code ErrorCode) {
	m.mu.errorCounter.Lock()
	defer m.mu.errorCounter.Unlock()
	m.errorCounter[code]++
}

// GetMetrics 获取所有指标
func (m *Metrics) GetMetrics() map[string]interface{} {
	result := make(map[string]interface{})

	m.mu.counter.RLock()
	for k, v := range m.counter {
		result[k] = v
	}
	m.mu.counter.RUnlock()

	m.mu.timer.RLock()
	for k, values := range m.timer {
		if len(values) == 0 {
			continue
		}
		sum := 0.0
		minVal, maxVal := values[0], values[0]
		for _, v := range values {
			sum += v
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
		result[k+".avg"] = sum / float64(len(values))
		result[k+".min"] = minVal
		result[k+".max"] = maxVal
		result[k+".count"] = len(values)
	}
	m.mu.timer.RUnlock()

	m.mu.errorCounter.RLock()
	for k, v := range m.errorCounter {
		result["error."+string(k)] = v
	}
	m.mu.errorCounter.RUnlock()

	return result
}
