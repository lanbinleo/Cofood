package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

const (
	defaultHost                    = "0.0.0.0"
	defaultPort                    = "8080"
	defaultDatabasePath            = "data/cofood.db"
	defaultDataFilePath            = "food-table.jsonl"
	defaultSiliconFlowBaseURL      = "https://api.siliconflow.cn/v1"
	defaultEmbeddingModel          = "Qwen/Qwen3-Embedding-8B"
	defaultEmbeddingDimensions     = 1024
	defaultMaxKeywordResults       = 20
	defaultMaxVectorResults        = 10
	defaultAutoEmbedOnStartup      = false
	defaultEmbeddingBatchSize      = 16
	defaultEmbeddingSourceMaxRunes = 2200
)

type Config struct {
	Host                    string
	Port                    string
	DatabasePath            string
	DataFilePath            string
	SiliconFlowBaseURL      string
	SiliconFlowAPIKey       string
	EmbeddingModel          string
	EmbeddingDimensions     int
	MaxKeywordResults       int
	MaxVectorResults        int
	AutoEmbedOnStartup      bool
	EmbeddingBatchSize      int
	EmbeddingSourceMaxRunes int
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Host:                    getEnv("APP_HOST", defaultHost),
		Port:                    getEnv("APP_PORT", defaultPort),
		DatabasePath:            getEnv("DATABASE_PATH", defaultDatabasePath),
		DataFilePath:            getEnv("DATA_FILE_PATH", defaultDataFilePath),
		SiliconFlowBaseURL:      getEnv("SILICONFLOW_BASE_URL", defaultSiliconFlowBaseURL),
		SiliconFlowAPIKey:       os.Getenv("SILICONFLOW_API_KEY"),
		EmbeddingModel:          getEnv("EMBEDDING_MODEL", defaultEmbeddingModel),
		EmbeddingDimensions:     getEnvInt("EMBEDDING_DIMENSIONS", defaultEmbeddingDimensions),
		MaxKeywordResults:       getEnvInt("MAX_KEYWORD_RESULTS", defaultMaxKeywordResults),
		MaxVectorResults:        getEnvInt("MAX_VECTOR_RESULTS", defaultMaxVectorResults),
		AutoEmbedOnStartup:      getEnvBool("AUTO_EMBED_ON_STARTUP", defaultAutoEmbedOnStartup),
		EmbeddingBatchSize:      getEnvInt("EMBEDDING_BATCH_SIZE", defaultEmbeddingBatchSize),
		EmbeddingSourceMaxRunes: getEnvInt("EMBEDDING_SOURCE_MAX_RUNES", defaultEmbeddingSourceMaxRunes),
	}

	if cfg.EmbeddingDimensions <= 0 {
		return Config{}, fmt.Errorf("EMBEDDING_DIMENSIONS must be positive")
	}
	if cfg.MaxKeywordResults <= 0 {
		return Config{}, fmt.Errorf("MAX_KEYWORD_RESULTS must be positive")
	}
	if cfg.MaxVectorResults <= 0 {
		return Config{}, fmt.Errorf("MAX_VECTOR_RESULTS must be positive")
	}
	if cfg.EmbeddingBatchSize <= 0 {
		return Config{}, fmt.Errorf("EMBEDDING_BATCH_SIZE must be positive")
	}

	return cfg, nil
}

func (c Config) HTTPAddr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
