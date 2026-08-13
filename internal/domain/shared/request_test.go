package shared

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatRequest_HeadersJSONOmitted(t *testing.T) {
	req := ChatRequest{
		Model:  "test-model",
		Stream: true,
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
			"OpenAI-Project":    "proj-123",
		},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Headers must NOT appear in serialized JSON (json:"-" tag).
	assert.NotContains(t, string(data), "anthropic-version")
	assert.NotContains(t, string(data), "OpenAI-Project")
	assert.NotContains(t, string(data), "headers")
}

func TestForwardedHeaders_SafeList(t *testing.T) {
	// Ensure the safe list covers the expected headers.
	expected := []string{
		"anthropic-version",
		"anthropic-beta",
		"OpenAI-Organization",
		"OpenAI-Project",
	}
	assert.Equal(t, expected, ForwardedHeaders)
}
