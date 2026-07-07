package text

import (
	"github.com/golllm/cmd/rag/model"
)

// RRFScore RRF 分数计算结果
type RRFScore struct {
	ChunkID     int64
	DocID       int64
	Content     string
	FileName    string
	VectorScore float64
	BM25Score   float64
	RRFScore    float64
	ParentID    int64
}

// RRF 混合召回排序
func RRF(
	vectorResults []model.Reference,
	bm25Results []BM25Result,
	k int,
	chunkMap map[int64]*model.DocumentChunk,
	docMap map[int64]*model.Document,
) []RRFScore {
	resultMap := make(map[int64]*RRFScore)

	for rank, ref := range vectorResults {
		if _, ok := resultMap[ref.ChunkID]; !ok {
			chunk := chunkMap[ref.ChunkID]
			resultMap[ref.ChunkID] = &RRFScore{
				ChunkID:     ref.ChunkID,
				DocID:       ref.DocID,
				Content:     ref.Content,
				FileName:    ref.FileName,
				ParentID:    chunk.ParentID,
				VectorScore: ref.Score,
			}
		}
		rankScore := float64(k) / float64(rank+1+k)
		resultMap[ref.ChunkID].RRFScore += rankScore
	}

	for rank, bm25 := range bm25Results {
		chunk := chunkMap[bm25.DocID]
		if chunk == nil {
			continue
		}
		doc := docMap[chunk.DocID]

		if _, ok := resultMap[bm25.DocID]; !ok {
			resultMap[bm25.DocID] = &RRFScore{
				ChunkID:   bm25.DocID,
				DocID:     chunk.DocID,
				Content:   bm25.Content,
				FileName:  doc.FileName,
				ParentID:  chunk.ParentID,
				BM25Score: bm25.Score,
			}
		}
		rankScore := float64(k) / float64(rank+1+k)
		resultMap[bm25.DocID].RRFScore += rankScore
	}

	results := make([]RRFScore, 0, len(resultMap))
	for _, v := range resultMap {
		results = append(results, *v)
	}

	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].RRFScore > results[i].RRFScore {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}
