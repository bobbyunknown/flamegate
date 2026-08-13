package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/meter"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/headroom"
	"github.com/bobbyunknown/flamegate/internal/shared/ponytail"
)

// TestHeadroomEndToEnd_PipelineMeterStore exercises the full Headroom flow end
// to end: the pipeline's token-saving stage calls an external proxy (an
// httptest.Server) to compress messages, the result flows through saveState
// into the meter, and the meter persists a schema.UsageRecord. We then read the
// record back and assert the Headroom token/byte/active fields and the Ponytail
// active flag were recorded correctly.
//
// This wires together the real pipeline (applyTokenSaving -> buildSaveState ->
// record), the real meter mapping, and a real migrated SQLite store, so it
// covers the pipeline -> meter -> store path.
func TestHeadroomEndToEnd_PipelineMeterStore(t *testing.T) {
	ctx := context.Background()
	// A long user message so the JSON body before compression is comfortably
	// large; the "real savings" scenario shrinks it dramatically.
	longText := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 80)

	newRequest := func() *core.ChatRequest {
		return &core.ChatRequest{
			Model:  "gpt-4o",
			System: "You are a helpful assistant.",
			Messages: []core.Message{
				{
					Role:    core.RoleUser,
					Content: []core.ContentPart{{Type: core.PartText, Text: longText}},
				},
			},
			Metadata: core.RequestMetadata{
				TenantID:   schema.DefaultTenantID,
				ClientKind: "claude-code",
			},
		}
	}

	tests := []struct {
		name string
		// proxyHandler is the /v1/compress responder; nil means Headroom is
		// disabled (no proxy is configured and the server is never consulted).
		proxyHandler    http.HandlerFunc
		headroomEnabled bool
		ponytailEnabled bool

		wantHeadroomActive bool
		wantHeadroomTokens int
		wantHeadroomBytesP func(t *testing.T, got int)
		wantPonytailActive bool
	}{
		{
			name:            "real non-phantom savings record Headroom active",
			headroomEnabled: true,
			ponytailEnabled: true,
			proxyHandler: func(w http.ResponseWriter, r *http.Request) {
				// Return a much shorter compressed message plus token stats so
				// the JSON body really shrinks (non-phantom) and tokens_saved>0.
				writeJSON(w, map[string]any{
					"messages": []map[string]any{
						{"role": "system", "content": "Helpful assistant."},
						{"role": "user", "content": "Summarize the fox text."},
					},
					"stats": map[string]any{
						"tokens_before": 1200,
						"tokens_after":  200,
						"tokens_saved":  1000,
					},
				})
			},
			wantHeadroomActive: true,
			wantHeadroomTokens: 1000,
			wantHeadroomBytesP: func(t *testing.T, got int) {
				require.Greater(t, got, 0, "real compression should record positive bytes saved")
			},
			wantPonytailActive: true,
		},
		{
			name:            "phantom savings record Headroom inactive with zero savings",
			headroomEnabled: true,
			ponytailEnabled: true,
			proxyHandler: func(w http.ResponseWriter, r *http.Request) {
				// Echo the request messages back unchanged (no real byte shrink)
				// while claiming tokens were saved. This is the phantom case:
				// the provider would still bill nearly the full amount.
				var body struct {
					Messages []json.RawMessage `json:"messages"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				writeJSON(w, map[string]any{
					"messages": body.Messages,
					"stats": map[string]any{
						"tokens_before": 1200,
						"tokens_after":  700,
						"tokens_saved":  500,
					},
				})
			},
			wantHeadroomActive: false,
			wantHeadroomTokens: 0,
			wantHeadroomBytesP: func(t *testing.T, got int) {
				require.Equal(t, 0, got, "phantom savings must record zero bytes saved")
			},
			wantPonytailActive: true,
		},
		{
			name:               "headroom disabled record inactive but Ponytail still flagged",
			headroomEnabled:    false,
			ponytailEnabled:    true,
			proxyHandler:       nil,
			wantHeadroomActive: false,
			wantHeadroomTokens: 0,
			wantHeadroomBytesP: func(t *testing.T, got int) {
				require.Equal(t, 0, got)
			},
			wantPonytailActive: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// Real migrated in-memory store + meter backed by its usage repo.
			db := newPipelineTestDB(t)
			m := meter.New(db.Usage(), nil, nil)
			p := New(Deps{Meter: m})

			var proxyURL string
			if tc.proxyHandler != nil {
				srv := httptest.NewServer(tc.proxyHandler)
				t.Cleanup(srv.Close)
				proxyURL = srv.URL
			}

			opts := Options{
				Headroom: headroom.Config{
					Enabled: tc.headroomEnabled,
					URL:     proxyURL,
					Timeout: 2 * time.Second,
				},
				Ponytail: ponytail.Config{
					Enabled: tc.ponytailEnabled,
					Level:   ponytail.LevelFull,
				},
			}

			req := newRequest()

			// Drive the real pipeline token-saving stage: this calls the proxy
			// over HTTP (Headroom) and injects the Ponytail block.
			slim, hr := p.applyTokenSaving(ctx, req, opts)
			save := buildSaveState(slim, hr, opts)

			// When Ponytail is enabled it must have injected its sentinel block.
			if tc.ponytailEnabled {
				require.Contains(t, req.System, "flamegate:ponytail",
					"ponytail block should be appended to the system prompt")
			}

			// Meter + persist the record (pipeline -> meter -> store).
			attempt := dispatch.Attempt{
				Target:  dispatch.Target{Provider: "openai", Model: "gpt-4o"},
				Account: schema.Account{ID: "acc-1"},
			}
			usage := core.Usage{PromptTokens: 100, CompletionTokens: 50}
			p.record(ctx, req.Metadata, attempt, usage, false, 5*time.Millisecond, save)

			// Read the persisted record back from the store.
			recent, err := db.Usage().Recent(ctx, schema.DefaultTenantID, 10)
			require.NoError(t, err)
			require.Len(t, recent, 1, "exactly one usage record should be persisted")
			rec := recent[0]

			require.Equal(t, tc.wantHeadroomActive, rec.HeadroomActive, "HeadroomActive")
			require.Equal(t, tc.wantHeadroomTokens, rec.HeadroomTokensSaved, "HeadroomTokensSaved")
			require.GreaterOrEqual(t, rec.HeadroomBytesSaved, 0, "HeadroomBytesSaved must be non-negative")
			tc.wantHeadroomBytesP(t, rec.HeadroomBytesSaved)
			require.Equal(t, tc.wantPonytailActive, rec.PonytailActive, "PonytailActive")
		})
	}
}

// newPipelineTestDB opens a migrated in-memory SQLite database for a single
// test, with the default tenant ensured so usage records can be attributed.
func newPipelineTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	require.NoError(t, db.EnsureDefault())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// writeJSON is a tiny helper for the fake proxy handlers.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
