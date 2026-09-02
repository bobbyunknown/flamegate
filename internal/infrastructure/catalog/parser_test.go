package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
)

func TestParseRawCatalog(t *testing.T) {
	rawJSON := []byte(`{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"models": {
				"gpt-4o": {
					"id": "gpt-4o",
					"name": "GPT-4o",
					"description": "Omni model with high intelligence and vision",
					"tool_call": true,
					"reasoning": false,
					"modalities": {
						"input": ["text", "image"],
						"output": ["text"]
					},
					"cost": {
						"input": 2.5,
						"output": 10.0,
						"cache_read": 1.25,
						"cache_write": 3.75,
						"reasoning": 0.0
					},
					"limit": {
						"context": 128000,
						"output": 16384
					}
				},
				"text-embedding-3-small": {
					"id": "text-embedding-3-small",
					"name": "Text Embedding 3 Small",
					"tool_call": false,
					"reasoning": false
				}
			}
		},
		"google": {
			"id": "google",
			"name": "Google",
			"models": {
				"gemini-2.5-flash": {
					"id": "gemini-2.5-flash",
					"name": "Gemini 2.5 Flash",
					"tool_call": true,
					"reasoning": true,
					"modalities": {
						"input": ["text", "image", "audio", "video", "pdf"],
						"output": ["text"]
					},
					"cost": {
						"input": 0.3,
						"output": 2.5,
						"cache_read": 0.03,
						"cache_write": 0.0,
						"reasoning": 2.5
					},
					"limit": {
						"context": 1048576,
						"output": 65536
					}
				}
			}
		},
		"anthropic": {
			"id": "anthropic",
			"name": "Anthropic",
			"models": {
				"claude-3-7-sonnet-20250219": {
					"id": "claude-3-7-sonnet-20250219",
					"name": "Claude 3.7 Sonnet",
					"tool_call": true,
					"reasoning": true,
					"modalities": {
						"input": ["text", "vision", "pdf"],
						"output": ["text"]
					},
					"cost": {
						"input": 3.0,
						"output": 15.0,
						"cache_read": 0.3,
						"cache_write": 3.75
					},
					"limit": {
						"context": 200000,
						"output": 64000
					}
				}
			}
		}
	}`)

	byProviderModel, canonicalIndex, err := catalog.ParseRawCatalog(rawJSON)
	require.NoError(t, err)
	require.NotNil(t, byProviderModel)
	require.NotNil(t, canonicalIndex)

	// Check OpenAI GPT-4o
	gpt4o, ok := byProviderModel["openai/gpt-4o"]
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", gpt4o.ID)
	assert.Equal(t, "GPT-4o", gpt4o.Name)
	assert.Equal(t, "openai", gpt4o.Provider)
	assert.Equal(t, "Omni model with high intelligence and vision", gpt4o.Description)
	assert.True(t, gpt4o.HasTools(), "tool_call: true must map to HasTools() == true")
	assert.False(t, gpt4o.Reasoning)
	assert.True(t, gpt4o.HasVision())
	assert.False(t, gpt4o.HasPDF())
	assert.False(t, gpt4o.HasAudioInput())
	assert.False(t, gpt4o.HasVideoInput())
	assert.Equal(t, 128000, gpt4o.Limits.Context)
	assert.Equal(t, 16384, gpt4o.Limits.Output)
	assert.Equal(t, 2.5, gpt4o.Cost.Input)
	assert.Equal(t, 10.0, gpt4o.Cost.Output)
	assert.Equal(t, 1.25, gpt4o.Cost.CacheRead)
	assert.Equal(t, 3.75, gpt4o.Cost.CacheWrite)

	// Check model with nil Cost, Limit, Modalities
	embed, ok := byProviderModel["openai/text-embedding-3-small"]
	require.True(t, ok)
	assert.Equal(t, "text-embedding-3-small", embed.ID)
	assert.False(t, embed.HasTools())
	assert.False(t, embed.HasVision())
	assert.False(t, embed.HasPDF())
	assert.Equal(t, 0, embed.Limits.Context)
	assert.Equal(t, 0.0, embed.Cost.Input)

	// Check Google Gemini 2.5 Flash
	gemini, ok := byProviderModel["google/gemini-2.5-flash"]
	require.True(t, ok)
	assert.Equal(t, "google", gemini.Provider)
	assert.True(t, gemini.HasTools())
	assert.True(t, gemini.Reasoning)
	assert.True(t, gemini.HasVision())
	assert.True(t, gemini.HasPDF())
	assert.True(t, gemini.HasAudioInput())
	assert.True(t, gemini.HasVideoInput())
	assert.Equal(t, 1048576, gemini.Limits.Context)
	assert.Equal(t, 65536, gemini.Limits.Output)
	assert.Equal(t, 0.3, gemini.Cost.Input)
	assert.Equal(t, 2.5, gemini.Cost.Output)
	assert.Equal(t, 0.03, gemini.Cost.CacheRead)
	assert.Equal(t, 2.5, gemini.Cost.Reasoning)

	// Check Anthropic Claude 3.7 Sonnet (with "vision" modality keyword)
	claude, ok := byProviderModel["anthropic/claude-3-7-sonnet-20250219"]
	require.True(t, ok)
	assert.True(t, claude.HasVision())
	assert.True(t, claude.HasPDF())
	assert.False(t, claude.HasAudioInput())

	// Canonical Index tests
	canonicalGemini, ok := canonicalIndex["gemini-2.5-flash"]
	require.True(t, ok)
	assert.Equal(t, "gemini-2.5-flash", canonicalGemini.ID)
	assert.Equal(t, "google", canonicalGemini.Provider)

	canonicalGPT, ok := canonicalIndex["gpt-4o"]
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", canonicalGPT.ID)
	assert.Equal(t, "openai", canonicalGPT.Provider)
}

func TestParseRawCatalog_InvalidJSON(t *testing.T) {
	_, _, err := catalog.ParseRawCatalog([]byte(`invalid json`))
	require.Error(t, err)
}

func TestExtractBaseModelSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "gpt-4o", expected: "gpt-4o"},
		{input: "openai/gpt-4o", expected: "gpt-4o"},
		{input: "meta-llama/llama-3.3-70b-instruct", expected: "llama-3.3-70b-instruct"},
		{input: "llama3:8b", expected: "llama3"},
		{input: "deepseek/DeepSeek-R1:latest", expected: "deepseek-r1"},
		{input: "  google/GEMINI-2.5-PRO  ", expected: "gemini-2.5-pro"},
		{input: "gemini-3.7-flash-high", expected: "gemini-3.7-flash"},
		{input: "gemini-2.5-flash-thinking", expected: "gemini-2.5-flash"},
		{input: "claude-3-7-sonnet-search", expected: "claude-3-7-sonnet"},
		{input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			actual := catalog.ExtractBaseModelSlug(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
