package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.
func (s *Handler) MountAdmin(r chi.Router) {
	// Streaming endpoints (SSE) stay as raw Chi
	r.Get("/usage/stream", s.adminUsageStream)
	r.Get("/console", s.adminConsoleLog)
	r.Delete("/console", s.adminConsoleClear)
	r.Get("/console/stream", s.adminConsoleStream)
	r.Post("/models/chat/stream", s.adminModelChatStream)

	// Settings endpoints (not yet migrated)
	r.Get("/settings/database", s.adminExportDatabase)
	r.Post("/settings/database", s.adminImportDatabase)
	r.Get("/settings/sqlite", s.adminSQLiteStatus)
	r.Get("/settings/sqlite/backup", s.adminSQLiteBackup)
	r.Post("/settings/sqlite/restore", s.adminSQLiteRestore)
	r.Post("/settings/proxy-test", s.adminTestProxy)

	// Tunnel SSE endpoint (must stay as raw Chi)
	r.Post("/tunnel/tailscale-install", s.adminTailscaleInstall)

	// Custom provider flows, extensions, CLI tools (raw Chi submounts)
	s.MountCustomProviders(r)
	s.MountExtensions(r)
	s.MountStore(r)
	s.MountCLITools(r)
	s.MountOAuth(r)

	// Branding endpoints (not yet migrated)
	r.Get("/settings/branding", s.adminGetBranding)
	r.Post("/settings/branding", s.adminUpdateBranding)

	// Guardrail SSE endpoint (must stay as raw Chi)
	r.Get("/guardrails/logs/stream", s.adminGuardrailLogStream)
}

func (s *Handler) adminModelChatStream(w http.ResponseWriter, r *http.Request) {
	if s.pipeline == nil {
		http.Error(w, `{"error":"pipeline not configured"}`, http.StatusInternalServerError)
		return
	}

	var body struct {
		Provider    string                 `json:"provider"`
		Model       string                 `json:"model"`
		Messages    []TestChatMessageInput `json:"messages"`
		System      string                 `json:"system,omitempty"`
		Temperature *float64               `json:"temperature,omitempty"`
		MaxTokens   int                    `json:"max_tokens,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON request"}`, http.StatusBadRequest)
		return
	}

	provider := strings.TrimSpace(body.Provider)
	model := strings.TrimSpace(body.Model)
	if model == "" {
		http.Error(w, `{"error":"model is required"}`, http.StatusBadRequest)
		return
	}

	fullModel := model
	if provider != "" && !strings.HasPrefix(model, provider+"/") {
		fullModel = provider + "/" + model
	}

	var msgs []core.Message
	if body.System != "" {
		msgs = append(msgs, core.Message{
			Role: "system",
			Content: []core.ContentPart{
				{Type: "text", Text: body.System},
			},
		})
	}
	for _, m := range body.Messages {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		msgs = append(msgs, core.Message{
			Role: core.Role(m.Role),
			Content: []core.ContentPart{
				{Type: "text", Text: m.Content},
			},
		})
	}
	if len(msgs) == 0 {
		http.Error(w, `{"error":"at least one message is required"}`, http.StatusBadRequest)
		return
	}

	maxTokens := body.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	chatReq := &core.ChatRequest{
		Model:       fullModel,
		Messages:    msgs,
		MaxTokens:   &maxTokens,
		Temperature: body.Temperature,
		Metadata: core.RequestMetadata{
			TenantID: adminTenant,
		},
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sendSSE := func(eventType string, data any) {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
		flusher.Flush()
	}

	start := time.Now()
	streamRes, err := s.pipeline.Stream(r.Context(), chatReq, pipeline.Options{})
	if err != nil {
		sendSSE("error", map[string]any{"error": err.Error(), "latency_ms": time.Since(start).Milliseconds()})
		return
	}

	var ttftMs int64
	firstToken := true
	var promptTokens, completionTokens int

	for chunk := range streamRes.Chunks {
		if firstToken {
			firstToken = false
			ttftMs = time.Since(start).Milliseconds()
		}
		if chunk.Err != nil {
			sendSSE("error", map[string]any{"error": chunk.Err.Error()})
			return
		}
		if chunk.Delta != "" {
			if chunk.Type == core.ChunkThinking {
				sendSSE("thinking", map[string]any{"delta": chunk.Delta})
			} else {
				sendSSE("delta", map[string]any{"delta": chunk.Delta})
			}
		}
		if chunk.Usage != nil {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
		}
	}

	totalLatency := time.Since(start).Milliseconds()
	sendSSE("done", map[string]any{
		"latency_ms":        totalLatency,
		"ttft_ms":           ttftMs,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"model":             fullModel,
	})
}

const adminTenant = schema.DefaultTenantID

