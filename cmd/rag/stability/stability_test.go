package stability

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golllm/cmd/rag/llm"
)

func TestRetryer_Execute_Success(t *testing.T) {
	retryer := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2,
		Timeout:      500 * time.Millisecond,
	})

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		return "success", nil
	}

	result, err := retryer.Execute(context.Background(), fn)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestRetryer_Execute_RetryableError(t *testing.T) {
	retryer := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2,
		Timeout:      500 * time.Millisecond,
	})

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("timeout")
		}
		return "success", nil
	}

	result, err := retryer.Execute(context.Background(), fn)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestRetryer_Execute_NonRetryableError(t *testing.T) {
	retryer := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2,
		Timeout:      500 * time.Millisecond,
	})

	callCount := 0
	fn := func(ctx context.Context) (interface{}, error) {
		callCount++
		return nil, errors.New("invalid request")
	}

	_, err := retryer.Execute(context.Background(), fn)
	if err == nil {
		t.Error("Expected error")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    50,
		WindowDuration:      100 * time.Millisecond,
		MinRequests:         4,
		SleepWindow:         1 * time.Second,
		HalfOpenMaxRequests: 2,
	})

	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, errors.New("error")
		})
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Expected circuit breaker to be open, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    50,
		WindowDuration:      100 * time.Millisecond,
		MinRequests:         2,
		SleepWindow:         100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, errors.New("error")
		})
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Expected circuit breaker to be open, got %s", cb.GetState())
	}

	time.Sleep(150 * time.Millisecond)

	if cb.GetState() != StateHalfOpen {
		t.Errorf("Expected circuit breaker to be half_open, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    50,
		WindowDuration:      100 * time.Millisecond,
		MinRequests:         2,
		SleepWindow:         100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, errors.New("error")
		})
	}

	time.Sleep(150 * time.Millisecond)

	_, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if cb.GetState() != StateClosed {
		t.Errorf("Expected circuit breaker to be closed, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    50,
		WindowDuration:      100 * time.Millisecond,
		MinRequests:         2,
		SleepWindow:         100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, errors.New("error")
		})
	}

	time.Sleep(150 * time.Millisecond)

	_, _ = cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("error")
	})

	if cb.GetState() != StateOpen {
		t.Errorf("Expected circuit breaker to be open, got %s", cb.GetState())
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		GlobalMaxQPS: 10,
	})

	for i := 0; i < 10; i++ {
		if !rl.Allow("", "") {
			t.Errorf("Expected request %d to be allowed", i)
		}
	}

	if rl.Allow("", "") {
		t.Error("Expected rate limit exceeded")
	}
}

func TestRateLimiter_PerUser(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		PerUserMaxQPS: 5,
	})

	for i := 0; i < 5; i++ {
		if !rl.Allow("user1", "") {
			t.Errorf("Expected request %d for user1 to be allowed", i)
		}
	}

	if rl.Allow("user1", "") {
		t.Error("Expected rate limit exceeded for user1")
	}

	if !rl.Allow("user2", "") {
		t.Error("Expected request for user2 to be allowed")
	}
}

func TestRateLimiter_PerIP(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		PerIPMaxQPS: 3,
	})

	for i := 0; i < 3; i++ {
		if !rl.Allow("", "192.168.1.1") {
			t.Errorf("Expected request %d for IP to be allowed", i)
		}
	}

	if rl.Allow("", "192.168.1.1") {
		t.Error("Expected rate limit exceeded for IP")
	}

	if !rl.Allow("", "192.168.1.2") {
		t.Error("Expected request for different IP to be allowed")
	}
}

type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	return m.response, m.err
}

func (m *mockLLMClient) ChatStream(ctx context.Context, messages []llm.Message, callback func(string)) error {
	return m.err
}

func (m *mockLLMClient) GetModel() string {
	return "mock-model"
}

func TestFallbackHandler_Execute_FallbackTriggered(t *testing.T) {
	primaryClient := &mockLLMClient{err: errors.New("503 service unavailable")}
	fallbackClient := &mockLLMClient{response: "fallback response"}

	fh := NewFallbackHandler(FallbackConfig{
		Enabled: true,
	}, fallbackClient)

	result, err := fh.Execute(context.Background(), []llm.Message{}, func(ctx context.Context) (string, error) {
		return primaryClient.Chat(ctx, []llm.Message{})
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "fallback response" {
		t.Errorf("Expected 'fallback response', got %v", result)
	}
	if !fh.IsInFallback() {
		t.Error("Expected fallback mode to be active")
	}
}

func TestFallbackHandler_Execute_PrimarySuccess(t *testing.T) {
	primaryClient := &mockLLMClient{response: "primary response"}
	fallbackClient := &mockLLMClient{response: "fallback response"}

	fh := NewFallbackHandler(FallbackConfig{
		Enabled: true,
	}, fallbackClient)

	result, err := fh.Execute(context.Background(), []llm.Message{}, func(ctx context.Context) (string, error) {
		return primaryClient.Chat(ctx, []llm.Message{})
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "primary response" {
		t.Errorf("Expected 'primary response', got %v", result)
	}
	if fh.IsInFallback() {
		t.Error("Expected fallback mode to be inactive")
	}
}

func TestTaskQueue_SubmitAndProcess(t *testing.T) {
	tq := NewTaskQueue(TaskQueueConfig{
		Enabled:           true,
		WorkerCount:       2,
		QueueCapacity:     10,
		MaxRetries:        1,
		RetryDelay:        10 * time.Millisecond,
		DeadLetterEnabled: true,
	})
	defer tq.Close()

	var completed int32
	tq.RegisterHandler(TaskTypeVectorize, func(ctx context.Context, task *Task) error {
		atomic.AddInt32(&completed, 1)
		return nil
	})

	taskID, err := tq.Submit(TaskTypeVectorize, "test_data")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&completed) != 1 {
		t.Errorf("Expected 1 completed task, got %d", completed)
	}
}

func TestTaskQueue_Retry(t *testing.T) {
	tq := NewTaskQueue(TaskQueueConfig{
		Enabled:           true,
		WorkerCount:       2,
		QueueCapacity:     10,
		MaxRetries:        2,
		RetryDelay:        10 * time.Millisecond,
		DeadLetterEnabled: true,
	})
	defer tq.Close()

	var attempt int32
	tq.RegisterHandler(TaskTypeVectorize, func(ctx context.Context, task *Task) error {
		atomic.AddInt32(&attempt, 1)
		if atomic.LoadInt32(&attempt) < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	_, err := tq.Submit(TaskTypeVectorize, "test_data")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&attempt) != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempt)
	}
}

func TestTaskQueue_DeadLetter(t *testing.T) {
	tq := NewTaskQueue(TaskQueueConfig{
		Enabled:           true,
		WorkerCount:       1,
		QueueCapacity:     10,
		MaxRetries:        1,
		RetryDelay:        10 * time.Millisecond,
		DeadLetterEnabled: true,
	})
	defer tq.Close()

	var attempt int32
	tq.RegisterHandler(TaskTypeVectorize, func(ctx context.Context, task *Task) error {
		atomic.AddInt32(&attempt, 1)
		return errors.New("persistent error")
	})

	taskID, err := tq.Submit(TaskTypeVectorize, "test_data")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&attempt) != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempt)
	}

	task, ok := tq.GetTask(taskID)
	if !ok {
		t.Error("Expected task to exist")
	}
	if task.Status != StatusDeadLetter {
		t.Errorf("Expected task status to be dead_letter, got %s", task.Status)
	}
}

func TestLLMClientWrapper_Chat_Success(t *testing.T) {
	primaryClient := &mockLLMClient{response: "success"}
	retryer := NewRetryer(RetryConfig{MaxRetries: 1})
	circuitBreaker := NewCircuitBreaker(CircuitBreakerConfig{Enabled: false})
	fallback := NewFallbackHandler(FallbackConfig{Enabled: false}, nil)
	logger := NewLogger(LogLevelInfo)
	metrics := NewMetrics()

	wrapper := NewLLMClientWrapper(primaryClient, retryer, circuitBreaker, fallback, logger, metrics)

	result, err := wrapper.Chat(context.Background(), []llm.Message{})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestLLMClientWrapper_Chat_CircuitOpen(t *testing.T) {
	primaryClient := &mockLLMClient{err: errors.New("503 service unavailable")}
	fallbackClient := &mockLLMClient{response: "fallback response"}
	retryer := NewRetryer(RetryConfig{MaxRetries: 0})
	circuitBreaker := NewCircuitBreaker(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    50,
		WindowDuration:      100 * time.Millisecond,
		MinRequests:         2,
		SleepWindow:         1 * time.Second,
		HalfOpenMaxRequests: 1,
	})
	fallback := NewFallbackHandler(FallbackConfig{Enabled: true}, fallbackClient)
	logger := NewLogger(LogLevelInfo)
	metrics := NewMetrics()

	wrapper := NewLLMClientWrapper(primaryClient, retryer, circuitBreaker, fallback, logger, metrics)

	for i := 0; i < 4; i++ {
		_, _ = wrapper.Chat(context.Background(), []llm.Message{})
	}

	if circuitBreaker.GetState() != StateOpen {
		t.Errorf("Expected circuit breaker to be open, got %s", circuitBreaker.GetState())
	}
}

func TestLogger_RequestLog(t *testing.T) {
	logger := NewLogger(LogLevelInfo)
	ctx := context.WithValue(context.Background(), "request_id", "test_req_001")
	ctx = context.WithValue(ctx, "user_id", "user_001")

	logger.RequestLog(ctx, "test request", 100.5, &TokenUsage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}, nil)
}

func TestLogger_Error(t *testing.T) {
	logger := NewLogger(LogLevelInfo)
	ctx := context.WithValue(context.Background(), "request_id", "test_req_001")

	logger.Error(ctx, ErrCodeModelError, errors.New("test error"))
}

func TestMetrics_RecordAndGet(t *testing.T) {
	metrics := NewMetrics()

	metrics.IncrementCounter("test_counter")
	metrics.IncrementCounter("test_counter")
	metrics.RecordLatency("test_latency", 100.0)
	metrics.RecordLatency("test_latency", 200.0)
	metrics.RecordError(ErrCodeModelError)

	result := metrics.GetMetrics()

	if result["test_counter"] != int64(2) {
		t.Errorf("Expected test_counter to be 2, got %v", result["test_counter"])
	}

	if result["test_latency.avg"] != 150.0 {
		t.Errorf("Expected test_latency.avg to be 150.0, got %v", result["test_latency.avg"])
	}
}

func TestIsRetryableHTTPStatus(t *testing.T) {
	testCases := []struct {
		statusCode int
		expected   bool
	}{
		{408, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{200, false},
		{400, false},
		{401, false},
		{404, false},
	}

	for _, tc := range testCases {
		result := IsRetryableHTTPStatus(tc.statusCode)
		if result != tc.expected {
			t.Errorf("Expected IsRetryableHTTPStatus(%d) to be %v, got %v", tc.statusCode, tc.expected, result)
		}
	}
}

func TestGetErrorCategory(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		expected ErrorCategory
	}{
		{ErrCodeBadRequest, CategoryClient},
		{ErrCodeUnauthorized, CategoryClient},
		{ErrCodeForbidden, CategoryClient},
		{ErrCodeNotFound, CategoryClient},
		{ErrCodeRateLimited, CategoryClient},
		{ErrCodeValidationFailed, CategoryClient},
		{ErrCodeModelTimeout, CategoryModel},
		{ErrCodeModelError, CategoryModel},
		{ErrCodeModelRateLimit, CategoryModel},
		{ErrCodeModelQuotaExceed, CategoryModel},
		{ErrCodeModelContentBlock, CategoryModel},
		{ErrCodeInternalError, CategorySystem},
		{ErrCodeCircuitOpen, CategorySystem},
		{ErrCodeServiceUnavailable, CategorySystem},
		{ErrCodeDatabaseError, CategorySystem},
		{ErrCodeNetworkError, CategorySystem},
	}

	for _, tc := range testCases {
		result := GetErrorCategory(tc.code)
		if result != tc.expected {
			t.Errorf("Expected GetErrorCategory(%s) to be %s, got %s", tc.code, tc.expected, result)
		}
	}
}

var ErrCodeBadRequest = ErrCodeInvalidRequest
