package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golllm/cmd/rag/config"
	"github.com/golllm/cmd/rag/handler"
	"github.com/golllm/cmd/rag/llm"
	"github.com/golllm/cmd/rag/redis"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/cmd/rag/service"
	"github.com/golllm/cmd/rag/stability"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/vector"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.LoadConfig("./config/config.toml")
	if err != nil {
		log.Printf("配置加载失败: %v，使用默认配置", err)
		cfg = &config.Config{
			Server:    config.ServerConfig{Host: "0.0.0.0", Port: 8080},
			VectorDB:  config.VectorDBConfig{Type: "milvus", Addr: "localhost:19530", Collection: "documents", Dim: 768},
			Embedding: config.EmbeddingConfig{Provider: "ollama", Model: "nomic-embed-text"},
			LLM:       config.LLMConfig{Provider: "ollama", Model: "qwen2.5:7b"},
			RAG:       config.RAGConfig{TopK: 5, MinScore: 0.6},
		}
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("RAG 知识库系统启动中...")
	log.Printf("Embedding: %s (%s)", cfg.Embedding.Provider, cfg.Embedding.Model)
	log.Printf("LLM: %s (%s)", cfg.LLM.Provider, cfg.LLM.Model)

	var vectorClient vector.VectorClient

	if cfg.VectorDB.Type == "milvus" {
		milvusClient, err := vector.NewClient(vector.Config{
			Addr:       cfg.VectorDB.Addr,
			Collection: cfg.VectorDB.Collection,
			Dim:        cfg.VectorDB.Dim,
		})
		if err != nil {
			log.Printf("Milvus 连接失败: %v，使用内存存储", err)
			vectorClient, _ = vector.NewInMemoryClient(vector.Config{
				Collection: cfg.VectorDB.Collection,
				Dim:        cfg.VectorDB.Dim,
			})
		} else {
			vectorClient = milvusClient
		}
	} else {
		vectorClient, _ = vector.NewInMemoryClient(vector.Config{
			Collection: cfg.VectorDB.Collection,
			Dim:        cfg.VectorDB.Dim,
		})
	}
	defer vectorClient.Close()

	hasCol, _ := vectorClient.HasCollection(context.Background())
	if !hasCol {
		vectorClient.CreateCollection(context.Background())
	}

	embedClient := embedding.NewClient(embedding.Config{
		Provider: cfg.Embedding.Provider,
		APIKey:   cfg.Embedding.APIKey,
		BaseURL:  cfg.Embedding.BaseURL,
		Model:    cfg.Embedding.Model,
	})

	db, err := repository.NewSQLite(cfg.VectorDB.Collection + ".db")
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

	primaryLLMClient, err := llm.NewClient(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKey, cfg.LLM.BaseURL)
	if err != nil {
		log.Fatalf("LLM 客户端创建失败: %v", err)
	}

	var fallbackLLMClient llm.Client
	if cfg.Fallback.Enabled && cfg.Fallback.FallbackProvider != "" && cfg.Fallback.FallbackModel != "" {
		fallbackLLMClient, err = llm.NewClient(cfg.Fallback.FallbackProvider, cfg.Fallback.FallbackModel, cfg.LLM.APIKey, cfg.LLM.BaseURL)
		if err != nil {
			log.Printf("降级模型创建失败: %v，降级功能将被禁用", err)
			cfg.Fallback.Enabled = false
		} else {
			log.Printf("降级模型配置成功: %s (%s)", cfg.Fallback.FallbackProvider, cfg.Fallback.FallbackModel)
		}
	}

	var redisClient *redis.Client
	if cfg.Redis.Addr != "" {
		redisClient = redis.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		log.Printf("Redis 连接成功: %s", cfg.Redis.Addr)
	} else {
		log.Printf("Redis 未配置，使用本地存储")
		
	}

	sessionTimeout := cfg.Chat.SessionTimeout
	if sessionTimeout <= 0 {
		sessionTimeout = 3600
	}

	maxHistorySize := cfg.Chat.MaxHistorySize
	if maxHistorySize <= 0 {
		maxHistorySize = 100
	}

	maxTokens := cfg.Chat.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	retryer := stability.NewRetryer(stability.RetryConfig{
		MaxRetries:   cfg.Retry.MaxRetries,
		InitialDelay: time.Duration(cfg.Retry.InitialDelay) * time.Millisecond,
		MaxDelay:     time.Duration(cfg.Retry.MaxDelay) * time.Millisecond,
		Multiplier:   cfg.Retry.Multiplier,
		Timeout:      time.Duration(cfg.Retry.Timeout) * time.Millisecond,
	})

	circuitBreaker := stability.NewCircuitBreaker(stability.CircuitBreakerConfig{
		Enabled:             cfg.CircuitBreaker.Enabled,
		FailureThreshold:    cfg.CircuitBreaker.FailureThreshold,
		WindowDuration:      time.Duration(cfg.CircuitBreaker.WindowDuration) * time.Millisecond,
		MinRequests:         cfg.CircuitBreaker.MinRequests,
		SleepWindow:         time.Duration(cfg.CircuitBreaker.SleepWindow) * time.Millisecond,
		HalfOpenMaxRequests: cfg.CircuitBreaker.HalfOpenMaxRequests,
	})

	fallbackHandler := stability.NewFallbackHandler(stability.FallbackConfig{
		Enabled:          cfg.Fallback.Enabled,
		FallbackProvider: cfg.Fallback.FallbackProvider,
		FallbackModel:    cfg.Fallback.FallbackModel,
	}, fallbackLLMClient)

	logger := stability.NewLogger(stability.LogLevelInfo)
	metrics := stability.NewMetrics()

	llmWrapper := stability.NewLLMClientWrapper(
		primaryLLMClient,
		retryer,
		circuitBreaker,
		fallbackHandler,
		logger,
		metrics,
	)

	rateLimiter := stability.NewRateLimiter(stability.RateLimitConfig{
		GlobalMaxQPS:        cfg.RateLimit.GlobalMaxQPS,
		GlobalMaxConcurrent: cfg.RateLimit.GlobalMaxConcurrent,
		PerUserMaxQPS:       cfg.RateLimit.PerUserMaxQPS,
		PerUserMaxDaily:     cfg.RateLimit.PerUserMaxDaily,
		PerIPMaxQPS:         cfg.RateLimit.PerIPMaxQPS,
		PerIPMaxConcurrent:  cfg.RateLimit.PerIPMaxConcurrent,
	})

	taskQueue := stability.NewTaskQueue(stability.TaskQueueConfig{
		Enabled:           cfg.AsyncTask.Enabled,
		WorkerCount:       cfg.AsyncTask.MaxWorkers,
		QueueCapacity:     cfg.AsyncTask.QueueCapacity,
		MaxRetries:        cfg.AsyncTask.MaxRetries,
		RetryDelay:        time.Duration(cfg.AsyncTask.RetryDelay) * time.Millisecond,
		DeadLetterEnabled: true,
	})
	defer taskQueue.Close()

	sessionManager := service.NewSessionManager(redisClient, db, sessionTimeout, maxHistorySize)
	tokenManager := service.NewTokenManager(cfg.LLM.Model, maxTokens, "")

	docService := service.NewDocumentService(embedClient, vectorClient, db, taskQueue)
	ragService := service.NewRAGService(embedClient, vectorClient, db, llmWrapper, cfg.RAG.TopK, cfg.RAG.MinScore, sessionManager, tokenManager, maxTokens)

	docHandler := handler.NewDocumentHandler(docService)
	chatHandler := handler.NewChatHandler(ragService)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggerMiddleware())
	r.Use(rateLimitMiddleware(rateLimiter))

	api := r.Group("/api/v1")
	{
		docs := api.Group("/documents")
		{
			docs.POST("/upload", docHandler.Upload)
			docs.GET("/list", docHandler.List)
			docs.GET("/:id", docHandler.Get)
			docs.DELETE("/:id", docHandler.Delete)
		}

		chat := api.Group("/chat")
		{
			chat.POST("/ask", chatHandler.Ask)
			chat.GET("/history/:session_id", chatHandler.History)
			chat.GET("/sessions", chatHandler.ListSessions)
			chat.DELETE("/sessions/:session_id", chatHandler.DeleteSession)
			chat.POST("/sessions/:session_id/reset", chatHandler.ResetSession)
			chat.POST("/sessions/:session_id/task/start", chatHandler.StartTask)
			chat.POST("/sessions/:session_id/task/cancel", chatHandler.CancelTask)
		}

		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":          "ok",
				"version":         "1.3",
				"model":           cfg.LLM.Model,
				"circuit_breaker": circuitBreaker.GetState().String(),
				"in_fallback":     fallbackHandler.IsInFallback(),
			})
		})

		api.GET("/metrics", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"rate_limit": rateLimiter.GetMetrics(),
				"circuit_breaker": map[string]interface{}{
					"state": circuitBreaker.GetState().String(),
				},
				"task_queue": taskQueue.GetStats(),
			})
		})
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("服务器启动成功: http://%s", addr)
	log.Printf("使用配置:")
	log.Printf("  - Embedding: %s (%s)", cfg.Embedding.Provider, cfg.Embedding.Model)
	log.Printf("  - LLM: %s (%s)", cfg.LLM.Provider, cfg.LLM.Model)
	log.Printf("  - VectorDB: %s", cfg.VectorDB.Type)
	log.Printf("  - RAG: TopK=%d, MinScore=%.2f", cfg.RAG.TopK, cfg.RAG.MinScore)
	log.Printf("  - 特性: 语义切片 + 父子文档 + 混合召回(RRF)")
	log.Printf("  - 稳定性: 重试(%d次) + 熔断(%d%%阈值) + 降级(%s) + 限流(全局%d QPS)",
		cfg.Retry.MaxRetries, cfg.CircuitBreaker.FailureThreshold, cfg.Fallback.FallbackModel, cfg.RateLimit.GlobalMaxQPS)

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	<-quit
	log.Println("服务器关闭中...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器强制关闭: %v", err)
	}

	taskQueue.Close()
	log.Println("服务器已关闭")
}

func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		log.Printf("[%s] %s %s", c.ClientIP(), c.Request.Method, c.Request.URL.Path)
		c.Next()
		duration := time.Since(start)
		log.Printf("[%s] %s %s %d %s", c.ClientIP(), c.Request.Method, c.Request.URL.Path, c.Writer.Status(), duration)
	}
}

func rateLimitMiddleware(rl *stability.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api/v1/health" || path == "/api/v1/metrics" {
			c.Next()
			return
		}

		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			userID = c.Query("user_id")
		}

		ip := c.ClientIP()

		if !rl.Allow(userID, ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"code":  429,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
