package connectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTier1Provider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{"custom-openai", true},
		{"custom-anthropic", true},
		{"custom-gemini", true},
		{"Custom-OpenAI", true}, // case insensitive
		{"openai", false},
		{"mistral", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := IsTier1Provider(tt.provider)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTier1Slugs(t *testing.T) {
	want := []string{"custom-openai", "custom-anthropic", "custom-gemini"}
	assert.Equal(t, want, tier1Slugs)
	assert.Len(t, tier1Slugs, 3)
}
