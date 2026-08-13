package guardrails

import (
	"context"
	"errors"
	"testing"
)

type fakeSettings struct {
	rows map[string]string
}

func (f *fakeSettings) Get(_ context.Context, key string) (string, error) {
	if f.rows == nil {
		return "", errors.New("no row")
	}
	v, ok := f.rows[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func TestTenantPolicy_DefaultsAllow(t *testing.T) {
	p := NewSettingsTenantPolicy(&fakeSettings{}, 0)
	if !p.AllowExternalEngines(context.Background(), "tenant") {
		t.Error("expected default allow=true when no key is set")
	}
}

func TestTenantPolicy_PerTenantOverrideWins(t *testing.T) {
	p := NewSettingsTenantPolicy(&fakeSettings{rows: map[string]string{
		"guardrails.allow_external_engines":        "true",
		"guardrails.allow_external_engines:tenant": "false",
	}}, 0)
	if p.AllowExternalEngines(context.Background(), "tenant") {
		t.Error("expected per-tenant false to override global true")
	}
}

func TestTenantPolicy_GlobalFlagFalseBlocksAll(t *testing.T) {
	p := NewSettingsTenantPolicy(&fakeSettings{rows: map[string]string{
		"guardrails.allow_external_engines": "false",
	}}, 0)
	if p.AllowExternalEngines(context.Background(), "any-tenant") {
		t.Error("expected global false to take effect")
	}
}

func TestTenantPolicy_NilSettings(t *testing.T) {
	p := NewSettingsTenantPolicy(nil, 0)
	if !p.AllowExternalEngines(context.Background(), "x") {
		t.Error("nil settings should default to allow=true (fail open is safer than blocking detectors)")
	}
}

func TestEngine_AppliesTenantOverrides(t *testing.T) {
	p := NewSettingsTenantPolicy(&fakeSettings{rows: map[string]string{
		"guardrails.allow_external_engines": "false",
	}}, 0)
	engine := NewEngine(EngineConfig{TenantPolicy: p})

	openaiPolicy := Policy{
		PII:      &PIIConfig{Enabled: true, Engine: "presidio"},
		Toxicity: &ToxicityConfig{Enabled: true, Engine: "openai"},
		Topics:   &TopicsConfig{Enabled: true, Engine: "embedding"},
	}
	clamped := engine.applyTenantOverrides(context.Background(), "any", openaiPolicy)
	if clamped.PII.Engine != "native" {
		t.Errorf("expected PII engine native, got %q", clamped.PII.Engine)
	}
	if clamped.Toxicity.Engine != "native" {
		t.Errorf("expected toxicity engine native, got %q", clamped.Toxicity.Engine)
	}
	if clamped.Topics.Engine != "keyword" {
		t.Errorf("expected topics engine keyword, got %q", clamped.Topics.Engine)
	}
}
