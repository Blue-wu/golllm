package service

import (
	"testing"
)

func TestTokenManager_Count(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "")

	testCases := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"Hello", 1},
		{"Hello world", 2},
		{"你好世界", 4},
		{"Hello, how are you?", 5},
	}

	for _, tc := range testCases {
		result := tm.Count(tc.text)
		if result <= 0 && tc.text != "" {
			t.Errorf("Count(%q) should be > 0, got %d", tc.text, result)
		}
	}
}

func TestTokenManager_CountMessage(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "")

	msg := Message{
		Role:    "user",
		Content: "Hello world",
	}

	tokens := tm.CountMessage(msg)
	if tokens <= 0 {
		t.Errorf("CountMessage should be > 0, got %d", tokens)
	}

	msgWithName := Message{
		Role:    "user",
		Name:    "test",
		Content: "Hello",
	}

	tokensWithName := tm.CountMessage(msgWithName)
	if tokensWithName <= 0 {
		t.Errorf("CountMessage with name should be > 0, got %d", tokensWithName)
	}
}

func TestTokenManager_CountMessages(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "")

	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}

	total := tm.CountMessages(messages)
	if total <= 0 {
		t.Errorf("CountMessages should be > 0, got %d", total)
	}

	if total < 6 {
		t.Errorf("CountMessages should be at least 6, got %d", total)
	}
}

func TestTokenManager_TruncateMessages(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 200, "")

	var messages []Message
	for i := 0; i < 50; i++ {
		messages = append(messages, Message{
			Role:    "user",
			Content: "Hello world this is a longer message to test truncation",
		})
	}

	truncated := tm.TruncateMessages(messages, 200)
	if len(truncated) == 0 {
		t.Error("TruncateMessages should return non-empty slice")
	}

	if len(truncated) >= 50 {
		t.Error("TruncateMessages should reduce message count")
	}
}

func TestTokenManager_TruncateMessages_Empty(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 10, "")

	messages := []Message{
		{Role: "user", Content: "This is a very long message that should exceed token limit"},
	}

	truncated := tm.TruncateMessages(messages, 5)
	if len(truncated) != 0 {
		t.Errorf("TruncateMessages should return empty slice for token overflow, got %d", len(truncated))
	}
}

func TestTokenManager_BuildMessages(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "You are a helpful assistant")

	history := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}

	newMsg := Message{Role: "user", Content: "How are you?"}

	result := tm.BuildMessages(history, newMsg)

	if len(result) != 4 {
		t.Errorf("BuildMessages should return 4 messages (1 system + 3 history), got %d", len(result))
	}

	if result[0].Role != "system" {
		t.Errorf("First message should be system role, got %s", result[0].Role)
	}

	if result[0].Content != "You are a helpful assistant" {
		t.Errorf("System prompt mismatch")
	}
}

func TestTokenManager_EstimateCost(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "")

	cost := tm.EstimateCost(1000, 1000)
	if cost <= 0 {
		t.Errorf("EstimateCost should be > 0, got %f", cost)
	}

	if cost > 0.01 {
		t.Errorf("EstimateCost should be reasonable, got %f", cost)
	}
}

func TestTokenManager_FormatTokenUsage(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "")

	result := tm.FormatTokenUsage(100, 200, 300)
	if result == "" {
		t.Error("FormatTokenUsage should return non-empty string")
	}

	expected := "Token使用: 提示词=100, 回复=200, 总计=300"
	if result != expected {
		t.Errorf("FormatTokenUsage mismatch: expected %q, got %q", expected, result)
	}
}

func TestTokenManager_GetMaxTokens(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 4096, "")

	if tm.GetMaxTokens() != 4096 {
		t.Errorf("GetMaxTokens should return 4096, got %d", tm.GetMaxTokens())
	}
}

func TestTokenManager_SetSystemPrompt(t *testing.T) {
	tm := NewTokenManager("gpt-3.5-turbo", 8192, "")

	newPrompt := "You are a coding assistant"
	tm.SetSystemPrompt(newPrompt)

	if tm.GetSystemPrompt() != newPrompt {
		t.Errorf("SetSystemPrompt failed: expected %q, got %q", newPrompt, tm.GetSystemPrompt())
	}
}
