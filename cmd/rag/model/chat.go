package model

import "time"

// ChatSession 会话模型
type ChatSession struct {
	ID        int64     `json:"id"`        // 会话ID
	SessionID string    `json:"session_id"` // 会话UUID
	Title     string    `json:"title"`      // 会话标题
	CreatedAt time.Time `json:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at"` // 更新时间
}

// ChatMessage 聊天消息模型
type ChatMessage struct {
	ID        int64     `json:"id"`        // 消息ID
	SessionID string    `json:"session_id"` // 会话UUID
	Role      string    `json:"role"`       // 角色: user, assistant, system
	Content   string    `json:"content"`    // 消息内容
	References []Reference `json:"references,omitempty"` // 引用文档
	CreatedAt time.Time `json:"created_at"` // 创建时间
}

// Reference 引用文档片段
type Reference struct {
	DocID    int64  `json:"doc_id"`    // 文档ID
	ChunkID  int64  `json:"chunk_id"` // 切片ID
	Content  string `json:"content"`  // 引用内容
	Score    float64 `json:"score"`   // 相似度分数
	FileName string `json:"file_name"`// 文件名
}

// AskRequest 问答请求
type AskRequest struct {
	Question  string `json:"question" binding:"required"` // 问题
	SessionID string `json:"session_id"`                  // 会话ID（可选）
	TopK      int    `json:"top_k"`                        // 召回数量
}

// AskResponse 问答响应
type AskResponse struct {
	SessionID  string      `json:"session_id"`  // 会话ID
	Question   string      `json:"question"`     // 问题
	Answer     string      `json:"answer"`       // 回答
	References []Reference `json:"references"`   // 引用文档
	Cost       int         `json:"cost"`          // Token 消耗（估算）
}

// ChatHistoryResponse 聊天历史响应
type ChatHistoryResponse struct {
	SessionID string        `json:"session_id"` // 会话ID
	Title     string        `json:"title"`      // 会话标题
	Messages  []ChatMessage `json:"messages"`  // 消息列表
}

// 角色常量
const (
	RoleUser      = "user"      // 用户
	RoleAssistant = "assistant" // 助手
	RoleSystem    = "system"    // 系统
)