package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/vector"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/search/main.go <query> [limit]")
		fmt.Println("Example: go run cmd/search/main.go \"Go语言并发编程\" 3")
		return
	}

	query := os.Args[1]
	limit := 3
	if len(os.Args) > 2 {
		limit, _ = strconv.Atoi(os.Args[2])
	}

	ctx := context.Background()

	vectorClient, err := vector.NewClient(vector.Config{
		Addr:       "localhost:19530",
		Collection: "documents",
		Dim:        1536,
	})
	if err != nil {
		log.Fatalf("Failed to connect to Milvus: %v", err)
	}
	defer vectorClient.Close()

	hasCol, err := vectorClient.HasCollection(ctx)
	if err != nil {
		log.Fatalf("Failed to check collection: %v", err)
	}
	if !hasCol {
		fmt.Println("Error: Collection 'documents' does not exist")
		fmt.Println("Please run: go run cmd/vector/main.go create-collection")
		return
	}

	embClient := embedding.NewClient(embedding.Config{
		APIKey:   os.Getenv("OPENAI_API_KEY"),
		Model:    "text-embedding-3-small",
		Provider: "openai",
	})

	queryVector, err := embClient.Embedding(ctx, query)
	if err != nil {
		log.Fatalf("Failed to create query embedding: %v", err)
	}

	results, err := vectorClient.Search(ctx, queryVector, limit, map[string]string{"ef": "128"})
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	fmt.Printf("\n=== 搜索结果 (关键词: %s) ===\n\n", query)
	if len(results) == 0 {
		fmt.Println("未找到相关文档")
		return
	}

	for i, res := range results {
		fmt.Printf("【结果 %d】相似度: %.4f\n", i+1, 1-res.Score)
		fmt.Println("------------------------")
		fmt.Println(formatText(res.Text))
		fmt.Println()
	}
}

func formatText(text string) string {
	lines := strings.Split(text, "\n")
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		if len(line) > 100 {
			result += line[:100] + "..."
		} else {
			result += line
		}
	}
	return result
}