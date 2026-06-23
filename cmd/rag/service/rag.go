package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/prompt"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/vector"

	"github.com/sashabaranov/go-openai"
)

// RAGService RAG 服务
type RAGService struct {
	ctx       context.Context
	embedder  *embedding.Client
	vectorDB  vector.VectorClient
	repo      *repository.SQLiteRepository
	prompt    *prompt.RAGPromptTemplate
	llmClient *openai.Client
}

// NewRAGService 创建 RAG 服务
func NewRAGService(
	embedder *embedding.Client,
	vectorDB vector.VectorClient,
	repo *repository.SQLiteRepository,
) *RAGService {
	return &RAGService{
		ctx:       context.Background(),
		embedder:  embedder,
		vectorDB:  vectorDB,
		repo:      repo,
		prompt:    prompt.NewRAGPromptTemplate(),
		llmClient: openai.NewClient(),
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
		topK = 5
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
	messages := make([]openai.ChatCompletionMessage, 0)

	// 添加历史消息（限制最近 10 条）
	startIdx := 0
	if len(history) > 10 {
		startIdx = len(history) - 10
	}
	for _, msg := range history[startIdx:] {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 添加当前对话
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	})
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	// 6. 调用大模型
	resp, err := s.llmClient.CreateChatCompletion(
		s.ctx,
		openai.ChatCompletionRequest{
			Model:    openai.GPT4oMini,
			Messages: messages,
		},
	)
	if err != nil {
		// 如果是本地模型，尝试使用 Ollama
		return s.askWithLocalModel(req, references)
	}

	answer := resp.Choices[0].Message.Content

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
		Cost:       resp.Usage.TotalTokens,
	}, nil
}

// askWithLocalModel 使用本地模型问答
func (s *RAGService) askWithLocalModel(req *model.AskRequest, references []model.Reference) (*model.AskResponse, error) {
	// 构建完整的 prompt
	fullPrompt, _ := s.prompt.BuildFullPrompt(req.Question, references)

	resp, err := s.llmClient.CreateChatCompletion(
		s.ctx,
		openai.ChatCompletionRequest{
			Model: "llama3.2",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: fullPrompt,
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("大模型调用失败: %w", err)
	}

	answer := resp.Choices[0].Message.Content

	// 保存对话记录
	s.repo.CreateMessage(&model.ChatMessage{
		SessionID:  req.SessionID,
		Role:       model.RoleUser,
		Content:    req.Question,
		References: references,
	})
	s.repo.CreateMessage(&model.ChatMessage{
		SessionID: req.SessionID,
		Role:      model.RoleAssistant,
		Content:   answer,
	})

	return &model.AskResponse{
		SessionID:  req.SessionID,
		Question:   req.Question,
		Answer:     answer,
		References: references,
		Cost:       resp.Usage.TotalTokens,
	}, nil
}

// Recall 召回相关文档
func (s *RAGService) Recall(question string, topK int) ([]model.Reference, error) {
	// 1. 生成问题的向量
	queryVec, err := s.embedder.Embed(s.ctx, question)
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}

	// 2. 向量检索
	results, err := s.vectorDB.Search(s.ctx, queryVec, topK, map[string]string{"ef": "128"})
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	// 3. 转换结果
	references := make([]model.Reference, 0)
	for _, result := range results {
		// 过滤低相似度结果（余弦距离阈值）
		if result.Score > 1.0 { // L2 距离，越小越相似
			continue
		}

		// 解析元数据获取文档ID
		docID := int64(0)
		if docIDStr, ok := result.Metadata["doc_id"]; ok {
			switch v := docIDStr.(type) {
			case float64:
				docID = int64(v)
			case int64:
				docID = v
			}
		}

		references = append(references, model.Reference{
			DocID:    docID,
			Content:  result.Text,
			Score:    result.Score,
			FileName: getFileName(result.Metadata),
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
	if fn, ok := metadata["file_name"]; ok {
		if str, ok := fn.(string); ok {
			return str
		}
	}
	return "未知文档"
}

// ExtractKeywords 提取关键词（用于增强检索）
func (s *RAGService) ExtractKeywords(question string) []string {
	// 简单的关键词提取，实际可用 NLP 库
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