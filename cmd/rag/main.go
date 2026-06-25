package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/golllm/cmd/rag/config"
	"github.com/golllm/cmd/rag/handler"
	"github.com/golllm/cmd/rag/llm"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/cmd/rag/service"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.LoadConfig("config.toml")
	if err != nil {
		log.Printf("配置加载失败: %v，使用默认配置", err)
		// 使用默认配置
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

	// 根据配置选择向量存储
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

	// Embedding 客户端
	embedClient := embedding.NewClient(embedding.Config{
		Provider: cfg.Embedding.Provider,
		APIKey:   cfg.Embedding.APIKey,
		BaseURL:  cfg.Embedding.BaseURL,
		Model:    cfg.Embedding.Model,
	})

	splitter := text.NewMarkdownSplitter()

	db, err := repository.NewSQLite(cfg.VectorDB.Collection + ".db")
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

	// LLM 客户端
	llmClient, err := llm.NewClient(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKey, cfg.LLM.BaseURL)
	if err != nil {
		log.Fatalf("LLM 客户端创建失败: %v", err)
	}

	docService := service.NewDocumentService(embedClient, vectorClient, splitter, db)
	ragService := service.NewRAGService(embedClient, vectorClient, db, llmClient, cfg.RAG.TopK, cfg.RAG.MinScore)

	docHandler := handler.NewDocumentHandler(docService)
	chatHandler := handler.NewChatHandler(ragService)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggerMiddleware())

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
		}

		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok", "version": "1.1", "model": cfg.LLM.Model})
		})
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("服务器启动成功: http://%s", addr)
	log.Printf("使用配置:")
	log.Printf("  - Embedding: %s (%s)", cfg.Embedding.Provider, cfg.Embedding.Model)
	log.Printf("  - LLM: %s (%s)", cfg.LLM.Provider, cfg.LLM.Model)
	log.Printf("  - VectorDB: %s", cfg.VectorDB.Type)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := r.Run(addr); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	<-quit
	log.Println("服务器关闭中...")
}

func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("[%s] %s %s", c.ClientIP(), c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}
