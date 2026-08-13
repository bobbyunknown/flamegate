package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func (s *Handler) validateAccountCredentials(ctx context.Context, acc schema.Account) error {
	if s.conns == nil || s.vault == nil {
		return nil // can't validate without registry + vault
	}
	// Skip validation for providers behind WAF/CDN that block probes.
	if spec, ok := connectors.SpecByID(acc.Provider); ok && spec.SkipValidation {
		return nil
	}
	conn, err := s.conns.Get(acc.Provider)
	if err != nil {
		return nil // provider has no connector; skip validation
	}
	v, ok := conn.(core.Validator)
	if !ok {
		return nil // connector doesn't support validation
	}
	creds, err := s.vault.Open(acc)
	if err != nil {
		return errors.New("could not decrypt credentials")
	}
	// Apply a reasonable timeout for the probe.
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	probeErr := v.Validate(probeCtx, creds)
	if probeErr == nil {
		return nil
	}

	return probeErr
}

// decodeJSON decodes a request body into v, writing a 400 on failure. It
// returns false when the caller should stop.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			WriteError(w, http.StatusBadRequest, "empty request body")
			return false
		}
		WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func defaultBool(v, def bool) bool {
	return v || (!v && def)
}

// validateChainName rejects combo names that would conflict with routing resolution.
// Names must be alphanumeric with hyphens/underscores only, no slashes, colons,
// or leading/trailing whitespace. This prevents ambiguity with "provider/model"
// and "chain:name" formats in resolveTargets.
func validateChainName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("combo name is required")
	}
	if len(name) > 128 {
		return fmt.Errorf("combo name too long (max 128 characters)")
	}
	if strings.ContainsAny(name, "/:\\@#?") {
		return fmt.Errorf("combo name cannot contain / : \\ @ # ? characters")
	}
	if strings.HasPrefix(name, "chain:") {
		return fmt.Errorf("combo name cannot start with 'chain:' prefix")
	}
	// Must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$
	for i, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if c == '-' || c == '_' {
			if i == 0 {
				return fmt.Errorf("combo name must start with a letter or digit")
			}
			continue
		}
		return fmt.Errorf("combo name can only contain letters, digits, hyphens, and underscores")
	}
	return nil
}
