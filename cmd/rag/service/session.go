package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/repository"
)

// SessionManager 会话管理器
// 负责会话的缓存和持久化管理，实现热数据Redis缓存+冷数据SQLite持久化的分层存储架构
// 核心特性：
//   - Redis缓存活跃会话，设置过期时间自动淘汰
//   - SQLite持久化所有会话数据，保证数据不丢失
//   - 自动降级：Redis不可用时直接使用SQLite
type SessionManager struct {
	ctx            context.Context              // 全局上下文
	redisClient    RedisClient                  // Redis客户端接口
	repo           *repository.SQLiteRepository // SQLite数据仓储
	sessionTimeout time.Duration                // 会话过期时间
	maxHistorySize int                          // 最大历史消息数
}

// RedisClient Redis客户端接口定义
// 抽象Redis操作，便于测试和替换实现
type RedisClient interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string, dest interface{}) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	LPush(ctx context.Context, key string, values ...interface{}) error
	RPush(ctx context.Context, key string, values ...interface{}) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	LLen(ctx context.Context, key string) (int64, error)
	LTrim(ctx context.Context, key string, start, stop int64) error
	Close() error
}

// NewSessionManager 创建会话管理器实例
// 参数：
//   - redisClient: Redis客户端（可为nil，此时只使用SQLite）
//   - repo: SQLite数据仓储
//   - sessionTimeout: 会话超时时间（秒）
//   - maxHistorySize: 最大历史消息数（0表示不限制）
func NewSessionManager(redisClient RedisClient, repo *repository.SQLiteRepository, sessionTimeout int, maxHistorySize int) *SessionManager {
	return &SessionManager{
		ctx:            context.Background(),
		redisClient:    redisClient,
		repo:           repo,
		sessionTimeout: time.Duration(sessionTimeout) * time.Second,
		maxHistorySize: maxHistorySize,
	}
}

// getSessionKey 生成会话在Redis中的key
func (sm *SessionManager) getSessionKey(sessionID string) string {
	return fmt.Sprintf("session:%s", sessionID)
}

// getMessagesKey 生成会话消息列表在Redis中的key
func (sm *SessionManager) getMessagesKey(sessionID string) string {
	return fmt.Sprintf("session:%s:messages", sessionID)
}

// CreateSession 创建会话
// 同时写入SQLite和Redis缓存
func (sm *SessionManager) CreateSession(sessionID, userID string) error {
	// 先写入SQLite保证持久化
	err := sm.repo.CreateSession(sessionID, userID)
	if err != nil {
		return err
	}

	// 如果Redis可用，写入缓存
	if sm.redisClient != nil {
		session := &model.ChatSession{
			SessionID: sessionID,
			UserID:    userID,
			Title:     "新对话",
			Status:    model.SessionStatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		sm.redisClient.Set(sm.ctx, sm.getSessionKey(sessionID), session, sm.sessionTimeout)
	}

	return nil
}

// GetSession 获取会话
// 优先从Redis缓存获取，缓存未命中时从SQLite读取并回写到Redis
func (sm *SessionManager) GetSession(sessionID string) (*model.ChatSession, error) {
	if sm.redisClient != nil {
		var session model.ChatSession
		err := sm.redisClient.Get(sm.ctx, sm.getSessionKey(sessionID), &session)
		if err == nil {
			// 访问后延长过期时间
			sm.redisClient.Expire(sm.ctx, sm.getSessionKey(sessionID), sm.sessionTimeout)
			return &session, nil
		}
	}

	// 缓存未命中，从SQLite读取
	dbSession, err := sm.repo.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if dbSession == nil {
		return nil, nil
	}

	// 回写到Redis缓存
	if sm.redisClient != nil {
		sm.redisClient.Set(sm.ctx, sm.getSessionKey(sessionID), dbSession, sm.sessionTimeout)
	}
	return dbSession, nil
}

// UpdateSessionTitle 更新会话标题
// 同步更新SQLite和Redis缓存
func (sm *SessionManager) UpdateSessionTitle(sessionID, title string) error {
	err := sm.repo.UpdateSessionTitle(sessionID, title)
	if err != nil {
		return err
	}

	if sm.redisClient != nil {
		var session model.ChatSession
		if sm.redisClient.Get(sm.ctx, sm.getSessionKey(sessionID), &session) == nil {
			session.Title = title
			session.UpdatedAt = time.Now()
			sm.redisClient.Set(sm.ctx, sm.getSessionKey(sessionID), session, sm.sessionTimeout)
		}
	}

	return nil
}

// UpdateSessionStatus 更新会话状态
// 同步更新SQLite和Redis缓存
func (sm *SessionManager) UpdateSessionStatus(sessionID, status string) error {
	err := sm.repo.UpdateSessionStatus(sessionID, status)
	if err != nil {
		return err
	}

	if sm.redisClient != nil {
		var session model.ChatSession
		if sm.redisClient.Get(sm.ctx, sm.getSessionKey(sessionID), &session) == nil {
			session.Status = status
			session.UpdatedAt = time.Now()
			sm.redisClient.Set(sm.ctx, sm.getSessionKey(sessionID), session, sm.sessionTimeout)
		}
	}

	return nil
}

// UpdateSessionContext 更新会话上下文（用于存储对话状态）
// 同步更新SQLite和Redis缓存
func (sm *SessionManager) UpdateSessionContext(sessionID, context string) error {
	err := sm.repo.UpdateSessionContext(sessionID, context)
	if err != nil {
		return err
	}

	if sm.redisClient != nil {
		var session model.ChatSession
		if sm.redisClient.Get(sm.ctx, sm.getSessionKey(sessionID), &session) == nil {
			session.Context = context
			session.UpdatedAt = time.Now()
			sm.redisClient.Set(sm.ctx, sm.getSessionKey(sessionID), session, sm.sessionTimeout)
		}
	}

	return nil
}

// ListSessions 获取用户的会话列表
// 从SQLite读取（列表查询不适合缓存）
func (sm *SessionManager) ListSessions(userID string, page, pageSize int) ([]model.ChatSession, int64, error) {
	return sm.repo.ListSessionsByUserID(userID, page, pageSize)
}

// DeleteSession 删除会话
// 同时删除SQLite和Redis中的数据
func (sm *SessionManager) DeleteSession(sessionID string) error {
	err := sm.repo.DeleteSession(sessionID)
	if err != nil {
		return err
	}

	if sm.redisClient != nil {
		sm.redisClient.Del(sm.ctx, sm.getSessionKey(sessionID), sm.getMessagesKey(sessionID))
	}
	return nil
}

// AppendMessage 追加消息到会话
// 同时写入SQLite和Redis缓存，并维护消息数量上限
func (sm *SessionManager) AppendMessage(sessionID string, msg *model.ChatMessage) error {
	// 先写入SQLite保证持久化
	err := sm.repo.CreateMessage(msg)
	if err != nil {
		return err
	}

	if sm.redisClient != nil {
		data, _ := json.Marshal(msg)
		sm.redisClient.RPush(sm.ctx, sm.getMessagesKey(sessionID), string(data))
		sm.redisClient.Expire(sm.ctx, sm.getMessagesKey(sessionID), sm.sessionTimeout)
		sm.redisClient.Expire(sm.ctx, sm.getSessionKey(sessionID), sm.sessionTimeout)

		// 维护消息数量上限
		if sm.maxHistorySize > 0 {
			sm.redisClient.LTrim(sm.ctx, sm.getMessagesKey(sessionID), int64(-sm.maxHistorySize), -1)
		}
	}

	return nil
}

// GetMessages 获取会话消息列表
// 优先从Redis缓存获取，缓存未命中时从SQLite读取并回写到Redis
func (sm *SessionManager) GetMessages(sessionID string) ([]model.ChatMessage, error) {
	if sm.redisClient != nil {
		messagesKey := sm.getMessagesKey(sessionID)
		exists, err := sm.redisClient.Exists(sm.ctx, messagesKey)
		if err == nil && exists {
			dataList, err := sm.redisClient.LRange(sm.ctx, messagesKey, 0, -1)
			if err != nil {
				return nil, err
			}

			var messages []model.ChatMessage
			for _, data := range dataList {
				var msg model.ChatMessage
				if json.Unmarshal([]byte(data), &msg) == nil {
					messages = append(messages, msg)
				}
			}

			// 访问后延长过期时间
			sm.redisClient.Expire(sm.ctx, messagesKey, sm.sessionTimeout)
			return messages, nil
		}
	}

	// 缓存未命中，从SQLite读取
	messages, err := sm.repo.GetChatHistory(sessionID)
	if err != nil {
		return nil, err
	}

	// 回写到Redis缓存
	if sm.redisClient != nil {
		messagesKey := sm.getMessagesKey(sessionID)
		for _, msg := range messages {
			data, _ := json.Marshal(msg)
			sm.redisClient.RPush(sm.ctx, messagesKey, string(data))
		}
		sm.redisClient.Expire(sm.ctx, messagesKey, sm.sessionTimeout)
	}

	return messages, nil
}
