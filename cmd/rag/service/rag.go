package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/golllm/cmd/rag/llm"
	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/prompt"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"
)

// RAGService RAG 核心服务，负责问题召回、上下文管理和回答生成
// 主要功能：
//   - 混合召回：向量检索 + BM25关键词检索，通过RRF算法融合结果
//   - 父子文档映射：子块用于检索匹配，父块用于生成完整回答
//   - 会话管理：维护对话历史，支持上下文感知的连续对话
//   - 回答生成：基于检索到的参考文档，调用LLM生成准确回答
//   - Token管理：Token计数与自动截断，确保不超出模型上下文窗口
type RAGService struct {
	ctx             context.Context              // 全局上下文
	embedder        *embedding.Client            // Embedding客户端，用于问题和文本向量化
	vectorDB        vector.VectorClient          // 向量数据库客户端，用于向量检索
	repo            *repository.SQLiteRepository // 数据仓储，用于文档、会话、消息的持久化
	prompt          *prompt.RAGPromptTemplate    // 提示词模板，用于构建LLM输入
	llmClient       llm.Client                   // LLM客户端，用于生成回答
	topK            int                          // 默认召回数量
	minScore        float32                      // 最低相似度阈值
	bm25Retriever   *text.BM25Retriever          // BM25关键词检索器
	sessionManager  *SessionManager              // 会话管理器，用于会话缓存和持久化
	tokenManager    *TokenManager                // Token管理器，用于Token计数和截断
	dialogueManager *DialogueManager             // 对话状态管理器，用于多轮任务式对话
	maxTokens       int                          // 最大Token数
}

// NewRAGService 创建 RAG 服务实例
// 参数：
//   - embedder: Embedding客户端
//   - vectorDB: 向量数据库客户端
//   - repo: SQLite数据仓储
//   - llmClient: LLM客户端
//   - topK: 默认召回数量
//   - minScore: 最低相似度阈值
//   - sessionManager: 会话管理器
//   - tokenManager: Token管理器
//   - maxTokens: 最大Token数
func NewRAGService(
	embedder *embedding.Client,
	vectorDB vector.VectorClient,
	repo *repository.SQLiteRepository,
	llmClient llm.Client,
	topK int,
	minScore float32,
	sessionManager *SessionManager,
	tokenManager *TokenManager,
	maxTokens int,
) *RAGService {
	dialogueManager := NewDialogueManager(sessionManager)

	return &RAGService{
		ctx:             context.Background(),
		embedder:        embedder,
		vectorDB:        vectorDB,
		repo:            repo,
		prompt:          prompt.NewRAGPromptTemplate(),
		llmClient:       llmClient,
		topK:            topK,
		minScore:        minScore,
		bm25Retriever:   text.NewBM25Retriever(),
		sessionManager:  sessionManager,
		tokenManager:    tokenManager,
		dialogueManager: dialogueManager,
		maxTokens:       maxTokens,
	}
}

// Ask 处理用户提问，执行完整的RAG流程
// 流程：
//  1. 获取或创建会话（支持用户隔离）
//  2. 获取对话历史（从Redis缓存或MySQL）
//  3. 执行混合召回（向量+BM25）
//  4. 构建提示词（系统提示词+用户问题）
//  5. Token计数与自动截断
//  6. 调用LLM生成回答
//  7. 保存对话记录（同时写入Redis和MySQL）
//
// 参数：
//   - req: 提问请求，包含问题、会话ID和用户ID
//
// 返回：
//   - AskResponse: 回答响应，包含回答内容和参考文档
func (s *RAGService) Ask(req *model.AskRequest) (*model.AskResponse, error) {
	sessionID := req.SessionID
	userID := req.UserID
	if userID == "" {
		userID = "default"
	}

	if sessionID == "" {
		sessionID = fmt.Sprintf("session_%d", time.Now().UnixNano())
		if s.sessionManager != nil {
			s.sessionManager.CreateSession(sessionID, userID)
		} else {
			s.repo.CreateSession(sessionID, userID)
		}
	}

	if s.dialogueManager != nil {
		state, _ := s.dialogueManager.GetDialogueState(sessionID)
		if state != nil && state.Status == DialogueStatusActive {
			response, _, err := s.dialogueManager.ProcessInput(sessionID, req.Question)
			if err != nil {
				return nil, err
			}

			if s.sessionManager != nil {
				s.sessionManager.AppendMessage(sessionID, &model.ChatMessage{
					SessionID: sessionID,
					Role:      model.RoleUser,
					Content:   req.Question,
				})
				s.sessionManager.AppendMessage(sessionID, &model.ChatMessage{
					SessionID: sessionID,
					Role:      model.RoleAssistant,
					Content:   response,
				})
			} else {
				s.repo.CreateMessage(&model.ChatMessage{
					SessionID: sessionID,
					Role:      model.RoleUser,
					Content:   req.Question,
				})
				s.repo.CreateMessage(&model.ChatMessage{
					SessionID: sessionID,
					Role:      model.RoleAssistant,
					Content:   response,
				})
			}

			return &model.AskResponse{
				SessionID:  sessionID,
				Question:   req.Question,
				Answer:     response,
				References: []model.Reference{},
				Cost:       0,
			}, nil
		}
	}

	var history []model.ChatMessage
	var err error
	if s.sessionManager != nil {
		history, err = s.sessionManager.GetMessages(sessionID)
	} else {
		history, err = s.repo.GetChatHistory(sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("获取会话历史失败: %w", err)
	}

	topK := req.TopK
	if topK <= 0 {
		topK = s.topK
	}

	references, err := s.HybridRecall(req.Question, topK)
	if err != nil {
		log.Printf("召回失败: %v", err)
	}

	var systemPrompt string
	var userPrompt string

	if len(references) > 0 {
		systemPrompt, _ = s.prompt.BuildSystemPrompt(references)
		userPrompt = s.prompt.BuildQuestionPrompt(req.Question)
	} else {
		systemPrompt = `你是一个专业的知识库助手。如果用户的问题涉及特定领域或文档内容，请说明你没有相关的参考信息。`
		userPrompt = req.Question
	}

	messages := make([]Message, 0)
	for _, msg := range history {
		messages = append(messages, Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if s.tokenManager != nil {
		messages = s.tokenManager.TruncateMessages(messages, s.maxTokens)
	}

	llmMessages := make([]llm.Message, 0)
	for _, msg := range messages {
		llmMessages = append(llmMessages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	llmMessages = append(llmMessages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})
	llmMessages = append(llmMessages, llm.Message{
		Role:    "user",
		Content: userPrompt,
	})

	answer, err := s.llmClient.Chat(s.ctx, llmMessages)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	totalTokens := 0
	if s.tokenManager != nil {
		totalTokens = s.tokenManager.CountMessages(messages) +
			s.tokenManager.Count(systemPrompt) +
			s.tokenManager.Count(userPrompt) +
			s.tokenManager.Count(answer)
	}

	if s.sessionManager != nil {
		s.sessionManager.AppendMessage(sessionID, &model.ChatMessage{
			SessionID:  sessionID,
			Role:       model.RoleUser,
			Content:    req.Question,
			References: references,
		})
		s.sessionManager.AppendMessage(sessionID, &model.ChatMessage{
			SessionID: sessionID,
			Role:      model.RoleAssistant,
			Content:   answer,
		})
	} else {
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
	}

	if len(history) == 0 {
		title := req.Question
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		if s.sessionManager != nil {
			s.sessionManager.UpdateSessionTitle(sessionID, title)
		} else {
			s.repo.UpdateSessionTitle(sessionID, title)
		}
	}

	return &model.AskResponse{
		SessionID:  sessionID,
		Question:   req.Question,
		Answer:     answer,
		References: references,
		Cost:       totalTokens,
	}, nil
}

// HybridRecall 混合召回函数，融合向量检索和BM25关键词检索结果
// 流程：
//  1. 并行执行向量检索和BM25检索
//  2. 获取所有文档切片和文档信息，构建映射表
//  3. 使用RRF算法融合两种检索结果
//  4. 将子块映射到父块，返回完整的父块内容作为参考
//
// 参数：
//   - question: 用户问题
//   - topK: 召回数量
//
// 返回：
//   - 参考文档列表，按RRF分数排序
func (s *RAGService) HybridRecall(question string, topK int) ([]model.Reference, error) {
	// 执行向量检索
	vectorResults, err := s.VectorRecall(question, topK)
	if err != nil {
		log.Printf("向量检索失败: %v", err)
	}

	// 执行BM25关键词检索
	bm25Results, err := s.BM25Recall(question, topK)
	if err != nil {
		log.Printf("BM25检索失败: %v", err)
	}

	// 两种检索都没有结果，直接返回空
	if len(vectorResults) == 0 && len(bm25Results) == 0 {
		return nil, nil
	}

	// 获取所有文档切片，构建ID到切片的映射
	chunks, err := s.getAllChunks()
	if err != nil {
		log.Printf("获取切片失败: %v", err)
		return vectorResults, nil
	}

	chunkMap := make(map[int64]*model.DocumentChunk)
	for i := range chunks {
		chunkMap[chunks[i].ID] = &chunks[i]
	}

	// 获取所有文档信息，构建ID到文档的映射
	docs, err := s.getAllDocuments()
	if err != nil {
		log.Printf("获取文档失败: %v", err)
		return vectorResults, nil
	}

	docMap := make(map[int64]*model.Document)
	for i := range docs {
		docMap[docs[i].ID] = &docs[i]
	}

	// 使用RRF算法融合向量检索和BM25检索结果
	rrfResults := text.RRF(vectorResults, bm25Results, 60, chunkMap, docMap)

	// 将子块映射到父块，去重后返回
	references := make([]model.Reference, 0, min(len(rrfResults), topK))
	seenParentIDs := make(map[int64]bool)

	for _, rrf := range rrfResults[:min(len(rrfResults), topK)] {
		// 如果有父块ID且未重复，则返回父块内容（更完整的上下文）
		if rrf.ParentID > 0 && !seenParentIDs[rrf.ParentID] {
			parentChunk, err := s.repo.GetParentChunk(rrf.ParentID)
			if err == nil && parentChunk != nil {
				references = append(references, model.Reference{
					DocID:    rrf.DocID,
					ChunkID:  rrf.ParentID,
					Content:  parentChunk.Content,
					Score:    rrf.RRFScore,
					FileName: rrf.FileName,
				})
				seenParentIDs[rrf.ParentID] = true
				continue
			}
		}

		// 没有父块或父块获取失败，返回子块内容
		references = append(references, model.Reference{
			DocID:    rrf.DocID,
			ChunkID:  rrf.ChunkID,
			Content:  rrf.Content,
			Score:    rrf.RRFScore,
			FileName: rrf.FileName,
		})
	}

	return references, nil
}

// VectorRecall 向量检索函数，基于余弦相似度检索相关文档切片
// 流程：
//  1. 将问题向量化
//  2. 在向量数据库中搜索相似向量
//  3. 解析返回结果，提取文档ID、切片ID、内容和分数
//
// 参数：
//   - question: 用户问题
//   - topK: 召回数量
//
// 返回：
//   - 参考文档列表，按相似度排序
func (s *RAGService) VectorRecall(question string, topK int) ([]model.Reference, error) {
	// 将问题转换为向量
	queryVec, err := s.embedder.Embedding(s.ctx, question)
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}

	// 在向量数据库中搜索相似向量
	results, err := s.vectorDB.Search(s.ctx, queryVec, topK, map[string]string{"ef": "128"})
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	references := make([]model.Reference, 0)
	for _, result := range results {
		// 过滤无效分数（余弦相似度应在0-1之间）
		if result.Score > 1.0 {
			continue
		}

		// 解析元数据（包含文档ID、切片ID、文件名等）
		var metadata map[string]interface{}
		json.Unmarshal([]byte(result.Metadata), &metadata)

		docID := int64(0)
		chunkID := int64(0)

		if metadata != nil {
			if v, ok := metadata["doc_id"]; ok {
				switch val := v.(type) {
				case float64:
					docID = int64(val)
				case int64:
					docID = val
				}
			}
			// 从metadata中获取数据库实际的chunk_id，而非chunk_index
			if v, ok := metadata["chunk_id"]; ok {
				switch val := v.(type) {
				case float64:
					chunkID = int64(val)
				case int64:
					chunkID = val
				}
			}
		}

		references = append(references, model.Reference{
			DocID:    docID,
			ChunkID:  chunkID,
			Content:  result.Text,
			Score:    float64(result.Score),
			FileName: getFileName(metadata),
		})
	}

	return references, nil
}

// BM25Recall BM25关键词检索函数，基于词频和逆文档频率检索相关文档切片
// 流程：
//  1. 获取所有文档切片
//  2. 构建BM25索引
//  3. 使用问题进行检索，返回相关切片
//
// 参数：
//   - question: 用户问题
//   - topK: 召回数量
//
// 返回：
//   - BM25检索结果列表，按相关性排序
func (s *RAGService) BM25Recall(question string, topK int) ([]text.BM25Result, error) {
	chunks, err := s.getAllChunks()
	if err != nil {
		return nil, err
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	// 将文档切片转换为BM25文档格式
	bm25Docs := make([]text.BM25Document, 0, len(chunks))
	for _, chunk := range chunks {
		bm25Docs = append(bm25Docs, text.BuildBM25Document(chunk.ID, chunk.Content))
	}

	// 构建BM25索引并执行检索
	s.bm25Retriever.BuildIndex(bm25Docs)
	return s.bm25Retriever.Retrieve(question, topK), nil
}

// getAllChunks 获取所有文档的切片信息
// 返回所有文档的所有切片，用于构建BM25索引和RRF融合
func (s *RAGService) getAllChunks() ([]model.DocumentChunk, error) {
	var allChunks []model.DocumentChunk

	docs, _, err := s.repo.ListDocuments(1, 1000)
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		chunks, err := s.repo.GetChunksByDocID(doc.ID)
		if err != nil {
			continue
		}
		allChunks = append(allChunks, chunks...)
	}

	return allChunks, nil
}

// getAllDocuments 获取所有文档信息
// 返回所有文档的基本信息，用于RRF融合时获取文件名等元数据
func (s *RAGService) getAllDocuments() ([]model.Document, error) {
	docs, _, err := s.repo.ListDocuments(1, 1000)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// GetHistory 获取会话历史记录
// 参数：
//   - sessionID: 会话ID
//
// 返回：
//   - ChatHistoryResponse: 会话历史响应，包含会话标题和消息列表
func (s *RAGService) GetHistory(sessionID string) (*model.ChatHistoryResponse, error) {
	var session *model.ChatSession
	var err error

	if s.sessionManager != nil {
		session, err = s.sessionManager.GetSession(sessionID)
	} else {
		session, err = s.repo.GetSession(sessionID)
	}
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("会话不存在")
	}

	var messages []model.ChatMessage
	if s.sessionManager != nil {
		messages, err = s.sessionManager.GetMessages(sessionID)
	} else {
		messages, err = s.repo.GetChatHistory(sessionID)
	}
	if err != nil {
		return nil, err
	}

	return &model.ChatHistoryResponse{
		SessionID: sessionID,
		Title:     session.Title,
		Messages:  messages,
	}, nil
}

// ListSessions 获取用户的会话列表
func (s *RAGService) ListSessions(userID string, page, pageSize int) (*model.SessionListResponse, error) {
	if s.sessionManager != nil {
		sessions, total, err := s.sessionManager.ListSessions(userID, page, pageSize)
		if err != nil {
			return nil, err
		}
		return &model.SessionListResponse{
			Sessions: sessions,
			Total:    total,
		}, nil
	}

	sessions, total, err := s.repo.ListSessionsByUserID(userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &model.SessionListResponse{
		Sessions: sessions,
		Total:    total,
	}, nil
}

// DeleteSession 删除会话
func (s *RAGService) DeleteSession(sessionID string) error {
	if s.sessionManager != nil {
		return s.sessionManager.DeleteSession(sessionID)
	}
	return s.repo.DeleteSession(sessionID)
}

// ResetSession 重置会话（清空消息，保留会话本身）
func (s *RAGService) ResetSession(sessionID string) error {
	if s.sessionManager != nil {
		s.sessionManager.UpdateSessionStatus(sessionID, model.SessionStatusActive)
		s.sessionManager.UpdateSessionContext(sessionID, "")
	} else {
		s.repo.UpdateSessionStatus(sessionID, model.SessionStatusActive)
		s.repo.UpdateSessionContext(sessionID, "")
	}
	return nil
}

// StartTask 启动任务式对话
func (s *RAGService) StartTask(sessionID, taskType string) (string, error) {
	if s.dialogueManager != nil {
		return s.dialogueManager.StartTask(sessionID, TaskType(taskType))
	}
	return "", fmt.Errorf("对话管理器未初始化")
}

// CancelTask 取消任务式对话
func (s *RAGService) CancelTask(sessionID string) error {
	if s.dialogueManager != nil {
		return s.dialogueManager.CancelTask(sessionID)
	}
	return fmt.Errorf("对话管理器未初始化")
}

// GetDialogueState 获取对话状态
func (s *RAGService) GetDialogueState(sessionID string) (*DialogueState, error) {
	if s.dialogueManager != nil {
		return s.dialogueManager.GetDialogueState(sessionID)
	}
	return nil, fmt.Errorf("对话管理器未初始化")
}

// getFileName 从元数据中提取文件名
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

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
