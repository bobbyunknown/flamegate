package connectors

import (
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestCatalogHasCoreProviders(t *testing.T) {
	want := []string{"custom-openai", "custom-anthropic", "custom-gemini"}
	for _, id := range want {
		if _, ok := SpecByID(id); !ok {
			t.Errorf("catalog missing provider %q", id)
		}
	}
	for _, id := range []string{"openai", "anthropic", "gemini", "groq", "openrouter", "tavily", "elevenlabs"} {
		if _, ok := SpecByID(id); ok {
			t.Errorf("catalog should not include purged provider %q", id)
		}
	}
}

func TestIsNativeSlug(t *testing.T) {
	for _, id := range []string{"custom-openai", "custom-anthropic", "custom-gemini"} {
		if !IsNativeSlug(id) {
			t.Errorf("IsNativeSlug(%q) = false, want true", id)
		}
	}
	if IsNativeSlug("openai") {
		t.Error("IsNativeSlug(openai) = true, want false")
	}
	if IsNativeSlug("xiaomi-mimo") {
		t.Error("IsNativeSlug(xiaomi-mimo) = true, want false")
	}
	if IsNativeSlug("openrouter") {
		t.Error("IsNativeSlug(openrouter) = true, want false")
	}
}

func TestSpecsByKindLLM(t *testing.T) {
	specs := SpecsByKind(core.ServiceLLM)
	got := map[string]bool{}
	for _, s := range specs {
		got[s.ID] = true
	}
	for _, id := range []string{"custom-openai", "custom-anthropic", "custom-gemini"} {
		if !got[id] {
			t.Errorf("kind llm: expected provider %q", id)
		}
	}
}

func TestModelsByKind(t *testing.T) {
	SetDynamicModels("custom-openai", []ModelSpec{m("my-model", "My Model")})
	defer SetDynamicModels("custom-openai", nil)

	llms := ModelsByKind(core.ServiceLLM)
	if len(llms) == 0 {
		t.Fatal("expected at least one LLM model in catalog")
	}
	for _, pm := range llms {
		if pm.Model.Kind != core.ServiceLLM {
			t.Errorf("ModelsByKind(llm) returned non-llm model %q (%q)", pm.Model.ID, pm.Model.Kind)
		}
	}
}

func TestFindModel(t *testing.T) {
	SetDynamicModels("custom-openai", []ModelSpec{m("gpt-4o", "GPT-4o")})
	defer SetDynamicModels("custom-openai", nil)

	if _, ok := FindModel("custom-openai", "gpt-4o"); !ok {
		t.Error("expected to find custom-openai/gpt-4o")
	}
	if _, ok := FindModel("custom-openai", "nonexistent-model"); ok {
		t.Error("expected miss for nonexistent model")
	}
}

func TestDrivableDialect(t *testing.T) {
	for _, d := range []core.Dialect{
		core.DialectOpenAI, core.DialectAnthropic, core.DialectGemini,
	} {
		if !DrivableDialect(d) {
			t.Errorf("dialect %q must be drivable", d)
		}
	}
}

func TestRegistryRegistersDrivableProviders(t *testing.T) {
	r := DefaultRegistry()
	for _, provider := range []string{"custom-openai", "custom-anthropic", "custom-gemini"} {
		if !r.Has(provider) {
			t.Errorf("registry should have connector for %q", provider)
		}
	}
	for _, provider := range []string{"openai", "anthropic", "gemini", "openrouter", "xiaomi-mimo", "does-not-exist"} {
		if r.Has(provider) {
			t.Errorf("registry should not have connector for %q", provider)
		}
	}
}

func TestExtensionSpecsInCatalog(t *testing.T) {
	RegisterExtensionSpec(ProviderSpec{ID: "xiaomi-mimo", DisplayName: "Xiaomi MiMo"})
	t.Cleanup(func() { UnregisterExtensionSpec("xiaomi-mimo") })

	spec, ok := SpecByID("xiaomi-mimo")
	if !ok {
		t.Fatal("expected SpecByID(xiaomi-mimo) after RegisterExtensionSpec")
	}
	if spec.Notice != "WASM extension" {
		t.Errorf("Notice = %q, want WASM extension", spec.Notice)
	}
	if IsNativeSlug("xiaomi-mimo") {
		t.Error("extension must not be native")
	}
}
