package admin

import "github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"

// guardrailTemplate is one starter policy surfaced in the dashboard's "From
// template" picker. The config is a full guardrails.Policy so the frontend
// can drop it into the editor without further massaging.
type guardrailTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Config      guardrails.Policy `json:"config"`
}

// guardrailTemplates returns the curated catalog. Keep this list short — every
// template adds cognitive load to the picker — and prefer templates that map
// to clearly distinct use cases.
func guardrailTemplates() []guardrailTemplate {
	enabled := true

	piiID := &guardrails.PIIConfig{
		Enabled:  true,
		Types:    []string{"EMAIL_ADDRESS", "PHONE_NUMBER", "ID_NIK", "ID_NPWP", "ID_PASSPORT", "CREDIT_CARD"},
		Strategy: guardrails.PIIStrategyReplace,
		MinScore: 0.7,
	}
	piiRedact := &guardrails.PIIConfig{
		Enabled:    true,
		Strategy:   guardrails.PIIStrategyRedact,
		MinScore:   0.6,
		ScanOutput: true,
	}
	piiBlock := &guardrails.PIIConfig{
		Enabled:  true,
		Strategy: guardrails.PIIStrategyBlock,
		MinScore: 0.5,
	}
	injectionBlock := &guardrails.InjectionConfig{
		Enabled:           true,
		SeverityThreshold: guardrails.SeverityMedium,
		Action:            guardrails.ActionBlock,
	}
	injectionWarn := &guardrails.InjectionConfig{
		Enabled:           true,
		SeverityThreshold: guardrails.SeverityLow,
		Action:            guardrails.ActionLogOnly,
	}
	topicsProgrammingAllow := &guardrails.TopicsConfig{
		Enabled: true,
		Mode:    "allow",
		Topics:  []string{"programming", "software", "devops", "general help"},
		Action:  guardrails.ActionBlock,
	}
	toxicityWarn := &guardrails.ToxicityConfig{
		Enabled:   true,
		Threshold: 60,
		Action:    guardrails.ActionLogOnly,
	}

	return []guardrailTemplate{
		{
			ID:          "indonesia-pii",
			Name:        "Indonesia PII",
			Description: "Mask Indonesian identifiers (NIK, NPWP, passport) plus generic PII (email, phone, card) using replace strategy at min_score 0.7.",
			Config: guardrails.Policy{
				Enabled: &enabled,
				PII:     piiID,
			},
		},
		{
			ID:          "strict-safety",
			Name:        "Strict safety",
			Description: "Block on any PII match, block prompt injection at medium severity, and allow only programming/general-help topics.",
			Config: guardrails.Policy{
				Enabled:   &enabled,
				PII:       piiBlock,
				Injection: injectionBlock,
				Topics:    topicsProgrammingAllow,
			},
		},
		{
			ID:          "compliance-audit",
			Name:        "Compliance audit (log-only)",
			Description: "Every detector enabled at action=log_only — useful as a dry run before tightening to warn/block.",
			Config: guardrails.Policy{
				Enabled: &enabled,
				PII:     piiRedact,
				Injection: &guardrails.InjectionConfig{
					Enabled: true, SeverityThreshold: guardrails.SeverityLow, Action: guardrails.ActionLogOnly,
				},
				Toxicity: toxicityWarn,
				Bias: &guardrails.BiasConfig{
					Enabled: true, Threshold: 50, Action: guardrails.ActionLogOnly,
				},
			},
		},
		{
			ID:          "public-chatbot",
			Name:        "Public chatbot",
			Description: "Redact PII (both directions), block prompt injection, scope topics to programming/general help.",
			Config: guardrails.Policy{
				Enabled:   &enabled,
				PII:       piiRedact,
				Injection: injectionBlock,
				Topics:    topicsProgrammingAllow,
				Toxicity: &guardrails.ToxicityConfig{
					Enabled: true, Threshold: 70, Action: guardrails.ActionBlock,
				},
			},
		},
		{
			ID:          "compliance-passive",
			Name:        "Compliance — alerts only",
			Description: "Lightweight log_only profile for environments that want visibility without breaking traffic.",
			Config: guardrails.Policy{
				Enabled:   &enabled,
				PII:       piiRedact,
				Injection: injectionWarn,
			},
		},
	}
}
