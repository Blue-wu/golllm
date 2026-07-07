package model

import "time"

// Document 文档模型
type Document struct {
	ID          int64     `json:"id"`          // 文档ID
	Title       string    `json:"title"`       // 文档标题
	FileName    string    `json:"file_name"`   // 文件名
	FileSize    int64     `json:"file_size"`   // 文件大小
	Content     string    `json:"content"`     // 文档内容
	ChunkCount  int       `json:"chunk_count"` // 切片数量
	Status      string    `json:"status"`      // 状态: pending, processing, completed, failed
	ErrorMsg    string    `json:"error_msg"`   // 错误信息
	CreatedAt   time.Time `json:"created_at"`  // 创建时间
	UpdatedAt   time.Time `json:"updated_at"`  // 更新时间
}

// DocumentChunk 文档切片模型（子块）
type DocumentChunk struct {
	ID         int64                  `json:"id"`         // 切片ID
	DocID      int64                  `json:"doc_id"`     // 文档ID
	ParentID   int64                  `json:"parent_id"`  // 父块ID
	Content    string                 `json:"content"`    // 切片内容
	ChunkIndex int                    `json:"chunk_index"`// 切片索引
	Metadata   map[string]interface{} `json:"metadata"`   // 元数据
	VectorID   string                 `json:"vector_id"`  // 向量ID
}

// ParentChunk 父块模型
type ParentChunk struct {
	ID        int64                  `json:"id"`         // 父块ID
	DocID     int64                  `json:"doc_id"`     // 文档ID
	Content   string                 `json:"content"`    // 父块内容
	Heading   string                 `json:"heading"`    // 标题
	Metadata  map[string]interface{} `json:"metadata"`   // 元数据
}

// SemanticChunk 语义切片结果
type SemanticChunk struct {
	ParentContent string   // 父块内容
	ChildContents []string // 子块内容列表
	Heading       string   // 标题
}

// UploadRequest 文档上传请求
type UploadRequest struct {
	Title string `form:"title" json:"title"` // 文档标题（可选，默认使用文件名）
}

// UploadResponse 文档上传响应
type UploadResponse struct {
	DocumentID int64  `json:"document_id"` // 文档ID
	Title     string `json:"title"`       // 文档标题
	Chunks    int    `json:"chunks"`     // 切片数量
	Message   string `json:"message"`     // 提示信息
}

// DocumentListResponse 文档列表响应
type DocumentListResponse struct {
	Total      int64       `json:"total"`      // 总数
	Page       int         `json:"page"`       // 当前页
	PageSize   int         `json:"page_size"`  // 每页数量
	Documents  []Document  `json:"documents"`  // 文档列表
}

// DocumentDetailResponse 文档详情响应
type DocumentDetailResponse struct {
	Document Document        `json:"document"` // 文档信息
	Chunks   []DocumentChunk `json:"chunks"`  // 切片列表
}

// DocumentStatus 文档状态常量
const (
	DocStatusPending    = "pending"    // 待处理
	DocStatusProcessing = "processing" // 处理中
	DocStatusCompleted  = "completed"  // 已完成
	DocStatusFailed     = "failed"     // 失败
)