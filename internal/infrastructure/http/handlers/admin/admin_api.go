package admin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/openapi"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// --- List Accounts ---

// AccountListItem is a single account in the list response.
type AccountListItem struct {
	ID             string `json:"id" doc:"Account UUID"`
	Provider       string `json:"provider" doc:"Provider ID"`
	Label          string `json:"label" doc:"Account label"`
	AuthKind       string `json:"auth_kind" doc:"Authentication kind (key, oauth, none)"`
	Priority       int    `json:"priority" doc:"Routing priority"`
	Disabled       bool   `json:"disabled" doc:"Whether account is disabled"`
	ProxyPoolID    string `json:"proxy_pool_id" doc:"Proxy pool ID"`
	NeedsReconnect bool   `json:"needs_reconnect" doc:"OAuth reconnection needed"`
	CreatedAt      string `json:"created_at" doc:"Creation timestamp"`
}

type ListAccountsInput struct {
	Query struct {
		Provider string `query:"provider" doc:"Filter by provider ID"`
		Disabled *bool  `query:"disabled" doc:"Filter by disabled status"`
		Limit    int    `query:"limit" doc:"Max results (default: 50, max: 500)" minimum:"1" maximum:"500"`
		Offset   int    `query:"offset" doc:"Results offset for pagination" minimum:"0"`
	}
}

type ListAccountsOutput struct {
	Body struct {
		Accounts []AccountListItem `json:"accounts"`
	}
}

func (s *Handler) HumaListAccounts(ctx context.Context, input *ListAccountsInput) (*ListAccountsOutput, error) {
	accs, err := s.accounts.ListByTenant(ctx, adminTenant)
	if err != nil {
		return nil, MapError(err)
	}
	
	// Filter by provider
	if input.Query.Provider != "" {
		filtered := make([]schema.Account, 0)
		for _, a := range accs {
			if a.Provider == input.Query.Provider {
				filtered = append(filtered, a)
			}
		}
		accs = filtered
	}
	
	// Filter by disabled status
	if input.Query.Disabled != nil {
		filtered := make([]schema.Account, 0)
		for _, a := range accs {
			if a.Disabled == *input.Query.Disabled {
				filtered = append(filtered, a)
			}
		}
		accs = filtered
	}
	
	// Apply pagination
	limit := input.Query.Limit
	if limit == 0 {
		limit = 50
	}
	offset := input.Query.Offset
	
	start := offset
	if start > len(accs) {
		start = len(accs)
	}
	end := start + limit
	if end > len(accs) {
		end = len(accs)
	}
	accs = accs[start:end]
	
	out := make([]AccountListItem, 0, len(accs))
	for _, a := range accs {
		out = append(out, AccountListItem{
			ID: a.ID, Provider: a.Provider, Label: a.Label,
			AuthKind: a.AuthKind, Priority: a.Priority,
			Disabled: a.Disabled, ProxyPoolID: a.ProxyPoolID,
			NeedsReconnect: a.NeedsReconnect, CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return &ListAccountsOutput{Body: struct {
		Accounts []AccountListItem `json:"accounts"`
	}{Accounts: out}}, nil
}

// --- Create Account ---

type CreateAccountBody struct {
	Provider          string `json:"provider" minLength:"1" doc:"Provider ID" example:"openai"`
	Label             string `json:"label,omitempty" doc:"Account label (defaults to provider display name if empty)" example:"Production OpenAI"`
	APIKey            string `json:"api_key,omitempty" doc:"API key (required for key-based providers)" format:"password"`
	BaseURL           string `json:"base_url,omitempty" doc:"Base URL override (required for custom providers)" format:"uri"`
	AzureEndpoint     string `json:"azure_endpoint,omitempty" hidden:"true" doc:"Azure OpenAI endpoint URL (Azure only)" format:"uri"`
	AzureDeployment   string `json:"azure_deployment,omitempty" hidden:"true" doc:"Azure OpenAI deployment name (Azure only)"`
	AzureAPIVersion   string `json:"azure_api_version,omitempty" hidden:"true" doc:"Azure OpenAI API version (Azure only)" example:"2024-02-01"`
	AzureOrganization string `json:"azure_organization,omitempty" hidden:"true" doc:"Azure organization (Azure only)"`
	ProxyPoolID       string `json:"proxy_pool_id,omitempty" doc:"Proxy pool ID (must reference existing pool)"`
	Priority          int    `json:"priority,omitempty" doc:"Routing priority (0-1000, default: 100, higher = preferred)" minimum:"0" maximum:"1000" example:"100"`
}

type CreateAccountInput struct {
	Body CreateAccountBody
}

type CreateAccountOutputBody struct {
	ID        string `json:"id" doc:"Created account UUID"`
	Provider  string `json:"provider" doc:"Provider ID"`
	Label     string `json:"label" doc:"Account label"`
	Priority  int    `json:"priority" doc:"Routing priority"`
	AuthKind  string `json:"auth_kind" doc:"Authentication kind"`
	Disabled  bool   `json:"disabled" doc:"Whether account is disabled"`
	CreatedAt string `json:"created_at" doc:"Creation timestamp (ISO 8601)"`
}

type CreateAccountOutput struct {
	Body CreateAccountOutputBody
}

func (s *Handler) HumaCreateAccount(ctx context.Context, input *CreateAccountInput) (*CreateAccountOutput, error) {
	body := input.Body
	if body.Provider == "" {
		return nil, huma.Error400BadRequest("provider is required")
	}
	spec, ok := connectors.SpecByID(body.Provider)
	if !ok {
		return nil, huma.Error400BadRequest("unknown provider: " + body.Provider)
	}
	if s.vault == nil {
		return nil, huma.Error500InternalServerError("vault not configured")
	}
	authKind := accountAuthKind(spec, body.APIKey)
	if authKind != schema.AuthNone && strings.TrimSpace(body.APIKey) == "" {
		return nil, huma.Error400BadRequest("api_key is required")
	}

	// SSRF Protection
	if body.BaseURL != "" {
		if err := httputil.ValidateBaseURL(body.BaseURL); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", body.BaseURL, "error", err)
			return nil, huma.Error400BadRequest("invalid base_url: URL blocked by security policy")
		}
	}
	if body.AzureEndpoint != "" {
		if err := httputil.ValidateBaseURL(body.AzureEndpoint); err != nil {
			s.log.Warn("blocked suspicious azure_endpoint", "url", body.AzureEndpoint, "error", err)
			return nil, huma.Error400BadRequest("invalid azure_endpoint: URL blocked by security policy")
		}
	}
	meta, err := providerAccountMetadata(spec, providerMetadataInput{
		BaseURL: body.BaseURL,
		AzureEndpoint: body.AzureEndpoint, AzureDeployment: body.AzureDeployment,
		AzureAPIVersion: body.AzureAPIVersion, AzureOrganization: body.AzureOrganization,
	})
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	now := time.Now()
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = spec.DisplayName
	}
	acc := schema.Account{
		ID: uuid.NewString(), TenantID: adminTenant, Provider: body.Provider,
		Label: label, AuthKind: string(authKind), Priority: defaultInt(body.Priority, 100),
		CreatedAt: now, UpdatedAt: now,
	}
	if body.ProxyPoolID != "" {
		acc.ProxyPoolID = body.ProxyPoolID
	}
	if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: body.APIKey, Metadata: meta}); err != nil {
		return nil, MapError(err)
	}
	if verr := s.validateAccountCredentials(ctx, acc); verr != nil {
		return nil, huma.Error400BadRequest(sanitizeError(s.log, verr, "credential validation failed"))
	}
	if err := s.accounts.Create(ctx, acc); err != nil {
		return nil, MapError(err)
	}
	return &CreateAccountOutput{Body: CreateAccountOutputBody{
		ID: acc.ID, Provider: acc.Provider, Label: acc.Label,
		Priority: acc.Priority, AuthKind: acc.AuthKind,
		Disabled: acc.Disabled, CreatedAt: acc.CreatedAt.Format(time.RFC3339),
	}}, nil
}

// --- Bulk Create Accounts ---

type BulkCreateAccountsBody struct {
	Provider          string            `json:"provider" minLength:"1" doc:"Provider ID for all items"`
	BaseURL           string            `json:"base_url,omitempty" format:"uri" doc:"Base URL override for all items (optional, per-item overrides this)"`
	AzureEndpoint     string            `json:"azure_endpoint,omitempty" hidden:"true" doc:"Azure OpenAI endpoint URL (Azure only)"`
	AzureDeployment   string            `json:"azure_deployment,omitempty" hidden:"true" doc:"Azure OpenAI deployment name (Azure only)"`
	AzureAPIVersion   string            `json:"azure_api_version,omitempty" hidden:"true" doc:"Azure OpenAI API version (Azure only)"`
	AzureOrganization string            `json:"azure_organization,omitempty" hidden:"true" doc:"Azure organization (Azure only)"`
	Priority          int               `json:"priority,omitempty" minimum:"0" maximum:"1000" example:"100" doc:"Default priority for all items (0-1000)"`
	ProxyPoolID       string            `json:"proxy_pool_id,omitempty"`
	Validate          bool              `json:"validate,omitempty" doc:"Probe credentials upstream before persisting (slower but catches bad keys)"`
	Items             []bulkAccountItem `json:"items" minItems:"1" maxItems:"1000" doc:"Accounts to create (max 1000)"`
}

type BulkCreateAccountsInput struct {
	Body BulkCreateAccountsBody
}

type BulkCreateAccountsOutputBody struct {
	Total   int                 `json:"total" doc:"Total items submitted"`
	Created int                 `json:"created" doc:"Successfully created accounts"`
	Failed  int                 `json:"failed" doc:"Items that failed creation"`
	Skipped int                 `json:"skipped" doc:"Items skipped (duplicates or validation)"`
	Results []bulkAccountResult `json:"results" doc:"Per-item outcomes"`
}

type BulkCreateAccountsOutput struct {
	Body BulkCreateAccountsOutputBody
}

func (s *Handler) HumaBulkCreateAccounts(ctx context.Context, input *BulkCreateAccountsInput) (*BulkCreateAccountsOutput, error) {
	body := input.Body
	if body.Provider == "" {
		return nil, huma.Error400BadRequest("provider is required")
	}
	spec, ok := connectors.SpecByID(body.Provider)
	if !ok {
		return nil, huma.Error400BadRequest("unknown provider: " + body.Provider)
	}
	if s.vault == nil {
		return nil, huma.Error500InternalServerError("vault not configured")
	}
	if len(body.Items) == 0 {
		return nil, huma.Error400BadRequest("items is required")
	}
	if len(body.Items) > bulkMaxItems {
		return nil, huma.Error400BadRequest(fmt.Sprintf("too many items: %d (max %d)", len(body.Items), bulkMaxItems))
	}

	// SSRF Protection
	if body.BaseURL != "" {
		if err := httputil.ValidateBaseURL(body.BaseURL); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", body.BaseURL, "error", err)
			return nil, huma.Error400BadRequest("invalid base_url: URL blocked by security policy")
		}
	}
	if body.AzureEndpoint != "" {
		if err := httputil.ValidateBaseURL(body.AzureEndpoint); err != nil {
			s.log.Warn("blocked suspicious azure_endpoint", "url", body.AzureEndpoint, "error", err)
			return nil, huma.Error400BadRequest("invalid azure_endpoint: URL blocked by security policy")
		}
	}

	results := make([]bulkAccountResult, len(body.Items))
	var seenMu, writeMu sync.Mutex
	seenKeys := map[string]struct{}{}
	sem := make(chan struct{}, bulkValidateConcurrency)
	var wg sync.WaitGroup

	for i, item := range body.Items {
		label := strings.TrimSpace(item.Label)
		key := strings.TrimSpace(item.APIKey)
		results[i] = bulkAccountResult{Index: i, Label: label}

		authKind := accountAuthKind(spec, key)
		if authKind != schema.AuthNone && key == "" {
			results[i].Status, results[i].Error = "error", "api_key is required"
			continue
		}
		seenMu.Lock()
		if key != "" {
			if _, dup := seenKeys[key]; dup {
				seenMu.Unlock()
				results[i].Status, results[i].Error = "skipped", "duplicate api key in batch"
				continue
			}
			seenKeys[key] = struct{}{}
		}
		seenMu.Unlock()

		baseURL := strings.TrimSpace(item.BaseURL)
		if baseURL == "" {
			baseURL = body.BaseURL
		}
		if baseURL != "" {
			if err := httputil.ValidateBaseURL(baseURL); err != nil {
				results[i].Status, results[i].Error = "error", "invalid base_url: URL blocked by security policy"
				continue
			}
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(i int, label, key, baseURL string, authKind schema.AuthKind) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := providerAccountMetadata(spec, providerMetadataInput{
				BaseURL: baseURL,
				AzureEndpoint: body.AzureEndpoint, AzureDeployment: body.AzureDeployment,
				AzureAPIVersion: body.AzureAPIVersion, AzureOrganization: body.AzureOrganization,
			})
			if err != nil {
				results[i].Status, results[i].Error = "error", err.Error()
				return
			}

			now := time.Now()
			displayLabel := label
			if displayLabel == "" {
				displayLabel = fmt.Sprintf("%s-%d", spec.DisplayName, i+1)
			}
			acc := schema.Account{
				ID: uuid.NewString(), TenantID: adminTenant, Provider: body.Provider,
				Label: displayLabel, AuthKind: string(authKind), Priority: defaultInt(body.Priority, 100),
				CreatedAt: now, UpdatedAt: now,
			}
			if body.ProxyPoolID != "" {
				acc.ProxyPoolID = body.ProxyPoolID
			}
			if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: key, Metadata: meta}); err != nil {
				results[i].Status, results[i].Error = "error", "vault seal failed"
				return
			}
			if body.Validate {
				if verr := s.validateAccountCredentials(ctx, acc); verr != nil {
					results[i].Status, results[i].Error = "error", sanitizeError(s.log, verr, "credential validation failed")
					return
				}
			}
			writeMu.Lock()
			err = s.accounts.Create(ctx, acc)
			writeMu.Unlock()
			if err != nil {
				results[i].Status, results[i].Error = "error", sanitizeError(s.log, err, "account creation failed")
				return
			}
			results[i].Status, results[i].ID, results[i].Label = "created", acc.ID, displayLabel
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
	return &BulkCreateAccountsOutput{Body: BulkCreateAccountsOutputBody{
		Total: len(results), Created: created, Failed: failed, Skipped: skipped, Results: results,
	}}, nil
}

// --- Delete Account ---

type DeleteAccountInput struct {
	ID string `path:"id" doc:"Account ID"`
}

type DeleteAccountOutput struct{}

func (s *Handler) HumaDeleteAccount(ctx context.Context, input *DeleteAccountInput) (*DeleteAccountOutput, error) {
	if err := s.accounts.Delete(ctx, input.ID); err != nil {
		return nil, MapError(err)
	}
	return &DeleteAccountOutput{}, nil
}

// --- Validate Key ---

type ValidateKeyBody struct {
	Provider          string `json:"provider" minLength:"1" doc:"Provider ID" example:"openai"`
	APIKey            string `json:"api_key,omitempty" doc:"API key" format:"password"`
	BaseURL           string `json:"base_url,omitempty" doc:"Base URL override" format:"uri"`
	AzureEndpoint     string `json:"azure_endpoint,omitempty" hidden:"true" doc:"Azure OpenAI endpoint URL" format:"uri"`
	AzureDeployment   string `json:"azure_deployment,omitempty" hidden:"true" doc:"Azure OpenAI deployment name"`
	AzureAPIVersion   string `json:"azure_api_version,omitempty" hidden:"true" doc:"Azure OpenAI API version" example:"2024-02-01"`
	AzureOrganization string `json:"azure_organization,omitempty" hidden:"true" doc:"Azure organization"`
}

type ValidateKeyInput struct {
	Body ValidateKeyBody
}

type ValidateKeyOutputBody struct {
	Status  string `json:"status" enum:"ok,failed" doc:"Validation result"`
	Message string `json:"message,omitempty" doc:"Error message if validation failed"`
}

type ValidateKeyOutput struct {
	Body ValidateKeyOutputBody
}

func (s *Handler) HumaValidateKey(ctx context.Context, input *ValidateKeyInput) (*ValidateKeyOutput, error) {
	body := input.Body
	if body.Provider == "" {
		return nil, huma.Error400BadRequest("provider is required")
	}
	spec, ok := connectors.SpecByID(body.Provider)
	if !ok {
		return nil, huma.Error400BadRequest("unknown provider: " + body.Provider)
	}
	authKind := accountAuthKind(spec, body.APIKey)
	if authKind != schema.AuthNone && strings.TrimSpace(body.APIKey) == "" {
		return nil, huma.Error400BadRequest("provider and api_key are required")
	}
	if s.vault == nil || s.conns == nil {
		return nil, huma.Error500InternalServerError("vault or connectors not configured")
	}

	// SSRF Protection
	if body.BaseURL != "" {
		if err := httputil.ValidateBaseURL(body.BaseURL); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", body.BaseURL, "error", err)
			return nil, huma.Error400BadRequest("invalid base_url: URL blocked by security policy")
		}
	}
	if body.AzureEndpoint != "" {
		if err := httputil.ValidateBaseURL(body.AzureEndpoint); err != nil {
			s.log.Warn("blocked suspicious azure_endpoint", "url", body.AzureEndpoint, "error", err)
			return nil, huma.Error400BadRequest("invalid azure_endpoint: URL blocked by security policy")
		}
	}
	meta, err := providerAccountMetadata(spec, providerMetadataInput{
		BaseURL: body.BaseURL,
		AzureEndpoint: body.AzureEndpoint, AzureDeployment: body.AzureDeployment,
		AzureAPIVersion: body.AzureAPIVersion, AzureOrganization: body.AzureOrganization,
	})
	if err != nil {
		return &ValidateKeyOutput{Body: ValidateKeyOutputBody{Status: "error", Message: err.Error()}}, nil
	}

	acc := schema.Account{
		ID: "validate-temp", Provider: body.Provider, AuthKind: string(authKind),
	}
	if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: body.APIKey, Metadata: meta}); err != nil {
		return nil, MapError(err)
	}
	if verr := s.validateAccountCredentials(ctx, acc); verr != nil {
		return &ValidateKeyOutput{Body: ValidateKeyOutputBody{Status: "error", Message: verr.Error()}}, nil
	}
	return &ValidateKeyOutput{Body: ValidateKeyOutputBody{Status: "ok"}}, nil
}

// --- Update Account ---

type UpdateAccountBody struct {
	Label    *string `json:"label,omitempty" doc:"Account label"`
	Priority *int    `json:"priority,omitempty" doc:"Routing priority (0-1000)" minimum:"0" maximum:"1000"`
	Disabled *bool   `json:"disabled,omitempty" doc:"Disable/enable this account"`
}

type UpdateAccountInput struct {
	ID   string `path:"id" doc:"Account ID"`
	Body UpdateAccountBody
}

type UpdateAccountOutputBody struct {
	ID       string `json:"id" doc:"Account UUID"`
	Provider string `json:"provider" doc:"Provider ID"`
	Label    string `json:"label" doc:"Account label"`
	Priority int    `json:"priority" doc:"Routing priority"`
	Disabled bool   `json:"disabled" doc:"Whether account is disabled"`
}

type UpdateAccountOutput struct {
	Body UpdateAccountOutputBody
}

func (s *Handler) HumaUpdateAccount(ctx context.Context, input *UpdateAccountInput) (*UpdateAccountOutput, error) {
	acc, err := s.accounts.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("account not found")
	}
	body := input.Body
	if body.Label != nil {
		acc.Label = *body.Label
	}
	if body.Priority != nil {
		acc.Priority = *body.Priority
	}
	if body.Disabled != nil {
		acc.Disabled = *body.Disabled
	}
	if err := s.accounts.Update(ctx, acc); err != nil {
		return nil, MapError(err)
	}
	return &UpdateAccountOutput{Body: UpdateAccountOutputBody{
		ID: acc.ID, Provider: acc.Provider, Label: acc.Label, Priority: acc.Priority, Disabled: acc.Disabled,
	}}, nil
}

// --- Test Account ---

type TestAccountInput struct {
	ID string `path:"id" doc:"Account ID"`
}

type TestAccountOutputBody struct {
	ID        string `json:"id" doc:"Account ID"`
	Provider  string `json:"provider" doc:"Provider type"`
	Label     string `json:"label" doc:"Account label"`
	Status    string `json:"status" enum:"ok,failed,timeout,unauthorized" doc:"Test result"`
	LatencyMs int64  `json:"latency_ms" doc:"Response latency in milliseconds"`
	Message   string `json:"message,omitempty" doc:"Error message if status is not ok"`
}

type TestAccountOutput struct {
	Body TestAccountOutputBody
}

func (s *Handler) HumaTestAccount(ctx context.Context, input *TestAccountInput) (*TestAccountOutput, error) {
	acc, err := s.accounts.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("account not found")
	}
	r := openapi.RequestFromContext(ctx)
	start := time.Now()
	verr := s.validateAccountCredentials(r.Context(), acc)
	latency := time.Since(start).Milliseconds()
	if verr != nil {
		return &TestAccountOutput{Body: TestAccountOutputBody{
			ID: acc.ID, Provider: acc.Provider, Label: acc.Label, Status: "failed", LatencyMs: latency, Message: verr.Error(),
		}}, nil
	}
	if acc.NeedsReconnect {
		if err := s.accounts.SetNeedsReconnect(ctx, acc.ID, false); err != nil {
			s.log.Warn("failed to clear needs_reconnect after successful test", "account", acc.ID, "err", err)
		}
	}
	return &TestAccountOutput{Body: TestAccountOutputBody{
		ID: acc.ID, Provider: acc.Provider, Label: acc.Label, Status: "ok", LatencyMs: latency,
	}}, nil
}

// --- Account Quota ---

// QuotaItem is a single quota entry in the quota response.
type QuotaItem struct {
	Name  string `json:"name" doc:"Quota name (e.g. requests_per_day)"`
	Limit any    `json:"limit" doc:"Quota limit (null = unlimited)"`
	Used  any    `json:"used" doc:"Current usage"`
	Reset string `json:"reset_at,omitempty" doc:"Next reset timestamp"`
}

type AccountQuotaInput struct {
	ID string `path:"id" doc:"Account ID"`
}

type AccountQuotaOutputBody struct {
	Provider  string `json:"provider" doc:"Provider type"`
	Supported bool   `json:"supported" doc:"Whether quota fetching is supported"`
	PlanName  string `json:"plan_name,omitempty" doc:"Plan or tier name"`
	Message   string `json:"message,omitempty" doc:"Additional information"`
	Quotas    []QuotaItem `json:"quotas,omitempty" doc:"Account quotas"`
}

type AccountQuotaOutput struct {
	Body AccountQuotaOutputBody
}

func (s *Handler) HumaAccountQuota(ctx context.Context, input *AccountQuotaInput) (*AccountQuotaOutput, error) {
	acc, err := s.accounts.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("account not found")
	}
	qs := connectors.GetQuotaSource(acc.Provider)
	if qs == nil {
		return &AccountQuotaOutput{Body: AccountQuotaOutputBody{
			Provider: acc.Provider, Supported: false, Message: "upstream quota not available for this provider",
		}}, nil
	}
	if s.vault == nil {
		return nil, huma.Error500InternalServerError("vault not configured")
	}
	creds, err := s.vault.Open(acc)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not decrypt credentials")
	}
	qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	quota, qerr := qs.FetchQuota(qctx, creds)
	if qerr != nil {
		return nil, huma.Error502BadGateway(qerr.Error())
	}
	quotas := make([]QuotaItem, 0, len(quota.Quotas))
	for _, q := range quota.Quotas {
		quotas = append(quotas, QuotaItem{
			Name: q.ResourceType, Limit: q.Limit, Used: q.Used, Reset: q.ResetAt,
		})
	}
	return &AccountQuotaOutput{Body: AccountQuotaOutputBody{
		Provider: acc.Provider, Supported: true, PlanName: quota.PlanName, Message: quota.Message, Quotas: quotas,
	}}, nil
}

// --- List Keys ---

type ListKeysInput struct{}

type ListKeysOutput struct {
	Body struct {
		Keys []map[string]any `json:"keys"`
	}
}

func (s *Handler) HumaListKeys(ctx context.Context, _ *ListKeysInput) (*ListKeysOutput, error) {
	keys, err := s.identity.List(ctx, adminTenant)
	if err != nil {
		return nil, MapError(err)
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		entry := map[string]any{
			"id": k.ID, "name": k.Name, "display": k.Display,
			"disabled": k.Disabled, "plan_id": k.PlanID, "created_at": k.CreatedAt,
		}
		// Resolve plan name.
		if k.PlanID != "" {
			if plan, perr := s.db.Plans().Get(ctx, k.PlanID); perr == nil {
				entry["plan_name"] = plan.Name
			}
		}
		// Attach allowed models (empty = all allowed).
		if models, merr := s.identity.Keys().GetAllowedModels(ctx, k.ID); merr == nil {
			entry["allowed_models"] = models
		}
		out = append(out, entry)
	}
	return &ListKeysOutput{Body: struct {
		Keys []map[string]any `json:"keys"`
	}{Keys: out}}, nil
}

// --- Create Key ---

type CreateKeyBody struct {
	Name              string   `json:"name" minLength:"1" doc:"Key name"`
		ProjectID         string   `json:"project_id,omitempty" doc:"Project ID"`
		PlanID            string   `json:"plan_id,omitempty" doc:"Plan ID"`
		BudgetLimitUSD    *float64 `json:"budget_limit_usd,omitempty" doc:"Budget limit in USD"`
		BudgetLimitTokens *int64   `json:"budget_limit_tokens,omitempty" doc:"Budget limit in tokens"`
		BudgetPeriod      string   `json:"budget_period,omitempty" doc:"Budget period: daily, weekly, monthly, total"`
		BudgetAlertPct    *int     `json:"budget_alert_pct,omitempty" doc:"Budget alert percentage"`
		BudgetHardCutoff  *bool    `json:"budget_hard_cutoff,omitempty" doc:"Hard cutoff when budget exceeded"`
	AllowedModels     []string `json:"allowed_models,omitempty" doc:"Allowed models"`
}

type CreateKeyInput struct {
	Body CreateKeyBody
}

type CreateKeyOutputBody struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Key           string         `json:"key"`
	Display       string         `json:"display"`
	PlanID        string         `json:"plan_id,omitempty"`
	Budget        map[string]any `json:"budget,omitempty"`
	AllowedModels []string       `json:"allowed_models,omitempty"`
	Plan          map[string]any `json:"plan,omitempty"`
}

type CreateKeyOutput struct {
	Body CreateKeyOutputBody
}

func (s *Handler) HumaCreateKey(ctx context.Context, input *CreateKeyInput) (*CreateKeyOutput, error) {
	body := input.Body
	if body.Name == "" {
		return nil, huma.Error400BadRequest("name is required")
	}
	if body.BudgetLimitUSD != nil && *body.BudgetLimitUSD < 0 {
		return nil, huma.Error400BadRequest("budget_limit_usd must not be negative")
	}
	if body.BudgetLimitTokens != nil && *body.BudgetLimitTokens < 0 {
		return nil, huma.Error400BadRequest("budget_limit_tokens must not be negative")
	}

	// Resolve plan if one was specified.
	var plan *schema.Plan
	if body.PlanID != "" {
		p, err := s.db.Plans().Get(ctx, body.PlanID)
		if err != nil {
			if errors.Is(err, schema.ErrNotFound) {
				return nil, huma.Error400BadRequest("plan not found")
			}
			return nil, MapError(err)
		}
		plan = &p
	}

	// Generate key material (crypto operations, no DB write yet).
	issued, err := s.identity.Generate(adminTenant, body.ProjectID, body.Name)
	if err != nil {
		return nil, MapError(err)
	}

	// Set plan_id on the key record.
	if plan != nil {
		issued.Record.PlanID = plan.ID
	}

	// Determine effective budget: per-key overrides > plan defaults.
	hasPerKeyBudget := (body.BudgetLimitUSD != nil && *body.BudgetLimitUSD > 0) ||
		(body.BudgetLimitTokens != nil && *body.BudgetLimitTokens > 0)
	hasPlanBudget := plan != nil && (plan.LimitMicros > 0 || plan.LimitTokens > 0)
	hasBudget := hasPerKeyBudget || hasPlanBudget
	hasPerKeyModels := len(body.AllowedModels) > 0
	hasPlanModels := plan != nil && plan.AllowedModels != ""
	hasModels := hasPerKeyModels || hasPlanModels

	if !hasBudget && !hasModels && plan == nil {
		// Simple path: no budget, no models, no plan — insert key directly.
		if err := s.identity.CreateFromIssued(ctx, issued); err != nil {
			return nil, MapError(err)
		}
		return &CreateKeyOutput{Body: struct {
			ID            string         `json:"id"`
			Name          string         `json:"name"`
			Key           string         `json:"key"`
			Display       string         `json:"display"`
			PlanID        string         `json:"plan_id,omitempty"`
			Budget        map[string]any `json:"budget,omitempty"`
			AllowedModels []string       `json:"allowed_models,omitempty"`
			Plan          map[string]any `json:"plan,omitempty"`
		}{
			ID: issued.Record.ID, Name: issued.Record.Name,
			Key: issued.Plaintext, Display: issued.Record.Display, PlanID: issued.Record.PlanID,
		}}, nil
	}

	// Transactional path: key + budget + model access atomically.
	tx := s.db.BeginTx(ctx)
	if err := tx.Error; err != nil {
		return nil, huma.Error500InternalServerError("transaction start failed")
	}
	defer func() { _ = tx.Rollback() }() // no-op after commit

	if err := s.identity.Keys().CreateOnTx(ctx, tx, issued.Record); err != nil {
		return nil, MapError(err)
	}

	var budgetRec schema.Budget
	if hasBudget {
		// Resolve effective values: per-key overrides win, then plan, then defaults.
		hardCutoff := true
		alertPct := 80
		period := "monthly"
		var limitMicros int64
		var limitTokens int64

		if plan != nil {
			hardCutoff = plan.HardCutoff
			alertPct = plan.AlertPct
			period = plan.Period
			limitMicros = plan.LimitMicros
			limitTokens = plan.LimitTokens
		}

		// Per-key overrides take precedence.
		if body.BudgetHardCutoff != nil {
			hardCutoff = *body.BudgetHardCutoff
		}
		if body.BudgetAlertPct != nil {
			alertPct = *body.BudgetAlertPct
		}
		if body.BudgetPeriod != "" {
			if p, ok := normalizeBudgetPeriod(body.BudgetPeriod); ok {
				period = p
			}
		}
		if body.BudgetLimitUSD != nil && *body.BudgetLimitUSD > 0 {
			limitMicros = int64(*body.BudgetLimitUSD * 1_000_000)
		}
		if body.BudgetLimitTokens != nil && *body.BudgetLimitTokens > 0 {
			limitTokens = *body.BudgetLimitTokens
		}

		if limitMicros <= 0 && limitTokens <= 0 {
			// Plan had no limits and no per-key overrides — skip budget creation.
			hasBudget = false
		} else {
			if alertPct < 1 || alertPct > 100 {
				return nil, huma.Error400BadRequest("budget_alert_pct must be between 1 and 100")
			}

			now := time.Now()
			budgetRec = schema.Budget{
				ID:          uuid.NewString(),
				TenantID:    adminTenant,
				ScopeKind:   string(schema.ScopeAPIKey),
				ScopeID:     issued.Record.ID,
				LimitMicros: limitMicros,
				LimitTokens: limitTokens,
				Period:      period,
				AlertPct:    alertPct,
				HardCutoff:  hardCutoff,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := s.budgets.CreateOnTx(ctx, tx, budgetRec); err != nil {
				return nil, MapError(err)
			}
		}
	}

	if hasModels {
		// Per-key models take precedence over plan models.
		effectiveModels := body.AllowedModels
		if !hasPerKeyModels && plan != nil {
			effectiveModels = schema.GetPlanAllowedModels(*plan)
		}
		if len(effectiveModels) > 0 {
			if err := s.identity.Keys().SetAllowedModelsOnTx(ctx, tx, issued.Record.ID, effectiveModels); err != nil {
				return nil, MapError(err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, huma.Error500InternalServerError("transaction commit failed")
	}

	// Invalidate the budget definition cache so the next request picks up
	// the newly-created budget immediately.
	if hasBudget && s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}

	resp := CreateKeyOutput{Body: struct {
		ID            string         `json:"id"`
		Name          string         `json:"name"`
		Key           string         `json:"key"`
		Display       string         `json:"display"`
		PlanID        string         `json:"plan_id,omitempty"`
		Budget        map[string]any `json:"budget,omitempty"`
		AllowedModels []string       `json:"allowed_models,omitempty"`
		Plan          map[string]any `json:"plan,omitempty"`
	}{
		ID: issued.Record.ID, Name: issued.Record.Name,
		Key: issued.Plaintext, Display: issued.Record.Display, PlanID: issued.Record.PlanID,
	}}
	if hasBudget {
		resp.Body.Budget = map[string]any{
			"id": budgetRec.ID, "scope_kind": string(budgetRec.ScopeKind),
			"limit_micros": budgetRec.LimitMicros, "limit_tokens": budgetRec.LimitTokens,
			"period": budgetRec.Period, "alert_pct": budgetRec.AlertPct, "hard_cutoff": budgetRec.HardCutoff,
		}
	}
	effectiveModels := body.AllowedModels
	if !hasPerKeyModels && plan != nil {
		effectiveModels = schema.GetPlanAllowedModels(*plan)
	}
	if len(effectiveModels) > 0 {
		resp.Body.AllowedModels = effectiveModels
	}
	if plan != nil {
		resp.Body.Plan = map[string]any{
			"id": plan.ID, "name": plan.Name,
		}
	}
	return &resp, nil
}

// --- Update Key ---

type UpdateKeyBody struct {
	Disabled      *bool    `json:"disabled,omitempty" doc:"Disable/enable key"`
	AllowedModels []string `json:"allowed_models,omitempty" doc:"Allowed models"`
}

type UpdateKeyInput struct {
	ID   string `path:"id" doc:"Key ID"`
	Body UpdateKeyBody
}

type UpdateKeyOutputBody struct {
		ID            string   `json:"id"`
		Disabled      *bool    `json:"disabled,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
}

type UpdateKeyOutput struct {
	Body UpdateKeyOutputBody
}

func (s *Handler) HumaUpdateKey(ctx context.Context, input *UpdateKeyInput) (*UpdateKeyOutput, error) {
	body := input.Body
	if body.Disabled == nil && body.AllowedModels == nil {
		return nil, huma.Error400BadRequest("disabled or allowed_models field is required")
	}
	if body.Disabled != nil {
		if err := s.identity.SetDisabled(ctx, input.ID, *body.Disabled); err != nil {
			return nil, MapError(err)
		}
	}
	if body.AllowedModels != nil {
		if err := s.identity.Keys().SetAllowedModels(ctx, input.ID, body.AllowedModels); err != nil {
			return nil, MapError(err)
		}
	}
	return &UpdateKeyOutput{Body: struct {
		ID            string   `json:"id"`
		Disabled      *bool    `json:"disabled,omitempty"`
		AllowedModels []string `json:"allowed_models,omitempty"`
	}{ID: input.ID, Disabled: body.Disabled, AllowedModels: body.AllowedModels}}, nil
}

// --- Delete Key ---

type DeleteKeyInput struct {
	ID string `path:"id" doc:"Key ID"`
}

type DeleteKeyOutput struct{}

func (s *Handler) HumaDeleteKey(ctx context.Context, input *DeleteKeyInput) (*DeleteKeyOutput, error) {
	if err := s.identity.Delete(ctx, input.ID); err != nil {
		return nil, MapError(err)
	}
	return &DeleteKeyOutput{}, nil
}

// --- Rotate Key ---

type RotateKeyInput struct {
	ID string `path:"id" doc:"Key ID"`
}

type RotateKeyOutputBody struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Key     string `json:"key"`
	Display string `json:"display"`
}

type RotateKeyOutput struct {
	Body RotateKeyOutputBody
}

func (s *Handler) HumaRotateKey(ctx context.Context, input *RotateKeyInput) (*RotateKeyOutput, error) {
	issued, err := s.identity.Rotate(ctx, input.ID)
	if err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			return nil, huma.Error404NotFound("key not found")
		}
		return nil, MapError(err)
	}

	return &RotateKeyOutput{
		Body: RotateKeyOutputBody{
			ID:      issued.Record.ID,
			Name:    issued.Record.Name,
			Key:     issued.Plaintext,
			Display: issued.Record.Display,
		},
	}, nil
}

// --- List Chains ---

type ListChainsInput struct{}

type ListChainsOutput struct {
	Body struct {
		Chains []map[string]any `json:"chains"`
	}
}

func (s *Handler) HumaListChains(ctx context.Context, _ *ListChainsInput) (*ListChainsOutput, error) {
	chains, err := s.chains.ListByTenant(ctx, adminTenant)
	if err != nil {
		s.log.WithError(err).Error("failed to list chains")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	out := make([]map[string]any, 0, len(chains))
	for _, c := range chains {
		steps := make([]map[string]any, 0, len(c.Steps))
		for _, st := range c.Steps {
			steps = append(steps, map[string]any{"provider": st.Provider, "model": st.Model, "position": st.Position})
		}
		entry := map[string]any{
			"id": c.ID, "name": c.Name, "strategy": c.Strategy, "steps": steps,
		}
		if c.FallbackProvider != "" && c.FallbackModel != "" {
			entry["fallback_provider"] = c.FallbackProvider
			entry["fallback_model"] = c.FallbackModel
		}
		out = append(out, entry)
	}

	return &ListChainsOutput{
		Body: struct {
			Chains []map[string]any `json:"chains"`
		}{Chains: out},
	}, nil
}

// --- Create Chain ---

// ChainStep is a single step in a chain definition.
type ChainStep struct {
	Provider string `json:"provider" doc:"Provider ID"`
	Model    string `json:"model" doc:"Model name"`
}


type CreateChainBody struct {
	Name             string `json:"name" doc:"Chain name"`
		Strategy         string `json:"strategy,omitempty" doc:"Strategy (priority, round-robin, etc.)"`
		FallbackProvider string `json:"fallback_provider,omitempty" doc:"Fallback provider ID"`
		FallbackModel    string `json:"fallback_model,omitempty" doc:"Fallback model name"`
	Steps            []ChainStep `json:"steps" doc:"Chain steps"`
}

type CreateChainInput struct {
	Body CreateChainBody
}

type CreateChainOutputBody struct {
		ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateChainOutput struct {
	Body CreateChainOutputBody
}

func (s *Handler) HumaCreateChain(ctx context.Context, input *CreateChainInput) (*CreateChainOutput, error) {
	if input.Body.Name == "" || len(input.Body.Steps) == 0 {
		return nil, huma.Error400BadRequest("name and at least one step are required")
	}

	if err := validateChainName(input.Body.Name); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Validate fallback provider if set.
	if input.Body.FallbackProvider != "" {
		if _, ok := connectors.SpecByID(input.Body.FallbackProvider); !ok {
			return nil, huma.Error400BadRequest("unknown fallback provider: " + input.Body.FallbackProvider)
		}
		if input.Body.FallbackModel == "" {
			return nil, huma.Error400BadRequest("fallback_model is required when fallback_provider is set")
		}
	}

	now := time.Now()
	chain := schema.Chain{
		ID:               uuid.NewString(),
		TenantID:         adminTenant,
		Name:             input.Body.Name,
		Strategy:         defaultStr(input.Body.Strategy, "priority"),
		FallbackProvider: input.Body.FallbackProvider,
		FallbackModel:    input.Body.FallbackModel,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	for i, st := range input.Body.Steps {
		if _, ok := connectors.SpecByID(st.Provider); !ok {
			return nil, huma.Error400BadRequest("unknown provider in step: " + st.Provider)
		}
		chain.Steps = append(chain.Steps, schema.ChainStep{
			ID: uuid.NewString(), ChainID: chain.ID, Position: i,
			Provider: st.Provider, Model: st.Model, CreatedAt: now,
		})
	}

	if err := s.chains.Create(ctx, chain); err != nil {
		s.log.WithError(err).Error("failed to create chain")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	return &CreateChainOutput{
		Body: struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{ID: chain.ID, Name: chain.Name},
	}, nil
}

// --- Update Chain ---

type UpdateChainBody struct {
	Name             *string `json:"name,omitempty" doc:"Chain name"`
		Strategy         *string `json:"strategy,omitempty" doc:"Strategy"`
		FallbackProvider *string `json:"fallback_provider,omitempty" doc:"Fallback provider ID"`
		FallbackModel    *string `json:"fallback_model,omitempty" doc:"Fallback model name"`
	Steps            *[]ChainStep `json:"steps,omitempty" doc:"Chain steps"`
}

type UpdateChainInput struct {
	ID   string `path:"id" doc:"Chain ID"`
	Body UpdateChainBody
}

type UpdateChainOutputBody struct {
		ID   string `json:"id"`
	Name string `json:"name"`
}

type UpdateChainOutput struct {
	Body UpdateChainOutputBody
}

func (s *Handler) HumaUpdateChain(ctx context.Context, input *UpdateChainInput) (*UpdateChainOutput, error) {
	existing, err := s.chains.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("chain not found")
	}

	if input.Body.Name != nil {
		if err := validateChainName(*input.Body.Name); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		existing.Name = *input.Body.Name
	}
	if input.Body.Strategy != nil {
		existing.Strategy = *input.Body.Strategy
	}
	if input.Body.FallbackProvider != nil {
		existing.FallbackProvider = *input.Body.FallbackProvider
	}
	if input.Body.FallbackModel != nil {
		existing.FallbackModel = *input.Body.FallbackModel
	}
	if input.Body.Steps != nil {
		now := time.Now()
		existing.Steps = make([]schema.ChainStep, len(*input.Body.Steps))
		for i, st := range *input.Body.Steps {
			existing.Steps[i] = schema.ChainStep{
				ID:        uuid.NewString(),
				ChainID:   input.ID,
				Position:  i,
				Provider:  st.Provider,
				Model:     st.Model,
				CreatedAt: now,
			}
		}
	}

	if err := s.chains.Update(ctx, existing); err != nil {
		s.log.WithError(err).Error("failed to update chain")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	return &UpdateChainOutput{
		Body: struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{ID: existing.ID, Name: existing.Name},
	}, nil
}

// --- Delete Chain ---

type DeleteChainInput struct {
	ID string `path:"id" doc:"Chain ID"`
}

type DeleteChainOutput struct{}

func (s *Handler) HumaDeleteChain(ctx context.Context, input *DeleteChainInput) (*DeleteChainOutput, error) {
	if err := s.chains.Delete(ctx, input.ID); err != nil {
		s.log.WithError(err).Error("failed to delete chain")
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &DeleteChainOutput{}, nil
}

// --- List Plans ---

type ListPlansInput struct{}

type ListPlansOutput struct {
	Body struct {
		Plans []map[string]any `json:"plans"`
	}
}

func (s *Handler) HumaListPlans(ctx context.Context, _ *ListPlansInput) (*ListPlansOutput, error) {
	plans, err := s.db.Plans().List(ctx, adminTenant)
	if err != nil {
		s.log.WithError(err).Error("failed to list plans")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	out := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		keyCount, _ := s.db.Plans().CountKeys(ctx, p.ID)
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "description": p.Description,
			"limit_micros": p.LimitMicros, "limit_tokens": p.LimitTokens,
			"rpm_limit": p.RPMLimit, "tpm_limit": p.TPMLimit, "concurrency_limit": p.ConcurrencyLimit,
			"period": p.Period, "alert_pct": p.AlertPct, "hard_cutoff": p.HardCutoff,
			"allowed_models": schema.GetPlanAllowedModels(p),
			"key_count":      keyCount,
			"created_at":     p.CreatedAt, "updated_at": p.UpdatedAt,
		})
	}

	return &ListPlansOutput{
		Body: struct {
			Plans []map[string]any `json:"plans"`
		}{Plans: out},
	}, nil
}

// --- Create Plan ---

type CreatePlanBody struct {
	Name             string   `json:"name" doc:"Plan name"`
	Description      string   `json:"description,omitempty" doc:"Plan description"`
	LimitUSD         float64  `json:"limit_usd" doc:"Cost limit in USD"`
	LimitTokens      int64    `json:"limit_tokens" doc:"Token limit"`
	RPMLimit         int64    `json:"rpm_limit" doc:"Requests per minute limit"`
	TPMLimit         int64    `json:"tpm_limit" doc:"Tokens per minute limit"`
	ConcurrencyLimit int64    `json:"concurrency_limit" doc:"Concurrent requests limit"`
	Period           string   `json:"period,omitempty" doc:"Budget period (daily, weekly, monthly, total)"`
	AlertPct         int      `json:"alert_pct,omitempty" doc:"Alert threshold percentage (1-100)"`
	HardCutoff       *bool    `json:"hard_cutoff,omitempty" doc:"Hard cutoff when limit reached"`
	AllowedModels    []string `json:"allowed_models,omitempty" doc:"Allowed model names"`
}

type CreatePlanInput struct {
	Body CreatePlanBody
}

type CreatePlanOutput struct {
	Body struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		Description      string   `json:"description"`
		LimitMicros      int64    `json:"limit_micros"`
		LimitTokens      int64    `json:"limit_tokens"`
		RPMLimit         int64    `json:"rpm_limit"`
		TPMLimit         int64    `json:"tpm_limit"`
		ConcurrencyLimit int64    `json:"concurrency_limit"`
		Period           string   `json:"period"`
		AlertPct         int      `json:"alert_pct"`
		HardCutoff       bool     `json:"hard_cutoff"`
		AllowedModels    []string `json:"allowed_models"`
	}
}

func (s *Handler) HumaCreatePlan(ctx context.Context, input *CreatePlanInput) (*CreatePlanOutput, error) {
	if input.Body.Name == "" {
		return nil, huma.Error400BadRequest("name is required")
	}
	if input.Body.LimitUSD < 0 {
		return nil, huma.Error400BadRequest("limit_usd must not be negative")
	}
	if input.Body.LimitTokens < 0 {
		return nil, huma.Error400BadRequest("limit_tokens must not be negative")
	}
	if input.Body.RPMLimit < 0 || input.Body.TPMLimit < 0 || input.Body.ConcurrencyLimit < 0 {
		return nil, huma.Error400BadRequest("rate limits must not be negative")
	}

	period, ok := normalizeBudgetPeriod(input.Body.Period)
	if !ok {
		return nil, huma.Error400BadRequest("invalid period")
	}

	alertPct := defaultInt(input.Body.AlertPct, 80)
	if alertPct < 1 || alertPct > 100 {
		return nil, huma.Error400BadRequest("alert_pct must be between 1 and 100")
	}

	hardCutoff := true
	if input.Body.HardCutoff != nil {
		hardCutoff = *input.Body.HardCutoff
	}

	now := time.Now()
	p := schema.Plan{
		ID:               uuid.NewString(),
		TenantID:         adminTenant,
		Name:             input.Body.Name,
		Description:      input.Body.Description,
		LimitMicros:      int64(input.Body.LimitUSD * 1_000_000),
		LimitTokens:      input.Body.LimitTokens,
		RPMLimit:         input.Body.RPMLimit,
		TPMLimit:         input.Body.TPMLimit,
		ConcurrencyLimit: input.Body.ConcurrencyLimit,
		Period:           period,
		AlertPct:         alertPct,
		HardCutoff:       hardCutoff,
		AllowedModels:    schema.SetPlanAllowedModels(input.Body.AllowedModels),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.db.Plans().Create(ctx, p); err != nil {
		s.log.WithError(err).Error("failed to create plan")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	return &CreatePlanOutput{
		Body: struct {
			ID               string   `json:"id"`
			Name             string   `json:"name"`
			Description      string   `json:"description"`
			LimitMicros      int64    `json:"limit_micros"`
			LimitTokens      int64    `json:"limit_tokens"`
			RPMLimit         int64    `json:"rpm_limit"`
			TPMLimit         int64    `json:"tpm_limit"`
			ConcurrencyLimit int64    `json:"concurrency_limit"`
			Period           string   `json:"period"`
			AlertPct         int      `json:"alert_pct"`
			HardCutoff       bool     `json:"hard_cutoff"`
			AllowedModels    []string `json:"allowed_models"`
		}{
			ID: p.ID, Name: p.Name, Description: p.Description,
			LimitMicros: p.LimitMicros, LimitTokens: p.LimitTokens,
			RPMLimit: p.RPMLimit, TPMLimit: p.TPMLimit, ConcurrencyLimit: p.ConcurrencyLimit,
			Period: p.Period, AlertPct: p.AlertPct, HardCutoff: p.HardCutoff,
			AllowedModels: schema.GetPlanAllowedModels(p),
		},
	}, nil
}

// --- Update Plan ---

type UpdatePlanBody struct {
	Name             *string  `json:"name,omitempty" doc:"Plan name"`
	Description      *string  `json:"description,omitempty" doc:"Plan description"`
	LimitUSD         *float64 `json:"limit_usd,omitempty" doc:"Cost limit in USD"`
	LimitTokens      *int64   `json:"limit_tokens,omitempty" doc:"Token limit"`
	RPMLimit         *int64   `json:"rpm_limit,omitempty" doc:"Requests per minute limit"`
	TPMLimit         *int64   `json:"tpm_limit,omitempty" doc:"Tokens per minute limit"`
	ConcurrencyLimit *int64   `json:"concurrency_limit,omitempty" doc:"Concurrent requests limit"`
	Period           *string  `json:"period,omitempty" doc:"Budget period"`
	AlertPct         *int     `json:"alert_pct,omitempty" doc:"Alert threshold percentage"`
	HardCutoff       *bool    `json:"hard_cutoff,omitempty" doc:"Hard cutoff when limit reached"`
	AllowedModels    []string `json:"allowed_models,omitempty" doc:"Allowed model names"`
}

type UpdatePlanInput struct {
	ID   string `path:"id" doc:"Plan ID"`
	Body UpdatePlanBody
}

type UpdatePlanOutput struct {
	Body struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		Description      string   `json:"description"`
		LimitMicros      int64    `json:"limit_micros"`
		LimitTokens      int64    `json:"limit_tokens"`
		RPMLimit         int64    `json:"rpm_limit"`
		TPMLimit         int64    `json:"tpm_limit"`
		ConcurrencyLimit int64    `json:"concurrency_limit"`
		Period           string   `json:"period"`
		AlertPct         int      `json:"alert_pct"`
		HardCutoff       bool     `json:"hard_cutoff"`
		AllowedModels    []string `json:"allowed_models"`
	}
}

func (s *Handler) HumaUpdatePlan(ctx context.Context, input *UpdatePlanInput) (*UpdatePlanOutput, error) {
	existing, err := s.db.Plans().Get(ctx, input.ID)
	if err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			return nil, huma.Error404NotFound("plan not found")
		}
		s.log.WithError(err).Error("failed to get plan")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	if input.Body.Name != nil {
		if *input.Body.Name == "" {
			return nil, huma.Error400BadRequest("name cannot be empty")
		}
		existing.Name = *input.Body.Name
	}
	if input.Body.Description != nil {
		existing.Description = *input.Body.Description
	}
	if input.Body.LimitUSD != nil {
		if *input.Body.LimitUSD < 0 {
			return nil, huma.Error400BadRequest("limit_usd must not be negative")
		}
		existing.LimitMicros = int64(*input.Body.LimitUSD * 1_000_000)
	}
	if input.Body.LimitTokens != nil {
		if *input.Body.LimitTokens < 0 {
			return nil, huma.Error400BadRequest("limit_tokens must not be negative")
		}
		existing.LimitTokens = *input.Body.LimitTokens
	}
	if input.Body.RPMLimit != nil {
		if *input.Body.RPMLimit < 0 {
			return nil, huma.Error400BadRequest("rpm_limit must not be negative")
		}
		existing.RPMLimit = *input.Body.RPMLimit
	}
	if input.Body.TPMLimit != nil {
		if *input.Body.TPMLimit < 0 {
			return nil, huma.Error400BadRequest("tpm_limit must not be negative")
		}
		existing.TPMLimit = *input.Body.TPMLimit
	}
	if input.Body.ConcurrencyLimit != nil {
		if *input.Body.ConcurrencyLimit < 0 {
			return nil, huma.Error400BadRequest("concurrency_limit must not be negative")
		}
		existing.ConcurrencyLimit = *input.Body.ConcurrencyLimit
	}
	if input.Body.Period != nil {
		period, ok := normalizeBudgetPeriod(*input.Body.Period)
		if !ok {
			return nil, huma.Error400BadRequest("invalid period")
		}
		existing.Period = period
	}
	if input.Body.AlertPct != nil {
		if *input.Body.AlertPct < 1 || *input.Body.AlertPct > 100 {
			return nil, huma.Error400BadRequest("alert_pct must be between 1 and 100")
		}
		existing.AlertPct = *input.Body.AlertPct
	}
	if input.Body.HardCutoff != nil {
		existing.HardCutoff = *input.Body.HardCutoff
	}
	if input.Body.AllowedModels != nil {
		existing.AllowedModels = schema.SetPlanAllowedModels(input.Body.AllowedModels)
	}
	existing.UpdatedAt = time.Now()

	if err := s.db.Plans().Update(ctx, existing); err != nil {
		s.log.WithError(err).Error("failed to update plan")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	return &UpdatePlanOutput{
		Body: struct {
			ID               string   `json:"id"`
			Name             string   `json:"name"`
			Description      string   `json:"description"`
			LimitMicros      int64    `json:"limit_micros"`
			LimitTokens      int64    `json:"limit_tokens"`
			RPMLimit         int64    `json:"rpm_limit"`
			TPMLimit         int64    `json:"tpm_limit"`
			ConcurrencyLimit int64    `json:"concurrency_limit"`
			Period           string   `json:"period"`
			AlertPct         int      `json:"alert_pct"`
			HardCutoff       bool     `json:"hard_cutoff"`
			AllowedModels    []string `json:"allowed_models"`
		}{
			ID: existing.ID, Name: existing.Name, Description: existing.Description,
			LimitMicros: existing.LimitMicros, LimitTokens: existing.LimitTokens,
			RPMLimit: existing.RPMLimit, TPMLimit: existing.TPMLimit, ConcurrencyLimit: existing.ConcurrencyLimit,
			Period: existing.Period, AlertPct: existing.AlertPct, HardCutoff: existing.HardCutoff,
			AllowedModels: schema.GetPlanAllowedModels(existing),
		},
	}, nil
}

// --- Delete Plan ---

type DeletePlanInput struct {
	ID string `path:"id" doc:"Plan ID"`
}

type DeletePlanOutput struct{}

func (s *Handler) HumaDeletePlan(ctx context.Context, input *DeletePlanInput) (*DeletePlanOutput, error) {
	keyCount, err := s.db.Plans().CountKeys(ctx, input.ID)
	if err != nil {
		s.log.WithError(err).Error("failed to count plan keys")
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if keyCount > 0 {
		return nil, huma.Error409Conflict(fmt.Sprintf("plan has %d API key(s) assigned — reassign or delete them first", keyCount))
	}

	if err := s.db.Plans().Delete(ctx, input.ID); err != nil {
		s.log.WithError(err).Error("failed to delete plan")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	return &DeletePlanOutput{}, nil
}

// --- List Plan Keys ---

type ListPlanKeysInput struct {
	ID string `path:"id" doc:"Plan ID"`
}

type ListPlanKeysOutput struct {
	Body struct {
		Keys []map[string]any `json:"keys"`
	}
}

func (s *Handler) HumaListPlanKeys(ctx context.Context, input *ListPlanKeysInput) (*ListPlanKeysOutput, error) {
	if _, err := s.db.Plans().Get(ctx, input.ID); err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			return nil, huma.Error404NotFound("plan not found")
		}
		s.log.WithError(err).Error("failed to get plan")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	keys, err := s.identity.List(ctx, adminTenant)
	if err != nil {
		s.log.WithError(err).Error("failed to list keys")
		return nil, huma.Error500InternalServerError("internal server error")
	}

	var out []map[string]any
	for _, k := range keys {
		if k.PlanID == input.ID {
			entry := map[string]any{
				"id": k.ID, "name": k.Name, "display": k.Display,
				"disabled": k.Disabled, "created_at": k.CreatedAt,
			}
			if models, merr := s.identity.Keys().GetAllowedModels(ctx, k.ID); merr == nil {
				entry["allowed_models"] = models
			}
			out = append(out, entry)
		}
	}

	return &ListPlanKeysOutput{
		Body: struct {
			Keys []map[string]any `json:"keys"`
		}{Keys: out},
	}, nil
}

// --- List Budgets ---

type ListBudgetsInput struct{}

type ListBudgetsOutput struct {
	Body struct {
		Budgets []map[string]any `json:"budgets"`
	}
}

func (s *Handler) HumaListBudgets(ctx context.Context, _ *ListBudgetsInput) (*ListBudgetsOutput, error) {
	budgets, err := s.budgets.ListByTenant(ctx, adminTenant)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	out := make([]map[string]any, 0, len(budgets))
	for _, b := range budgets {
		out = append(out, map[string]any{
			"id": b.ID, "scope_kind": b.ScopeKind, "scope_id": b.ScopeID,
			"limit_micros": b.LimitMicros, "limit_tokens": b.LimitTokens,
			"period": b.Period, "alert_pct": b.AlertPct, "hard_cutoff": b.HardCutoff,
		})
	}
	return &ListBudgetsOutput{Body: struct {
		Budgets []map[string]any `json:"budgets"`
	}{Budgets: out}}, nil
}

// --- Budget Status ---

type BudgetStatusInput struct{}

type BudgetStatusOutput struct {
	Body struct {
		Budgets []map[string]any `json:"budgets"`
	}
}

func (s *Handler) HumaBudgetStatus(ctx context.Context, _ *BudgetStatusInput) (*BudgetStatusOutput, error) {
	budgets, err := s.budgets.ListByTenant(ctx, adminTenant)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}

	now := time.Now()
	scopes := make([]schema.SpendScope, 0, len(budgets))
	sinceByBudget := make([]time.Time, len(budgets))
	for i, b := range budgets {
		since := budget.PeriodStart(b.Period, now)
		sinceByBudget[i] = since
		scopes = append(scopes, schema.SpendScope{Kind: schema.BudgetScope(b.ScopeKind), ScopeID: b.ScopeID, Since: since})
	}
	spendResults, err := s.usage.SpendAndTokensBatch(ctx, scopes)
	if err != nil {
		s.log.Error("budget status: batch spend lookup failed", "err", err)
		spendResults = make([]schema.SpendResult, len(budgets))
	}

	out := make([]map[string]any, 0, len(budgets))
	for i, b := range budgets {
		since := sinceByBudget[i]
		var spent, tokens int64
		if i < len(spendResults) {
			spent = spendResults[i].CostMicros
			tokens = spendResults[i].Tokens
		}

		pctUsed := 0.0
		if b.LimitMicros > 0 {
			pctUsed = float64(spent) / float64(b.LimitMicros) * 100
		}
		tokPctUsed := 0.0
		if b.LimitTokens > 0 {
			tokPctUsed = float64(tokens) / float64(b.LimitTokens) * 100
		}

		scopeName := string(b.ScopeKind)
		if schema.BudgetScope(b.ScopeKind) == schema.ScopeAPIKey {
			if key, kerr := s.identity.Get(ctx, b.ScopeID); kerr == nil && key.Name != "" {
				scopeName = key.Name
			}
		}

		out = append(out, map[string]any{
			"id":              b.ID,
			"scope_kind":      b.ScopeKind,
			"scope_id":        b.ScopeID,
			"scope_name":      scopeName,
			"limit_micros":    b.LimitMicros,
			"limit_tokens":    b.LimitTokens,
			"period":          b.Period,
			"alert_pct":       b.AlertPct,
			"hard_cutoff":     b.HardCutoff,
			"spent_micros":    spent,
			"spent_tokens":    tokens,
			"pct_used":        pctUsed,
			"tokens_pct_used": tokPctUsed,
			"period_start":    since,
		})
	}
	return &BudgetStatusOutput{Body: struct {
		Budgets []map[string]any `json:"budgets"`
	}{Budgets: out}}, nil
}

// --- Create Budget ---

type CreateBudgetBody struct {
	ScopeKind   string  `json:"scope_kind,omitempty"`
	ScopeID     string  `json:"scope_id,omitempty"`
	LimitUSD    float64 `json:"limit_usd,omitempty"`
	LimitTokens int64   `json:"limit_tokens,omitempty"`
	Period      string  `json:"period,omitempty"`
	AlertPct    int     `json:"alert_pct,omitempty"`
	HardCutoff  *bool   `json:"hard_cutoff,omitempty"`
}

type CreateBudgetInput struct {
	Body CreateBudgetBody
}

type CreateBudgetOutputBody struct {
	ID string `json:"id"`
}

type CreateBudgetOutput struct {
	Body CreateBudgetOutputBody
}

func (s *Handler) HumaCreateBudget(ctx context.Context, input *CreateBudgetInput) (*CreateBudgetOutput, error) {
	if input.Body.LimitUSD <= 0 && input.Body.LimitTokens <= 0 {
		return nil, huma.Error400BadRequest("limit_usd or limit_tokens must be positive")
	}
	if input.Body.LimitUSD < 0 {
		return nil, huma.Error400BadRequest("limit_usd must not be negative")
	}
	if input.Body.LimitTokens < 0 {
		return nil, huma.Error400BadRequest("limit_tokens must not be negative")
	}
	period, ok := normalizeBudgetPeriod(input.Body.Period)
	if !ok {
		return nil, huma.Error400BadRequest("invalid budget period")
	}
	alertPct := defaultInt(input.Body.AlertPct, 80)
	if alertPct < 1 || alertPct > 100 {
		return nil, huma.Error400BadRequest("alert_pct must be between 1 and 100")
	}
	scopeKind := schema.BudgetScope(defaultStr(input.Body.ScopeKind, string(schema.ScopeTenant)))
	scopeID := strings.TrimSpace(input.Body.ScopeID)
	switch scopeKind {
	case schema.ScopeTenant:
		scopeID = defaultStr(scopeID, adminTenant)
	case schema.ScopeAPIKey:
		if scopeID == "" {
			return nil, huma.Error400BadRequest("scope_id is required for api_key budgets")
		}
		if _, err := s.identity.Get(ctx, scopeID); err != nil {
			return nil, huma.Error400BadRequest("api key not found")
		}
	case schema.ScopeProject:
		if scopeID == "" {
			return nil, huma.Error400BadRequest("scope_id is required for project budgets")
		}
	default:
		return nil, huma.Error400BadRequest("invalid budget scope")
	}
	hardCutoff := true
	if input.Body.HardCutoff != nil {
		hardCutoff = *input.Body.HardCutoff
	}

	now := time.Now()
	b := schema.Budget{
		ID:          uuid.NewString(),
		TenantID:    adminTenant,
		ScopeKind:   string(scopeKind),
		ScopeID:     scopeID,
		LimitMicros: int64(input.Body.LimitUSD * 1_000_000),
		LimitTokens: input.Body.LimitTokens,
		Period:      period,
		AlertPct:    alertPct,
		HardCutoff:  hardCutoff,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.budgets.Create(ctx, b); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}
	return &CreateBudgetOutput{Body: struct {
		ID string `json:"id"`
	}{ID: b.ID}}, nil
}

// --- Update Budget ---

type UpdateBudgetBody struct {
	LimitUSD    *float64 `json:"limit_usd,omitempty"`
	LimitTokens *int64   `json:"limit_tokens,omitempty"`
	Period      *string  `json:"period,omitempty"`
	AlertPct    *int     `json:"alert_pct,omitempty"`
	HardCutoff  *bool    `json:"hard_cutoff,omitempty"`
}

type UpdateBudgetInput struct {
	ID   string `path:"id" doc:"Budget ID"`
	Body UpdateBudgetBody
}

type UpdateBudgetOutput struct {
	Status int
}

func (s *Handler) HumaUpdateBudget(ctx context.Context, input *UpdateBudgetInput) (*UpdateBudgetOutput, error) {
	existing, err := s.budgets.Get(ctx, input.ID)
	if err != nil {
		if errors.Is(err, schema.ErrNotFound) {
			return nil, huma.Error404NotFound("budget not found")
		}
		return nil, huma.Error500InternalServerError("internal server error")
	}

	if input.Body.LimitUSD != nil {
		if *input.Body.LimitUSD < 0 {
			return nil, huma.Error400BadRequest("limit_usd must not be negative")
		}
		existing.LimitMicros = int64(*input.Body.LimitUSD * 1_000_000)
	}
	if input.Body.LimitTokens != nil {
		if *input.Body.LimitTokens < 0 {
			return nil, huma.Error400BadRequest("limit_tokens must not be negative")
		}
		existing.LimitTokens = *input.Body.LimitTokens
	}
	if input.Body.Period != nil {
		period, ok := normalizeBudgetPeriod(*input.Body.Period)
		if !ok {
			return nil, huma.Error400BadRequest("invalid budget period")
		}
		existing.Period = period
	}
	if input.Body.AlertPct != nil {
		if *input.Body.AlertPct < 1 || *input.Body.AlertPct > 100 {
			return nil, huma.Error400BadRequest("alert_pct must be between 1 and 100")
		}
		existing.AlertPct = *input.Body.AlertPct
	}
	if input.Body.HardCutoff != nil {
		existing.HardCutoff = *input.Body.HardCutoff
	}
	if existing.LimitMicros <= 0 && existing.LimitTokens <= 0 {
		return nil, huma.Error400BadRequest("limit_usd or limit_tokens must be positive")
	}
	existing.UpdatedAt = time.Now()

	if err := s.budgets.Update(ctx, existing); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}
	return &UpdateBudgetOutput{Status: 204}, nil
}

// --- Delete Budget ---

type DeleteBudgetInput struct {
	ID string `path:"id"`
}

type DeleteBudgetOutput struct {
	Status int
}

func (s *Handler) HumaDeleteBudget(ctx context.Context, input *DeleteBudgetInput) (*DeleteBudgetOutput, error) {
	if err := s.budgets.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}
	return &DeleteBudgetOutput{Status: 204}, nil
}

// --- Usage Summary ---

type UsageSummaryInput struct {
	Period string `query:"period" doc:"Time period: today, week, month (default)"`
}

type UsageSummaryOutput struct {
	Body struct {
		TotalRequests    int     `json:"total_requests"`
		PromptTokens     int64   `json:"prompt_tokens"`
		CompletionTokens int64   `json:"completion_tokens"`
		CachedTokens     int64   `json:"cached_tokens"`
		CacheWriteTokens int64   `json:"cache_write_tokens"`
		CostUSD          float64 `json:"cost_usd"`
		CacheHits        int64   `json:"cache_hits"`
		Since            string  `json:"since"`
	}
}

func (s *Handler) HumaUsageSummary(ctx context.Context, input *UsageSummaryInput) (*UsageSummaryOutput, error) {
	period := input.Period
	since := time.Now().AddDate(0, 0, -30)
	switch period {
	case "today":
		now := time.Now()
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "week":
		since = time.Now().AddDate(0, 0, -7)
	case "month", "":
		since = time.Now().AddDate(0, -1, 0)
	}

	sum, err := s.usage.Summarize(ctx, adminTenant, since)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &UsageSummaryOutput{Body: struct {
		TotalRequests    int     `json:"total_requests"`
		PromptTokens     int64   `json:"prompt_tokens"`
		CompletionTokens int64   `json:"completion_tokens"`
		CachedTokens     int64   `json:"cached_tokens"`
		CacheWriteTokens int64   `json:"cache_write_tokens"`
		CostUSD          float64 `json:"cost_usd"`
		CacheHits        int64   `json:"cache_hits"`
		Since            string  `json:"since"`
	}{
		TotalRequests:    sum.TotalRequests,
		PromptTokens:     sum.PromptTokens,
		CompletionTokens: sum.CompletionTokens,
		CachedTokens:     sum.CachedTokens,
		CacheWriteTokens: sum.CacheWriteTokens,
		CostUSD:          float64(sum.CostMicros) / 1_000_000,
		CacheHits:        sum.CacheHits,
		Since:            since.Format(time.RFC3339),
	}}, nil
}

// --- Usage Insights ---

type UsageInsightsInput struct {
	Period string `query:"period" doc:"Time period: today, week, month (default)"`
	Tz     string `query:"tz" doc:"IANA timezone (e.g. Asia/Jakarta)"`
}

type UsageInsightsOutput struct {
	Body map[string]any
}

func (s *Handler) HumaUsageInsights(ctx context.Context, input *UsageInsightsInput) (*UsageInsightsOutput, error) {
	since := sinceForPeriod(input.Period, input.Tz)

	var (
		sum           schema.Summary
		breakdown     []schema.ProviderUsage
		recent        []schema.RecentRecord
		timeline      []schema.TimeBucket
		ruleSavings   []schema.RuleSavings
		clientSavings []schema.ClientSavings
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		sum, err = s.usage.Summarize(gctx, adminTenant, since)
		return err
	})
	g.Go(func() error {
		var err error
		breakdown, err = s.usage.Breakdown(gctx, adminTenant, since)
		return err
	})
	g.Go(func() error {
		var err error
		recent, err = s.usage.Recent(gctx, adminTenant, 8)
		return err
	})
	g.Go(func() error {
		var err error
		timeline, err = s.usage.Timeline(gctx, adminTenant, since, time.Now(), 24)
		return err
	})
	g.Go(func() error {
		clientSavings, _ = s.usage.SavingsByClient(gctx, adminTenant, since)
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if sum.SlimBytesSaved > 0 {
		ruleSavings, _ = s.usage.SavingsByRule(ctx, adminTenant, since)
	}

	providers := make([]map[string]any, 0, len(breakdown))
	for _, p := range breakdown {
		share := 0.0
		if sum.TotalRequests > 0 {
			share = float64(p.TotalRequests) / float64(sum.TotalRequests) * 100
		}
		display, color, icon := p.Provider, "", ""
		if spec, ok := connectors.SpecByID(p.Provider); ok {
			display = spec.DisplayName
			color = spec.Color
			icon = "/providers/" + spec.ID + ".png"
		}
		providers = append(providers, map[string]any{
			"provider":          p.Provider,
			"display_name":      display,
			"color":             color,
			"icon":              icon,
			"total_requests":    p.TotalRequests,
			"prompt_tokens":     p.PromptTokens,
			"completion_tokens": p.CompletionTokens,
			"cost_usd":          float64(p.CostMicros) / 1_000_000,
			"share_pct":         share,
		})
	}

	recentRows := make([]map[string]any, 0, len(recent))
	for _, rec := range recent {
		entry := map[string]any{
			"id":         rec.ID,
			"provider":   rec.Provider,
			"model":      rec.Model,
			"tokens":     rec.PromptTokens + rec.CompletionTokens,
			"cost_usd":   float64(rec.CostMicros) / 1_000_000,
			"cache_hit":  rec.CacheHit,
			"latency_ms": rec.LatencyMS,
			"created_at": rec.CreatedAt,
		}
		if rec.TTFTMS > 0 {
			entry["ttft_ms"] = rec.TTFTMS
		}
		if rec.SlimBytesSaved > 0 {
			entry["slim_bytes_saved"] = rec.SlimBytesSaved
			entry["slim_tokens_saved"] = rec.SlimTokensSaved
		}
		if rec.SlimRules != "" {
			entry["slim_rules"] = rec.SlimRules
		}
		if rec.CavemanActive {
			entry["caveman_active"] = true
		}
		if rec.TerseActive {
			entry["terse_active"] = true
		}
		recentRows = append(recentRows, entry)
	}

	buckets := bucketTimeline(timeline, since, time.Now(), 24)
	busiestIdx, busiestCount := 0, int64(0)
	for i, b := range buckets {
		if b.count > busiestCount {
			busiestCount, busiestIdx = b.count, i
		}
	}
	series := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		series = append(series, map[string]any{"label": b.label, "count": b.count})
	}

	var successRate float64
	if sum.TotalRequests > 0 {
		successRate = float64(sum.SuccessCount) / float64(sum.TotalRequests)
	} else {
		successRate = 1
	}
	avgLatency := int(sum.AvgLatencyMS)
	avgTTFT := int(sum.AvgTTFTMS)

	busiest := ""
	if busiestCount > 0 && busiestIdx < len(buckets) {
		busiest = buckets[busiestIdx].label
	}

	rules := make([]map[string]any, 0, len(ruleSavings))
	for _, rs := range ruleSavings {
		rules = append(rules, map[string]any{
			"rule":         rs.Rule,
			"count":        rs.Count,
			"bytes_saved":  rs.BytesSaved,
			"tokens_saved": rs.BytesSaved / 4,
		})
	}

	blendedInputPerToken := blendedInputRate(sum)
	usdPerToken := func(tokens int64) float64 {
		return float64(tokens) * blendedInputPerToken
	}

	byClient := make([]map[string]any, 0, len(clientSavings))
	for _, cs := range clientSavings {
		if cs.SlimTokensSaved == 0 && cs.CavemanRequests == 0 && cs.TerseRequests == 0 &&
			cs.HeadroomTokensSaved == 0 && cs.PonytailRequests == 0 {
			continue
		}
		byClient = append(byClient, map[string]any{
			"client":                cs.Client,
			"requests":              cs.Requests,
			"bytes_saved":           cs.SlimBytesSaved,
			"tokens_saved":          cs.SlimTokensSaved,
			"usd_saved":             usdPerToken(cs.SlimTokensSaved),
			"caveman_requests":      cs.CavemanRequests,
			"terse_requests":        cs.TerseRequests,
			"headroom_tokens_saved": cs.HeadroomTokensSaved,
			"ponytail_requests":     cs.PonytailRequests,
		})
	}

	result := map[string]any{
		"summary": map[string]any{
			"total_requests":     sum.TotalRequests,
			"prompt_tokens":      sum.PromptTokens,
			"completion_tokens":  sum.CompletionTokens,
			"cached_tokens":      sum.CachedTokens,
			"cache_write_tokens": sum.CacheWriteTokens,
			"cost_usd":           float64(sum.CostMicros) / 1_000_000,
			"cache_hits":         sum.CacheHits,
			"success_rate":       successRate,
			"avg_latency_ms":     avgLatency,
			"avg_ttft_ms":        avgTTFT,
			"since":              since,
		},
		"savings": map[string]any{
			"slim_bytes_saved":      sum.SlimBytesSaved,
			"slim_tokens_saved":     sum.SlimTokensSaved,
			"caveman_requests":      sum.CavemanRequests,
			"terse_requests":        sum.TerseRequests,
			"headroom_tokens_saved": sum.HeadroomTokensSaved,
			"headroom_requests":     sum.HeadroomRequests,
			"ponytail_requests":     sum.PonytailRequests,
			"usd_saved":             usdPerToken(sum.SlimTokensSaved),
			"usd_saved_estimate":    true,
			"rules":                 rules,
			"by_client":             byClient,
		},
		"providers": providers,
		"recent":    recentRows,
		"series":    series,
		"busiest":   busiest,
	}
	return &UsageInsightsOutput{Body: result}, nil
}

// --- Model Usage ---

type ModelUsageInput struct {
	Period string `query:"period" doc:"Time period: today, week, month (default)"`
	Tz     string `query:"tz" doc:"IANA timezone (e.g. Asia/Jakarta)"`
}

type ModelUsageOutput struct {
	Body struct {
		Models []map[string]any `json:"models"`
	}
}

func (s *Handler) HumaModelUsage(ctx context.Context, input *ModelUsageInput) (*ModelUsageOutput, error) {
	since := sinceForPeriod(input.Period, input.Tz)

	models, err := s.usage.ByModel(ctx, adminTenant, since)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}

	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		display := m.Provider
		if spec, ok := connectors.SpecByID(m.Provider); ok {
			display = spec.DisplayName
		}
		entry := map[string]any{
			"provider":          m.Provider,
			"provider_name":     display,
			"model":             m.Model,
			"total_requests":    m.TotalRequests,
			"prompt_tokens":     m.PromptTokens,
			"completion_tokens": m.CompletionTokens,
			"cost_usd":          float64(m.CostMicros) / 1_000_000,
		}
		out = append(out, entry)
	}
	return &ModelUsageOutput{Body: struct {
		Models []map[string]any `json:"models"`
	}{Models: out}}, nil
}

// --- Quota Usage ---

type QuotaUsageInput struct {
	Period   string `query:"period" doc:"Time period: today, week, month (default)"`
	Tz       string `query:"tz" doc:"IANA timezone (e.g. Asia/Jakarta)"`
	Provider string `query:"provider" doc:"Filter by provider"`
}

type QuotaUsageOutput struct {
	Body struct {
		Accounts []map[string]any `json:"accounts"`
		Since    string           `json:"since"`
	}
}

func (s *Handler) HumaQuotaUsage(ctx context.Context, input *QuotaUsageInput) (*QuotaUsageOutput, error) {
	since := sinceForPeriod(input.Period, input.Tz)

	accs, err := s.accounts.ListByTenant(ctx, adminTenant)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if input.Provider != "" {
		filtered := accs[:0]
		for _, a := range accs {
			if a.Provider == input.Provider {
				filtered = append(filtered, a)
			}
		}
		accs = filtered
	}
	byAcct, err := s.usage.ByAccount(ctx, adminTenant, since)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	usageByID := make(map[string]schema.AccountUsage, len(byAcct))
	for _, u := range byAcct {
		usageByID[u.AccountID] = u
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	out := make([]map[string]any, 0, len(accs))

	for i, a := range accs {
		u := usageByID[a.ID]
		status := "active"
		if a.Disabled {
			status = "paused"
		} else if a.CooldownUntil != nil && a.CooldownUntil.After(time.Now()) {
			status = "needs_attention"
		}
		display := a.Provider
		var inputPerM, outputPerM float64
		var providerNotice string
		if spec, ok := connectors.SpecByID(a.Provider); ok {
			display = spec.DisplayName
			inputPerM = spec.InputPerM
			outputPerM = spec.OutputPerM
			providerNotice = spec.Notice
		}
		usageType := "token"
		if connectors.GetQuotaSource(a.Provider) != nil {
			usageType = "credit"
		}
		entry := map[string]any{
			"id":                 a.ID,
			"provider":           a.Provider,
			"provider_name":      display,
			"label":              a.Label,
			"auth_kind":          a.AuthKind,
			"priority":           a.Priority,
			"status":             status,
			"usage_type":         usageType,
			"total_requests":     u.TotalRequests,
			"prompt_tokens":      u.PromptTokens,
			"completion_tokens":  u.CompletionTokens,
			"cached_tokens":      u.CachedTokens,
			"cache_write_tokens": u.CacheWriteTokens,
			"cost_usd":           float64(u.CostMicros) / 1_000_000,
			"input_per_m":        inputPerM,
			"output_per_m":       outputPerM,
			"updated_at":         a.UpdatedAt,
		}
		if providerNotice != "" {
			entry["notice"] = providerNotice
		}
		out = append(out, entry)

		if qs := connectors.GetQuotaSource(a.Provider); qs != nil && !a.Disabled {
			if creds, err := s.vault.Open(a); err == nil {
				wg.Add(1)
				go func(idx int, qs connectors.QuotaSource, c any) {
					defer wg.Done()
					quotaCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
					quota, qerr := qs.FetchQuota(quotaCtx, c.(core.Credentials))
					cancel()
					if qerr == nil && quota != nil {
						var quotas []map[string]any
						for _, q := range quota.Quotas {
							quotas = append(quotas, map[string]any{
								"resource_type": q.ResourceType,
								"used":          q.Used,
								"limit":         q.Limit,
								"remaining":     q.Remaining,
								"reset_at":      q.ResetAt,
							})
						}

						mu.Lock()
						out[idx]["plan_name"] = quota.PlanName
						out[idx]["message"] = quota.Message
						if len(quotas) > 0 {
							out[idx]["upstream_quotas"] = quotas
						}
						mu.Unlock()
					}
				}(i, qs, creds)
			}
		}
	}
	wg.Wait()
	return &QuotaUsageOutput{Body: struct {
		Accounts []map[string]any `json:"accounts"`
		Since    string           `json:"since"`
	}{Accounts: out, Since: since.Format(time.RFC3339)}}, nil
}
