package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/repository"
)

type MockRedisClient struct {
	data map[string]string
	ttls map[string]time.Time
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string]string),
		ttls: make(map[string]time.Time),
	}
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, _ := json.Marshal(value)
	m.data[key] = string(data)
	m.ttls[key] = time.Now().Add(ttl)
	return nil
}

func (m *MockRedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	if val, ok := m.data[key]; ok {
		if tm, ok := m.ttls[key]; ok && time.Now().Before(tm) {
			return json.Unmarshal([]byte(val), dest)
		}
		delete(m.data, key)
		delete(m.ttls, key)
	}
	return ErrKeyNotFound
}

var ErrKeyNotFound = &redisError{"key not found"}

type redisError struct {
	msg string
}

func (e *redisError) Error() string { return e.msg }

func (m *MockRedisClient) Del(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		delete(m.data, key)
		delete(m.ttls, key)
	}
	return nil
}

func (m *MockRedisClient) Exists(ctx context.Context, key string) (bool, error) {
	if _, ok := m.data[key]; ok {
		if tm, ok := m.ttls[key]; ok && time.Now().Before(tm) {
			return true, nil
		}
		delete(m.data, key)
		delete(m.ttls, key)
	}
	return false, nil
}

func (m *MockRedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if _, ok := m.data[key]; ok {
		m.ttls[key] = time.Now().Add(ttl)
	}
	return nil
}

func (m *MockRedisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return nil
}

func (m *MockRedisClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	return nil
}

func (m *MockRedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return []string{}, nil
}

func (m *MockRedisClient) LLen(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

func (m *MockRedisClient) LTrim(ctx context.Context, key string, start, stop int64) error {
	return nil
}

func (m *MockRedisClient) Close() error {
	return nil
}

func TestSessionManager_CreateSession(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	mockRedis := NewMockRedisClient()
	sm := NewSessionManager(mockRedis, repo, 3600, 100)

	err = sm.CreateSession("test-session-1", "user-1")
	if err != nil {
		t.Errorf("CreateSession failed: %v", err)
	}

	session, err := sm.GetSession("test-session-1")
	if err != nil {
		t.Errorf("GetSession failed: %v", err)
	}

	if session == nil {
		t.Error("Session should not be nil")
	}

	if session.SessionID != "test-session-1" {
		t.Errorf("Expected session ID 'test-session-1', got '%s'", session.SessionID)
	}

	if session.UserID != "user-1" {
		t.Errorf("Expected user ID 'user-1', got '%s'", session.UserID)
	}

	if session.Status != model.SessionStatusActive {
		t.Errorf("Expected status 'active', got '%s'", session.Status)
	}
}

func TestSessionManager_GetSession_CacheHit(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	mockRedis := NewMockRedisClient()
	sm := NewSessionManager(mockRedis, repo, 3600, 100)

	err = sm.CreateSession("test-session-cache", "user-cache")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	session1, err := sm.GetSession("test-session-cache")
	if err != nil {
		t.Errorf("First GetSession failed: %v", err)
	}

	session2, err := sm.GetSession("test-session-cache")
	if err != nil {
		t.Errorf("Second GetSession failed: %v", err)
	}

	if session1.SessionID != session2.SessionID {
		t.Error("Session IDs should match")
	}
}

func TestSessionManager_UpdateSessionTitle(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	mockRedis := NewMockRedisClient()
	sm := NewSessionManager(mockRedis, repo, 3600, 100)

	err = sm.CreateSession("test-session-title", "user-title")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = sm.UpdateSessionTitle("test-session-title", "My New Title")
	if err != nil {
		t.Errorf("UpdateSessionTitle failed: %v", err)
	}

	session, err := sm.GetSession("test-session-title")
	if err != nil {
		t.Errorf("GetSession failed: %v", err)
	}

	if session.Title != "My New Title" {
		t.Errorf("Expected title 'My New Title', got '%s'", session.Title)
	}
}

func TestSessionManager_DeleteSession(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	mockRedis := NewMockRedisClient()
	sm := NewSessionManager(mockRedis, repo, 3600, 100)

	err = sm.CreateSession("test-session-delete", "user-delete")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = sm.DeleteSession("test-session-delete")
	if err != nil {
		t.Errorf("DeleteSession failed: %v", err)
	}

	session, err := sm.GetSession("test-session-delete")
	if err != nil {
		t.Errorf("GetSession after delete failed: %v", err)
	}

	if session != nil {
		t.Error("Session should be nil after deletion")
	}
}

func TestSessionManager_ListSessions(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	mockRedis := NewMockRedisClient()
	sm := NewSessionManager(mockRedis, repo, 3600, 100)

	err = sm.CreateSession("test-session-list-1", "user-list")
	if err != nil {
		t.Fatalf("CreateSession 1 failed: %v", err)
	}

	err = sm.CreateSession("test-session-list-2", "user-list")
	if err != nil {
		t.Fatalf("CreateSession 2 failed: %v", err)
	}

	err = sm.CreateSession("test-session-list-3", "user-other")
	if err != nil {
		t.Fatalf("CreateSession 3 failed: %v", err)
	}

	sessions, total, err := sm.ListSessions("user-list", 1, 10)
	if err != nil {
		t.Errorf("ListSessions failed: %v", err)
	}

	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionManager_WithoutRedis(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)

	err = sm.CreateSession("test-session-no-redis", "user-no-redis")
	if err != nil {
		t.Errorf("CreateSession without Redis failed: %v", err)
	}

	session, err := sm.GetSession("test-session-no-redis")
	if err != nil {
		t.Errorf("GetSession without Redis failed: %v", err)
	}

	if session == nil {
		t.Error("Session should not be nil")
	}

	if session.SessionID != "test-session-no-redis" {
		t.Errorf("Expected session ID 'test-session-no-redis', got '%s'", session.SessionID)
	}
}
