package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cofood/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InitSchema() error {
	schema := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`CREATE TABLE IF NOT EXISTS foods (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_oid TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			name_norm TEXT NOT NULL,
			nickname TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT '',
			image_url TEXT NOT NULL DEFAULT '',
			update_time INTEGER NOT NULL DEFAULT 0,
			search_text TEXT NOT NULL,
			raw_document TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (unixepoch())
		);`,
		`CREATE INDEX IF NOT EXISTS idx_foods_name_norm ON foods(name_norm);`,
		`CREATE INDEX IF NOT EXISTS idx_foods_type ON foods(type);`,
		`CREATE TABLE IF NOT EXISTS food_aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			food_id INTEGER NOT NULL,
			alias TEXT NOT NULL,
			alias_norm TEXT NOT NULL,
			alias_type TEXT NOT NULL,
			UNIQUE(food_id, alias, alias_type),
			FOREIGN KEY(food_id) REFERENCES foods(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_food_aliases_norm ON food_aliases(alias_norm);`,
		`CREATE TABLE IF NOT EXISTS food_nutrients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			food_id INTEGER NOT NULL,
			nutrient_name TEXT NOT NULL,
			raw_value TEXT NOT NULL,
			amount REAL NULL,
			unit TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL,
			UNIQUE(food_id, nutrient_name),
			FOREIGN KEY(food_id) REFERENCES foods(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_food_nutrients_food_id ON food_nutrients(food_id);`,
		`CREATE TABLE IF NOT EXISTS food_embeddings (
			food_id INTEGER PRIMARY KEY,
			model TEXT NOT NULL,
			vector_json TEXT NOT NULL,
			vector_dim INTEGER NOT NULL,
			source_text TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY(food_id) REFERENCES foods(id) ON DELETE CASCADE
		);`,
	}

	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) FoodCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM foods`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) EmbeddingCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM food_embeddings`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) UpsertFood(ctx context.Context, food model.Food) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO foods (source_oid, name, name_norm, nickname, type, url, image_url, update_time, search_text, raw_document)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_oid) DO UPDATE SET
			name = excluded.name,
			name_norm = excluded.name_norm,
			nickname = excluded.nickname,
			type = excluded.type,
			url = excluded.url,
			image_url = excluded.image_url,
			update_time = excluded.update_time,
			search_text = excluded.search_text,
			raw_document = excluded.raw_document
	`, food.SourceOID, food.Name, NormalizeText(food.Name), food.Nickname, food.Type, food.URL, food.ImageURL, food.UpdateTime, food.SearchText, food.RawDocument)
	if err != nil {
		return 0, err
	}

	foodID, idErr := result.LastInsertId()
	if idErr != nil || foodID == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT id FROM foods WHERE source_oid = ?`, food.SourceOID).Scan(&foodID); err != nil {
			return 0, err
		}
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM food_aliases WHERE food_id = ?`, foodID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM food_nutrients WHERE food_id = ?`, foodID); err != nil {
		return 0, err
	}

	for _, alias := range BuildAliases(food) {
		if _, err = tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO food_aliases (food_id, alias, alias_norm, alias_type)
			VALUES (?, ?, ?, ?)
		`, foodID, alias.Alias, alias.AliasNorm, alias.AliasType); err != nil {
			return 0, err
		}
	}

	for _, nutrient := range food.Nutrients {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO food_nutrients (food_id, nutrient_name, raw_value, amount, unit, sort_order)
			VALUES (?, ?, ?, ?, ?, ?)
		`, foodID, nutrient.Name, nutrient.RawValue, nutrient.Amount, nutrient.Unit, nutrient.Order); err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return foodID, nil
}

func (s *Store) UpsertEmbedding(ctx context.Context, record model.EmbeddingRecord) error {
	raw, err := json.Marshal(record.Vector)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO food_embeddings (food_id, model, vector_json, vector_dim, source_text, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(food_id) DO UPDATE SET
			model = excluded.model,
			vector_json = excluded.vector_json,
			vector_dim = excluded.vector_dim,
			source_text = excluded.source_text,
			updated_at = excluded.updated_at
	`, record.FoodID, record.Model, string(raw), record.VectorDim, record.SourceText, record.UpdatedUnix)
	return err
}

func (s *Store) SearchExact(ctx context.Context, query string, limit int) ([]model.SearchResult, error) {
	norm := NormalizeText(query)
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT f.id, f.source_oid, f.name, f.nickname, f.type, f.url, f.image_url, f.update_time, f.search_text, f.raw_document,
		       CASE
		           WHEN f.name_norm = ? THEN 'name'
		           ELSE 'alias'
		       END AS matched_by
		FROM foods f
		LEFT JOIN food_aliases a ON a.food_id = f.id
		WHERE f.name_norm = ? OR a.alias_norm = ?
		ORDER BY CASE WHEN f.name_norm = ? THEN 0 ELSE 1 END, f.id
		LIMIT ?
	`, norm, norm, norm, norm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchResults(rows, "exact")
}

func (s *Store) SearchKeyword(ctx context.Context, query string, limit int) ([]model.SearchResult, error) {
	likeQuery := "%" + EscapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT f.id, f.source_oid, f.name, f.nickname, f.type, f.url, f.image_url, f.update_time, f.search_text, f.raw_document,
		       CASE
		           WHEN f.name LIKE ? ESCAPE '\' THEN 'name'
		           WHEN f.nickname LIKE ? ESCAPE '\' THEN 'nickname'
		           WHEN a.alias LIKE ? ESCAPE '\' THEN 'alias'
		           ELSE 'type'
		       END AS matched_by
		FROM foods f
		LEFT JOIN food_aliases a ON a.food_id = f.id
		WHERE f.name LIKE ? ESCAPE '\'
		   OR f.nickname LIKE ? ESCAPE '\'
		   OR f.type LIKE ? ESCAPE '\'
		   OR a.alias LIKE ? ESCAPE '\'
		ORDER BY CASE
		           WHEN f.name LIKE ? ESCAPE '\' THEN 0
		           WHEN f.nickname LIKE ? ESCAPE '\' THEN 1
		           ELSE 2
		         END,
		         LENGTH(f.name),
		         f.id
		LIMIT ?
	`, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, likeQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchResults(rows, "keyword")
}

func (s *Store) ListFoodsWithoutEmbeddings(ctx context.Context, limit int) ([]model.Food, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.source_oid, f.name, f.nickname, f.type, f.url, f.image_url, f.update_time, f.search_text, f.raw_document
		FROM foods f
		LEFT JOIN food_embeddings e ON e.food_id = f.id
		WHERE e.food_id IS NULL
		ORDER BY f.id
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	foods := make([]model.Food, 0)
	for rows.Next() {
		var food model.Food
		if err := rows.Scan(&food.ID, &food.SourceOID, &food.Name, &food.Nickname, &food.Type, &food.URL, &food.ImageURL, &food.UpdateTime, &food.SearchText, &food.RawDocument); err != nil {
			return nil, err
		}
		foods = append(foods, food)
	}
	return foods, rows.Err()
}

func (s *Store) LoadEmbeddings(ctx context.Context, modelName string) ([]model.EmbeddingRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT food_id, model, vector_json, vector_dim, source_text, updated_at
		FROM food_embeddings
		WHERE model = ?
	`, modelName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.EmbeddingRecord, 0)
	for rows.Next() {
		var (
			record    model.EmbeddingRecord
			vectorRaw string
		)
		if err := rows.Scan(&record.FoodID, &record.Model, &vectorRaw, &record.VectorDim, &record.SourceText, &record.UpdatedUnix); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(vectorRaw), &record.Vector); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) GetFoodsByIDs(ctx context.Context, ids []int64) (map[int64]model.Food, error) {
	if len(ids) == 0 {
		return map[int64]model.Food{}, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, source_oid, name, nickname, type, url, image_url, update_time, search_text, raw_document
		FROM foods
		WHERE id IN (%s)
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	foods := make(map[int64]model.Food, len(ids))
	for rows.Next() {
		var food model.Food
		if err := rows.Scan(&food.ID, &food.SourceOID, &food.Name, &food.Nickname, &food.Type, &food.URL, &food.ImageURL, &food.UpdateTime, &food.SearchText, &food.RawDocument); err != nil {
			return nil, err
		}
		foods[food.ID] = food
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	nutrientRows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT food_id, nutrient_name, raw_value, amount, unit, sort_order
		FROM food_nutrients
		WHERE food_id IN (%s)
		ORDER BY food_id, sort_order
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer nutrientRows.Close()

	for nutrientRows.Next() {
		var (
			foodID   int64
			nutrient model.Nutrient
			amount   sql.NullFloat64
		)
		if err := nutrientRows.Scan(&foodID, &nutrient.Name, &nutrient.RawValue, &amount, &nutrient.Unit, &nutrient.Order); err != nil {
			return nil, err
		}
		if amount.Valid {
			nutrient.Amount = &amount.Float64
		}
		food := foods[foodID]
		food.Nutrients = append(food.Nutrients, nutrient)
		foods[foodID] = food
	}

	return foods, nutrientRows.Err()
}

func scanSearchResults(rows *sql.Rows, matchType string) ([]model.SearchResult, error) {
	results := make([]model.SearchResult, 0)
	seen := make(map[int64]struct{})

	for rows.Next() {
		var (
			result    model.SearchResult
			matchedBy string
		)
		if err := rows.Scan(
			&result.Food.ID,
			&result.Food.SourceOID,
			&result.Food.Name,
			&result.Food.Nickname,
			&result.Food.Type,
			&result.Food.URL,
			&result.Food.ImageURL,
			&result.Food.UpdateTime,
			&result.Food.SearchText,
			&result.Food.RawDocument,
			&matchedBy,
		); err != nil {
			return nil, err
		}

		if _, ok := seen[result.Food.ID]; ok {
			continue
		}

		seen[result.Food.ID] = struct{}{}
		result.MatchType = matchType
		result.MatchedBy = matchedBy
		results = append(results, result)
	}

	return results, rows.Err()
}

func BuildAliases(food model.Food) []model.Alias {
	seen := make(map[string]struct{})
	aliases := make([]model.Alias, 0, 6)

	addAlias := func(value, aliasType string) {
		value = strings.TrimSpace(value)
		if value == "" || strings.EqualFold(value, "无") {
			return
		}
		key := aliasType + "::" + value
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		aliases = append(aliases, model.Alias{
			Alias:     value,
			AliasNorm: NormalizeText(value),
			AliasType: aliasType,
		})
	}

	addAlias(food.Name, "name")
	addAlias(food.Nickname, "nickname")
	for _, item := range splitNicknameAliases(food.Nickname) {
		addAlias(item, "nickname")
	}
	return aliases
}

func NormalizeText(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	replacer := strings.NewReplacer(
		"\t", " ",
		"\n", " ",
		"\r", " ",
		"，", ",",
		"（", "(",
		"）", ")",
		"、", " ",
	)
	input = replacer.Replace(input)
	return strings.Join(strings.Fields(input), " ")
}

func EscapeLike(input string) string {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `%`, `\%`)
	input = strings.ReplaceAll(input, `_`, `\_`)
	return input
}

func splitNicknameAliases(nickname string) []string {
	replaced := strings.NewReplacer("、", ",", "，", ",", "；", ",", ";", ",").Replace(nickname)
	parts := strings.Split(replaced, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || strings.EqualFold(part, "无") {
			continue
		}
		result = append(result, part)
	}
	return result
}

func NowUnix() int64 {
	return time.Now().Unix()
}
