package prompt

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/golllm/cmd/rag/model"
)

// RAGPromptTemplate RAG 提示词模板
type RAGPromptTemplate struct {
	systemTemplate string
	questionTemplate string
}

// NewRAGPromptTemplate 创建 RAG 提示词模板
func NewRAGPromptTemplate() *RAGPromptTemplate {
	return &RAGPromptTemplate{
		systemTemplate: `你是一个专业的知识库助手。请根据以下参考文档回答用户的问题。

## 参考文档
{{.References}}

## 回答要求
1. 基于参考文档的内容进行回答，不要编造信息
2. 如果参考文档中没有相关信息，请如实说明
3. 回答要准确、简洁、有条理
4. 在回答结束时，可以引用相关文档片段

## 回答格式
[回答内容]

---
参考来源：
{{.Sources}}
`,

		questionTemplate: "问题：{{.Question}}\n\n请根据参考文档回答这个问题。",
	}
}

// BuildSystemPrompt 构建系统提示词
func (t *RAGPromptTemplate) BuildSystemPrompt(references []model.Reference) (string, string) {
	if len(references) == 0 {
		return "", ""
	}

	var buf bytes.Buffer
	var sources bytes.Buffer

	for i, ref := range references {
		buf.WriteString(fmt.Sprintf("\n[%d] %s\n", i+1, ref.Content))
		sources.WriteString(fmt.Sprintf("- %s (相关性: %.2f)\n", ref.FileName, ref.Score))
	}

	// 渲染模板
	tmpl, _ := template.New("system").Parse(t.systemTemplate)
	var systemPrompt bytes.Buffer
	tmpl.Execute(&systemPrompt, map[string]interface{}{
		"References": buf.String(),
		"Sources":   sources.String(),
	})

	return systemPrompt.String(), sources.String()
}

// BuildQuestionPrompt 构建问题提示词
func (t *RAGPromptTemplate) BuildQuestionPrompt(question string) string {
	tmpl, _ := template.New("question").Parse(t.questionTemplate)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]interface{}{
		"Question": question,
	})
	return buf.String()
}

// BuildFullPrompt 构建完整的 RAG Prompt
func (t *RAGPromptTemplate) BuildFullPrompt(question string, references []model.Reference) (string, error) {
	systemPrompt, sources := t.BuildSystemPrompt(references)
	if systemPrompt == "" {
		return "请回答用户的问题：" + question, nil
	}

	fullPrompt := fmt.Sprintf(`%s

%s

%s`, systemPrompt, t.BuildQuestionPrompt(question), sources)

	return fullPrompt, nil
}

// SimplePrompt 简单提示词（用于无召回结果时）
func SimplePrompt(question string) string {
	return fmt.Sprintf(`你是一个专业的知识库助手。请回答用户的问题。

问题：%s

请基于你的知识库回答，如果不确定请如实说明。`, question)
}