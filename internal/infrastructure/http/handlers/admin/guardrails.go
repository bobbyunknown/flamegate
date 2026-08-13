package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/pii"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// mountGuardrails registers the guardrail admin endpoints under /api/guardrails.
// All endpoints are loopback + session protected by the parent router; no extra
// middleware is needed here.
func (s *Handler) MountGuardrails(r chi.Router) {
	if s.guardrailRepo == nil {
		// Guardrails not wired (older app build). Endpoints respond 501 so the
		// frontend degrades gracefully instead of seeing 404 + retry storms.
		r.HandleFunc("/guardrails*", func(w http.ResponseWriter, _ *http.Request) {
			WriteError(w, http.StatusNotImplemented, "guardrails not wired")
		})
		return
	}
	r.Get("/guardrails", s.adminListGuardrails)
	r.Post("/guardrails", s.adminCreateGuardrail)
	r.Get("/guardrails/effective", s.adminEffectiveGuardrail)
	r.Get("/guardrails/entities", s.adminListGuardrailEntities)
	r.Get("/guardrails/templates", s.adminListGuardrailTemplates)
	r.Get("/guardrails/export", s.adminExportGuardrails)
	r.Post("/guardrails/import", s.adminImportGuardrails)
	r.Get("/guardrails/tenant-flags", s.adminGetGuardrailTenantFlags)
	r.Put("/guardrails/tenant-flags", s.adminPutGuardrailTenantFlags)
	r.Get("/guardrails/logs", s.adminListGuardrailLogs)
	r.Get("/guardrails/logs/stream", s.adminGuardrailLogStream)
	r.Post("/guardrails/test", s.adminTestGuardrail)
	r.Get("/guardrails/{id}", s.adminGetGuardrail)
	r.Patch("/guardrails/{id}", s.adminUpdateGuardrail)
	r.Delete("/guardrails/{id}", s.adminDeleteGuardrail)
}

// guardrailDTO is the JSON shape exchanged with the dashboard.
type guardrailDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Scope     string            `json:"scope"`
	ScopeID   string            `json:"scope_id"`
	Enabled   bool              `json:"enabled"`
	Config    guardrails.Policy `json:"config"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

func toDTO(p schema.GuardrailPolicy) guardrailDTO {
	cfg, _ := guardrails.UnmarshalPolicy(p.Config)
	return guardrailDTO{
		ID:        p.ID,
		Name:      p.Name,
		Scope:     string(p.Scope),
		ScopeID:   p.ScopeID,
		Enabled:   p.Enabled,
		Config:    cfg,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Handler) adminListGuardrails(w http.ResponseWriter, r *http.Request) {
	scope := schema.GuardrailScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	rows, err := s.guardrailRepo.List(r.Context(), schema.DefaultTenantID, string(scope))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "list guardrails: "+err.Error())
		return
	}
	out := make([]guardrailDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, toDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"guardrails": out})
}

func (s *Handler) adminGetGuardrail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.guardrailRepo.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "guardrail not found")
		return
	}
	writeJSON(w, http.StatusOK, toDTO(p))
}

type guardrailWriteInput struct {
	Name    string             `json:"name"`
	Scope   string             `json:"scope"`
	ScopeID string             `json:"scope_id"`
	Enabled *bool              `json:"enabled,omitempty"`
	Config  *guardrails.Policy `json:"config,omitempty"`
}

func (s *Handler) adminCreateGuardrail(w http.ResponseWriter, r *http.Request) {
	var in guardrailWriteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	scope := schema.GuardrailScope(strings.TrimSpace(in.Scope))
	if !isValidScope(scope) {
		WriteError(w, http.StatusBadRequest, "invalid scope: "+in.Scope)
		return
	}
	scopeID := strings.TrimSpace(in.ScopeID)
	if scope == schema.GuardrailScopeGlobal {
		scopeID = ""
	} else if scopeID == "" {
		WriteError(w, http.StatusBadRequest, "scope_id required for non-global scope")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		// Auto-name policies users don't bother naming.
		in.Name = defaultPolicyName(scope, scopeID)
	}

	cfg := guardrails.Policy{}
	if in.Config != nil {
		cfg = *in.Config
	}
	cfgJSON, err := guardrails.MarshalPolicy(cfg)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid config: "+err.Error())
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	now := time.Now().UTC()
	p := schema.GuardrailPolicy{
		ID:        newGuardrailID(),
		TenantID:  schema.DefaultTenantID,
		Scope:     string(scope),
		ScopeID:   scopeID,
		Name:      in.Name,
		Enabled:   enabled,
		Config:    cfgJSON,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.guardrailRepo.Upsert(r.Context(), p); err != nil {
		WriteError(w, http.StatusInternalServerError, "save guardrail: "+err.Error())
		return
	}
	s.invalidateGuardrail(p.TenantID, schema.GuardrailScope(p.Scope), p.ScopeID)

	// Upsert may have hit an existing row; re-fetch by scope to return the canonical row.
	final, err := s.guardrailRepo.GetByScope(r.Context(), schema.DefaultTenantID, scope, scopeID)
	if err != nil {
		final = p
	}
	writeJSON(w, http.StatusOK, toDTO(final))
}

func (s *Handler) adminUpdateGuardrail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.guardrailRepo.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "guardrail not found")
		return
	}
	var in guardrailWriteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Name) != "" {
		existing.Name = in.Name
	}
	if in.Enabled != nil {
		existing.Enabled = *in.Enabled
	}
	if in.Config != nil {
		cfgJSON, err := guardrails.MarshalPolicy(*in.Config)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid config: "+err.Error())
			return
		}
		existing.Config = cfgJSON
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.guardrailRepo.Upsert(r.Context(), existing); err != nil {
		WriteError(w, http.StatusInternalServerError, "save guardrail: "+err.Error())
		return
	}
	s.invalidateGuardrail(existing.TenantID, schema.GuardrailScope(existing.Scope), existing.ScopeID)
	writeJSON(w, http.StatusOK, toDTO(existing))
}

func (s *Handler) adminDeleteGuardrail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.guardrailRepo.Get(r.Context(), id)
	if err == nil {
		s.invalidateGuardrail(existing.TenantID, schema.GuardrailScope(existing.Scope), existing.ScopeID)
	}
	if err := s.guardrailRepo.Delete(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "delete guardrail: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminEffectiveGuardrail returns the merged effective policy for the given
// scope dimensions. Used by the dashboard's "what will actually apply" preview.
func (s *Handler) adminEffectiveGuardrail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	key := guardrails.Key{
		TenantID: schema.DefaultTenantID,
		Provider: q.Get("provider"),
		Model:    q.Get("model"),
		ChainID:  q.Get("chain"),
		APIKeyID: q.Get("apikey"),
	}
	policy := s.guardrails.EffectivePolicy(r.Context(), key)
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":  key,
		"policy": policy,
	})
}

// adminListGuardrailEntities returns the PII entity catalog so the dashboard
// can render its multi-select without hardcoding the list.
func (s *Handler) adminListGuardrailEntities(w http.ResponseWriter, _ *http.Request) {
	entities := pii.AllEntities()
	out := make([]string, len(entities))
	for i, e := range entities {
		out[i] = string(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entities": out})
}

func (s *Handler) adminListGuardrailLogs(w http.ResponseWriter, r *http.Request) {
	if s.guardrailLogs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"logs": []any{}})
		return
	}
	q := r.URL.Query()
	f := schema.GuardrailLogFilter{ // nolint:exhaustruct // optional fields
		APIKeyID: q.Get("api_key_id"),
		Detector: q.Get("detector"),
		Action:   q.Get("action"),
	}
	if v := q.Get("limit"); v != "" {
		// best-effort parse; the repo clamps to a sane upper bound
		var n int
		_, _ = parseIntInto(v, &n)
		f.Limit = n
	}
	rows, err := s.guardrailLogs.List(r.Context(), schema.DefaultTenantID, f)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "list guardrail logs: "+err.Error())
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, e := range rows {
		var findings any
		_ = json.Unmarshal([]byte(e.Findings), &findings)
		out = append(out, map[string]any{
			"id":         e.ID,
			"request_id": e.RequestID,
			"api_key_id": e.APIKeyID,
			"provider":   e.Provider,
			"model":      e.Model,
			"chain_id":   e.ChainID,
			"detector":   e.Detector,
			"direction":  e.Direction,
			"action":     e.Action,
			"severity":   e.Severity,
			"reason":     e.Reason,
			"findings":   findings,
			"created_at": e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": out})
}

// adminTestGuardrail dry-runs the engine against caller-supplied text and
// returns the decisions without touching any provider. Used by the dashboard's
// test panel for tuning policies.
type testGuardrailInput struct {
	Text   string             `json:"text"`
	Config *guardrails.Policy `json:"config,omitempty"`
}

func (s *Handler) adminTestGuardrail(w http.ResponseWriter, r *http.Request) {
	// Rate-limit the test endpoint per-session. Without a cap an attacker
	// with a stolen dashboard cookie could use this to enumerate regex
	// behavior or trigger ReDoS via crafted text. 10 requests/minute is
	// generous for interactive tuning but tight enough to throttle abuse.
	if s.guardrailTestRL != nil {
		if !s.guardrailTestRL.allow(rateLimitKeyFor(r)) {
			WriteError(w, http.StatusTooManyRequests, "guardrails test rate-limit exceeded; try again in a minute")
			return
		}
	}
	var in testGuardrailInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if in.Text == "" {
		WriteError(w, http.StatusBadRequest, "text is required")
		return
	}

	// Synthetic request. Metadata is tagged with a test-* request id and a
	// "test-panel" api key id so audit log rows from the dashboard's test
	// panel are distinguishable from real traffic. The engine's logDecision
	// uses these fields verbatim.
	req := &core.ChatRequest{
		Metadata: core.RequestMetadata{
			TenantID:  schema.DefaultTenantID,
			APIKeyID:  "test-panel",
			RequestID: "test-" + newGuardrailID()[3:11],
		},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: in.Text}}},
		},
	}
	// If the caller supplied an inline policy, run a one-off engine to avoid
	// touching the stored ones. Otherwise use the live engine + tenant defaults.
	if in.Config != nil {
		s.runOneOffGuardrails(w, r.Context(), req, *in.Config)
		return
	}
	res := s.guardrails.Inbound(r.Context(), req)
	writeJSON(w, http.StatusOK, map[string]any{
		"action":    res.Action,
		"reason":    res.Reason,
		"decisions": res.Decisions,
	})
}

func (s *Handler) runOneOffGuardrails(w http.ResponseWriter, ctx context.Context, req *core.ChatRequest, pol guardrails.Policy) {
	// Build a temporary engine that resolves to `pol` regardless of stored
	// policies, but reuses the live audit writer so test-panel runs show up
	// in the dashboard's Audit Logs tab alongside real traffic.
	tmp := guardrails.NewEngine(guardrails.EngineConfig{
		Resolver:  guardrails.NewResolver(&staticResolver{pol: pol}, 0),
		Audit:     s.guardrails.Audit(),
		Detectors: s.guardrails.Detectors(),
		Logger:    s.log,
	})
	res := tmp.Inbound(ctx, req)
	// Flush the audit writer so the row from this test run appears in the
	// dashboard immediately (the ticker would otherwise hold it ~1s).
	if a := s.guardrails.Audit(); a != nil {
		a.FlushNow()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action":    res.Action,
		"reason":    res.Reason,
		"decisions": res.Decisions,
	})
}

// staticResolver returns the same policy regardless of the scope tuple. Used
// only by the test endpoint so the user can preview a config in isolation.
type staticResolver struct {
	pol guardrails.Policy
}

func (s *staticResolver) GetByScope(_ context.Context, tenantID string, scope schema.GuardrailScope, scopeID string) (schema.GuardrailPolicy, error) {
	if scope != schema.GuardrailScopeGlobal {
		return schema.GuardrailPolicy{}, schema.ErrNotFound
	}
	cfg, err := guardrails.MarshalPolicy(s.pol)
	if err != nil {
		return schema.GuardrailPolicy{}, err
	}
	return schema.GuardrailPolicy{
		TenantID: tenantID,
		Scope:    string(scope),
		ScopeID:  scopeID,
		Enabled:  true,
		Config:   cfg,
	}, nil
}

func (s *Handler) invalidateGuardrail(tenantID string, scope schema.GuardrailScope, scopeID string) {
	if s.guardrails == nil {
		return
	}
	if res := s.guardrails.Resolver(); res != nil {
		res.Invalidate(tenantID, scope, scopeID)
	}
}

func isValidScope(s schema.GuardrailScope) bool {
	switch s {
	case schema.GuardrailScopeGlobal,
		schema.GuardrailScopeProvider,
		schema.GuardrailScopeModel,
		schema.GuardrailScopeChain,
		schema.GuardrailScopeAPIKey:
		return true
	}
	return false
}

func defaultPolicyName(scope schema.GuardrailScope, scopeID string) string {
	if scope == schema.GuardrailScopeGlobal {
		return "Global Guardrails"
	}
	return string(scope) + ":" + scopeID
}

func newGuardrailID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "gp_" + hex.EncodeToString(b[:])
}

// parseIntInto is a tiny strconv-free helper for the limit query param. Returns
// the digit count consumed and any parse error. Keeps imports lean.
func parseIntInto(s string, dst *int) (int, error) {
	n := 0
	consumed := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
		consumed++
	}
	*dst = n
	return consumed, nil
}
