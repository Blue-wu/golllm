package main

import (
	"fmt"

	"github.com/golllm/cmd/rag/config"
)

func main() {
	cfg, err := config.LoadConfig("config.toml")
	if err != nil {
		fmt.Printf("Load config failed: %v\n", err)
		return
	}

	fmt.Printf("Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("VectorDB Type: %s\n", cfg.VectorDB.Type)
	fmt.Printf("VectorDB Addr: %s\n", cfg.VectorDB.Addr)
	fmt.Printf("Embedding Provider: %s\n", cfg.Embedding.Provider)
	fmt.Printf("Embedding Model: %s\n", cfg.Embedding.Model)
	fmt.Printf("LLM Provider: %s\n", cfg.LLM.Provider)
	fmt.Printf("LLM Model: %s\n", cfg.LLM.Model)
	fmt.Printf("RAG TopK: %d\n", cfg.RAG.TopK)
}
