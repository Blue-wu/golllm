package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
)

type InMemoryClient struct {
	data      map[int64]*SearchResult
	mu        sync.RWMutex
	config    Config
	filePath  string
}

func NewInMemoryClient(cfg Config) (*InMemoryClient, error) {
	err := os.MkdirAll("./data", 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	client := &InMemoryClient{
		data:     make(map[int64]*SearchResult),
		config:   cfg,
		filePath: fmt.Sprintf("./data/%s.json", cfg.Collection),
	}

	err = client.loadFromFile()
	if err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to load from file: %v", err)
	}

	return client, nil
}

func (c *InMemoryClient) loadFromFile() error {
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return err
	}

	var results []SearchResult
	err = json.Unmarshal(data, &results)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range results {
		c.data[r.ID] = &r
	}

	return nil
}

func (c *InMemoryClient) saveToFile() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make([]SearchResult, 0, len(c.data))
	for _, r := range c.data {
		results = append(results, *r)
	}

	data, err := json.Marshal(results)
	if err != nil {
		return err
	}

	return os.WriteFile(c.filePath, data, 0644)
}

func (c *InMemoryClient) Close() error {
	return c.saveToFile()
}

func (c *InMemoryClient) CreateCollection(ctx context.Context, indexType string) error {
	log.Printf("Collection '%s' created successfully", c.config.Collection)
	return nil
}

func (c *InMemoryClient) DropCollection(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[int64]*SearchResult)
	err := os.Remove(c.filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to drop collection: %w", err)
	}

	log.Printf("Collection '%s' dropped successfully", c.config.Collection)
	return nil
}

func (c *InMemoryClient) HasCollection(ctx context.Context) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data) > 0, nil
}

func (c *InMemoryClient) ListCollections(ctx context.Context) ([]string, error) {
	files, err := os.ReadDir("./data")
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	var collections []string
	for _, file := range files {
		name := file.Name()
		if len(name) > 5 && name[len(name)-5:] == ".json" {
			collections = append(collections, name[:len(name)-5])
		}
	}
	return collections, nil
}

func (c *InMemoryClient) CreateIndex(ctx context.Context, indexType string, params map[string]string) error {
	log.Printf("In-memory storage doesn't need index, skipping")
	return nil
}

func (c *InMemoryClient) Insert(ctx context.Context, id int64, vector []float32, text string, metadata map[string]interface{}) error {
	var metaStr string
	if metadata != nil {
		metaBytes, _ := json.Marshal(metadata)
		metaStr = string(metaBytes)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[id] = &SearchResult{
		ID:       id,
		Vector:   vector,
		Text:     text,
		Metadata: metaStr,
	}

	return nil
}

func (c *InMemoryClient) BatchInsert(ctx context.Context, ids []int64, vectors [][]float32, texts []string, metadatas []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, id := range ids {
		c.data[id] = &SearchResult{
			ID:       id,
			Vector:   vectors[i],
			Text:     texts[i],
			Metadata: metadatas[i],
		}
	}

	return nil
}

func (c *InMemoryClient) GetByID(ctx context.Context, id int64) (*SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result, ok := c.data[id]
	if !ok {
		return nil, fmt.Errorf("no record found with id: %d", id)
	}

	return result, nil
}

func (c *InMemoryClient) Search(ctx context.Context, queryVector []float32, limit int, params map[string]string) ([]SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []SearchResult
	for _, r := range c.data {
		score := cosineDistance(queryVector, r.Vector)
		results = append(results, SearchResult{
			ID:       r.ID,
			Score:    score,
			Vector:   r.Vector,
			Text:     r.Text,
			Metadata: r.Metadata,
		})
	}

	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score < results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (c *InMemoryClient) DeleteByID(ctx context.Context, ids []int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, id := range ids {
		delete(c.data, id)
	}

	return nil
}

func (c *InMemoryClient) Flush(ctx context.Context) error {
	return c.saveToFile()
}

func cosineDistance(a, b []float32) float32 {
	var dot, magA, magB float32
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	magA = float32(math.Sqrt(float64(magA)))
	magB = float32(math.Sqrt(float64(magB)))
	if magA == 0 || magB == 0 {
		return 1
	}
	return 1 - dot/(magA*magB)
}