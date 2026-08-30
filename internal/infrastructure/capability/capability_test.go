package capability

import (
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
)

// TestOf verifies the capability set projected from a resolved profile across
// the four-step fallback chain.
func TestOf(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected core.CapabilitySet
	}{
		{
			name:  "mimo v2.5 has vision and 1M context",
			model: "mimo-v2.5",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapLongContext,
			),
		},
		{
			name:  "mimo omni adds audio input",
			model: "mimo-v2-omni",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapAudioInput,
				core.CapLongContext,
			),
		},
		{
			name:  "exact id overrides generic pattern",
			model: "glm-4.6v",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapReasoning,
				// 128k context -> not long context
			),
		},
		{
			name:  "generic glm family is text reasoning, long context",
			model: "glm-5",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapReasoning,
				core.CapLongContext,
			),
		},
		{
			name:  "gemini 2.5 is fully multimodal with search",
			model: "gemini-2.5-flash",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapAudioInput,
				core.CapVideoInput,
				core.CapReasoning,
				core.CapWebSearch,
				core.CapLongContext,
			),
		},
		{
			name:  "gpt-5 keeps structured output and web search",
			model: "gpt-5",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapVision,
				core.CapReasoning,
				core.CapWebSearch,
				core.CapStructuredOutput,
				core.CapLongContext,
			),
		},
		{
			name:  "small context model is not long context",
			model: "gpt-3.5-turbo",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
			),
		},
		{
			name:  "image-only model drops tool calling",
			model: "gpt-image-1",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapImageOutput,
				core.CapLongContext,
			),
		},
		{
			name:  "unknown model falls back to the floor",
			model: "totally-unknown-xyz",
			expected: core.NewCapabilitySet(
				core.CapStreaming,
				core.CapToolCalling,
				core.CapLongContext,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Of(tt.model)
			if !equalSets(got, tt.expected) {
				t.Errorf("Of(%q) = %v, want %v", tt.model, got, tt.expected)
			}
		})
	}
}

// TestOfProviderOverride verifies that a provider-scoped override takes
// precedence over exact-id and pattern matches.
func TestOfProviderOverride(t *testing.T) {
	// Without provider context, glm-5v-turbo matches no specific entry and
	// falls through the glm pattern (no vision).
	bare := Of("glm-5v-turbo")
	if bare.Has(core.CapVision) {
		t.Fatalf("expected bare glm-5v-turbo to lack vision, got %v", bare)
	}

	// With the codebuddy-cn provider, the override grants vision.
	scoped := OfProvider("codebuddy-cn", "glm-5v-turbo")
	if !scoped.Has(core.CapVision) {
		t.Errorf("expected codebuddy-cn/glm-5v-turbo to have vision, got %v", scoped)
	}
	if !scoped.Has(core.CapReasoning) {
		t.Errorf("expected codebuddy-cn/glm-5v-turbo to have reasoning, got %v", scoped)
	}
}

// TestResolveProfileChain spot-checks profile fields across each resolution
// step: exact id, provider override, and pattern fallback.
func TestResolveProfileChain(t *testing.T) {
	// Exact id: claude-opus-4.6 carries adaptive thinking and a 1M context.
	if p := ResolveProfile("", "claude-opus-4.6"); p.ThinkingFormat != "claude-adaptive" || p.ContextWindow != 1000000 {
		t.Errorf("claude-opus-4.6 = %+v, want claude-adaptive / 1000000", p)
	}

	// Vendor-prefixed id resolves the same as the bare id.
	if p := ResolveProfile("", "anthropic/claude-opus-4.6"); p.ThinkingFormat != "claude-adaptive" {
		t.Errorf("anthropic/claude-opus-4.6 thinking = %q, want claude-adaptive", p.ThinkingFormat)
	}

	// Provider override: locked thinking on a codebuddy-cn model.
	if p := ResolveProfile("codebuddy-cn", "deepseek-v3-2-volc"); p.ThinkingCanDisable {
		t.Errorf("codebuddy-cn/deepseek-v3-2-volc should not allow disabling thinking")
	}

	// Pattern fallback: a thinking-only model cannot disable thinking.
	if p := ResolveProfile("", "deepseek-r1"); !p.Reasoning || p.ThinkingCanDisable {
		t.Errorf("deepseek-r1 = %+v, want reasoning with locked thinking", p)
	}

	// Floor: an unknown model keeps tools and the default window.
	if p := ResolveProfile("", "totally-unknown-xyz"); !p.Tools || p.ContextWindow != 200000 {
		t.Errorf("unknown model = %+v, want tools / 200000 floor", p)
	}
}

// TestRequired verifies request-shape inference. Structured output and
// reasoning are adapted downstream rather than gated, so they are intentionally
// absent from the required set even when the request carries them.
func TestRequired(t *testing.T) {
	req := &core.ChatRequest{
		Tools:          []core.Tool{{Name: "lookup"}},
		Stream:         true,
		ResponseFormat: []byte(`{"type":"json_schema"}`),
		Reasoning:      &core.ReasoningConfig{Effort: "high"},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartImage},
				{Type: core.PartAudio},
			}},
		},
	}
	got := Required(req)
	want := core.NewCapabilitySet(
		core.CapToolCalling,
		core.CapStreaming,
		core.CapVision,
		core.CapAudioInput,
	)
	if !equalSets(got, want) {
		t.Errorf("Required() = %v, want %v", got, want)
	}
}

// equalSets reports whether two capability sets contain exactly the same
// capabilities.
func equalSets(a, b core.CapabilitySet) bool {
	return a.Satisfies(b) && b.Satisfies(a)
}

type mockCatalogSource struct {
	models map[string]*catalog.ModelSpec
}

func (m *mockCatalogSource) FindModel(provider, modelID string) (*catalog.ModelSpec, bool) {
	if provider != "" {
		if spec, ok := m.models[provider+"/"+modelID]; ok {
			return spec, true
		}
	}
	if spec, ok := m.models[modelID]; ok {
		return spec, true
	}
	return nil, false
}

func TestResolveProfile_WithCatalogEnrichment(t *testing.T) {
	mockSrc := &mockCatalogSource{
		models: map[string]*catalog.ModelSpec{
			"antigravity/gemini-3.7-flash-high": {
				ID:       "gemini-3.7-flash-high",
				Provider: "antigravity",
				Limits: catalog.ModelLimits{
					Context: 1048576,
					Output:  65536,
				},
				Modalities: catalog.ModelModalities{
					Input:  []string{"text", "image", "audio", "video", "pdf"},
					Output: []string{"text"},
				},
				Reasoning: true,
				ToolCall:  true,
			},
			"google/gemini-2.5-flash": {
				ID:       "gemini-2.5-flash",
				Provider: "google",
				Limits: catalog.ModelLimits{
					Context: 1048576,
					Output:  65536,
				},
				Modalities: catalog.ModelModalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Reasoning: true,
				ToolCall:  true,
			},
			"anthropic/claude-opus-4.7": {
				ID:       "claude-opus-4.7",
				Provider: "anthropic",
				Limits: catalog.ModelLimits{
					Context: 1000000,
					Output:  128000,
				},
				Modalities: catalog.ModelModalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Reasoning: true,
				ToolCall:  true,
			},
			"custom/nova-v1": {
				ID:       "nova-v1",
				Provider: "custom",
				Limits: catalog.ModelLimits{
					Context: 500000,
					Output:  32000,
				},
				Modalities: catalog.ModelModalities{
					Input:  []string{"text", "image", "audio", "video", "pdf"},
					Output: []string{"text"},
				},
				Reasoning: true,
				ToolCall:  true,
			},
		},
	}

	SetCatalogSource(mockSrc)
	defer SetCatalogSource(nil)

	t.Run("antigravity gemini-3.7-flash-high preserves gemini-level thinking format and gains catalog specs", func(t *testing.T) {
		p := ResolveProfile("antigravity", "gemini-3.7-flash-high")
		if p.ContextWindow != 1048576 {
			t.Errorf("ContextWindow = %d, want 1048576", p.ContextWindow)
		}
		if !p.Vision {
			t.Errorf("expected Vision to be true")
		}
		if !p.PDF {
			t.Errorf("expected PDF to be true")
		}
		if !p.AudioInput {
			t.Errorf("expected AudioInput to be true")
		}
		if !p.VideoInput {
			t.Errorf("expected VideoInput to be true")
		}
		if !p.Reasoning {
			t.Errorf("expected Reasoning to be true")
		}
		if !p.Tools {
			t.Errorf("expected Tools to be true")
		}
		if p.ThinkingFormat != "gemini-level" {
			t.Errorf("ThinkingFormat = %q, want gemini-level", p.ThinkingFormat)
		}
		if p.ThinkingCanDisable {
			t.Errorf("ThinkingCanDisable = true, want false for gemini-3")
		}
	})

	t.Run("google gemini-2.5-flash preserves gemini-budget thinking format and range", func(t *testing.T) {
		p := ResolveProfile("google", "gemini-2.5-flash")
		if p.ContextWindow != 1048576 {
			t.Errorf("ContextWindow = %d, want 1048576", p.ContextWindow)
		}
		if !p.Vision {
			t.Errorf("expected Vision to be true")
		}
		if !p.PDF {
			t.Errorf("expected PDF to be true")
		}
		if !p.Reasoning {
			t.Errorf("expected Reasoning to be true")
		}
		if p.ThinkingFormat != "gemini-budget" {
			t.Errorf("ThinkingFormat = %q, want gemini-budget", p.ThinkingFormat)
		}
		if p.ThinkingRange == nil || p.ThinkingRange.Max != 24576 {
			t.Errorf("ThinkingRange = %+v, want max 24576", p.ThinkingRange)
		}
	})

	t.Run("claude-opus-4.7 retains adaptive thinking format with enriched catalog modalities", func(t *testing.T) {
		p := ResolveProfile("anthropic", "claude-opus-4.7")
		if p.ContextWindow != 1000000 {
			t.Errorf("ContextWindow = %d, want 1000000", p.ContextWindow)
		}
		if !p.Vision {
			t.Errorf("expected Vision to be true")
		}
		if !p.PDF {
			t.Errorf("expected PDF to be true")
		}
		if !p.Reasoning {
			t.Errorf("expected Reasoning to be true")
		}
		if p.ThinkingFormat != "claude-adaptive" {
			t.Errorf("ThinkingFormat = %q, want claude-adaptive", p.ThinkingFormat)
		}
	})

	t.Run("unknown catalog model resolves dynamic limits and modalities", func(t *testing.T) {
		p := ResolveProfile("custom", "nova-v1")
		if p.ContextWindow != 500000 {
			t.Errorf("ContextWindow = %d, want 500000", p.ContextWindow)
		}
		if p.MaxOutput != 32000 {
			t.Errorf("MaxOutput = %d, want 32000", p.MaxOutput)
		}
		if !p.Vision {
			t.Errorf("expected Vision to be true")
		}
		if !p.PDF {
			t.Errorf("expected PDF to be true")
		}
		if !p.AudioInput {
			t.Errorf("expected AudioInput to be true")
		}
		if !p.VideoInput {
			t.Errorf("expected VideoInput to be true")
		}
		if !p.Reasoning {
			t.Errorf("expected Reasoning to be true")
		}
		if !p.Tools {
			t.Errorf("expected Tools to be true")
		}
		if p.ThinkingFormat != "" {
			t.Errorf("ThinkingFormat = %q, want empty", p.ThinkingFormat)
		}
	})
}

