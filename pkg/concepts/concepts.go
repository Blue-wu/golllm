package concepts

// LLM 核心概念学习笔记
// 对应 B 站视频 BV1ByVg6FE9q 前 3P 和 BV1ykVt6sEbu 通识篇

import (
	"fmt"
)

// 大模型核心概念

/*
【Token】
- Token 是文本处理的基本单位
- 中文：1个汉字 ≈ 1-2 个 Token
- 英文：1个单词 ≈ 1-3 个 Token
- 标点符号、空格也算 Token
- 输入和输出都消耗 Token
- 上下文窗口限制 = 最大 Token 数

【上下文窗口 (Context Window)】
- 模型一次能处理的最大 Token 数量
- 包括输入和输出总和
- 超出后会被截断
- 不同模型上下文窗口不同:
  - GPT-4: 128K tokens
  - GPT-3.5: 16K tokens
  - Claude: 200K tokens
  - 通义千问: 128K tokens

【温度系数 (Temperature)】
- 控制输出的随机性
- 范围: 0.0 - 2.0
- 0.0: 确定输出，最"正确"的答案
- 1.0: 中等随机性
- 2.0: 高随机性，创意输出
- 推荐:
  - 问答/代码: 0.1-0.3
  - 写作/创意: 0.7-1.0

【Embedding】
- 将文本映射为向量
- 语义相似的内容向量距离近
- 用于:
  - 相似度匹配
  - RAG 检索
  - 文本分类
  - 聚类分析

【Prompt 工程基础】
1. 清晰明确的任务描述
2. 提供上下文和背景信息
3. 指定输出格式
4. Few-shot 示例
5. 角色设定 (System Prompt)

【Function Call (函数调用)】
- 让模型调用外部函数/工具
- 扩展模型能力边界
- 实现步骤:
  1. 定义函数 schema
  2. 发送给模型
  3. 模型决定调用哪个函数
  4. 执行函数返回结果
  5. 将结果发回模型生成最终回答

【RAG (Retrieval-Augmented Generation)】
- 检索增强生成
- 流程:
  1. 文档切分
  2. Embedding 向量化
  3. 存储到向量数据库
  4. 用户提问 → 检索相关片段
  5. 将检索结果注入 Prompt
  6. 模型基于上下文生成回答
- 优势: 解决知识时效性问题

【Agent 核心定义】
- Agent = LLM + Tools + 记忆
- 核心能力:
  - 理解任务目标
  - 规划执行步骤
  - 调用工具
  - 自我反思修正
- ReAct 模式: Thought → Action → Observation
*/

// PrintConcepts 打印核心概念
func PrintConcepts() {
	concepts := []string{
		"Token: 文本处理基本单位",
		"上下文窗口: 模型单次处理的最大 Token 数",
		"温度系数: 控制输出随机性 (0.0-2.0)",
		"Embedding: 文本→向量映射",
		"Prompt 工程: 优化提示词以获得更好输出",
		"Function Call: 让模型调用外部函数",
		"RAG: 检索增强生成，解决知识时效问题",
		"Agent: LLM + Tools + 记忆，自主执行任务",
	}

	fmt.Println("=== 大模型核心概念 ===")
	for _, c := range concepts {
		fmt.Println("-", c)
	}
}

// TokenEstimate 估算 Token 数量（简化版）
func TokenEstimate(text string) int {
	// 非常粗略的估算：中文按字符数，英文按单词数
	count := 0
	for _, r := range text {
		if r > 127 {
			count += 2 // 中文字符
		} else {
			count++ // ASCII 字符
		}
	}
	return (count + 2) / 3 // 简化估算
}
