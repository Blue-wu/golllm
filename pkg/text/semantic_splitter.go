package text

import (
	"regexp"
	"strings"

	"github.com/golllm/cmd/rag/model"
)

// SemanticSplitter 语义切片器
type SemanticSplitter struct {
	EmbedFunc    func(texts []string) ([][]float32, error)
	SimilarityThreshold float32 // 相似度阈值，低于此值则分割
	MaxChunkSize int           // 最大切片大小（字符）
	MinChunkSize int           // 最小切片大小（字符）
}

// NewSemanticSplitter 创建语义切片器
func NewSemanticSplitter(embedFunc func(texts []string) ([][]float32, error)) *SemanticSplitter {
	return &SemanticSplitter{
		EmbedFunc:           embedFunc,
		SimilarityThreshold: 0.75,
		MaxChunkSize:        1000,
		MinChunkSize:        50,
	}
}

// Split 执行语义切片
func (s *SemanticSplitter) Split(text string) ([]model.SemanticChunk, error) {
	sentences := s.splitSentences(text)

	if len(sentences) == 0 {
		return nil, nil
	}

	vectors, err := s.EmbedFunc(sentences)
	if err != nil {
		return nil, err
	}

	chunks := s.groupBySimilarity(sentences, vectors)

	return chunks, nil
}

// splitSentences 按句子拆分文本
func (s *SemanticSplitter) splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")

	var sentences []string
	var current string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != "" {
				sentences = append(sentences, current)
				current = ""
			}
			continue
		}

		if strings.HasPrefix(line, "#") {
			if current != "" {
				sentences = append(sentences, current)
				current = ""
			}
			sentences = append(sentences, line)
			continue
		}

		parts := regexp.MustCompile(`([。！？；；?!;]+)`).Split(line, -1)
		separators := regexp.MustCompile(`([。！？；；?!;]+)`).FindAllString(line, -1)

		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if i < len(separators) {
				current += part + separators[i]
			} else {
				current += part
			}
			if len(current) > s.MaxChunkSize {
				sentences = append(sentences, current)
				current = ""
			}
		}
	}

	if current != "" {
		sentences = append(sentences, current)
	}

	return sentences
}

// groupBySimilarity 根据相似度分组句子
func (s *SemanticSplitter) groupBySimilarity(sentences []string, vectors [][]float32) []model.SemanticChunk {
	if len(sentences) == 0 {
		return nil
	}

	chunks := make([]model.SemanticChunk, 0)
	currentChunk := model.SemanticChunk{
		ParentContent: sentences[0],
		ChildContents: []string{sentences[0]},
	}

	for i := 1; i < len(sentences); i++ {
		if len(vectors) <= i || len(vectors) <= i-1 {
			currentChunk.ParentContent += "\n" + sentences[i]
			currentChunk.ChildContents = append(currentChunk.ChildContents, sentences[i])
			continue
		}

		similarity := cosineSimilarity(vectors[i], vectors[i-1])

		isHeading := strings.HasPrefix(sentences[i], "#")

		if similarity < s.SimilarityThreshold || isHeading ||
			len(currentChunk.ParentContent)+len(sentences[i]) > s.MaxChunkSize {

			if len(currentChunk.ParentContent) >= s.MinChunkSize {
				chunks = append(chunks, currentChunk)
			} else if len(chunks) > 0 {
				chunks[len(chunks)-1].ParentContent += "\n" + currentChunk.ParentContent
				chunks[len(chunks)-1].ChildContents = append(chunks[len(chunks)-1].ChildContents, currentChunk.ChildContents...)
			}

			currentChunk = model.SemanticChunk{
				ParentContent: sentences[i],
				ChildContents: []string{sentences[i]},
			}

			if isHeading {
				currentChunk.Heading = strings.TrimPrefix(strings.TrimSpace(sentences[i]), "#")
				currentChunk.Heading = strings.TrimSpace(currentChunk.Heading)
			}
		} else {
			currentChunk.ParentContent += "\n" + sentences[i]
			currentChunk.ChildContents = append(currentChunk.ChildContents, sentences[i])
		}
	}

	if len(currentChunk.ParentContent) >= s.MinChunkSize {
		chunks = append(chunks, currentChunk)
	} else if len(chunks) > 0 {
		chunks[len(chunks)-1].ParentContent += "\n" + currentChunk.ParentContent
		chunks[len(chunks)-1].ChildContents = append(chunks[len(chunks)-1].ChildContents, currentChunk.ChildContents...)
	}

	return chunks
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(v1, v2 []float32) float32 {
	if len(v1) != len(v2) {
		return 0
	}

	var dotProduct float32 = 0
	var mag1 float32 = 0
	var mag2 float32 = 0

	for i := 0; i < len(v1); i++ {
		dotProduct += v1[i] * v2[i]
		mag1 += v1[i] * v1[i]
		mag2 += v2[i] * v2[i]
	}

	if mag1 == 0 || mag2 == 0 {
		return 0
	}

	return dotProduct / (float32(mag1) * float32(mag2))
}