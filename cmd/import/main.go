package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "import",
		Usage: "Import documents to Milvus vector database",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "milvus", Value: "localhost:19530", Usage: "Milvus address"},
			&cli.StringFlag{Name: "collection", Value: "documents", Usage: "Collection name"},
			&cli.IntFlag{Name: "dim", Value: 1536, Usage: "Vector dimension"},
			&cli.StringFlag{Name: "api-key", Usage: "Embedding API key"},
			&cli.StringFlag{Name: "provider", Value: "deepseek", Usage: "Embedding provider: openai, deepseek"},
			&cli.StringFlag{Name: "file", Required: true, Usage: "Document file path"},
			&cli.StringFlag{Name: "splitter", Value: "fixed", Usage: "Text splitter: fixed, paragraph, markdown"},
			&cli.IntFlag{Name: "chunk-size", Value: 500, Usage: "Chunk size for fixed splitter"},
			&cli.IntFlag{Name: "chunk-overlap", Value: 50, Usage: "Chunk overlap for fixed splitter"},
		},
		Action: importDocument,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func importDocument(c *cli.Context) error {
	milvusAddr := c.String("milvus")
	collection := c.String("collection")
	dim := c.Int("dim")
	apiKey := c.String("api-key")
	provider := c.String("provider")
	filePath := c.String("file")
	splitterType := c.String("splitter")
	chunkSize := c.Int("chunk-size")
	chunkOverlap := c.Int("chunk-overlap")

	log.Printf("Loading document: %s", filePath)
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var chunks []text.Chunk
	switch splitterType {
	case "fixed":
		splitter := text.NewFixedLengthSplitter(chunkSize, chunkOverlap)
		chunks = splitter.Split(string(content))
	case "paragraph":
		splitter := text.NewParagraphSplitter()
		chunks = splitter.Split(string(content))
	case "markdown":
		splitter := text.NewMarkdownSplitter()
		chunks = splitter.Split(string(content))
	default:
		return fmt.Errorf("unsupported splitter type: %s", splitterType)
	}

	log.Printf("Split into %d chunks", len(chunks))

	embClient := embedding.NewClient(embedding.Config{
		APIKey:   apiKey,
		Provider: provider,
		Model:    "deepseek-embedding",
	})

	log.Printf("Generating embeddings for %d chunks...", len(chunks))
	vectors, err := embClient.Embeddings(context.Background(), extractTexts(chunks))
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	log.Printf("Connecting to Milvus: %s", milvusAddr)
	vecClient, err := vector.NewClient(vector.Config{
		Addr:       milvusAddr,
		Collection: collection,
		Dim:        dim,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to Milvus: %w", err)
	}
	defer vecClient.Close()

	hasCollection, err := vecClient.HasCollection(context.Background())
	if err != nil {
		return fmt.Errorf("failed to check collection: %w", err)
	}

	if !hasCollection {
		log.Printf("Creating collection: %s", collection)
		if err := vecClient.CreateCollection(context.Background()); err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
		log.Printf("Creating HNSW index...")
		if err := vecClient.CreateIndex(context.Background(), "HNSW", map[string]string{}); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Printf("Inserting %d vectors...", len(vectors))
	ids := make([]int64, len(chunks))
	texts := make([]string, len(chunks))
	metadatas := make([]string, len(chunks))

	for i, chunk := range chunks {
		ids[i] = int64(time.Now().UnixNano()) + int64(i)
		texts[i] = chunk.Content
		meta := map[string]interface{}{
			"filename": filepath.Base(filePath),
			"offset":   chunk.Start,
		}
		for k, v := range chunk.Metadata {
			meta[k] = v
		}
		metaBytes, _ := json.Marshal(meta)
		metadatas[i] = string(metaBytes)
	}

	if err := vecClient.BatchInsert(context.Background(), ids, vectors, texts, metadatas); err != nil {
		return fmt.Errorf("failed to batch insert: %w", err)
	}

	if err := vecClient.Flush(context.Background()); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	log.Printf("Successfully imported %d chunks to collection '%s'", len(chunks), collection)
	return nil
}

func extractTexts(chunks []text.Chunk) []string {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return texts
}