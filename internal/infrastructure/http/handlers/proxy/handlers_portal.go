package proxy

import (
	"net/http"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func (s *Handler) HandleKeyUsage(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	ctx := r.Context()

	// Get budgets scoped to this key.
	budgets, err := s.budgets.ListByScope(ctx, schema.ScopeAPIKey, key.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list budgets")
		return
	}

	type budgetOut struct {
		Period        string  `json:"period"`
		LimitTokens   int64   `json:"limit_tokens"`
		TokensUsed    int64   `json:"tokens_used"`
		TokensRemain  int64   `json:"tokens_remaining"`
		TokensPctUsed float64 `json:"tokens_pct_used"`
		LimitUSD      float64 `json:"limit_usd"`
		SpentUSD      float64 `json:"spent_usd"`
		USDRemaining  float64 `json:"usd_remaining"`
		USDUsed       float64 `json:"usd_pct_used"`
		Alert         bool    `json:"alert"`
	}

	var budgetOuts []budgetOut
	for _, b := range budgets {
		since := budget.PeriodStart(b.Period, time.Now())
		costMicros, tokens, err := s.usage.SpendAndTokens(ctx, schema.BudgetScope(b.ScopeKind), b.ScopeID, since)
		if err != nil {
			s.log.Error("key usage: spend lookup failed", "err", err)
			continue
		}

		bo := budgetOut{
			Period:      b.Period,
			LimitTokens: b.LimitTokens,
			TokensUsed:  tokens,
			LimitUSD:    float64(b.LimitMicros) / 1_000_000,
			SpentUSD:    float64(costMicros) / 1_000_000,
		}
		if b.LimitTokens > 0 {
			bo.TokensRemain = b.LimitTokens - tokens
			if bo.TokensRemain < 0 {
				bo.TokensRemain = 0
			}
			bo.TokensPctUsed = float64(tokens) / float64(b.LimitTokens) * 100
		}
		if b.LimitMicros > 0 {
			bo.USDRemaining = bo.LimitUSD - bo.SpentUSD
			if bo.USDRemaining < 0 {
				bo.USDRemaining = 0
			}
			bo.USDUsed = float64(costMicros) / float64(b.LimitMicros) * 100
		}
		// Alert if either threshold crossed.
		if b.AlertPct > 0 {
			if (b.LimitMicros > 0 && costMicros*100 >= b.LimitMicros*int64(b.AlertPct)) ||
				(b.LimitTokens > 0 && tokens*100 >= b.LimitTokens*int64(b.AlertPct)) {
				bo.Alert = true
			}
		}
		budgetOuts = append(budgetOuts, bo)
	}

	// Get allowed models for this key.
	allowedModels, err := s.identity.Keys().GetAllowedModels(ctx, key.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get model access")
		return
	}

	// Get current period summary scoped to this specific key.
	now := time.Now()
	summary, err := s.usage.SummarizeByKey(ctx, key.ID, time.Time{})
	if err != nil {
		s.log.Error("key usage: summarize failed", "err", err)
	}

	daily, _ := s.usage.DailyByKey(ctx, key.ID, now.AddDate(0, 0, -30))
	var dailyOut []map[string]any
	for _, d := range daily {
		dailyOut = append(dailyOut, map[string]any{
			"date": d.Date, "requests": d.Requests,
			"prompt_tokens": d.PromptTokens, "completion_tokens": d.CompletionTokens,
			"cost_usd": float64(d.CostMicros) / 1_000_000,
		})
	}

	models, _ := s.usage.ByModelByKey(ctx, key.ID, now.AddDate(0, 0, -30))
	var modelOut []map[string]any
	for _, m := range models {
		modelOut = append(modelOut, map[string]any{
			"provider": m.Provider, "model": m.Model,
			"total_requests": m.TotalRequests,
			"prompt_tokens":  m.PromptTokens, "completion_tokens": m.CompletionTokens,
			"cost_usd": float64(m.CostMicros) / 1_000_000,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"key_id":         key.ID,
		"key_name":       key.Name,
		"budgets":        budgetOuts,
		"allowed_models": allowedModels,
		"current_period": map[string]any{
			"prompt_tokens":     summary.PromptTokens,
			"completion_tokens": summary.CompletionTokens,
			"total_requests":    summary.TotalRequests,
			"cost_usd":          float64(summary.CostMicros) / 1_000_000,
		},
		"daily":  dailyOut,
		"models": modelOut,
	})
}
