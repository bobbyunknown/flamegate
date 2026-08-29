package cache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIEmbedder_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/embeddings", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req embeddingRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "text-embedding-3-small", req.Model)

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
			}{
				{Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	emb := NewAPIEmbedder(APIEmbedderConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key",
		Model:   "text-embedding-3-small",
		Dims:    4,
	})

	vec, err := emb.Embed(context.Background(), "Hello world")
	require.NoError(t, err)
	require.Len(t, vec, 4)
	require.InDelta(t, float32(0.1), vec[0], 1e-6)
	require.InDelta(t, float32(0.4), vec[3], 1e-6)
}

func TestAPIEmbedder_EmbedDifferentTexts(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var vec []float32
		if callCount == 1 {
			vec = []float32{0.9, 0.1, 0.0}
		} else {
			vec = []float32{0.85, 0.15, 0.05}
		}
		json.NewEncoder(w).Encode(embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
			}{{Embedding: vec}},
		})
	}))
	defer srv.Close()

	emb := NewAPIEmbedder(APIEmbedderConfig{BaseURL: srv.URL, Dims: 3})

	vec1, err := emb.Embed(context.Background(), "How do Go channels work?")
	require.NoError(t, err)

	vec2, err := emb.Embed(context.Background(), "Explain Go channels")
	require.NoError(t, err)

	// Different texts should produce different vectors.
	require.NotEqual(t, vec1, vec2)

	// But they should be semantically similar (high cosine).
	score := cosine(vec1, vec2)
	require.Greater(t, score, 0.9, "semantically similar texts should have high cosine")
}

func TestAPIEmbedder_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	emb := NewAPIEmbedder(APIEmbedderConfig{BaseURL: srv.URL, Dims: 3})
	_, err := emb.Embed(context.Background(), "test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestAPIEmbedder_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embeddingResponse{Data: nil})
	}))
	defer srv.Close()

	emb := NewAPIEmbedder(APIEmbedderConfig{BaseURL: srv.URL, Dims: 3})
	_, err := emb.Embed(context.Background(), "test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty embedding")
}

func TestAPIEmbedder_APIErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(embeddingResponse{
			Error: &struct {
				Message string `json:"message"`
			}{Message: "invalid model"},
		})
	}))
	defer srv.Close()

	emb := NewAPIEmbedder(APIEmbedderConfig{BaseURL: srv.URL, Dims: 3})
	_, err := emb.Embed(context.Background(), "test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}
