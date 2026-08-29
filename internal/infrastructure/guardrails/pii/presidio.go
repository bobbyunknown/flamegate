package pii

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PresidioEngine calls a Microsoft Presidio HTTP analyzer to recognize PII.
// It is an *alternative* to the native Go recognizers — selected per-policy
// via PIIConfig.Engine = "presidio". When the analyzer is unreachable the
// engine returns no matches and the detector silently falls back to native
// (see pii.Detector.recognize).
//
// Phase 2 wires only the /analyze endpoint. Anonymization is still done in
// Go (applyStrategy), so we don't need the anonymizer sidecar.
type PresidioEngine struct {
	httpClient *http.Client
	analyzer   string // base URL, e.g. http://localhost:5001
	language   string // BCP-47, defaults to "en"
}

// PresidioConfig configures the engine.
type PresidioConfig struct {
	// AnalyzerURL is the base URL of the Presidio analyzer service. Empty
	// disables the engine.
	AnalyzerURL string
	// Timeout caps each analyze call. Defaults to 5s.
	Timeout time.Duration
	// Language is the BCP-47 language code passed to Presidio. Defaults to
	// "en"; set to "id" for Indonesian if the sidecar has the model loaded.
	Language string
}

// NewPresidioEngine builds a Presidio-backed engine. Returns nil when no
// analyzer URL is configured — callers should check before using.
func NewPresidioEngine(cfg PresidioConfig) *PresidioEngine {
	if strings.TrimSpace(cfg.AnalyzerURL) == "" {
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	return &PresidioEngine{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		analyzer:   strings.TrimRight(cfg.AnalyzerURL, "/"),
		language:   cfg.Language,
	}
}

// presidioRequest mirrors the Presidio /analyze POST body.
type presidioRequest struct {
	Text           string   `json:"text"`
	Language       string   `json:"language"`
	Entities       []string `json:"entities,omitempty"`
	ScoreThreshold float64  `json:"score_threshold,omitempty"`
	ReturnDecision bool     `json:"return_decision_process,omitempty"`
}

// presidioFinding mirrors one row in the Presidio /analyze response.
type presidioFinding struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

// Recognize calls the analyzer and converts the response into the native
// Match shape so the rest of the PII pipeline (severity, applyStrategy, audit
// findings) keeps working unchanged.
func (e *PresidioEngine) Recognize(ctx context.Context, text string, allowed map[Entity]bool, minScore float64) ([]Match, error) {
	if text == "" {
		return nil, nil
	}
	body, err := json.Marshal(presidioRequest{
		Text:           text,
		Language:       e.language,
		Entities:       allowedEntityList(allowed),
		ScoreThreshold: minScore,
	})
	if err != nil {
		return nil, fmt.Errorf("pii/presidio: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.analyzer+"/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pii/presidio: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pii/presidio: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("pii/presidio: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pii/presidio: status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var findings []presidioFinding
	if err := json.Unmarshal(raw, &findings); err != nil {
		return nil, fmt.Errorf("pii/presidio: parse: %w", err)
	}
	out := make([]Match, 0, len(findings))
	for _, f := range findings {
		if f.Start < 0 || f.End > len(text) || f.End <= f.Start {
			continue
		}
		ent := Entity(strings.ToUpper(f.EntityType))
		if allowed != nil && !allowed[ent] {
			continue
		}
		if f.Score < minScore {
			continue
		}
		out = append(out, Match{
			Entity: ent,
			Start:  f.Start,
			End:    f.End,
			Text:   text[f.Start:f.End],
			Score:  f.Score,
		})
	}
	return mergeOverlaps(out), nil
}

func allowedEntityList(allowed map[Entity]bool) []string {
	if len(allowed) == 0 {
		return nil
	}
	out := make([]string, 0, len(allowed))
	for e := range allowed {
		out = append(out, string(e))
	}
	return out
}
