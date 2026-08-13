package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func (s *Handler) adminTestProxy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProxyURL string `json:"proxyUrl"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ProxyURL == "" {
		WriteError(w, http.StatusBadRequest, "proxyUrl is required")
		return
	}

	// Validate proxy URL syntax only — proxy URLs are admin-configured trusted
	// infrastructure, so SSRF restrictions (which guard outbound target URLs)
	// do not apply here. Localhost proxies (Clash, V2Ray, etc.) are expected.
	parsed, err := url.Parse(body.ProxyURL)
	if err != nil || parsed.Host == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid proxy URL: " + err.Error()})
		return
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "unsupported proxy scheme: " + parsed.Scheme})
		return
	}

	start := time.Now()
	transport := &http.Transport{Proxy: http.ProxyURL(parsed)}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://httpbin.org/ip", nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	result := map[string]any{
		"ok":        resp.StatusCode < 400,
		"status":    resp.StatusCode,
		"elapsedMs": elapsed.Milliseconds(),
	}

	// Parse exit IP from httpbin.org/ip response body.
	if resp.StatusCode < 400 {
		var ipInfo struct {
			Origin string `json:"origin"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ipInfo); err == nil && ipInfo.Origin != "" {
			result["exitIP"] = ipInfo.Origin
		}
	} else {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(errBody) > 0 {
			result["error"] = string(errBody)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// ---- helpers ----------------------------------------------------------------

type providerMetadataInput struct {
	BaseURL           string
	Region            string
	AccountID         string
	AzureEndpoint     string
	AzureDeployment   string
	AzureAPIVersion   string
	AzureOrganization string
}

func accountAuthKind(spec connectors.ProviderSpec, apiKey string) schema.AuthKind {
	if strings.TrimSpace(apiKey) == "" && spec.AuthKind == "none" {
		return schema.AuthNone
	}
	return schema.AuthAPIKey
}

func providerAccountMetadata(spec connectors.ProviderSpec, in providerMetadataInput) (map[string]string, error) {
	meta := map[string]string{}

	baseURL := strings.TrimSpace(in.BaseURL)
	if in.Region != "" {
		meta["region"] = strings.TrimSpace(in.Region)
		if resolved := connectors.ResolveRegionBaseURL(spec.ID, in.Region); resolved != "" {
			baseURL = resolved
		}
	}
	if spec.BaseURL == "" && spec.ID != "azure" && baseURL == "" {
		return nil, fmt.Errorf("base_url is required for %s", spec.DisplayName)
	}
	if baseURL != "" {
		meta["base_url"] = baseURL
	}

	switch spec.ID {
	case "cloudflare-ai":
		accountID := strings.TrimSpace(in.AccountID)
		if accountID == "" {
			return nil, errors.New("account_id is required for Cloudflare Workers AI")
		}
		// OpenAICompatible resolves {accountId} placeholders from Extra.
		meta["accountId"] = accountID
	case "azure":
		endpoint := strings.TrimRight(strings.TrimSpace(in.AzureEndpoint), "/")
		deployment := strings.TrimSpace(in.AzureDeployment)
		if endpoint == "" {
			return nil, errors.New("azure_endpoint is required for Azure OpenAI")
		}
		if deployment == "" {
			return nil, errors.New("azure_deployment is required for Azure OpenAI")
		}
		meta["azure_endpoint"] = endpoint
		meta["deployment"] = deployment
		if v := strings.TrimSpace(in.AzureAPIVersion); v != "" {
			meta["api_version"] = v
		}
		if v := strings.TrimSpace(in.AzureOrganization); v != "" {
			meta["organization"] = v
		}
	}

	return meta, nil
}

// validateAccountCredentials unseals an account's credentials and, if the
// connector implements core.Validator, probes the upstream to confirm they are
// accepted. Returns nil when validation passes or the connector does not support
// it. No-auth accounts still run connector probes when available so local
// endpoints such as Ollama/SearXNG can verify reachability.
//
// When the initial probe fails with an auth error and the account is OAuth,
// it retries once after forcing a token refresh (even if the token hasn't
// reached its local expiry — tokens can be invalidated server-side before
// expiry). A permanent refresh failure marks the account as needing
// reconnection.
