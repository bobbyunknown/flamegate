package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// OAuth bridge: the host is a generic gateway — it runs the browser callback
// loopback, talks to the WASM guest via the chat entrypoint's capability
// dispatch, and persists tokens into the vault. All provider OAuth logic
// (authorize URL, token exchange, refresh) lives in the guest extension, so
// adding a new OAuth provider never requires a host build.

type oauthState struct {
	slug    string
	expires time.Time
}

var (
	oauthStateMu sync.Mutex
	oauthStates  = map[string]oauthState{}
)

// MountOAuth registers the generic per-extension OAuth flow endpoints.
func (s *Handler) MountOAuth(r chi.Router) {
	r.Post("/oauth/{slug}/start", s.adminOAuthStart)
	r.Get("/oauth/{slug}/callback", s.adminOAuthCallback)
	r.Post("/oauth/{slug}/exchange", s.adminOAuthExchange)
	r.Post("/oauth/{slug}/refresh", s.adminOAuthRefresh)
}

// adminOAuthStart asks the guest to build an authorize URL and redirects the
// browser there. The guest embeds our generated state and redirect_uri.
func (s *Handler) adminOAuthStart(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if s.db == nil || s.wasmEngine == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension oauth not available")
		return
	}
	ext, err := s.db.Extensions().FindBySlug(r.Context(), slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, "extension not found")
		return
	}
	if ext.State != "ACTIVE" {
		WriteError(w, http.StatusBadRequest, "extension not active")
		return
	}

	state := uuid.NewString()
	redirectURI := oauthCallbackURI(r, slug)
	oauthStateMu.Lock()
	oauthStates[state] = oauthState{slug: slug, expires: time.Now().Add(15 * time.Minute)}
	oauthStateMu.Unlock()

	res, err := s.wasmEngine.CallCapability(r.Context(), slug, "oauth_authorize", map[string]any{
		"redirect_uri": redirectURI,
		"state":        state,
	})
	if err != nil {
		WriteError(w, http.StatusBadGateway, "oauth_authorize failed: "+err.Error())
		return
	}
	if res == nil {
		WriteError(w, http.StatusNotImplemented, "extension does not support oauth")
		return
	}
	authURL, _ := res["url"].(string)
	if authURL == "" {
		WriteError(w, http.StatusBadGateway, "guest returned no authorize url")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authorize_url": authURL,
		"state":         state,
		"redirect_uri":  redirectURI,
		"display_name":  ext.Name,
	})
}

// adminOAuthCallback receives the provider redirect in the popup browser window.
// It serves an HTML loopback page that communicates the auth code back to the
// dashboard via postMessage, BroadcastChannel, and localStorage, then closes itself.
func (s *Handler) adminOAuthCallback(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	errStr := q.Get("error")
	errDesc := q.Get("error_description")
	if errDesc == "" {
		errDesc = q.Get("message")
	}

	// Consume valid state from cache if present
	if state != "" {
		oauthStateMu.Lock()
		if st, ok := oauthStates[state]; ok && st.slug == slug && time.Now().Before(st.expires) {
			delete(oauthStates, state)
		}
		oauthStateMu.Unlock()
	}

	// If explicit JSON requested (API client)
	accept := r.Header.Get("Accept")
	if q.Get("format") == "json" || (strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")) {
		if errStr != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": errStr, "error_description": errDesc})
			return
		}
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing code"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": slug, "code": code, "state": state})
		return
	}

	// Render HTML callback helper for popup / tab
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderOAuthCallbackHTML(slug, code, state, errStr, errDesc)))
}

type oauthExchangeBody struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
	State       string `json:"state"`
	Label       string `json:"label"`
}

// adminOAuthExchange swaps an authorization code or token blob with the guest
// extension and saves the sealed credentials to the accounts vault.
func (s *Handler) adminOAuthExchange(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if s.db == nil || s.wasmEngine == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension oauth not available")
		return
	}

	var req oauthExchangeBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body: " + err.Error()})
		return
	}

	rawCode := strings.TrimSpace(req.Code)
	if rawCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing code"})
		return
	}

	// If user pasted a full callback URL or query string, extract code value
	if idx := strings.Index(rawCode, "code="); idx != -1 {
		extracted := rawCode[idx+5:]
		if amp := strings.Index(extracted, "&"); amp != -1 {
			extracted = extracted[:amp]
		}
		rawCode = extracted
	}

	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = oauthCallbackURI(r, slug)
	}

	s.log.WithFields(logrus.Fields{
		"slug":        slug,
		"code_len":    len(rawCode),
		"redirectURI": redirectURI,
	}).Info("adminOAuthExchange: executing oauth_exchange capability")

	res, err := s.wasmEngine.CallCapability(r.Context(), slug, "oauth_exchange", map[string]any{
		"code": rawCode, "redirect_uri": redirectURI,
	})
	if err != nil {
		s.log.WithError(err).Error("adminOAuthExchange: CallCapability returned error")
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "oauth_exchange failed: " + err.Error()})
		return
	}
	if res == nil {
		s.log.Warn("adminOAuthExchange: extension returned nil capability response")
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "extension does not support oauth exchange"})
		return
	}
	if gErr, _ := res["error"].(string); gErr != "" {
		s.log.WithField("guest_error", gErr).Error("adminOAuthExchange: guest returned error")
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": gErr})
		return
	}
	accessToken, _ := res["access_token"].(string)
	if accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "guest returned no access_token"})
		return
	}
	refreshToken, _ := res["refresh_token"].(string)
	label := req.Label
	if label == "" {
		label, _ = res["email"].(string)
	}
	if label == "" {
		label, _ = res["name"].(string)
	}
	expiresAt := parseOAuthExpiry(res["expires_at"])

	acc, created, err := s.upsertOAuthAccount(r.Context(), slug, label)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.vault.Seal(&acc, vault.NewSecret{
		AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if created {
		err = s.accounts.Create(r.Context(), acc)
	} else {
		err = s.accounts.Update(r.Context(), acc)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "provider": slug, "account_id": acc.ID, "label": acc.Label,
	})
}

// adminOAuthRefresh rotates an access token via the guest using the stored
// refresh token, then persists the new credentials.
func (s *Handler) adminOAuthRefresh(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	accs, err := s.accounts.ListByProvider(r.Context(), adminTenant, slug)
	if err != nil || len(accs) == 0 {
		WriteError(w, http.StatusNotFound, "no account for extension")
		return
	}
	acc := accs[0]
	refreshToken, err := s.vault.OpenRefreshToken(acc)
	if err != nil {
		WriteError(w, http.StatusBadGateway, "no refresh token: "+err.Error())
		return
	}
	res, err := s.wasmEngine.CallCapability(r.Context(), slug, "oauth_refresh", map[string]any{
		"refresh_token": refreshToken,
	})
	if err != nil {
		WriteError(w, http.StatusBadGateway, "oauth_refresh failed: "+err.Error())
		return
	}
	if res == nil {
		WriteError(w, http.StatusNotImplemented, "extension does not support oauth refresh")
		return
	}
	accessToken, _ := res["access_token"].(string)
	if accessToken == "" {
		WriteError(w, http.StatusBadGateway, "guest returned no access_token")
		return
	}
	if err := s.vault.Seal(&acc, vault.NewSecret{
		AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: parseOAuthExpiry(res["expires_at"]),
	}); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.accounts.Update(r.Context(), acc); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "account_id": acc.ID})
}

// upsertOAuthAccount returns an existing account for the extension, or builds
// a fresh one to be created after the token seal.
func (s *Handler) upsertOAuthAccount(ctx context.Context, slug, label string) (schema.Account, bool, error) {
	accs, err := s.accounts.ListByProvider(ctx, adminTenant, slug)
	if err != nil {
		return schema.Account{}, false, err
	}
	if len(accs) > 0 {
		acc := accs[0]
		if label != "" && acc.Label == slug {
			acc.Label = label
		}
		return acc, false, nil
	}
	name := slug
	if ext, err := s.db.Extensions().FindBySlug(ctx, slug); err == nil && ext.Name != "" {
		name = ext.Name
	}
	if label != "" {
		name = label
	}
	now := time.Now()
	return schema.Account{
		ID: uuid.NewString(), TenantID: adminTenant, Provider: slug, Label: name,
		AuthKind: string(schema.AuthOAuth), Priority: 100, CreatedAt: now, UpdatedAt: now,
	}, true, nil
}

// oauthCallbackURI builds the externally reachable callback URL for the slug by
// trimming the trailing /start and appending /callback.
func oauthCallbackURI(r *http.Request, slug string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	}
	return scheme + "://" + r.Host + strings.TrimSuffix(r.URL.Path, "/start") + "/callback"
}

func parseOAuthExpiry(v any) *time.Time {
	s, _ := v.(string)
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

func renderOAuthCallbackHTML(slug, code, state, errStr, errDesc string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>FlameGate — OAuth Authorization</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #090d16;
      color: #f3f4f6;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      padding: 1.5rem;
    }
    .card {
      background: #111827;
      border: 1px solid #1f293d;
      border-radius: 16px;
      padding: 2.25rem 2rem;
      max-width: 440px;
      width: 100%;
      text-align: center;
      box-shadow: 0 20px 40px rgba(0,0,0,0.6);
    }
    .icon-wrap {
      width: 56px;
      height: 56px;
      margin: 0 auto 1.25rem;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 26px;
    }
    .success { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
    .error { background: rgba(239, 68, 68, 0.15); color: #ef4444; }
    .spinner {
      border: 3px solid #1f293d;
      border-top: 3px solid #f97316;
      border-radius: 50%;
      width: 44px;
      height: 44px;
      animation: spin 1s linear infinite;
      margin: 0 auto 1.25rem;
    }
    h2 { font-size: 1.25rem; font-weight: 600; margin-bottom: 0.5rem; color: #ffffff; }
    p { font-size: 0.875rem; color: #9ca3af; line-height: 1.5; margin-bottom: 1.25rem; }
    .code-box {
      background: #030712;
      border: 1px solid #1f2937;
      border-radius: 8px;
      padding: 0.75rem;
      font-family: ui-monospace, monospace;
      font-size: 0.75rem;
      color: #93c5fd;
      word-break: break-all;
      text-align: left;
      margin-top: 1rem;
      max-height: 100px;
      overflow-y: auto;
    }
    .btn {
      display: inline-block;
      background: #f97316;
      color: #ffffff;
      padding: 0.5rem 1.25rem;
      border-radius: 8px;
      font-size: 0.875rem;
      font-weight: 500;
      text-decoration: none;
      cursor: pointer;
      border: none;
      transition: background 0.2s;
    }
    .btn:hover { background: #ea580c; }
    @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
  </style>
</head>
<body>
  <div class="card">
    <div id="icon-container" class="spinner"></div>
    <h2 id="title">Completing Authorization...</h2>
    <p id="desc">Connecting your account to FlameGate. Please wait...</p>
    <div id="manual-area" style="display:none;">
      <p style="font-size: 0.8rem; color: #6b7280; margin-bottom: 0.5rem;">If window doesn't close, copy this code to the dashboard:</p>
      <div id="code-preview" class="code-box"></div>
    </div>
  </div>

  <script>
    (function() {
      const payload = {
        type: "oauth_callback",
        slug: ` + jsonString(slug) + `,
        code: ` + jsonString(code) + `,
        state: ` + jsonString(state) + `,
        error: ` + jsonString(errStr) + `,
        errorDescription: ` + jsonString(errDesc) + `,
        fullUrl: window.location.href
      };

      let delivered = false;

      // 1. Post to window opener (popup)
      try {
        if (window.opener) {
          window.opener.postMessage(payload, "*");
          delivered = true;
        }
      } catch (e) {}

      // 2. BroadcastChannel
      try {
        const bc = new BroadcastChannel("oauth_callback");
        bc.postMessage(payload);
        bc.close();
        delivered = true;
      } catch (e) {}

      // 3. LocalStorage event
      try {
        localStorage.setItem("oauth_callback", JSON.stringify({
          ...payload,
          timestamp: Date.now()
        }));
        delivered = true;
      } catch (e) {}

      const iconEl = document.getElementById("icon-container");
      const titleEl = document.getElementById("title");
      const descEl = document.getElementById("desc");
      const manualArea = document.getElementById("manual-area");
      const codePreview = document.getElementById("code-preview");

      if (payload.error) {
        iconEl.className = "icon-wrap error";
        iconEl.innerHTML = "✕";
        titleEl.textContent = "Authorization Failed";
        descEl.textContent = payload.errorDescription || payload.error || "An error occurred during authentication.";
      } else if (payload.code) {
        iconEl.className = "icon-wrap success";
        iconEl.innerHTML = "✓";
        titleEl.textContent = "Authorization Successful";
        descEl.textContent = "Your account has been authorized. This window will close automatically.";
        
        if (window.opener && delivered) {
          setTimeout(function() {
            window.close();
          }, 1200);
        } else {
          manualArea.style.display = "block";
          codePreview.textContent = payload.code;
        }
      } else {
        iconEl.className = "icon-wrap error";
        iconEl.innerHTML = "✕";
        titleEl.textContent = "No Code Received";
        descEl.textContent = "Provider completed authorization but did not return a valid code.";
      }
    })();
  </script>
</body>
</html>`
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}