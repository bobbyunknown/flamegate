package admin

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// bulkMaxItems caps the number of credentials accepted in a single bulk import
// to bound memory, upstream validation fan-out, and DB write time.
const bulkMaxItems = 1000

// bulkValidateConcurrency bounds how many upstream credential probes run at
// once during a bulk import with validation enabled.
const bulkValidateConcurrency = 6

type bulkAccountItem struct {
	Label   string `json:"label" doc:"Display label for this account"`
	APIKey  string `json:"api_key" doc:"Provider API key or token"`
	BaseURL string `json:"base_url" format:"uri" doc:"Base URL for this account (overrides bulk base_url)"`
}

type bulkAccountsRequest struct {
	Provider string `json:"provider"`
	// Shared settings applied to every item unless an item overrides them.
	BaseURL           string `json:"base_url"`
	Region            string `json:"region"`
	AccountID         string `json:"account_id"`
	AzureEndpoint     string `json:"azure_endpoint"`
	AzureDeployment   string `json:"azure_deployment"`
	AzureAPIVersion   string `json:"azure_api_version"`
	AzureOrganization string `json:"azure_organization"`
	Priority          int    `json:"priority"`
	ProxyPoolID       string `json:"proxy_pool_id"`
	// Validate probes each credential against the upstream before persisting.
	// Off by default for bulk to avoid slow imports and upstream rate limits.
	Validate bool              `json:"validate"`
	Items    []bulkAccountItem `json:"items"`
}

type bulkAccountResult struct {
	Index  int    `json:"index" doc:"Position in the input items array"`
	Label  string `json:"label" doc:"Label from input item"`
	Status string `json:"status" enum:"created,error,skipped" doc:"Outcome status"` // created | error | skipped
	ID     string `json:"id,omitempty" doc:"Created account ID (when status=created)"`
	Error  string `json:"error,omitempty" doc:"Error message (when status=error)"`
}

// adminBulkCreateAccounts imports many provider credentials in one request. It
// reuses the same sealing, metadata, validation, and persistence path as the
// single-create handler, but reports a per-item outcome so partial failures
// don't abort the whole batch. Upstream validation (when enabled) runs with a
// bounded worker pool; DB writes are serialized to stay friendly to SQLite.
func (s *Handler) adminBulkCreateAccounts(w http.ResponseWriter, r *http.Request) {
	var body bulkAccountsRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" {
		WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}
	spec, ok := connectors.SpecByID(body.Provider)
	if !ok {
		WriteError(w, http.StatusBadRequest, "unknown provider: "+body.Provider)
		return
	}
	if s.vault == nil {
		WriteError(w, http.StatusInternalServerError, "vault not configured")
		return
	}
	if len(body.Items) == 0 {
		WriteError(w, http.StatusBadRequest, "items is required")
		return
	}
	if len(body.Items) > bulkMaxItems {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("too many items: %d (max %d)", len(body.Items), bulkMaxItems))
		return
	}

	// SSRF Protection: validate the shared base/endpoint URLs once up front.
	if body.BaseURL != "" {
		if err := httputil.ValidateBaseURL(body.BaseURL); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", body.BaseURL, "error", err)
			WriteError(w, http.StatusBadRequest, "invalid base_url: URL blocked by security policy")
			return
		}
	}
	if body.AzureEndpoint != "" {
		if err := httputil.ValidateBaseURL(body.AzureEndpoint); err != nil {
			s.log.Warn("blocked suspicious azure_endpoint", "url", body.AzureEndpoint, "error", err)
			WriteError(w, http.StatusBadRequest, "invalid azure_endpoint: URL blocked by security policy")
			return
		}
	}

	results := make([]bulkAccountResult, len(body.Items))
	var (
		seen    = map[string]struct{}{} // de-dup api keys within the batch
		seenMu  sync.Mutex
		writeMu sync.Mutex // serialize DB writes (SQLite-friendly)
		sem     = make(chan struct{}, bulkValidateConcurrency)
		wg      sync.WaitGroup
	)

	for i, item := range body.Items {
		label := strings.TrimSpace(item.Label)
		key := strings.TrimSpace(item.APIKey)
		results[i] = bulkAccountResult{Index: i, Label: label}

		authKind := accountAuthKind(spec, key)
		if authKind != schema.AuthNone && key == "" {
			results[i].Status = "error"
			results[i].Error = "api_key is required"
			continue
		}

		// De-duplicate identical keys within the same batch.
		if key != "" {
			seenMu.Lock()
			if _, dup := seen[key]; dup {
				seenMu.Unlock()
				results[i].Status = "skipped"
				results[i].Error = "duplicate api key in batch"
				continue
			}
			seen[key] = struct{}{}
			seenMu.Unlock()
		}

		// Per-item base URL overrides the shared one when present.
		baseURL := strings.TrimSpace(item.BaseURL)
		if baseURL == "" {
			baseURL = body.BaseURL
		}
		if baseURL != "" {
			if err := httputil.ValidateBaseURL(baseURL); err != nil {
				results[i].Status = "error"
				results[i].Error = "invalid base_url: URL blocked by security policy"
				continue
			}
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(i int, label, key, baseURL string, authKind schema.AuthKind) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := providerAccountMetadata(spec, providerMetadataInput{
				BaseURL:           baseURL,
				Region:            body.Region,
				AccountID:         body.AccountID,
				AzureEndpoint:     body.AzureEndpoint,
				AzureDeployment:   body.AzureDeployment,
				AzureAPIVersion:   body.AzureAPIVersion,
				AzureOrganization: body.AzureOrganization,
			})
			if err != nil {
				results[i].Status = "error"
				results[i].Error = err.Error()
				return
			}

			now := time.Now()
			displayLabel := label
			if displayLabel == "" {
				displayLabel = fmt.Sprintf("%s-%d", spec.DisplayName, i+1)
			}
			acc := schema.Account{
				ID:        uuid.NewString(),
				TenantID:  adminTenant,
				Provider:  body.Provider,
				Label:     displayLabel,
				AuthKind:  string(authKind),
				Priority:  defaultInt(body.Priority, 100),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if body.ProxyPoolID != "" {
				acc.ProxyPoolID = body.ProxyPoolID
			}
			if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: key, Metadata: meta}); err != nil {
				results[i].Status = "error"
				results[i].Error = "vault seal failed"
				return
			}

			if body.Validate {
				if verr := s.validateAccountCredentials(r.Context(), acc); verr != nil {
					results[i].Status = "error"
					results[i].Error = sanitizeError(s.log, verr, "credential validation failed")
					return
				}
			}

			writeMu.Lock()
			err = s.accounts.Create(r.Context(), acc)
			writeMu.Unlock()
			if err != nil {
				results[i].Status = "error"
				results[i].Error = sanitizeError(s.log, err, "account creation failed")
				return
			}
			results[i].Status = "created"
			results[i].ID = acc.ID
			results[i].Label = displayLabel
		}(i, label, key, baseURL, authKind)
	}

	wg.Wait()

	sort.Slice(results, func(a, b int) bool { return results[a].Index < results[b].Index })
	var created, failed, skipped int
	for _, res := range results {
		switch res.Status {
		case "created":
			created++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(results),
		"created": created,
		"failed":  failed,
		"skipped": skipped,
		"results": results,
	})
}

// adminAccountQuota fetches upstream quota/credit info for a specific account.

// ---- chains -----------------------------------------------------------------
