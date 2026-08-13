package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func (s *Handler) adminExportDatabase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	export := map[string]any{}

	// Optional passphrase enables a portable backup: each sealed credential is
	// re-keyed from the local master key to a passphrase-derived key, so the
	// backup can be restored on a machine with a different master key.
	passphrase := strings.TrimSpace(r.URL.Query().Get("passphrase"))
	portable := passphrase != ""
	export["portable"] = portable

	// Export providers (accounts) — includes encrypted credentials.
	accs, _ := s.accounts.ListByTenant(ctx, adminTenant)
	accountsOut := make([]map[string]any, 0, len(accs))
	for _, a := range accs {
		out := map[string]any{
			"id": a.ID, "provider": a.Provider, "label": a.Label,
			"auth_kind": a.AuthKind, "priority": a.Priority,
			"disabled": a.Disabled, "proxy_pool_id": a.ProxyPoolID,
			"metadata": a.Metadata,
		}
		if portable {
			if err := s.exportPortableSecrets(out, a, passphrase); err != nil {
				s.consoleLog.Log("ERROR", fmt.Sprintf("Portable export failed for account %s", a.ID), err.Error())
				WriteError(w, http.StatusInternalServerError, "portable export failed: cannot re-key account "+a.ID+" (master key mismatch?)")
				return
			}
		} else {
			if a.SecretWrappedDEK != "" {
				out["secret_wrapped_dek"] = a.SecretWrappedDEK
				out["secret_ciphertext"] = a.SecretCiphertext
			}
			if a.TokenWrappedDEK != "" {
				out["token_wrapped_dek"] = a.TokenWrappedDEK
				out["token_ciphertext"] = a.TokenCiphertext
			}
			if a.RefreshWrappedDEK != "" {
				out["refresh_wrapped_dek"] = a.RefreshWrappedDEK
				out["refresh_ciphertext"] = a.RefreshCiphertext
			}
		}
		if a.TokenExpiresAt != nil {
			out["token_expires_at"] = a.TokenExpiresAt
		}
		accountsOut = append(accountsOut, out)
	}
	export["accounts"] = accountsOut

	// Export chains.
	chains, _ := s.chains.ListByTenant(ctx, adminTenant)
	chainsOut := make([]map[string]any, 0, len(chains))
	for _, c := range chains {
		steps := make([]map[string]any, 0, len(c.Steps))
		for _, st := range c.Steps {
			steps = append(steps, map[string]any{
				"provider": st.Provider, "model": st.Model, "position": st.Position,
			})
		}
		chainsOut = append(chainsOut, map[string]any{
			"name": c.Name, "strategy": c.Strategy, "steps": steps,
		})
	}
	export["chains"] = chainsOut

	// Export API keys (names only, not hashes).
	keys, _ := s.identity.List(ctx, adminTenant)
	keysOut := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		keysOut = append(keysOut, map[string]any{
			"name": k.Name, "disabled": k.Disabled,
		})
	}
	export["keys"] = keysOut

	// Export budgets.
	budgets, _ := s.budgets.ListByTenant(ctx, adminTenant)
	budgetsOut := make([]map[string]any, 0, len(budgets))
	for _, b := range budgets {
		budgetsOut = append(budgetsOut, map[string]any{
			"scope_kind": b.ScopeKind, "scope_id": b.ScopeID,
			"limit_micros": b.LimitMicros, "period": b.Period,
			"alert_pct": b.AlertPct, "hard_cutoff": b.HardCutoff,
		})
	}
	export["budgets"] = budgetsOut

	// Export proxy pools.
	pools, _ := s.pools.List(ctx)
	poolsOut := make([]map[string]any, 0, len(pools))
	for _, p := range pools {
		poolsOut = append(poolsOut, map[string]any{
			"name": p.Name, "proxy_url": p.ProxyURL, "no_proxy": p.NoProxy,
			"strict": p.Strict, "is_active": p.IsActive,
		})
	}
	export["proxy_pools"] = poolsOut

	// Export settings.
	export["endpoint_settings"] = s.loadEndpointSettings(ctx)
	export["access_settings"] = s.loadAccessSettings(ctx)

	// Export aliases.
	aliases, _ := s.aliases.List(ctx)
	aliasMap := map[string]string{}
	for _, a := range aliases {
		aliasMap[a.Alias] = a.Target
	}
	export["aliases"] = aliasMap

	writeJSON(w, http.StatusOK, export)
}

func (s *Handler) adminImportDatabase(w http.ResponseWriter, r *http.Request) {
	var payload map[string]json.RawMessage
	if !decodeJSON(w, r, &payload) {
		return
	}
	ctx := r.Context()
	imported := 0

	// A portable backup carries passphrase-encrypted secrets; the passphrase is
	// supplied alongside the payload so we can re-key into the local master key.
	portable := false
	if raw, ok := payload["portable"]; ok {
		_ = json.Unmarshal(raw, &portable)
	}
	passphrase := ""
	if raw, ok := payload["passphrase"]; ok {
		_ = json.Unmarshal(raw, &passphrase)
	}
	passphrase = strings.TrimSpace(passphrase)
	if portable && passphrase == "" {
		WriteError(w, http.StatusBadRequest, "this backup is portable: a passphrase is required to import it")
		return
	}

	// Import providers (accounts) — preserves encrypted credentials.
	if raw, ok := payload["accounts"]; ok {
		var accounts []struct {
			ID                string                `json:"id"`
			Provider          string                `json:"provider"`
			Label             string                `json:"label"`
			AuthKind          string                `json:"auth_kind"`
			Priority          int                   `json:"priority"`
			Disabled          bool                  `json:"disabled"`
			ProxyPoolID       string                `json:"proxy_pool_id"`
			Metadata          string                `json:"metadata"`
			SecretWrappedDEK  string                `json:"secret_wrapped_dek"`
			SecretCiphertext  string                `json:"secret_ciphertext"`
			TokenWrappedDEK   string                `json:"token_wrapped_dek"`
			TokenCiphertext   string                `json:"token_ciphertext"`
			RefreshWrappedDEK string                `json:"refresh_wrapped_dek"`
			RefreshCiphertext string                `json:"refresh_ciphertext"`
			PortableSecret    portableAccountSecret `json:"portable_secret"`
			TokenExpiresAt    *string               `json:"token_expires_at"`
		}
		if err := json.Unmarshal(raw, &accounts); err == nil {
			for _, a := range accounts {
				now := time.Now()
				var expiresAt *time.Time
				if a.TokenExpiresAt != nil {
					if t, err := time.Parse(time.RFC3339, *a.TokenExpiresAt); err == nil {
						expiresAt = &t
					}
				}
				acc := schema.Account{
					ID:                defaultStr(a.ID, uuid.NewString()),
					TenantID:          adminTenant,
					Provider:          a.Provider,
					Label:             a.Label,
					AuthKind:          defaultStr(a.AuthKind, "api_key"),
					SecretWrappedDEK:  a.SecretWrappedDEK,
					SecretCiphertext:  a.SecretCiphertext,
					TokenWrappedDEK:   a.TokenWrappedDEK,
					TokenCiphertext:   a.TokenCiphertext,
					RefreshWrappedDEK: a.RefreshWrappedDEK,
					RefreshCiphertext: a.RefreshCiphertext,
					TokenExpiresAt:    expiresAt,
					Metadata:          a.Metadata,
					Priority:          defaultInt(a.Priority, 100),
					Disabled:          a.Disabled,
					ProxyPoolID:       a.ProxyPoolID,
					CreatedAt:         now,
					UpdatedAt:         now,
				}
				if portable {
					if err := s.importPortableSecrets(&acc, a.PortableSecret, passphrase); err != nil {
						s.consoleLog.Log("ERROR", fmt.Sprintf("Portable import failed for account %s", acc.ID), err.Error())
						WriteError(w, http.StatusBadRequest, "portable import failed: wrong passphrase or corrupt backup")
						return
					}
				}
				if err := s.accounts.Create(ctx, acc); err == nil {
					imported++
				}
			}
		}
	}

	// Import chains.
	if raw, ok := payload["chains"]; ok {
		var chains []struct {
			Name     string `json:"name"`
			Strategy string `json:"strategy"`
			Steps    []struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
				Position int    `json:"position"`
			} `json:"steps"`
		}
		if err := json.Unmarshal(raw, &chains); err == nil {
			for _, c := range chains {
				now := time.Now()
				chain := schema.Chain{
					ID:        uuid.NewString(),
					TenantID:  adminTenant,
					Name:      c.Name,
					Strategy:  defaultStr(c.Strategy, "priority"),
					CreatedAt: now,
					UpdatedAt: now,
				}
				for _, st := range c.Steps {
					chain.Steps = append(chain.Steps, schema.ChainStep{
						ID: uuid.NewString(), ChainID: chain.ID, Position: st.Position,
						Provider: st.Provider, Model: st.Model, CreatedAt: now,
					})
				}
				if err := s.chains.Create(ctx, chain); err == nil {
					imported++
				}
			}
		}
	}

	// Import budgets.
	if raw, ok := payload["budgets"]; ok {
		var budgets []struct {
			ScopeKind   string `json:"scope_kind"`
			ScopeID     string `json:"scope_id"`
			LimitMicros int64  `json:"limit_micros"`
			Period      string `json:"period"`
			AlertPct    int    `json:"alert_pct"`
			HardCutoff  bool   `json:"hard_cutoff"`
		}
		if err := json.Unmarshal(raw, &budgets); err == nil {
			for _, b := range budgets {
				now := time.Now()
				budget := schema.Budget{
					ID:          uuid.NewString(),
					TenantID:    adminTenant,
					ScopeKind:   defaultStr(b.ScopeKind, string(schema.ScopeTenant)),
					ScopeID:     defaultStr(b.ScopeID, adminTenant),
					LimitMicros: b.LimitMicros,
					Period:      defaultStr(b.Period, "monthly"),
					AlertPct:    defaultInt(b.AlertPct, 80),
					HardCutoff:  b.HardCutoff,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := s.budgets.Create(ctx, budget); err == nil {
					imported++
				}
			}
		}
		if s.budgetEngine != nil {
			s.budgetEngine.InvalidateBudgetCache()
		}
	}

	// Import proxy pools.
	if raw, ok := payload["proxy_pools"]; ok {
		var pools []struct {
			Name     string `json:"name"`
			ProxyURL string `json:"proxy_url"`
			NoProxy  string `json:"no_proxy"`
			Strict   bool   `json:"strict"`
			IsActive bool   `json:"is_active"`
		}
		if err := json.Unmarshal(raw, &pools); err == nil {
			for _, p := range pools {
				now := time.Now()
				pool := schema.ProxyPool{
					ID:         uuid.NewString(),
					Name:       p.Name,
					Type:       "http",
					ProxyURL:   p.ProxyURL,
					NoProxy:    p.NoProxy,
					Strict:     p.Strict,
					IsActive:   defaultBool(p.IsActive, true),
					TestStatus: "unknown",
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				if err := s.pools.Create(ctx, pool); err == nil {
					imported++
				}
			}
		}
	}

	// Import endpoint settings.
	if raw, ok := payload["endpoint_settings"]; ok {
		if err := s.settings.Set(ctx, endpointSettingsKey, string(raw)); err == nil {
			imported++
		}
	}

	// Import aliases.
	if raw, ok := payload["aliases"]; ok {
		var aliases map[string]string
		if err := json.Unmarshal(raw, &aliases); err == nil {
			for alias, target := range aliases {
				_ = s.aliases.Set(ctx, alias, target)
				imported++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"imported": imported})
}

// ---- proxy test -------------------------------------------------------------
