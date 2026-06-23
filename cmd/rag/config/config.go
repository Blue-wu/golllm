package config

import (
	"os"
)

// Config 应用配置
type Config struct {
	// 服务器配置
	ServerAddr string

	// Milvus 配置
	MilvusAddr  string
	Collection  string
	EmbeddingDim int
	IndexType   string

	// Embedding 配置
	EmbeddingProvider string
	EmbeddingModel   string
	OllamaURL        string

	// 数据库配置
	DBPath string
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServerAddr:       getEnv("SERVER_ADDR", "localhost:8080"),
		MilvusAddr:       getEnv("MILVUS_ADDR", "localhost:19530"),
		Collection:       getEnv("MILVUS_COLLECTION", "documents"),
		EmbeddingDim:     768, // nomic-embed-text 维度
		IndexType:        "HNSW",
		EmbeddingProvider: getEnv("EMBEDDING_PROVIDER", "ollama"),
		EmbeddingModel:   getEnv("EMBEDDING_MODEL", "nomic-embed-text"),
		OllamaURL:        getEnv("OLLAMA_URL", "http://localhost:11434"),
		DBPath:           getEnv("DB_PATH", "./data/rag.db"),
	}
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}