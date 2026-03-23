package search

import (
	"context"

	"cofood/internal/config"
	"cofood/internal/database"
	"cofood/internal/embedding"
	"cofood/internal/model"
	"cofood/internal/vector"
)

type Service struct {
	store       *database.Store
	embedClient *embedding.Client
	cfg         config.Config
	index       *vector.Index
}

type NameSearchResponse struct {
	Query          string               `json:"query"`
	ExactMatches   []model.SearchResult `json:"exact_matches"`
	KeywordMatches []model.SearchResult `json:"keyword_matches"`
}

type VectorSearchResponse struct {
	Query           string               `json:"query"`
	ExactMatches    []model.SearchResult `json:"exact_matches"`
	VectorMatches   []model.SearchResult `json:"vector_matches"`
	EmbeddingModel  string               `json:"embedding_model"`
	EmbeddingLoaded bool                 `json:"embedding_loaded"`
}

func NewService(store *database.Store, embedClient *embedding.Client, cfg config.Config) (*Service, error) {
	items, err := store.LoadEmbeddings(context.Background(), cfg.EmbeddingModel)
	if err != nil {
		return nil, err
	}

	return &Service{
		store:       store,
		embedClient: embedClient,
		cfg:         cfg,
		index:       vector.NewIndex(items),
	}, nil
}

func (s *Service) SearchByName(ctx context.Context, query string) (NameSearchResponse, error) {
	exact, err := s.store.SearchExact(ctx, query, s.cfg.MaxKeywordResults)
	if err != nil {
		return NameSearchResponse{}, err
	}
	keyword, err := s.store.SearchKeyword(ctx, query, s.cfg.MaxKeywordResults)
	if err != nil {
		return NameSearchResponse{}, err
	}

	exact, err = s.hydrateResults(ctx, exact)
	if err != nil {
		return NameSearchResponse{}, err
	}
	keyword = removeDuplicates(keyword, exact)
	keyword, err = s.hydrateResults(ctx, keyword)
	if err != nil {
		return NameSearchResponse{}, err
	}

	return NameSearchResponse{
		Query:          query,
		ExactMatches:   exact,
		KeywordMatches: keyword,
	}, nil
}

func (s *Service) SearchByVector(ctx context.Context, query string) (VectorSearchResponse, error) {
	exact, err := s.store.SearchExact(ctx, query, s.cfg.MaxVectorResults)
	if err != nil {
		return VectorSearchResponse{}, err
	}
	exact, err = s.hydrateResults(ctx, exact)
	if err != nil {
		return VectorSearchResponse{}, err
	}

	response := VectorSearchResponse{
		Query:           query,
		ExactMatches:    exact,
		VectorMatches:   []model.SearchResult{},
		EmbeddingModel:  s.cfg.EmbeddingModel,
		EmbeddingLoaded: s.index.Size() > 0,
	}

	if !s.embedClient.Enabled() || s.index.Size() == 0 {
		return response, nil
	}

	vectors, err := s.embedClient.EmbedTexts(ctx, []string{query})
	if err != nil {
		return VectorSearchResponse{}, err
	}

	candidates := s.index.Search(vectors[0], s.cfg.MaxVectorResults+len(exact))
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.FoodID)
	}

	foods, err := s.store.GetFoodsByIDs(ctx, ids)
	if err != nil {
		return VectorSearchResponse{}, err
	}

	seen := make(map[int64]struct{}, len(exact))
	for _, item := range exact {
		seen[item.Food.ID] = struct{}{}
	}

	for _, candidate := range candidates {
		if _, ok := seen[candidate.FoodID]; ok {
			continue
		}
		food, ok := foods[candidate.FoodID]
		if !ok {
			continue
		}
		response.VectorMatches = append(response.VectorMatches, model.SearchResult{
			Food:       food,
			MatchType:  "vector",
			MatchedBy:  "embedding",
			Score:      candidate.Similarity,
			Similarity: candidate.Similarity,
		})
		if len(response.VectorMatches) >= s.cfg.MaxVectorResults {
			break
		}
	}

	return response, nil
}

func (s *Service) hydrateResults(ctx context.Context, results []model.SearchResult) ([]model.SearchResult, error) {
	ids := make([]int64, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.Food.ID)
	}

	foods, err := s.store.GetFoodsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	for index := range results {
		if food, ok := foods[results[index].Food.ID]; ok {
			results[index].Food = food
		}
	}
	return results, nil
}

func removeDuplicates(results []model.SearchResult, exact []model.SearchResult) []model.SearchResult {
	seen := make(map[int64]struct{}, len(exact))
	for _, item := range exact {
		seen[item.Food.ID] = struct{}{}
	}

	filtered := make([]model.SearchResult, 0, len(results))
	for _, item := range results {
		if _, ok := seen[item.Food.ID]; ok {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
