package main

import (
	"context"
	"fmt"
	"log"

	"github.com/golllm/pkg/embedding"
)

func main() {
	client := embedding.NewClient(embedding.Config{
		Provider: "ollama",
		Model:    "nomic-embed-text",
		BaseURL:  "http://localhost:11434/v1",
	})

	vec, err := client.Embedding(context.Background(), "Hello World")
	if err != nil {
		log.Fatalf("Embedding failed: %v", err)
	}

	fmt.Printf("Vector length: %d\n", len(vec))
	if len(vec) > 0 {
		fmt.Printf("First 5 values: %v\n", vec[:5])
	}
}
