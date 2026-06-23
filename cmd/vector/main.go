package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	ctx := context.Background()

	vectorClient, err := vector.NewVectorClient(vector.Config{
		Addr:       "localhost:19530",
		Collection: "documents",
		Dim:        768, // nomic-embed-text 向量维度
	}, true) // true = 使用真实 Milvus
	if err != nil {
		log.Fatalf("Failed to create vector client: %v", err)
	}
	defer vectorClient.Close()

	switch command {
	case "create-collection":
		err := vectorClient.CreateCollection(ctx, "HNSW")
		if err != nil {
			log.Fatalf("Failed to create collection: %v", err)
		}
		fmt.Println("Collection created successfully")

	case "drop-collection":
		err := vectorClient.DropCollection(ctx)
		if err != nil {
			log.Fatalf("Failed to drop collection: %v", err)
		}
		fmt.Println("Collection dropped successfully")

	case "list-collections":
		cols, err := vectorClient.ListCollections(ctx)
		if err != nil {
			log.Fatalf("Failed to list collections: %v", err)
		}
		fmt.Println("Collections:")
		for _, col := range cols {
			fmt.Printf("  - %s\n", col)
		}

	case "create-index":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run cmd/vector/main.go create-index <HNSW|IVF_FLAT>")
			return
		}
		indexType := os.Args[2]
		err := vectorClient.CreateIndex(ctx, indexType, map[string]string{"nlist": "128"})
		if err != nil {
			log.Fatalf("Failed to create index: %v", err)
		}
		fmt.Printf("Index '%s' created successfully\n", indexType)

	case "insert":
		if len(os.Args) < 4 {
			fmt.Println("Usage: go run cmd/vector/main.go insert <id> <text>")
			return
		}
		id, _ := strconv.ParseInt(os.Args[2], 10, 64)
		textContent := os.Args[3]

		// 使用 Ollama 本地模型（无需 API Key）
		embClient := embedding.NewClient(embedding.Config{
			Model:    "nomic-embed-text",
			Provider: "ollama",
		})

		vectorData, err := embClient.Embedding(ctx, textContent)
		if err != nil {
			log.Fatalf("Failed to create embedding: %v", err)
		}

		err = vectorClient.Insert(ctx, id, vectorData, textContent, nil)
		if err != nil {
			log.Fatalf("Failed to insert: %v", err)
		}
		fmt.Printf("Inserted id=%d successfully\n", id)

	case "batch-insert":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run cmd/vector/main.go batch-insert <file.txt>")
			return
		}
		filePath := os.Args[2]
		err := batchInsertFromFile(ctx, vectorClient, filePath)
		if err != nil {
			log.Fatalf("Batch insert failed: %v", err)
		}
		fmt.Println("Batch insert completed successfully")

	case "query":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run cmd/vector/main.go query <id>")
			return
		}
		id, _ := strconv.ParseInt(os.Args[2], 10, 64)
		result, err := vectorClient.GetByID(ctx, id)
		if err != nil {
			log.Fatalf("Query failed: %v", err)
		}
		fmt.Printf("ID: %d\nText: %s\n", result.ID, result.Text)

	case "delete":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run cmd/vector/main.go delete <id1,id2,...>")
			return
		}
		idsStr := os.Args[2]
		var ids []int64
		for _, s := range strings.Split(idsStr, ",") {
			id, _ := strconv.ParseInt(s, 10, 64)
			ids = append(ids, id)
		}
		err := vectorClient.DeleteByID(ctx, ids)
		if err != nil {
			log.Fatalf("Delete failed: %v", err)
		}
		fmt.Println("Delete completed successfully")

	case "search":
		if len(os.Args) < 4 {
			fmt.Println("Usage: go run cmd/vector/main.go search <query> <limit>")
			return
		}
		query := os.Args[2]
		limit, _ := strconv.Atoi(os.Args[3])

		// 使用 Ollama 本地模型（无需 API Key）
		embClient := embedding.NewClient(embedding.Config{
			Model:    "nomic-embed-text",
			Provider: "ollama",
		})

		queryVector, err := embClient.Embedding(ctx, query)
		if err != nil {
			log.Fatalf("Failed to create query embedding: %v", err)
		}

		results, err := vectorClient.Search(ctx, queryVector, limit, map[string]string{"ef": "128"})
		if err != nil {
			log.Fatalf("Search failed: %v", err)
		}

		fmt.Printf("Search results (top %d):\n", limit)
		for i, res := range results {
			fmt.Printf("%d. Score: %.4f\n   Text: %s\n\n", i+1, res.Score, truncate(res.Text, 200))
		}

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

func batchInsertFromFile(ctx context.Context, vc vector.VectorClient, filePath string) error {
	content, err := text.ReadFile(filePath)
	if err != nil {
		return err
	}

	fileType := text.DetectFileType(filePath)
	var splitter text.Splitter
	switch fileType {
	case "markdown":
		splitter = text.NewMarkdownSplitter()
	default:
		splitter = text.NewFixedLengthSplitter(500, 50)
	}

	chunks := splitter.Split(content)
	fmt.Printf("Split into %d chunks\n", len(chunks))

	// 使用 Ollama 本地模型（无需 API Key）
	embClient := embedding.NewClient(embedding.Config{
		Model:    "nomic-embed-text",
		Provider: "ollama",
	})

	ids := make([]int64, 0, len(chunks))
	vectors := make([][]float32, 0, len(chunks))
	texts := make([]string, 0, len(chunks))
	metadatas := make([]string, 0, len(chunks))

	for i, chunk := range chunks {
		vec, err := embClient.Embedding(ctx, chunk.Content)
		if err != nil {
			return fmt.Errorf("failed to embed chunk %d: %w", i, err)
		}

		metaBytes, _ := json.Marshal(chunk.Metadata)

		ids = append(ids, int64(i+1))
		vectors = append(vectors, vec)
		texts = append(texts, chunk.Content)
		metadatas = append(metadatas, string(metaBytes))

		if len(ids) >= 100 || i == len(chunks)-1 {
			err := vc.BatchInsert(ctx, ids, vectors, texts, metadatas)
			if err != nil {
				return fmt.Errorf("batch insert failed: %w", err)
			}
			fmt.Printf("Inserted %d chunks\n", len(ids))
			ids = ids[:0]
			vectors = vectors[:0]
			texts = texts[:0]
			metadatas = metadatas[:0]
		}
	}

	return vc.Flush(ctx)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  go run cmd/vector/main.go <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  create-collection          Create collection")
	fmt.Println("  drop-collection            Drop collection")
	fmt.Println("  list-collections           List all collections")
	fmt.Println("  create-index <type>        Create index (HNSW|IVF_FLAT)")
	fmt.Println("  insert <id> <text>         Insert single document")
	fmt.Println("  batch-insert <file>        Batch insert from file")
	fmt.Println("  query <id>                 Query by ID")
	fmt.Println("  delete <ids>               Delete by IDs (comma separated)")
	fmt.Println("  search <query> <limit>     Similarity search")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  OPENAI_API_KEY - API key for embedding service")
}