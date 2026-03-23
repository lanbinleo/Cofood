package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cofood/internal/config"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	dimensions int
}

type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    strings.TrimRight(cfg.SiliconFlowBaseURL, "/"),
		apiKey:     cfg.SiliconFlowAPIKey,
		model:      cfg.EmbeddingModel,
		dimensions: cfg.EmbeddingDimensions,
	}
}

func (c *Client) Enabled() bool {
	return c.apiKey != ""
}

func (c *Client) EmbedTexts(ctx context.Context, inputs []string) ([][]float64, error) {
	if len(inputs) == 0 {
		return [][]float64{}, nil
	}
	if !c.Enabled() {
		return nil, fmt.Errorf("siliconflow api key is not configured")
	}

	payload, err := json.Marshal(embedRequest{
		Model:      c.model,
		Input:      inputs,
		Dimensions: c.dimensions,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("siliconflow embeddings failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed embedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data) != len(inputs) {
		return nil, fmt.Errorf("unexpected embedding count: got %d want %d", len(parsed.Data), len(inputs))
	}

	result := make([][]float64, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(inputs) {
			return nil, fmt.Errorf("unexpected embedding index: %d", item.Index)
		}
		result[item.Index] = item.Embedding
	}
	return result, nil
}
