package prompt

import (
	"fmt"
	"regexp"
	"strings"
)

type Template struct {
	Name        string
	Description string
	Template    string
}

type Manager struct {
	templates map[string]*Template
}

func NewManager() *Manager {
	return &Manager{
		templates: make(map[string]*Template),
	}
}

func (m *Manager) Register(tmpl Template) {
	m.templates[tmpl.Name] = &tmpl
}

func (m *Manager) RegisterSimple(name, description, template string) {
	m.Register(Template{
		Name:        name,
		Description: description,
		Template:    template,
	})
}

func (m *Manager) Get(name string) (*Template, bool) {
	tmpl, ok := m.templates[name]
	return tmpl, ok
}

func (m *Manager) Render(name string, vars map[string]string) (string, error) {
	tmpl, ok := m.templates[name]
	if !ok {
		return "", fmt.Errorf("template '%s' not found", name)
	}
	return RenderString(tmpl.Template, vars)
}

func RenderString(template string, vars map[string]string) (string, error) {
	result := template

	re := regexp.MustCompile(`\{\{\.(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		varName := match[1]
		if value, ok := vars[varName]; ok {
			result = strings.ReplaceAll(result, match[0], value)
		} else {
			return "", fmt.Errorf("variable '%s' not found", varName)
		}
	}

	return result, nil
}

func HasUnrenderedVars(template string) bool {
	re := regexp.MustCompile(`\{\{\.(\w+)\}\}`)
	return re.MatchString(template)
}

var SystemPrompts = struct {
	Assistant  string
	Coder      string
	Analyst    string
	Translator string
}{
	Assistant: "你是一个乐于助人的AI助手。请根据用户的问题，提供准确、有用的回答。",

	Coder: "你是一个专业的程序员。你擅长编写、调试和优化代码。请用清晰的方式解释技术概念。\n技能：{{.language}}\n要求：代码规范、注释清晰、考虑边界情况",

	Analyst: "你是一个专业的数据分析师。你擅长分析数据、发现规律、提出建议。\n分析目标：{{.goal}}\n数据范围：{{.scope}}",

	Translator: "你是一个专业的翻译助手。请将以下{{.from_lang}}文本翻译成{{.to_lang}}。\n要求：准确、流畅、符合目标语言的表达习惯。",
}

var UserPrompts = struct {
	Question    string
	CodeReview  string
	Summary     string
}{
	Question: "请回答以下问题：\n{{.question}}\n\n要求：\n1. 准确理解问题\n2. 回答简洁明了\n3. 如有需要，提供相关建议",

	CodeReview: "请审查以下{{.language}}代码：\n\n```" + "{{.language}}" + "\n" + "{{.code}}" + "\n```\n\n审查要点：\n1. 代码正确性\n2. 性能优化\n3. 安全漏洞\n4. 代码规范",

	Summary: "请为以下文章写一份摘要：\n\n{{.content}}\n\n摘要要求：\n1. 简洁概括文章主旨\n2. 保留关键信息\n3. 长度控制在{{.max_length}}字以内",
}

func (m *Manager) RegisterDefaultTemplates() {
	m.RegisterSimple("assistant", "通用助手", SystemPrompts.Assistant)
	m.RegisterSimple("coder", "代码助手", SystemPrompts.Coder)
	m.RegisterSimple("analyst", "数据分析师", SystemPrompts.Analyst)
	m.RegisterSimple("translator", "翻译助手", SystemPrompts.Translator)
	m.RegisterSimple("question", "通用问答", UserPrompts.Question)
	m.RegisterSimple("code_review", "代码审查", UserPrompts.CodeReview)
	m.RegisterSimple("summary", "文章摘要", UserPrompts.Summary)
}