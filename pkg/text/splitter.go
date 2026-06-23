package text

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Chunk 文本切片块
type Chunk struct {
	Content   string                 // 切片内容
	Start     int                    // 在原文中的起始位置
	End       int                    // 在原文中的结束位置
	Metadata  map[string]interface{} // 元数据（如切片类型、标题信息等）
}

// Splitter 文本切片器接口
type Splitter interface {
	Split(text string) []Chunk
}

// FixedLengthSplitter 固定长度切片器
// 按固定字符数切片，支持重叠区域
type FixedLengthSplitter struct {
	ChunkSize    int // 每个切片的字符数
	Overlap      int // 切片之间的重叠字符数
}

// NewFixedLengthSplitter 创建固定长度切片器
// 参数:
//   - chunkSize: 每个切片的字符数
//   - overlap: 切片之间的重叠字符数（避免切断语义）
func NewFixedLengthSplitter(chunkSize, overlap int) *FixedLengthSplitter {
	return &FixedLengthSplitter{
		ChunkSize: chunkSize,
		Overlap:   overlap,
	}
}

// Split 执行固定长度切片
// 策略：
//   1. 按指定长度切分文本
//   2. 在空格处切分，避免截断单词
//   3. 切片之间有重叠区域，保持上下文连贯性
func (s *FixedLengthSplitter) Split(text string) []Chunk {
	chunks := make([]Chunk, 0)
	textLen := len(text)
	start := 0

	for start < textLen {
		end := start + s.ChunkSize
		if end > textLen {
			end = textLen
		}

		chunkText := text[start:end]
		// 如果不是最后一块，尝试在空格处切分，避免截断单词
		if end < textLen {
			lastSpace := strings.LastIndex(chunkText, " ")
			if lastSpace > s.ChunkSize/2 {
				end = start + lastSpace
				chunkText = text[start:end]
			}
		}

		chunks = append(chunks, Chunk{
			Content:  chunkText,
			Start:    start,
			End:      end,
			Metadata: map[string]interface{}{"chunk_type": "fixed_length"},
		})

		// 下一块的起始位置 = 当前块结束位置 - 重叠长度
		start = end - s.Overlap
		if start < 0 {
			start = 0
		}
		if start >= textLen {
			break
		}
	}

	return chunks
}

// ParagraphSplitter 按段落切片器
// 以空行分隔段落，每个段落作为一个切片
type ParagraphSplitter struct{}

// NewParagraphSplitter 创建段落切片器
func NewParagraphSplitter() *ParagraphSplitter {
	return &ParagraphSplitter{}
}

// Split 执行段落切片
// 策略：按 "\n\n"（两个换行符）分隔段落
func (s *ParagraphSplitter) Split(text string) []Chunk {
	chunks := make([]Chunk, 0)
	paragraphs := strings.Split(text, "\n\n")
	start := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			start += 2
			continue
		}

		end := start + len(para)
		chunks = append(chunks, Chunk{
			Content:  para,
			Start:    start,
			End:      end,
			Metadata: map[string]interface{}{"chunk_type": "paragraph"},
		})
		start = end + 2
	}

	return chunks
}

// MarkdownSplitter Markdown 标题层级切片器
// 按标题层级（#、##、### 等）切片，保留文档结构
type MarkdownSplitter struct{}

// NewMarkdownSplitter 创建 Markdown 切片器
func NewMarkdownSplitter() *MarkdownSplitter {
	return &MarkdownSplitter{}
}

// headingRegex 匹配 Markdown 标题的正则表达式
// 格式：# 标题 或 ## 标题
var headingRegex = regexp.MustCompile(`^(#{1,6})\s+(.+)`)

// Split 执行 Markdown 标题层级切片
// 策略：
//   1. 按标题层级切分，每个标题及其内容作为一个切片
//   2. 保留标题信息在元数据中（heading, heading_level）
func (s *MarkdownSplitter) Split(text string) []Chunk {
	chunks := make([]Chunk, 0)
	scanner := bufio.NewScanner(strings.NewReader(text))

	var currentContent strings.Builder
	var currentHeading string
	var headingLevel int
	start := 0

	for scanner.Scan() {
		line := scanner.Text()
		// 检测标题行
		if match := headingRegex.FindStringSubmatch(line); match != nil {
			// 如果已有内容，先保存上一个切片
			if currentContent.Len() > 0 && currentHeading != "" {
				chunks = append(chunks, Chunk{
					Content: currentContent.String(),
					Start:   start,
					End:     start + currentContent.Len(),
					Metadata: map[string]interface{}{
						"chunk_type":     "markdown",
						"heading":        currentHeading,
						"heading_level":  headingLevel,
					},
				})
				start += currentContent.Len() + len(currentHeading) + 2
			}
			// 开始新的切片
			currentContent.Reset()
			currentHeading = match[2]
			headingLevel = len(match[1])
		} else {
			// 非标题行，添加到当前内容
			if currentContent.Len() > 0 {
				currentContent.WriteString("\n")
			}
			currentContent.WriteString(line)
		}
	}

	// 保存最后一个切片
	if currentContent.Len() > 0 {
		chunks = append(chunks, Chunk{
			Content: currentContent.String(),
			Start:   start,
			End:     start + currentContent.Len(),
			Metadata: map[string]interface{}{
				"chunk_type":     "markdown",
				"heading":        currentHeading,
				"heading_level":  headingLevel,
			},
		})
	}

	return chunks
}

// ReadFile 读取文件内容
func ReadFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// DetectFileType 检测文件类型
// 返回: "markdown" 或 "txt"
func DetectFileType(filePath string) string {
	if strings.HasSuffix(filePath, ".md") || strings.HasSuffix(filePath, ".markdown") {
		return "markdown"
	}
	if strings.HasSuffix(filePath, ".txt") {
		return "txt"
	}
	return "txt"
}