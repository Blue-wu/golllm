package vector

import (
	"context"
)

type VectorClient interface {
	Close() error
	CreateCollection(ctx context.Context) error
	DropCollection(ctx context.Context) error
	HasCollection(ctx context.Context) (bool, error)
	ListCollections(ctx context.Context) ([]string, error)
	CreateIndex(ctx context.Context, indexType string, params map[string]string) error
	Insert(ctx context.Context, id int64, vector []float32, text string, metadata map[string]interface{}) error
	BatchInsert(ctx context.Context, ids []int64, vectors [][]float32, texts []string, metadatas []string) error
	GetByID(ctx context.Context, id int64) (*SearchResult, error)
	Search(ctx context.Context, queryVector []float32, limit int, params map[string]string) ([]SearchResult, error)
	Delete(ctx context.Context, ids []int64) error
	Flush(ctx context.Context) error
}

func NewVectorClient(cfg Config, useMilvus bool) (VectorClient, error) {
	if useMilvus {
		return NewClient(cfg)
	}
	return NewInMemoryClient(cfg)
}
