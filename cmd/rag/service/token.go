package service

import (
	"fmt"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// TokenManager Token管理器
// 负责对话中的Token计数和消息截断，确保对话不超过模型的Token限制
// 核心特性：
//   - 使用tiktoken-go库精确计算Token数量
//   - 自动截断历史消息，优先保留最新消息
//   - 系统提示词保护，避免被截断
type TokenManager struct {
	encoder       *tiktoken.Tiktoken // Token编码器
	maxTokens     int                // 最大Token数
	systemPrompt  string             // 系统提示词
	tokensPerMsg  int                // 每条消息的基础Token开销
	tokensPerName int                // 每个名字的Token开销
}

// Message 消息结构
// 用于Token计数和消息截断
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// NewTokenManager 创建Token管理器实例
// 参数：
//   - model: 模型名称（用于选择对应的Token编码）
//   - maxTokens: 最大Token数限制
//   - systemPrompt: 系统提示词（会被保护不被截断）
func NewTokenManager(model string, maxTokens int, systemPrompt string) *TokenManager {
	encoder, err := tiktoken.EncodingForModel(model)
	if err != nil {
		encoder, _ = tiktoken.GetEncoding("cl100k_base")
	}

	return &TokenManager{
		encoder:       encoder,
		maxTokens:     maxTokens,
		systemPrompt:  systemPrompt,
		tokensPerMsg:  3,
		tokensPerName: 1,
	}
}

// Count 计算文本的Token数量
func (tm *TokenManager) Count(text string) int {
	tokens := tm.encoder.Encode(text, nil, nil)
	return len(tokens)
}

// CountMessage 计算单条消息的Token数量
// 包括消息结构开销（角色、名字等）
func (tm *TokenManager) CountMessage(msg Message) int {
	tokens := tm.tokensPerMsg
	tokens += tm.Count(msg.Content)
	if msg.Name != "" {
		tokens += tm.tokensPerName
	}
	return tokens
}

// CountMessages 计算多条消息的总Token数量
func (tm *TokenManager) CountMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += tm.CountMessage(msg)
	}
	return total
}

// TruncateMessages 截断消息列表，使其总Token数不超过限制
// 策略：
//  1. 首先计算系统提示词的Token数并保留
//  2. 从最新消息开始向前累加，直到达到Token上限
//  3. 优先保留最新消息，丢弃最旧消息
//
// 参数：
//   - messages: 原始消息列表
//   - maxTokens: 最大Token数（0表示使用默认值）
//
// 返回：截断后的消息列表
func (tm *TokenManager) TruncateMessages(messages []Message, maxTokens int) []Message {
	if maxTokens <= 0 {
		maxTokens = tm.maxTokens
	}

	// 系统提示词的Token数
	systemPromptTokens := tm.Count(tm.systemPrompt)

	// 可用的Token数（减去系统提示词和预留空间）
	availableTokens := maxTokens - systemPromptTokens - 100

	if availableTokens <= 0 {
		return []Message{}
	}

	// 从后向前遍历，优先保留最新消息
	var result []Message
	totalTokens := 0

	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := tm.CountMessage(messages[i])

		if totalTokens+msgTokens <= availableTokens {
			result = append([]Message{messages[i]}, result...)
			totalTokens += msgTokens
		} else {
			break
		}
	}

	return result
}

// GetMaxTokens 获取最大Token数限制
func (tm *TokenManager) GetMaxTokens() int {
	return tm.maxTokens
}

// SetSystemPrompt 设置系统提示词
func (tm *TokenManager) SetSystemPrompt(prompt string) {
	tm.systemPrompt = prompt
}

// GetSystemPrompt 获取系统提示词
func (tm *TokenManager) GetSystemPrompt() string {
	return tm.systemPrompt
}

// BuildMessages 构建完整的消息列表（系统提示词+历史消息+新消息）
// 参数：
//   - history: 历史消息列表
//   - newMsg: 新消息
//
// 返回：完整的消息列表（已自动截断）
func (tm *TokenManager) BuildMessages(history []Message, newMsg Message) []Message {
	fullMessages := append(history, newMsg)
	truncated := tm.TruncateMessages(fullMessages, tm.maxTokens)

	var result []Message
	if tm.systemPrompt != "" {
		result = append(result, Message{
			Role:    "system",
			Content: tm.systemPrompt,
		})
	}
	result = append(result, truncated...)

	return result
}

// EstimateCost 估算Token消耗对应的费用（单位：美元）
// 基于OpenAI的定价：$0.0015 / 1K tokens（输入），$0.002 / 1K tokens（输出）
func (tm *TokenManager) EstimateCost(inputTokens, outputTokens int) float64 {
	inputCost := float64(inputTokens) / 1000 * 0.0015
	outputCost := float64(outputTokens) / 1000 * 0.002
	return inputCost + outputCost
}

// FormatTokenUsage 格式化Token使用情况
func (tm *TokenManager) FormatTokenUsage(promptTokens, completionTokens, totalTokens int) string {
	return fmt.Sprintf("Token使用: 提示词=%d, 回复=%d, 总计=%d", promptTokens, completionTokens, totalTokens)
}
