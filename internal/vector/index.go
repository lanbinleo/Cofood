package vector

import (
	"math"
	"sort"

	"cofood/internal/model"
)

type Candidate struct {
	FoodID     int64
	Similarity float64
}

type Index struct {
	items []model.EmbeddingRecord
}

func NewIndex(items []model.EmbeddingRecord) *Index {
	return &Index{items: items}
}

func (i *Index) Size() int {
	return len(i.items)
}

func (i *Index) Search(query []float64, topK int) []Candidate {
	if len(query) == 0 || topK <= 0 {
		return nil
	}

	candidates := make([]Candidate, 0, len(i.items))
	for _, item := range i.items {
		score := cosineSimilarity(query, item.Vector)
		if math.IsNaN(score) || math.IsInf(score, 0) {
			continue
		}
		candidates = append(candidates, Candidate{
			FoodID:     item.FoodID,
			Similarity: score,
		})
	}

	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].Similarity == candidates[b].Similarity {
			return candidates[a].FoodID < candidates[b].FoodID
		}
		return candidates[a].Similarity > candidates[b].Similarity
	})

	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}

	var dot, normLeft, normRight float64
	for i := range left {
		dot += left[i] * right[i]
		normLeft += left[i] * left[i]
		normRight += right[i] * right[i]
	}
	if normLeft == 0 || normRight == 0 {
		return 0
	}
	return dot / (math.Sqrt(normLeft) * math.Sqrt(normRight))
}
