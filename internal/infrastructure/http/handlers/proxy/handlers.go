package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"

	"github.com/go-chi/chi/v5"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/middleware"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// maxBodyBytes caps inbound request bodies to protect against oversized uploads.
const maxBodyBytes = 32 << 20 // 32 MiB

// logRequest logs a completed request to the access log file and console log buffer.
func (s *Handler) logRequest(keyName, provider, model string, tokens int, costMicros int64, latencyMs int, cacheHit bool, err error) {
	cost := float64(costMicros) / 1_000_000
	if err != nil {
		middleware.WriteLLMLog(fmt.Sprintf("[ERROR] provider=%s model=%s key=%s latency=%dms error=%v",
			provider, model, keyName, latencyMs, err))
		if s.log != nil {
			s.log.Debugf("[LLM ERROR] provider=%s model=%s key=%s latency=%dms error=%v",
				provider, model, keyName, latencyMs, err)
		}
	} else {
		cacheNote := ""
		if cacheHit {
			cacheNote = " (cache-hit)"
		}
		middleware.WriteLLMLog(fmt.Sprintf("[COMPLETED] provider=%s model=%s key=%s tokens=%d cost=$%.4f latency=%dms%s",
			provider, model, keyName, tokens, cost, latencyMs, cacheNote))
		if s.log != nil {
			s.log.Debugf("[LLM COMPLETED] provider=%s model=%s key=%s tokens=%d cost=$%.4f latency=%dms%s",
				provider, model, keyName, tokens, cost, latencyMs, cacheNote)
		}
	}

	if s.consoleLog == nil {
		return
	}

	if err != nil {
		detail := fmt.Sprintf("Key:      %s\nProvider: %s\nModel:    %s\nLatency:  %dms\n\n%v",
			keyName, provider, model, latencyMs, err)
		s.consoleLog.Log("ERROR",
			fmt.Sprintf("Request failed · %s · %s", model, humanDuration(latencyMs)),
			detail)
		return
	}

	level := "INFO"
	if latencyMs > 8000 {
		level = "WARN"
	}
	cacheNote := ""
	if cacheHit {
		cacheNote = " · cache hit"
	}
	msg := fmt.Sprintf("Request completed · %s · %s tokens · $%.4f · %s%s",
		model, humanInt(tokens), cost, humanDuration(latencyMs), cacheNote)
	detail := fmt.Sprintf(
		"Key:      %s\nProvider: %s\nModel:    %s\nTokens:   %s\nCost:     $%.4f\nLatency:  %dms\nCache:    %v",
		keyName, provider, model, humanInt(tokens), cost, latencyMs, cacheHit)
	s.consoleLog.Log(level, msg, detail)
}

// handleOpenAIChat serves /v1/chat/completions in the OpenAI dialect.
func (s *Handler) HandlePortalKeyUsage(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		WriteError(w, http.StatusBadRequest, "missing key id")
		return
	}

	ctx := r.Context()
	key, err := s.identity.Keys().Get(ctx, keyID)
	if err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "key not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "failed to get key")
		return
	}

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
		if b.AlertPct > 0 {
			if (b.LimitMicros > 0 && costMicros*100 >= b.LimitMicros*int64(b.AlertPct)) ||
				(b.LimitTokens > 0 && tokens*100 >= b.LimitTokens*int64(b.AlertPct)) {
				bo.Alert = true
			}
		}
		budgetOuts = append(budgetOuts, bo)
	}

	allowedModels, err := s.identity.Keys().GetAllowedModels(ctx, key.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get model access")
		return
	}

	// Get current period summary scoped to this specific key.
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	summary, err := s.usage.SummarizeByKey(ctx, key.ID, periodStart)
	if err != nil {
		s.log.Error("key usage: summarize failed", "err", err)
	}

	// Daily usage series for the portal chart (last 30 days).
	daily, _ := s.usage.DailyByKey(ctx, key.ID, now.AddDate(0, 0, -30))
	var dailyOut []map[string]any
	for _, d := range daily {
		dailyOut = append(dailyOut, map[string]any{
			"date":              d.Date,
			"requests":          d.Requests,
			"prompt_tokens":     d.PromptTokens,
			"completion_tokens": d.CompletionTokens,
			"cost_usd":          float64(d.CostMicros) / 1_000_000,
		})
	}

	// Per-model breakdown for this key (last 30 days).
	models, _ := s.usage.ByModelByKey(ctx, key.ID, now.AddDate(0, 0, -30))
	var modelOut []map[string]any
	for _, m := range models {
		modelOut = append(modelOut, map[string]any{
			"provider":          m.Provider,
			"model":             m.Model,
			"total_requests":    m.TotalRequests,
			"prompt_tokens":     m.PromptTokens,
			"completion_tokens": m.CompletionTokens,
			"cost_usd":          float64(m.CostMicros) / 1_000_000,
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

// detectClient identifies the calling tool from request headers, used for
// telemetry, savings attribution, and client-specific quirks. Best-effort.
//
// Known clients map to stable friendly labels so they aggregate cleanly. Any
// other client is normalized from its User-Agent product token rather than
// dropped, so every request is attributable. Falls back to "unknown" when no
// usable signal exists, so optimization savings are never silently uncounted.
func detectClient(r *http.Request) string {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	switch {
	case strings.Contains(ua, "claude"):
		return "claude-code"
	case strings.Contains(ua, "cursor"):
		return "cursor"
	case strings.Contains(ua, "codex"):
		return "codex"
	case strings.Contains(ua, "cline"):
		return "cline"
	case strings.Contains(ua, "copilot"):
		return "copilot"
	case strings.Contains(ua, "kilo"):
		return "kilo-code"
	case strings.Contains(ua, "opencode"):
		return "opencode"
	case strings.Contains(ua, "droid"):
		return "droid"
	case strings.Contains(ua, "aider"):
		return "aider"
	case strings.Contains(ua, "roo"):
		return "roo-code"
	}
	// Generic fallback: derive a clean label from the User-Agent product token
	// (the text before the first '/' or whitespace), so any client is counted.
	if label := normalizeClientLabel(ua); label != "" {
		return label
	}
	// SDK callers often omit a descriptive UA but set a stainless language hint.
	if lang := strings.TrimSpace(r.Header.Get("x-stainless-lang")); lang != "" {
		return "sdk-" + sanitizeClientToken(strings.ToLower(lang))
	}
	return "unknown"
}

// normalizeClientLabel extracts a stable, lowercase client label from a
// User-Agent string by taking the leading product token (before '/' or space)
// and stripping noise. Returns "" when nothing usable remains.
func normalizeClientLabel(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	// Take the first product token: "foo-cli/1.2.3 (...)" -> "foo-cli".
	token := ua
	if i := strings.IndexAny(token, "/ \t"); i >= 0 {
		token = token[:i]
	}
	token = sanitizeClientToken(token)
	// Ignore generic HTTP libraries that carry no product identity.
	switch token {
	case "", "mozilla", "python-requests", "python", "go-http-client",
		"node-fetch", "axios", "curl", "okhttp", "java", "undici":
		return ""
	}
	return token
}

// sanitizeClientToken keeps only [a-z0-9-_.] and trims separators, so labels
// are safe to store and group on without surprising characters.
func sanitizeClientToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_.")
}

func requestAffinityKey(r *http.Request, req *core.ChatRequest) string {
	for _, header := range affinityHeaders {
		if v := strings.TrimSpace(r.Header.Get(header)); v != "" {
			return hashAffinityValue("header:"+strings.ToLower(header), v)
		}
	}

	if v := extraAffinityKey(req); v != "" {
		return hashAffinityValue("body", v)
	}
	if req == nil {
		return ""
	}
	seed := conversationSeed(req)
	if seed == "" {
		return ""
	}
	return hashAffinityValue("fingerprint", seed)
}

func hashAffinityValue(source, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(source + "\x00" + value))
	return source + ":" + hex.EncodeToString(sum[:])
}

// affinityHeaders is the ordered list of HTTP headers checked for routing affinity.
var affinityHeaders = []string{
	"X-FlameGate-Affinity",
	"X-Conversation-ID",
	"X-Thread-ID",
	"X-Session-ID",
	"X-Amp-Thread-ID",
	"X-Client-Request-ID",
	"OpenAI-Conversation-ID",
}

// extraAffinityKey extracts an affinity key from the already-parsed
// ChatRequest.Extra map, avoiding a full JSON re-parse of the request body.
func extraAffinityKey(req *core.ChatRequest) string {
	if req == nil || len(req.Extra) == 0 {
		return ""
	}
	for _, key := range affinityBodyKeys {
		if v := rawString(req.Extra[key]); v != "" {
			return key + ":" + v
		}
	}
	if v := rawString(req.Extra["conversation"]); v != "" {
		return "conversation:" + v
	}
	if v := rawObjectString(req.Extra["conversation"], "id"); v != "" {
		return "conversation.id:" + v
	}
	if v := rawObjectString(req.Extra["metadata"], "conversation_id"); v != "" {
		return "metadata.conversation_id:" + v
	}
	if v := rawObjectString(req.Extra["metadata"], "thread_id"); v != "" {
		return "metadata.thread_id:" + v
	}
	if v := rawObjectString(req.Extra["metadata"], "session_id"); v != "" {
		return "metadata.session_id:" + v
	}
	if v := rawObjectString(req.Extra["metadata"], "user_id"); v != "" {
		return "metadata.user_id:" + v
	}
	return ""
}

// affinityBodyKeys are the top-level JSON keys checked for routing affinity.
var affinityBodyKeys = []string{
	"conversation_id",
	"thread_id",
	"session_id",
	"prompt_cache_key",
	"previous_response_id",
	"parent_id",
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func rawObjectString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return rawString(obj[key])
}

func conversationSeed(req *core.ChatRequest) string {
	var b strings.Builder
	b.WriteString(req.Metadata.APIKeyID)
	b.WriteByte('\n')
	b.WriteString(req.Metadata.ClientKind)
	b.WriteByte('\n')
	b.WriteString(string(req.Metadata.SourceDialect))
	b.WriteByte('\n')
	b.WriteString(req.Model)
	if system := strings.TrimSpace(req.System); system != "" {
		b.WriteString("\nsystem:")
		b.WriteString(limitAffinityText(system))
	}
	seenText := 0
	for _, msg := range req.Messages {
		if msg.Role != core.RoleUser {
			continue
		}
		text := strings.TrimSpace(msg.TextContent())
		if text == "" {
			continue
		}
		b.WriteString("\nuser:")
		b.WriteString(limitAffinityText(text))
		seenText++
		if seenText >= 1 {
			break
		}
	}
	if seenText == 0 {
		return ""
	}
	return b.String()
}

func limitAffinityText(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max]
}
