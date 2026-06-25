package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/golllm/cmd/rag/config"
	"github.com/golllm/cmd/rag/handler"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/cmd/rag/service"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("RAG 知识库系统启动中...")

	var vectorClient vector.VectorClient
	milvusClient, err := vector.NewClient(vector.Config{
		Addr:       cfg.MilvusAddr,
		Collection: cfg.Collection,
		Dim:        cfg.EmbeddingDim,
	})
	if err != nil {
		log.Printf("Milvus 连接失败: %v，使用内存存储", err)
		vectorClient, _ = vector.NewInMemoryClient(vector.Config{
			Collection: cfg.Collection,
			Dim:        cfg.EmbeddingDim,
		})
	} else {
		vectorClient = milvusClient
	}
	defer vectorClient.Close()

	hasCol, _ := vectorClient.HasCollection(context.Background())
	if !hasCol {
		vectorClient.CreateCollection(context.Background())
	}

	embedClient := embedding.NewClient(embedding.Config{
		Provider: cfg.EmbeddingProvider,
		APIKey:   os.Getenv("OPENAI_API_KEY"),
		BaseURL:  cfg.OllamaURL,
		Model:    cfg.EmbeddingModel,
	})

	splitter := text.NewMarkdownSplitter()

	db, err := repository.NewSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

	docService := service.NewDocumentService(embedClient, vectorClient, splitter, db)
	ragService := service.NewRAGService(embedClient, vectorClient, db, cfg.OllamaURL)

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
			c.JSON(200, gin.H{"status": "ok", "version": "1.0"})
		})
	}

	addr := cfg.ServerAddr
	log.Printf("服务器启动成功: http://%s", addr)

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