package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Server    ServerConfig
	VectorDB  VectorDBConfig
	Embedding EmbeddingConfig
	LLM       LLMConfig
	RAG       RAGConfig
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

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{
		Server:    ServerConfig{Host: "localhost", Port: 8080},
		VectorDB:  VectorDBConfig{Type: "milvus", Addr: "localhost:19530", Collection: "documents", Dim: 768},
		Embedding: EmbeddingConfig{Provider: "ollama", Model: "nomic-embed-text"},
		LLM:       LLMConfig{Provider: "ollama", Model: "qwen2.5:7b"},
		RAG:       RAGConfig{TopK: 5, MinScore: 0.5},
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
		}
	}

	cfg.Embedding.APIKey = os.Getenv("OPENAI_API_KEY")
	cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")

	return cfg, nil
}
