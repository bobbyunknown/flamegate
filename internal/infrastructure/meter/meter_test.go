package meter

import (
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestCostMicrosUsesExactModelPriceBeforeProviderFallback(t *testing.T) {
	m := New(nil,
		map[string]Price{
			"anthropic": {InputPerM: 1, OutputPerM: 2},
		},
		map[string]Price{
			"anthropic/claude-opus-4-7": {InputPerM: 15, OutputPerM: 75},
		},
	)

	cost := m.CostMicros("anthropic", "claude-opus-4-7", core.Usage{
		PromptTokens:     1_000_000,
		CompletionTokens: 1_000_000,
	}, false)

	if cost != 90_000_000 {
		t.Fatalf("CostMicros() = %d, want 90000000", cost)
	}
}

func TestCostMicrosFallsBackToProviderPrice(t *testing.T) {
	m := New(nil,
		map[string]Price{
			"anthropic": {InputPerM: 1, OutputPerM: 2},
		},
		nil,
	)

	cost := m.CostMicros("anthropic", "unknown-model", core.Usage{
		PromptTokens:     1_000_000,
		CompletionTokens: 1_000_000,
	}, false)

	if cost != 3_000_000 {
		t.Fatalf("CostMicros() = %d, want 3000000", cost)
	}
}

func TestCostMicrosAppliesCacheAndReasoningRates(t *testing.T) {
	m := New(nil, nil, map[string]Price{
		"openai/o4-mini": {
			InputPerM:       2,
			OutputPerM:      8,
			CachedInputPerM: 0.5,
			CacheWritePerM:  2.5,
			ReasoningPerM:   8,
		},
	})

	cost := m.CostMicros("openai", "o4-mini", core.Usage{
		PromptTokens:     1_600,
		CompletionTokens: 200,
		CachedTokens:     500,
		CacheWriteTokens: 100,
		ReasoningTokens:  50,
	}, false)

	if cost != 4_500 {
		t.Fatalf("CostMicros() = %d, want 4500", cost)
	}
}

func TestCostMicrosCacheHitIsFree(t *testing.T) {
	m := New(nil, map[string]Price{
		"openai": {InputPerM: 100, OutputPerM: 100},
	}, nil)

	cost := m.CostMicros("openai", "gpt-5", core.Usage{
		PromptTokens:     1_000_000,
		CompletionTokens: 1_000_000,
	}, true)

	if cost != 0 {
		t.Fatalf("CostMicros() = %d, want 0", cost)
	}
}
