package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golllm/cmd/rag/llm"
	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/prompt"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/vector"
)

// RAGService RAG 问答服务
type RAGService struct {
	ctx       context.Context
	embedder  *embedding.Client
	vectorDB  vector.VectorClient
	repo      *repository.SQLiteRepository
	prompt    *prompt.RAGPromptTemplate
	llmClient llm.Client
	topK      int
	minScore  float32
}

// NewRAGService 创建 RAG 服务
func NewRAGService(
	embedder *embedding.Client,
	vectorDB vector.VectorClient,
	repo *repository.SQLiteRepository,
	llmClient llm.Client,
	topK int,
	minScore float32,
) *RAGService {
	return &RAGService{
		ctx:       context.Background(),
		embedder:  embedder,
		vectorDB:  vectorDB,
		repo:      repo,
		prompt:    prompt.NewRAGPromptTemplate(),
		llmClient: llmClient,
		topK:      topK,
		minScore:  minScore,
	}
}

// Ask 问答
func (s *RAGService) Ask(req *model.AskRequest) (*model.AskResponse, error) {
	// 1. 获取或创建会话
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("session_%d", time.Now().UnixNano())
		s.repo.CreateSession(sessionID)
	}

	// 2. 获取会话历史
	history, err := s.repo.GetChatHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("获取会话历史失败: %w", err)
	}

	// 3. 召回相关文档
	topK := req.TopK
	if topK <= 0 {
		topK = s.topK
	}
	references, err := s.Recall(req.Question, topK)
	if err != nil {
		log.Printf("召回失败: %v", err)
	}

	// 4. 构建 Prompt
	var systemPrompt string
	var userPrompt string

	if len(references) > 0 {
		systemPrompt, _ = s.prompt.BuildSystemPrompt(references)
		userPrompt = s.prompt.BuildQuestionPrompt(req.Question)
	} else {
		// 无召回结果时的处理
		systemPrompt = `你是一个专业的知识库助手。如果用户的问题涉及特定领域或文档内容，请说明你没有相关的参考信息。`
		userPrompt = req.Question
	}

	// 5. 构建消息列表
	messages := make([]llm.Message, 0)

	// 添加历史消息（限制最近 10 条）
	startIdx := 0
	if len(history) > 10 {
		startIdx = len(history) - 10
	}
	for _, msg := range history[startIdx:] {
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 添加当前对话
	messages = append(messages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userPrompt,
	})

	// 6. 调用大模型
	answer, err := s.llmClient.Chat(s.ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	// 7. 保存对话记录
	s.repo.CreateMessage(&model.ChatMessage{
		SessionID:  sessionID,
		Role:       model.RoleUser,
		Content:    req.Question,
		References: references,
	})
	s.repo.CreateMessage(&model.ChatMessage{
		SessionID: sessionID,
		Role:      model.RoleAssistant,
		Content:   answer,
	})

	// 8. 如果是新会话，更新标题
	if len(history) == 0 {
		title := req.Question
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		s.repo.UpdateSessionTitle(sessionID, title)
	}

	return &model.AskResponse{
		SessionID:  sessionID,
		Question:   req.Question,
		Answer:     answer,
		References: references,
		Cost:       0, // 本地模型不计 token
	}, nil
}

// Recall 召回相关文档
func (s *RAGService) Recall(question string, topK int) ([]model.Reference, error) {
	queryVec, err := s.embedder.Embedding(s.ctx, question)
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}

	results, err := s.vectorDB.Search(s.ctx, queryVec, topK, map[string]string{"ef": "128"})
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	references := make([]model.Reference, 0)
	for _, result := range results {
		if result.Score > 1.0 {
			continue
		}

		var metadata map[string]interface{}
		json.Unmarshal([]byte(result.Metadata), &metadata)

		docID := int64(0)
		if metadata != nil {
			if docIDStr, ok := metadata["doc_id"]; ok {
				switch v := docIDStr.(type) {
				case float64:
					docID = int64(v)
				case int64:
					docID = v
				}
			}
		}

		references = append(references, model.Reference{
			DocID:    docID,
			Content:  result.Text,
			Score:    float64(result.Score),
			FileName: getFileName(metadata),
		})
	}

	return references, nil
}

// GetHistory 获取聊天历史
func (s *RAGService) GetHistory(sessionID string) (*model.ChatHistoryResponse, error) {
	session, err := s.repo.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("会话不存在")
	}

	messages, err := s.repo.GetChatHistory(sessionID)
	if err != nil {
		return nil, err
	}

	return &model.ChatHistoryResponse{
		SessionID: sessionID,
		Title:     session.Title,
		Messages:  messages,
	}, nil
}

// getFileName 从元数据中获取文件名
func getFileName(metadata map[string]interface{}) string {
	if metadata == nil {
		return "未知文档"
	}
	if fn, ok := metadata["file_name"]; ok {
		if str, ok := fn.(string); ok {
			return str
		}
	}
	return "未知文档"
}

// ExtractKeywords 提取关键词（用于增强检索）
func (s *RAGService) ExtractKeywords(question string) []string {
	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true, "和": true,
		"我": true, "你": true, "他": true, "她": true, "它": true,
		"这": true, "那": true, "什么": true, "怎么": true, "如何": true,
		"为什么": true, "哪里": true, "哪个": true, "能不能": true,
	}

	words := strings.Fields(question)
	keywords := make([]string, 0)

	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}
