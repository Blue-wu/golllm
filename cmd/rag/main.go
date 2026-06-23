package main

import (
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

// @title RAG 知识库系统
// @version 1.0
// @description 基于 Milvus + Ollama 的本地 RAG 知识库问答系统
// @host localhost:8080
// @BasePath /api/v1
func main() {
	// 加载配置
	cfg := config.Load()

	// 初始化日志
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("🚀 RAG 知识库系统启动中...")

	// 初始化向量数据库客户端
	vectorClient, err := vector.NewClient(vector.Config{
		Address:     cfg.MilvusAddr,
		Collection:  cfg.Collection,
		Dimension:   cfg.EmbeddingDim,
		IndexType:   cfg.IndexType,
		MetricType:  vector.MetricTypeL2,
	})
	if err != nil {
		log.Printf("⚠️  Milvus 连接失败: %v，使用内存存储", err)
		vectorClient, _ = vector.NewInMemoryClient(vector.Config{
			Collection:  cfg.Collection,
			Dimension:   cfg.EmbeddingDim,
			IndexType:   "HNSW",
			MetricType:  vector.MetricTypeL2,
		})
	}
	defer vectorClient.Close()

	// 初始化 Embedding 客户端
	embedClient, err := embedding.NewClient(embedding.Config{
		Provider: cfg.EmbeddingProvider,
		APIKey:   os.Getenv("OPENAI_API_KEY"),
		BaseURL:  cfg.OllamaURL,
		Model:    cfg.EmbeddingModel,
	})
	if err != nil {
		log.Fatalf("❌ Embedding 服务初始化失败: %v", err)
	}

	// 初始化文本切片器
	splitter := text.NewMarkdownSplitter()

	// 初始化数据库（用于存储文档元信息和会话历史）
	db, err := repository.NewSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}
	defer db.Close()

	// 初始化服务层
	docService := service.NewDocumentService(embedClient, vectorClient, splitter, db)
	ragService := service.NewRAGService(embedClient, vectorClient, db)

	// 初始化处理器
	docHandler := handler.NewDocumentHandler(docService)
	chatHandler := handler.NewChatHandler(ragService)

	// 初始化 Gin 路由
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggerMiddleware())

	// 注册路由
	api := r.Group("/api/v1")
	{
		// 文档管理接口
		docs := api.Group("/documents")
		{
			docs.POST("/upload", docHandler.Upload)
			docs.GET("/list", docHandler.List)
			docs.GET("/:id", docHandler.Get)
			docs.DELETE("/:id", docHandler.Delete)
		}

		// 问答接口
		chat := api.Group("/chat")
		{
			chat.POST("/ask", chatHandler.Ask)
			chat.GET("/history/:session_id", chatHandler.History)
		}

		// 健康检查
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"version": "1.0",
			})
		})
	}

	// 启动服务器
	addr := cfg.ServerAddr
	log.Printf("✅ 服务器启动成功: http://%s", addr)
	log.Printf("📚 API 文档: http://%s/api/v1/health", addr)
	log.Printf("📖 上传文档: POST http://%s/api/v1/documents/upload", addr)
	log.Printf("💬 知识问答: POST http://%s/api/v1/chat/ask", addr)

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := r.Run(addr); err != nil {
			log.Fatalf("❌ 服务器启动失败: %v", err)
		}
	}()

	<-quit
	log.Println("👋 服务器关闭中...")
}

// loggerMiddleware 日志中间件
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("[%s] %s %s", c.ClientIP(), c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}