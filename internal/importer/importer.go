package importer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cofood/internal/config"
	"cofood/internal/database"
	"cofood/internal/embedding"
	"cofood/internal/model"
)

var valuePattern = regexp.MustCompile(`^\s*([+-]?\d+(?:\.\d+)?)\s*(.*)\s*$`)

type Service struct {
	store       *database.Store
	embedClient *embedding.Client
	cfg         config.Config
}

func NewService(store *database.Store, embedClient *embedding.Client, cfg config.Config) *Service {
	return &Service{
		store:       store,
		embedClient: embedClient,
		cfg:         cfg,
	}
}

func (s *Service) Bootstrap(ctx context.Context) error {
	foodCount, err := s.store.FoodCount(ctx)
	if err != nil {
		return err
	}

	if foodCount == 0 {
		log.Printf("foods table is empty, importing from %s", s.cfg.DataFilePath)
		if err := s.ImportJSONL(ctx, s.cfg.DataFilePath); err != nil {
			return err
		}
		foodCount, err = s.store.FoodCount(ctx)
		if err != nil {
			return err
		}
		log.Printf("food import completed, total=%d", foodCount)
	}

	if !s.cfg.AutoEmbedOnStartup || !s.embedClient.Enabled() {
		return nil
	}

	embeddingCount, err := s.store.EmbeddingCount(ctx)
	if err != nil {
		return err
	}
	log.Printf("embedding backfill starting: existing=%d total=%d model=%s", embeddingCount, foodCount, s.cfg.EmbeddingModel)

	return s.BackfillEmbeddings(ctx)
}

func (s *Service) ImportJSONL(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open jsonl: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for lineNo := 1; scanner.Scan(); lineNo++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw model.RawFood
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return fmt.Errorf("decode line %d: %w", lineNo, err)
		}

		food, err := transformRawFood(raw, line, s.cfg.EmbeddingSourceMaxRunes)
		if err != nil {
			return fmt.Errorf("transform line %d: %w", lineNo, err)
		}

		if _, err := s.store.UpsertFood(ctx, food); err != nil {
			return fmt.Errorf("store line %d: %w", lineNo, err)
		}
	}

	return scanner.Err()
}

func (s *Service) BackfillEmbeddings(ctx context.Context) error {
	if !s.embedClient.Enabled() {
		return nil
	}

	totalFoods, err := s.store.FoodCount(ctx)
	if err != nil {
		return err
	}
	completed, err := s.store.EmbeddingCount(ctx)
	if err != nil {
		return err
	}

	for {
		foods, err := s.store.ListFoodsWithoutEmbeddings(ctx, s.cfg.EmbeddingBatchSize)
		if err != nil {
			return err
		}
		if len(foods) == 0 {
			log.Printf("embedding backfill completed: total=%d", completed)
			return nil
		}

		startedAt := time.Now()
		inputs := make([]string, 0, len(foods))
		for _, food := range foods {
			inputs = append(inputs, food.SearchText)
		}

		vectors, err := s.embedClient.EmbedTexts(ctx, inputs)
		if err != nil {
			return err
		}

		for i, food := range foods {
			if err := s.store.UpsertEmbedding(ctx, model.EmbeddingRecord{
				FoodID:      food.ID,
				Model:       s.cfg.EmbeddingModel,
				Vector:      vectors[i],
				VectorDim:   len(vectors[i]),
				SourceText:  food.SearchText,
				UpdatedUnix: database.NowUnix(),
			}); err != nil {
				return err
			}
		}

		completed += len(foods)
		log.Printf(
			"embedding backfill progress: %d/%d (batch=%d elapsed=%s)",
			completed,
			totalFoods,
			len(foods),
			time.Since(startedAt).Round(time.Millisecond),
		)
	}
}

func transformRawFood(raw model.RawFood, rawLine string, maxRunes int) (model.Food, error) {
	food := model.Food{
		SourceOID:   raw.ID.OID,
		Name:        strings.TrimSpace(raw.Name),
		Nickname:    strings.TrimSpace(raw.Nickname),
		Type:        strings.TrimSpace(raw.Type),
		URL:         strings.TrimSpace(raw.URL),
		ImageURL:    strings.TrimSpace(raw.ImageURL),
		UpdateTime:  raw.UpdateTime,
		RawDocument: rawLine,
	}

	keys := make([]string, 0, len(raw.Info))
	for key := range raw.Info {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for index, key := range keys {
		nutrient := model.Nutrient{
			Name:     key,
			RawValue: strings.TrimSpace(raw.Info[key]),
			Order:    index + 1,
		}
		amount, unit := parseRawValue(nutrient.RawValue)
		nutrient.Amount = amount
		nutrient.Unit = unit
		food.Nutrients = append(food.Nutrients, nutrient)
	}

	food.SearchText = buildSearchText(food, maxRunes)
	return food, nil
}

func parseRawValue(raw string) (*float64, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ""
	}

	matches := valuePattern.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return nil, raw
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return nil, raw
	}

	unit := strings.TrimSpace(matches[2])
	return &value, unit
}

func buildSearchText(food model.Food, maxRunes int) string {
	parts := []string{fmt.Sprintf("名称: %s", food.Name)}
	if food.Nickname != "" && !strings.EqualFold(food.Nickname, "无") {
		parts = append(parts, fmt.Sprintf("别名: %s", food.Nickname))
	}
	if food.Type != "" {
		parts = append(parts, fmt.Sprintf("类别: %s", food.Type))
	}
	for _, nutrient := range food.Nutrients {
		parts = append(parts, fmt.Sprintf("%s: %s", nutrient.Name, nutrient.RawValue))
	}

	text := strings.Join(parts, "；")
	runes := []rune(text)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return text
}
