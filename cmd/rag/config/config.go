package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Server         ServerConfig
	VectorDB       VectorDBConfig
	Redis          RedisConfig
	Embedding      EmbeddingConfig
	LLM            LLMConfig
	RAG            RAGConfig
	Chat           ChatConfig
	Retry          RetryConfig
	CircuitBreaker CircuitBreakerConfig
	Fallback       FallbackConfig
	RateLimit      RateLimitConfig
	AsyncTask      AsyncTaskConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type VectorDBConfig struct {
	Type       string
	Addr       string
	Collection string
	Dim        int
}

type EmbeddingConfig struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

type LLMConfig struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

type RAGConfig struct {
	TopK     int
	MinScore float32
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type ChatConfig struct {
	MaxTokens      int
	SessionTimeout int
	MaxHistorySize int
}

type RetryConfig struct {
	MaxRetries   int
	InitialDelay int
	MaxDelay     int
	Multiplier   int
	Timeout      int
}

type CircuitBreakerConfig struct {
	Enabled             bool
	FailureThreshold    int
	WindowDuration      int
	MinRequests         int
	SleepWindow         int
	HalfOpenMaxRequests int
}

type FallbackConfig struct {
	Enabled          bool
	FallbackProvider string
	FallbackModel    string
}

type RateLimitConfig struct {
	GlobalMaxQPS        int
	GlobalMaxConcurrent int
	PerUserMaxQPS       int
	PerUserMaxDaily     int
	PerIPMaxQPS         int
	PerIPMaxConcurrent  int
}

type AsyncTaskConfig struct {
	Enabled       bool
	MaxWorkers    int
	QueueCapacity int
	MaxRetries    int
	RetryDelay    int
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{
		Server:    ServerConfig{Host: "localhost", Port: 8080},
		VectorDB:  VectorDBConfig{Type: "milvus", Addr: "localhost:19530", Collection: "documents", Dim: 768},
		Redis:     RedisConfig{Addr: "localhost:6379", Password: "", DB: 0},
		Embedding: EmbeddingConfig{Provider: "ollama", Model: "nomic-embed-text"},
		LLM:       LLMConfig{Provider: "ollama", Model: "qwen2.5:7b"},
		RAG:       RAGConfig{TopK: 5, MinScore: 0.5},
		Chat:      ChatConfig{MaxTokens: 4096, SessionTimeout: 3600, MaxHistorySize: 50},
		Retry: RetryConfig{
			MaxRetries:   3,
			InitialDelay: 1000,
			MaxDelay:     10000,
			Multiplier:   2,
			Timeout:      60000,
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:             true,
			FailureThreshold:    50,
			WindowDuration:      60000,
			MinRequests:         10,
			SleepWindow:         30000,
			HalfOpenMaxRequests: 3,
		},
		Fallback: FallbackConfig{
			Enabled:          true,
			FallbackProvider: "ollama",
			FallbackModel:    "qwen2:7b",
		},
		RateLimit: RateLimitConfig{
			GlobalMaxQPS:        100,
			GlobalMaxConcurrent: 500,
			PerUserMaxQPS:       20,
			PerUserMaxDaily:     1000,
			PerIPMaxQPS:         30,
			PerIPMaxConcurrent:  10,
		},
		AsyncTask: AsyncTaskConfig{
			Enabled:       true,
			MaxWorkers:    4,
			QueueCapacity: 1000,
			MaxRetries:    3,
			RetryDelay:    5000,
		},
	}

	lines := strings.Split(string(data), "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 去除注释
		if idx := strings.Index(value, "#"); idx != -1 {
			value = strings.TrimSpace(value[:idx])
		}
		value = strings.Trim(value, "\"'")

		switch currentSection + "." + key {
		case "server.host":
			cfg.Server.Host = value
		case "server.port":
			fmt.Sscanf(value, "%d", &cfg.Server.Port)
		case "vector.type":
			cfg.VectorDB.Type = value
		case "vector.addr":
			cfg.VectorDB.Addr = value
		case "vector.collection":
			cfg.VectorDB.Collection = value
		case "vector.dim":
			fmt.Sscanf(value, "%d", &cfg.VectorDB.Dim)
		case "embedding.provider":
			cfg.Embedding.Provider = value
		case "embedding.model":
			cfg.Embedding.Model = value
		case "embedding.api_key":
			cfg.Embedding.APIKey = value
		case "embedding.base_url":
			cfg.Embedding.BaseURL = value
		case "llm.provider":
			cfg.LLM.Provider = value
		case "llm.model":
			cfg.LLM.Model = value
		case "llm.api_key":
			cfg.LLM.APIKey = value
		case "llm.base_url":
			cfg.LLM.BaseURL = value
		case "rag.topk":
			fmt.Sscanf(value, "%d", &cfg.RAG.TopK)
		case "rag.min_score":
			fmt.Sscanf(value, "%f", &cfg.RAG.MinScore)
		case "redis.addr":
			cfg.Redis.Addr = value
		case "redis.password":
			cfg.Redis.Password = value
		case "redis.db":
			fmt.Sscanf(value, "%d", &cfg.Redis.DB)
		case "chat.max_tokens":
			fmt.Sscanf(value, "%d", &cfg.Chat.MaxTokens)
		case "chat.session_timeout":
			fmt.Sscanf(value, "%d", &cfg.Chat.SessionTimeout)
		case "chat.max_history_size":
			fmt.Sscanf(value, "%d", &cfg.Chat.MaxHistorySize)
		case "retry.max_retries":
			fmt.Sscanf(value, "%d", &cfg.Retry.MaxRetries)
		case "retry.initial_delay":
			fmt.Sscanf(value, "%d", &cfg.Retry.InitialDelay)
		case "retry.max_delay":
			fmt.Sscanf(value, "%d", &cfg.Retry.MaxDelay)
		case "retry.multiplier":
			fmt.Sscanf(value, "%d", &cfg.Retry.Multiplier)
		case "retry.timeout":
			fmt.Sscanf(value, "%d", &cfg.Retry.Timeout)
		case "circuit_breaker.enabled":
			fmt.Sscanf(value, "%t", &cfg.CircuitBreaker.Enabled)
		case "circuit_breaker.failure_threshold":
			fmt.Sscanf(value, "%d", &cfg.CircuitBreaker.FailureThreshold)
		case "circuit_breaker.window_duration":
			fmt.Sscanf(value, "%d", &cfg.CircuitBreaker.WindowDuration)
		case "circuit_breaker.min_requests":
			fmt.Sscanf(value, "%d", &cfg.CircuitBreaker.MinRequests)
		case "circuit_breaker.sleep_window":
			fmt.Sscanf(value, "%d", &cfg.CircuitBreaker.SleepWindow)
		case "circuit_breaker.half_open_max_requests":
			fmt.Sscanf(value, "%d", &cfg.CircuitBreaker.HalfOpenMaxRequests)
		case "fallback.enabled":
			fmt.Sscanf(value, "%t", &cfg.Fallback.Enabled)
		case "fallback.fallback_provider":
			cfg.Fallback.FallbackProvider = value
		case "fallback.fallback_model":
			cfg.Fallback.FallbackModel = value
		case "rate_limit.global_max_qps":
			fmt.Sscanf(value, "%d", &cfg.RateLimit.GlobalMaxQPS)
		case "rate_limit.global_max_concurrent":
			fmt.Sscanf(value, "%d", &cfg.RateLimit.GlobalMaxConcurrent)
		case "rate_limit.per_user_max_qps":
			fmt.Sscanf(value, "%d", &cfg.RateLimit.PerUserMaxQPS)
		case "rate_limit.per_user_max_daily":
			fmt.Sscanf(value, "%d", &cfg.RateLimit.PerUserMaxDaily)
		case "rate_limit.per_ip_max_qps":
			fmt.Sscanf(value, "%d", &cfg.RateLimit.PerIPMaxQPS)
		case "rate_limit.per_ip_max_concurrent":
			fmt.Sscanf(value, "%d", &cfg.RateLimit.PerIPMaxConcurrent)
		case "async_task.enabled":
			fmt.Sscanf(value, "%t", &cfg.AsyncTask.Enabled)
		case "async_task.max_workers":
			fmt.Sscanf(value, "%d", &cfg.AsyncTask.MaxWorkers)
		case "async_task.queue_capacity":
			fmt.Sscanf(value, "%d", &cfg.AsyncTask.QueueCapacity)
		case "async_task.max_retries":
			fmt.Sscanf(value, "%d", &cfg.AsyncTask.MaxRetries)
		case "async_task.retry_delay":
			fmt.Sscanf(value, "%d", &cfg.AsyncTask.RetryDelay)
		}
	}

	cfg.Embedding.APIKey = os.Getenv("OPENAI_API_KEY")
	cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")

	return cfg, nil
}
