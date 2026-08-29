package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIEmbedder calls an OpenAI-compatible /v1/embeddings endpoint to produce
// semantic vectors. Any provider that implements the OpenAI embeddings API
// (OpenAI, Voyage, Cohere via proxy, local servers like Ollama/text-embeddings-
// inference) works out of the box.
type APIEmbedder struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string // e.g. "https://api.openai.com/v1"
	model      string // e.g. "text-embedding-3-small"
	dims       int
}

// APIEmbedderConfig configures the embedding API client.
type APIEmbedderConfig struct {
	BaseURL string // OpenAI-compatible base URL (default: https://api.openai.com/v1)
	APIKey  string // Bearer token
	Model   string // embedding model name (default: text-embedding-3-small)
	Dims    int    // output dimensions (default: 1536)
}

// NewAPIEmbedder builds an embedder that calls an OpenAI-compatible embeddings API.
func NewAPIEmbedder(cfg APIEmbedderConfig) *APIEmbedder {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}
	if cfg.Dims <= 0 {
		cfg.Dims = 1536
	}
	return &APIEmbedder{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		apiKey:     cfg.APIKey,
		baseURL:    cfg.BaseURL,
		model:      cfg.Model,
		dims:       cfg.Dims,
	}
}

// embeddingRequest is the wire format for the OpenAI embeddings API.
type embeddingRequest struct {
	Input      string `json:"input"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// embeddingResponse is the wire format for the OpenAI embeddings API response.
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed calls the embeddings API and returns the resulting vector.
func (e *APIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, err := json.Marshal(embeddingRequest{
		Input:      text,
		Model:      e.model,
		Dimensions: e.dims,
	})
	if err != nil {
		return nil, fmt.Errorf("embedder: marshal request: %w", err)
	}

	url := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedder: http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, fmt.Errorf("embedder: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedder: api returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("embedder: parse response: %w", err)
	}
	if embResp.Error != nil {
		return nil, fmt.Errorf("embedder: api error: %s", embResp.Error.Message)
	}
	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedder: empty embedding in response")
	}

	return embResp.Data[0].Embedding, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
