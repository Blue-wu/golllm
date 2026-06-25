package config

import (
	"fmt"
	"os"
)

// Config 应用配置
type Config struct {
	// 服务器配置
	Server ServerConfig

	// 向量数据库配置
	VectorDB VectorDBConfig

	// Embedding 模型配置
	Embedding EmbeddingConfig

	// LLM 模型配置
	LLM LLMConfig

	// RAG 配置
	RAG RAGConfig
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host string
	Port int
}

// VectorDBConfig 向量数据库配置
type VectorDBConfig struct {
	Type      string // "milvus" 或 "memory"
	Addr      string // Milvus 地址
	Collection string
	Dim       int
}

// EmbeddingConfig Embedding 模型配置
type EmbeddingConfig struct {
	Provider string // "openai", "ollama", "deepseek"
	Model   string
	APIKey  string
	BaseURL string // 自定义 API 地址
}

// LLMConfig LLM 模型配置
type LLMConfig struct {
	Provider string // "openai", "ollama", "deepseek"
	Model   string
	APIKey  string
	BaseURL string
}

// RAGConfig RAG 配置
type RAGConfig struct {
	TopK         int     // 召回数量
	MinScore     float32 // 最小相似度分数
	MaxContextLen int    // 最大上下文长度
}

// LoadConfig 加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 简单的配置文件解析
	cfg := &Config{}
	
	lines := string(data)
	
	// 解析 [server]
	if val := getValue(lines, "server.host"); val != "" {
		cfg.Server.Host = val
	} else {
		cfg.Server.Host = "localhost"
	}
	if val := getValue(lines, "server.port"); val != "" {
		fmt.Sscanf(val, "%d", &cfg.Server.Port)
	} else {
		cfg.Server.Port = 8080
	}

	// 解析 [vector]
	cfg.VectorDB.Type = getValue(lines, "vector.type")
	if cfg.VectorDB.Type == "" {
		cfg.VectorDB.Type = "milvus"
	}
	cfg.VectorDB.Addr = getValue(lines, "vector.addr")
	if cfg.VectorDB.Addr == "" {
		cfg.VectorDB.Addr = "localhost:19530"
	}
	cfg.VectorDB.Collection = getValue(lines, "vector.collection")
	if cfg.VectorDB.Collection == "" {
		cfg.VectorDB.Collection = "documents"
	}

	// 解析 [embedding]
	cfg.Embedding.Provider = getValue(lines, "embedding.provider")
	if cfg.Embedding.Provider == "" {
		cfg.Embedding.Provider = "ollama"
	}
	cfg.Embedding.Model = getValue(lines, "embedding.model")
	if cfg.Embedding.Model == "" {
		cfg.Embedding.Model = "nomic-embed-text"
	}
	cfg.Embedding.APIKey = os.Getenv("OPENAI_API_KEY")
	cfg.Embedding.BaseURL = getValue(lines, "embedding.base_url")

	// 解析 [llm]
	cfg.LLM.Provider = getValue(lines, "llm.provider")
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = "ollama"
	}
	cfg.LLM.Model = getValue(lines, "llm.model")
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "qwen2.5:7b"
	}
	cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")
	cfg.LLM.BaseURL = getValue(lines, "llm.base_url")

	// 解析 [rag]
	if val := getValue(lines, "rag.topk"); val != "" {
		fmt.Sscanf(val, "%d", &cfg.RAG.TopK)
	} else {
		cfg.RAG.TopK = 5
	}
	if val := getValue(lines, "rag.min_score"); val != "" {
		fmt.Sscanf(val, "%f", &cfg.RAG.MinScore)
	} else {
		cfg.RAG.MinScore = 0.5
	}

	return cfg, nil
}

// getValue 从配置文本中获取值
func getValue(content, key string) string {
	lines := []byte(content)
	keyBytes := []byte(key + " = ")
	
	for i := 0; i < len(lines)-len(keyBytes); i++ {
		match := true
		for j := 0; j < len(keyBytes); j++ {
			if lines[i+j] != keyBytes[j] {
				match = false
				break
			}
		}
		if match {
			// 找到值
			start := i + len(keyBytes)
			// 去除引号
			if start < len(lines) && lines[start] == '"' {
				start++
				end := start
				for end < len(lines) && lines[end] != '"' && lines[end] != '\n' {
					end++
				}
				return string(lines[start:end])
			}
			// 去除空白
			end := start
			for end < len(lines) && lines[end] != '\n' && lines[end] != '#' {
				end++
			}
			return trimSpaces(string(lines[start:end]))
		}
	}
	return ""
}

// trimSpaces 去除首尾空白
func trimSpaces(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
