package text

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// BM25Retriever BM25 关键词检索器
type BM25Retriever struct {
	K1        float64 // 词频饱和度参数
	B         float64 // 文档长度归一化参数
	documents []BM25Document
	avgDocLen float64
}

type BM25Document struct {
	ID       int64
	Content  string
	WordFreq map[string]int
	Length   int
}

// NewBM25Retriever 创建 BM25 检索器
func NewBM25Retriever() *BM25Retriever {
	return &BM25Retriever{
		K1: 1.2,
		B:  0.75,
	}
}

// BuildIndex 构建索引
func (r *BM25Retriever) BuildIndex(documents []BM25Document) {
	r.documents = documents

	totalLen := 0
	for _, doc := range documents {
		totalLen += doc.Length
	}
	if len(documents) > 0 {
		r.avgDocLen = float64(totalLen) / float64(len(documents))
	}
}

// Retrieve 检索文档
func (r *BM25Retriever) Retrieve(query string, topK int) []BM25Result {
	queryWords := r.tokenize(query)
	if len(queryWords) == 0 {
		return nil
	}

	results := make([]BM25Result, 0, len(r.documents))

	for _, doc := range r.documents {
		score := r.score(queryWords, doc)
		if score > 0 {
			results = append(results, BM25Result{
				DocID:   doc.ID,
				Content: doc.Content,
				Score:   score,
			})
		}
	}

	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > topK {
		return results[:topK]
	}
	return results
}

// score 计算单个文档的 BM25 分数
func (r *BM25Retriever) score(queryWords []string, doc BM25Document) float64 {
	var score float64

	for _, word := range queryWords {
		tf := float64(doc.WordFreq[word])
		if tf == 0 {
			continue
		}

		idf := r.idf(word)

		numerator := tf * (r.K1 + 1)
		denominator := tf + r.K1*(1-r.B+r.B*float64(doc.Length)/r.avgDocLen)

		score += idf * numerator / denominator
	}

	return score
}

// idf 计算逆文档频率
func (r *BM25Retriever) idf(word string) float64 {
	docCount := 0
	for _, doc := range r.documents {
		if doc.WordFreq[word] > 0 {
			docCount++
		}
	}

	if docCount == 0 {
		return 0
	}

	return math.Log((float64(len(r.documents)-docCount)+0.5)/(float64(docCount)+0.5) + 1)
}

// tokenize 分词
func (r *BM25Retriever) tokenize(text string) []string {
	text = strings.ToLower(text)

	words := make([]string, 0)
	currentWord := make([]rune, 0)

	for _, ch := range text {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			currentWord = append(currentWord, ch)
		} else {
			if len(currentWord) > 0 {
				words = append(words, string(currentWord))
				currentWord = make([]rune, 0)
			}
		}
	}

	if len(currentWord) > 0 {
		words = append(words, string(currentWord))
	}

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
		"should": true, "may": true, "might": true, "must": true, "shall": true, "can": true,
		"need": true, "dare": true, "ought": true, "used": true, "to": true, "of": true,
		"in": true, "for": true, "on": true, "with": true, "at": true, "by": true, "from": true,
		"as": true, "into": true, "through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "under": true, "again": true, "further": true,
		"then": true, "once": true, "here": true, "there": true, "when": true, "where": true,
		"why": true, "how": true, "all": true, "each": true, "few": true, "more": true, "most": true,
		"other": true, "some": true, "such": true, "no": true, "nor": true, "not": true, "only": true,
		"own": true, "same": true, "so": true, "than": true, "too": true, "very": true, "just": true,
		"but": true, "and": true, "if": true, "or": true, "because": true, "until": true, "while": true,
		"this": true, "that": true, "these": true, "those": true, "what": true, "which": true, "who": true,
		"whom": true, "its": true, "my": true, "your": true, "our": true, "their": true, "i": true,
		"you": true, "he": true, "she": true, "it": true, "we": true, "they": true, "me": true,
		"him": true, "her": true, "us": true, "them": true, "go": true, "语言": true, "是": true,
		"的": true, "在": true, "和": true, "与": true, "有": true, "我": true, "你": true,
		"他": true, "她": true, "它": true, "我们": true, "你们": true, "他们": true, "这": true,
		"那": true, "这些": true, "那些": true, "什么": true, "哪个": true, "谁": true, "怎么": true,
		"为什么": true, "因为": true, "所以": true, "但是": true, "如果": true, "或者": true,
		"以及": true, "还有": true, "比如": true, "例如": true, "就是": true, "都是": true,
		"可以": true, "可能": true, "应该": true, "必须": true, "需要": true, "已经": true,
		"正在": true, "将要": true, "曾经": true, "从来": true, "总是": true, "经常": true,
		"偶尔": true, "很少": true, "从不": true, "一起": true, "自己": true,
		"各自": true, "互相": true, "彼此": true, "每": true, "各个": true, "任何": true,
		"某些": true, "许多": true, "少量": true, "全部": true, "部分": true, "一些": true,
		"一点": true, "很多": true, "太少": true, "更多": true, "更少": true, "足够": true,
		"不足": true, "完全": true, "几乎": true, "大约": true, "大概": true, "仅仅": true,
		"只": true, "才": true, "就": true, "还": true, "也": true, "又": true, "再": true,
		"更": true, "最": true, "很": true, "非常": true, "特别": true, "尤其": true,
		"主要": true, "基本": true, "根本": true, "简直": true, "实在": true, "确实": true,
		"真的": true, "当然": true, "自然": true, "必然": true, "也许": true,
		"或许": true, "恐怕": true, "其实": true, "实际上": true, "事实上": true, "总之": true,
		"总而言之": true, "举例来说": true, "也就是说": true,
		"换句话说": true, "相反": true, "反之": true, "另外": true, "此外": true, "同时": true,
		"与此同时": true, "然而": true, "不过": true, "尽管": true, "虽然": true, "即使": true,
		"假如": true, "倘若": true, "万一": true, "除非": true, "只要": true, "只有": true,
		"以便": true, "为了": true, "以免": true, "省得": true, "否则": true, "要不": true,
		"不然": true, "要么": true, "还是": true, "与其": true, "不如": true,
		"宁可": true, "宁愿": true, "就算": true, "哪怕": true, "不管": true,
		"无论": true, "不论": true, "不管怎样": true, "无论如何": true,
	}

	filtered := make([]string, 0)
	for _, word := range words {
		if !stopWords[word] && len(word) > 1 {
			filtered = append(filtered, word)
		}
	}

	return filtered
}

// BM25Result BM25 检索结果
type BM25Result struct {
	DocID   int64   `json:"doc_id"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// BuildBM25Document 构建 BM25 文档
func BuildBM25Document(id int64, content string) BM25Document {
	words := tokenizeForBM25(content)
	wordFreq := make(map[string]int)
	for _, word := range words {
		wordFreq[word]++
	}

	return BM25Document{
		ID:       id,
		Content:  content,
		WordFreq: wordFreq,
		Length:   len(content),
	}
}

func tokenizeForBM25(text string) []string {
	text = strings.ToLower(text)
	re := regexp.MustCompile(`[\w一-龥]+`)
	matches := re.FindAllString(text, -1)

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
		"should": true, "may": true, "might": true, "must": true, "shall": true, "can": true,
		"to": true, "of": true, "in": true, "for": true, "on": true, "with": true, "at": true,
		"by": true, "from": true, "as": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true, "between": true, "under": true,
		"again": true, "further": true, "then": true, "once": true, "here": true, "there": true,
		"when": true, "where": true, "why": true, "how": true, "all": true, "each": true,
		"few": true, "more": true, "most": true, "other": true, "some": true, "such": true,
		"no": true, "nor": true, "not": true, "only": true, "own": true, "same": true,
		"so": true, "than": true, "too": true, "very": true, "just": true, "but": true,
		"and": true, "if": true, "or": true, "because": true, "until": true, "while": true,
		"this": true, "that": true, "these": true, "those": true, "what": true, "which": true,
		"who": true, "whom": true, "its": true, "my": true, "your": true, "our": true,
		"their": true, "i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "me": true, "him": true, "her": true, "us": true,
		"them": true, "go": true, "语言": true, "是": true, "的": true, "在": true,
		"和": true, "与": true, "有": true, "我": true, "你": true, "他": true, "她": true,
		"它": true, "我们": true, "你们": true, "他们": true, "这": true, "那": true,
		"这些": true, "那些": true, "什么": true, "哪个": true, "谁": true, "怎么": true,
		"为什么": true, "因为": true, "所以": true, "但是": true, "如果": true, "或者": true,
	}

	filtered := make([]string, 0)
	for _, word := range matches {
		if !stopWords[word] && len(word) > 1 {
			filtered = append(filtered, word)
		}
	}

	return filtered
}
