package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// Config Milvus 客户端配置
type Config struct {
	Addr       string // Milvus 服务器地址，例如 "localhost:19530"
	Collection string // 集合名称
	Dim        int    // 向量维度，例如 1536（OpenAI embedding-ada-002）
}

// Client Milvus 客户端封装
type Client struct {
	config         Config          // 客户端配置
	milvusClient   client.Client   // Milvus SDK 原生客户端
}

// NewClient 创建新的 Milvus 客户端
// 参数: cfg - 客户端配置
// 返回: 客户端实例或错误
func NewClient(cfg Config) (*Client, error) {
	c, err := client.NewClient(context.Background(), client.Config{
		Address: cfg.Addr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to milvus: %w", err)
	}

	return &Client{
		config:       cfg,
		milvusClient: c,
	}, nil
}

// Close 关闭客户端连接
func (c *Client) Close() error {
	return c.milvusClient.Close()
}

// CreateCollection 创建新的集合（Collection）
// 集合包含以下字段:
//   - id: 主键（Int64）
//   - vector: 向量字段（FloatVector）
//   - text: 原始文本（VarChar）
//   - metadata: 元数据（JSON）
func (c *Client) CreateCollection(ctx context.Context, indexType string) error {
	// 创建 ID 字段：主键，非自增
	idField := entity.NewField().
		WithName("id").
		WithDataType(entity.FieldTypeInt64).
		WithIsPrimaryKey(true).
		WithIsAutoID(false)

	// 创建向量字段：浮点向量，维度由配置决定
	vectorField := entity.NewField().
		WithName("vector").
		WithDataType(entity.FieldTypeFloatVector).
		WithDim(int64(c.config.Dim))

	// 创建文本字段：最大长度 65535 字符
	textField := entity.NewField().
		WithName("text").
		WithDataType(entity.FieldTypeVarChar).
		WithMaxLength(65535)

	// 创建元数据字段：JSON 格式
	metadataField := entity.NewField().
		WithName("metadata").
		WithDataType(entity.FieldTypeJSON)

	// 构建集合 Schema
	schema := entity.NewSchema().
		WithName(c.config.Collection).
		WithDescription("Document embedding collection").
		WithField(idField).
		WithField(vectorField).
		WithField(textField).
		WithField(metadataField)

	// 创建集合，shardNum=1 表示单分片
	err := c.milvusClient.CreateCollection(ctx, schema, 1)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	// 创建索引（HNSW 索引，搜索前必须创建索引）
	idx, err := entity.NewIndexHNSW(entity.L2, 16, 128)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	err = c.milvusClient.CreateIndex(ctx, c.config.Collection, "vector", idx, false)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	// 加载集合到内存（搜索前必须加载）
	err = c.milvusClient.LoadCollection(ctx, c.config.Collection, false)
	if err != nil {
		return fmt.Errorf("failed to load collection: %w", err)
	}

	log.Printf("Collection '%s' created with HNSW index and loaded successfully", c.config.Collection)
	return nil
}

// DropCollection 删除集合
func (c *Client) DropCollection(ctx context.Context) error {
	err := c.milvusClient.DropCollection(ctx, c.config.Collection)
	if err != nil {
		return fmt.Errorf("failed to drop collection: %w", err)
	}
	log.Printf("Collection '%s' dropped successfully", c.config.Collection)
	return nil
}

// HasCollection 检查集合是否存在
func (c *Client) HasCollection(ctx context.Context) (bool, error) {
	return c.milvusClient.HasCollection(ctx, c.config.Collection)
}

// ListCollections 列出所有集合名称
func (c *Client) ListCollections(ctx context.Context) ([]string, error) {
	cols, err := c.milvusClient.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}
	result := make([]string, 0, len(cols))
	for _, col := range cols {
		result = append(result, col.Name)
	}
	return result, nil
}

// CreateIndex 为向量字段创建索引
// 支持的索引类型:
//   - HNSW: Hierarchical Navigable Small World，适合小规模数据、高召回率
//     参数: M (连接数，默认16), efConstruction (构建时搜索范围，默认200)
//   - IVF_FLAT: Inverted File with Flat，适合大规模数据、追求速度
//     参数: nlist (聚类中心数，默认128)
func (c *Client) CreateIndex(ctx context.Context, indexType string, params map[string]string) error {
	var index entity.Index
	var err error

	switch indexType {
	case "HNSW":
		// HNSW 索引参数
		M := 16
		efConstruction := 200
		if val, ok := params["M"]; ok {
			fmt.Sscanf(val, "%d", &M)
		}
		if val, ok := params["efConstruction"]; ok {
			fmt.Sscanf(val, "%d", &efConstruction)
		}
		index, err = entity.NewIndexHNSW(entity.L2, M, efConstruction)
	case "IVF_FLAT":
		// IVF_FLAT 索引参数
		nlist := 128
		if val, ok := params["nlist"]; ok {
			fmt.Sscanf(val, "%d", &nlist)
		}
		index, err = entity.NewIndexIvfFlat(entity.L2, nlist)
	default:
		return fmt.Errorf("unsupported index type: %s", indexType)
	}

	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	// 在 vector 字段上创建索引，async=false 表示同步创建
	err = c.milvusClient.CreateIndex(ctx, c.config.Collection, "vector", index, false)
	if err != nil {
		return fmt.Errorf("failed to create index on collection: %w", err)
	}

	log.Printf("Index '%s' created successfully on collection '%s'", indexType, c.config.Collection)
	return nil
}

// Insert 插入单条向量数据
// 参数:
//   - id: 数据 ID
//   - vector: 向量数据
//   - text: 原始文本
//   - metadata: 元数据（可为 nil）
func (c *Client) Insert(ctx context.Context, id int64, vector []float32, text string, metadata map[string]interface{}) error {
	// 将数据转换为列格式（Milvus 要求）
	ids := []int64{id}
	vectors := [][]float32{vector}
	texts := []string{text}

	// 序列化元数据
	var metaBytes []byte
	if metadata != nil {
		metaBytes, _ = json.Marshal(metadata)
	} else {
		metaBytes = []byte("{}")
	}
	metadatas := [][]byte{metaBytes}

	// 创建列对象
	idCol := entity.NewColumnInt64("id", ids)
	vecCol := entity.NewColumnFloatVector("vector", c.config.Dim, vectors)
	textCol := entity.NewColumnVarChar("text", texts)
	metaCol := entity.NewColumnJSONBytes("metadata", metadatas)

	// 插入数据（partitionName 为空表示默认分区）
	_, err := c.milvusClient.Insert(ctx, c.config.Collection, "", idCol, vecCol, textCol, metaCol)
	if err != nil {
		return fmt.Errorf("failed to insert: %w", err)
	}
	return nil
}

// BatchInsert 批量插入向量数据
// 参数:
//   - ids: ID 列表
//   - vectors: 向量列表
//   - texts: 文本列表
//   - metadatas: 元数据列表（JSON 字符串）
func (c *Client) BatchInsert(ctx context.Context, ids []int64, vectors [][]float32, texts []string, metadatas []string) error {
	// 创建列对象
	idCol := entity.NewColumnInt64("id", ids)
	vecCol := entity.NewColumnFloatVector("vector", c.config.Dim, vectors)
	textCol := entity.NewColumnVarChar("text", texts)

	// 转换元数据格式
	metaBytes := make([][]byte, len(metadatas))
	for i, m := range metadatas {
		if m == "" {
			metaBytes[i] = []byte("{}")
		} else {
			metaBytes[i] = []byte(m)
		}
	}
	metaCol := entity.NewColumnJSONBytes("metadata", metaBytes)

	// 批量插入
	_, err := c.milvusClient.Insert(ctx, c.config.Collection, "", idCol, vecCol, textCol, metaCol)
	if err != nil {
		return fmt.Errorf("failed to batch insert: %w", err)
	}
	return nil
}

// GetByID 根据 ID 查询单条记录
func (c *Client) GetByID(ctx context.Context, id int64) (*SearchResult, error) {
	// 构建查询表达式
	expr := fmt.Sprintf("id == %d", id)
	result, err := c.milvusClient.Query(ctx, c.config.Collection, nil, expr, []string{"id", "vector", "text", "metadata"})
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	if result.Len() == 0 {
		return nil, fmt.Errorf("no record found with id: %d", id)
	}

	return parseQueryResultSet(result), nil
}

// Search 向量相似度检索
// 参数:
//   - queryVector: 查询向量
//   - limit: 返回结果数量
//   - params: 搜索参数（如 ef）
// 返回: 按相似度排序的结果列表
func (c *Client) Search(ctx context.Context, queryVector []float32, limit int, params map[string]string) ([]SearchResult, error) {
	// HNSW 搜索参数：ef 表示搜索时的候选数量
	ef := 128
	if val, ok := params["ef"]; ok {
		fmt.Sscanf(val, "%d", &ef)
	}

	sp, err := entity.NewIndexHNSWSearchParam(ef)
	if err != nil {
		return nil, fmt.Errorf("failed to create search param: %w", err)
	}

	// 将查询向量转换为 Milvus 格式
	vectors := []entity.Vector{entity.FloatVector(queryVector)}

	// 执行搜索
	//   - metricType: entity.L2 表示欧氏距离
	//   - topK: 返回结果数量
	results, err := c.milvusClient.Search(ctx, c.config.Collection, nil, "",
		[]string{"id", "vector", "text", "metadata"}, vectors, "vector", entity.L2, limit, sp)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	return parseSearchResults(&results[0]), nil
}

// DeleteByID 根据 ID 删除记录
func (c *Client) DeleteByID(ctx context.Context, ids []int64) error {
	// 构建删除表达式：id in [1,2,3]
	expr := fmt.Sprintf("id in [%s]", joinInts(ids))
	err := c.milvusClient.Delete(ctx, c.config.Collection, "", expr)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}
	return nil
}

// Flush 强制将内存中的数据持久化到磁盘
func (c *Client) Flush(ctx context.Context) error {
	err := c.milvusClient.Flush(ctx, c.config.Collection, true)
	if err != nil {
		return err
	}
	// 刷新后重新加载 collection
	return c.milvusClient.LoadCollection(ctx, c.config.Collection, false)
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       int64    // 数据 ID
	Score    float32  // 相似度分数（距离，越小越相似）
	Text     string   // 原始文本
	Vector   []float32 // 向量数据
	Metadata string   // 元数据（JSON 字符串）
}

// parseQueryResultSet 解析查询结果集
func parseQueryResultSet(result client.ResultSet) *SearchResult {
	// 从结果集中获取各列数据
	idCol := result.GetColumn("id").(*entity.ColumnInt64)
	vecCol := result.GetColumn("vector").(*entity.ColumnFloatVector)
	textCol := result.GetColumn("text").(*entity.ColumnVarChar)
	metaCol := result.GetColumn("metadata").(*entity.ColumnJSONBytes)

	return &SearchResult{
		ID:       idCol.Data()[0],
		Vector:   vecCol.Data()[0],
		Text:     textCol.Data()[0],
		Metadata: string(metaCol.Data()[0]),
	}
}

// parseSearchResults 解析搜索结果
func parseSearchResults(result *client.SearchResult) []SearchResult {
	results := make([]SearchResult, 0, result.ResultCount)

	// 获取 ID 列
	idCol := result.IDs.(*entity.ColumnInt64)
	// 从 Fields 中获取其他列
	vecCol := result.Fields.GetColumn("vector").(*entity.ColumnFloatVector)
	textCol := result.Fields.GetColumn("text").(*entity.ColumnVarChar)
	metaCol := result.Fields.GetColumn("metadata").(*entity.ColumnJSONBytes)

	// 遍历结果，构建 SearchResult 列表
	for i := 0; i < result.ResultCount; i++ {
		results = append(results, SearchResult{
			ID:       idCol.Data()[i],
			Score:    result.Scores[i],  // 相似度分数
			Text:     textCol.Data()[i],
			Vector:   vecCol.Data()[i],
			Metadata: string(metaCol.Data()[i]),
		})
	}

	return results
}

// joinInts 将 ID 列表转换为逗号分隔的字符串
func joinInts(ids []int64) string {
	result := ""
	for i, id := range ids {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%d", id)
	}
	return result
}