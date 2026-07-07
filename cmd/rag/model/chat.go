package model

import "time"

// ChatSession 会话模型
type ChatSession struct {
	ID        int64     `json:"id"`         // 会话ID
	SessionID string    `json:"session_id"` // 会话UUID
	UserID    string    `json:"user_id"`    // 用户ID（用于会话隔离）
	Title     string    `json:"title"`      // 会话标题
	Status    string    `json:"status"`     // 会话状态: active, closed
	Context   string    `json:"context"`    // 对话上下文/状态信息
	CreatedAt time.Time `json:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at"` // 更新时间
}

// ChatMessage 聊天消息模型
type ChatMessage struct {
	ID         int64       `json:"id"`                   // 消息ID
	SessionID  string      `json:"session_id"`           // 会话UUID
	Role       string      `json:"role"`                 // 角色: user, assistant, system
	Content    string      `json:"content"`              // 消息内容
	References []Reference `json:"references,omitempty"` // 引用文档
	CreatedAt  time.Time   `json:"created_at"`           // 创建时间
}

// Reference 引用文档片段
type Reference struct {
	DocID    int64   `json:"doc_id"`    // 文档ID
	ChunkID  int64   `json:"chunk_id"`  // 切片ID
	Content  string  `json:"content"`   // 引用内容
	Score    float64 `json:"score"`     // 相似度分数
	FileName string  `json:"file_name"` // 文件名
}

// AskRequest 问答请求
type AskRequest struct {
	Question  string `json:"question" binding:"required"` // 问题
	SessionID string `json:"session_id"`                  // 会话ID（可选）
	UserID    string `json:"user_id"`                     // 用户ID（用于会话隔离）
	TopK      int    `json:"top_k"`                       // 召回数量
}

// AskResponse 问答响应
type AskResponse struct {
	SessionID  string      `json:"session_id"` // 会话ID
	Question   string      `json:"question"`   // 问题
	Answer     string      `json:"answer"`     // 回答
	References []Reference `json:"references"` // 引用文档
	Cost       int         `json:"cost"`       // Token 消耗（估算）
}

// ChatHistoryResponse 聊天历史响应
type ChatHistoryResponse struct {
	SessionID string        `json:"session_id"` // 会话ID
	Title     string        `json:"title"`      // 会话标题
	Messages  []ChatMessage `json:"messages"`   // 消息列表
}

// 角色常量
const (
	RoleUser      = "user"      // 用户
	RoleAssistant = "assistant" // 助手
	RoleSystem    = "system"    // 系统
)

// 会话状态常量
const (
	SessionStatusActive = "active" // 活跃
	SessionStatusClosed = "closed" // 已关闭
)

// SessionListResponse 会话列表响应
type SessionListResponse struct {
	Sessions []ChatSession `json:"sessions"` // 会话列表
	Total    int64         `json:"total"`    // 总数
}

// SessionCreateRequest 会话创建请求
type SessionCreateRequest struct {
	UserID string `json:"user_id"` // 用户ID
	Title  string `json:"title"`   // 会话标题（可选）
}

// SessionUpdateRequest 会话更新请求
type SessionUpdateRequest struct {
	Title  string `json:"title"`  // 新标题
	Status string `json:"status"` // 新状态
}

// TokenUsage Token使用统计
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`     // 提示词Token数
	CompletionTokens int `json:"completion_tokens"` // 完成Token数
	TotalTokens      int `json:"total_tokens"`      // 总Token数
}
