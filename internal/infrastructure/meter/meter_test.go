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

	// standardInput = 1600 - 500 - 100 = 1000 tokens * $2/M = 2000 micros
	// cachedInput = 500 tokens * $0.5/M = 250 micros
	// cacheWrite = 100 tokens * $2.5/M = 250 micros
	// reasoning = 50 tokens * $8/M = 400 micros
	// standardOutput = (200 - 50) = 150 tokens * $8/M = 1200 micros
	// total = 2000 + 250 + 250 + 400 + 1200 = 4100 micros
	if cost != 4_100 {
		t.Fatalf("CostMicros() = %d, want 4100", cost)
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

func TestMeter_CostMicros_AntiDoubleBilling(t *testing.T) {
	m := New(nil, nil, map[string]Price{
		"openai/o1": {
			InputPerM:     15,
			OutputPerM:    60,
			ReasoningPerM: 60,
		},
		"anthropic/claude-3-7-sonnet": {
			InputPerM:     3,
			OutputPerM:    15,
			ReasoningPerM: 30, // distinct reasoning rate for clear distinction
		},
	})

	t.Run("OpenAI o1 reasoning deduction", func(t *testing.T) {
		// 1000 prompt, 500 completion total (of which 300 are reasoning tokens)
		// With anti double-billing:
		// input: 1000 * 15 = 15000 micros
		// reasoning: 300 * 60 = 18000 micros
		// standard output: (500 - 300) = 200 * 60 = 12000 micros
		// total: 15000 + 18000 + 12000 = 45000 micros
		// Double-billed total would have been: 15000 + 18000 + (500 * 60) = 63000 micros
		usage := core.Usage{
			PromptTokens:     1_000,
			CompletionTokens: 500,
			ReasoningTokens:  300,
		}
		cost := m.CostMicros("openai", "o1", usage, false)
		if cost != 45_000 {
			t.Fatalf("CostMicros() = %d, want 45000 (anti double-billing)", cost)
		}
	})

	t.Run("Claude 3.7 Thinking with different reasoning rate", func(t *testing.T) {
		// 2000 prompt, 1000 completion (400 reasoning)
		// input: 2000 * 3 = 6000 micros
		// reasoning: 400 * 30 = 12000 micros
		// standard output: (1000 - 400) = 600 * 15 = 9000 micros
		// total: 6000 + 12000 + 9000 = 27000 micros
		// Double-billed total would have been: 6000 + 12000 + (1000 * 15) = 33000 micros
		usage := core.Usage{
			PromptTokens:     2_000,
			CompletionTokens: 1_000,
			ReasoningTokens:  400,
		}
		cost := m.CostMicros("anthropic", "claude-3-7-sonnet", usage, false)
		if cost != 27_000 {
			t.Fatalf("CostMicros() = %d, want 27000, got %d", cost, cost)
		}
	})

	t.Run("Reasoning tokens exceeding completion tokens clamped to zero standard output", func(t *testing.T) {
		// Edge case: malformed usage where ReasoningTokens > CompletionTokens
		usage := core.Usage{
			PromptTokens:     1_000,
			CompletionTokens: 200,
			ReasoningTokens:  300,
		}
		// input: 1000 * 15 = 15000
		// reasoning: 300 * 60 = 18000
		// standard output clamped to 0: 0 * 60 = 0
		// total: 33000
		cost := m.CostMicros("openai", "o1", usage, false)
		if cost != 33_000 {
			t.Fatalf("CostMicros() = %d, want 33000", cost)
		}
	})
}

type testPriceSource struct {
	prices map[string]Price
}

func (s *testPriceSource) GetPrice(provider, model string) (Price, bool) {
	p, ok := s.prices[provider+"/"+model]
	return p, ok
}

func TestMeter_PriceSource_Integration(t *testing.T) {
	m := New(nil,
		map[string]Price{
			"google": {InputPerM: 1, OutputPerM: 2},
		},
		map[string]Price{
			"google/gemini-2.5-pro": {InputPerM: 5, OutputPerM: 20},
		},
	)

	// Before setting PriceSource, uses static modelPrices
	cost := m.CostMicros("google", "gemini-2.5-pro", core.Usage{
		PromptTokens:     1_000_000,
		CompletionTokens: 1_000_000,
	}, false)
	if cost != 25_000_000 {
		t.Fatalf("CostMicros() before PriceSource = %d, want 25000000", cost)
	}

	// Install dynamic PriceSource
	src := &testPriceSource{
		prices: map[string]Price{
			"google/gemini-2.5-pro": {InputPerM: 1.25, OutputPerM: 5.0},
			"google/gemini-2.5-flash": {InputPerM: 0.15, OutputPerM: 0.60},
		},
	}
	m.SetPriceSource(src)

	t.Run("PriceSource overrides modelPrices", func(t *testing.T) {
		cost := m.CostMicros("google", "gemini-2.5-pro", core.Usage{
			PromptTokens:     1_000_000,
			CompletionTokens: 1_000_000,
		}, false)
		// 1M * 1.25 + 1M * 5.0 = 6_250_000 micros
		if cost != 6_250_000 {
			t.Fatalf("CostMicros() with PriceSource = %d, want 6250000", cost)
		}
	})

	t.Run("PriceSource resolves new models", func(t *testing.T) {
		cost := m.CostMicros("google", "gemini-2.5-flash", core.Usage{
			PromptTokens:     1_000_000,
			CompletionTokens: 1_000_000,
		}, false)
		// 1M * 0.15 + 1M * 0.60 = 750_000 micros
		if cost != 750_000 {
			t.Fatalf("CostMicros() with PriceSource for flash = %d, want 750000", cost)
		}
	})

	t.Run("Falls back to provider pricing if model not in PriceSource or modelPrices", func(t *testing.T) {
		cost := m.CostMicros("google", "unlisted-model", core.Usage{
			PromptTokens:     1_000_000,
			CompletionTokens: 1_000_000,
		}, false)
		// Falls back to provider price: 1M * 1 + 1M * 2 = 3_000_000 micros
		if cost != 3_000_000 {
			t.Fatalf("CostMicros() fallback = %d, want 3000000", cost)
		}
	})
}

