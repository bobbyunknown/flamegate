package admin

import (
	"net/http"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/go-chi/chi/v5"
)

// sessionCookie is the name of the dashboard session cookie.
const sessionCookie = "fg_session"

// mountAuth registers unauthenticated auth endpoints and the session-protected
// dashboard status endpoint.
// Note: login is registered separately in server.go with rate limiting.
func (s *Handler) MountAuth(r chi.Router) {
	r.Post("/logout", s.HandleLogout)
	// Status reports onboarding/default-password state so the UI can decide
	// whether to show the onboarding flow. Safe to expose unauthenticated: it
	// reveals only booleans, never secrets.
	r.Get("/status", s.HandleAuthStatus)
}

// mountAuthenticated registers session-protected auth actions.
func (s *Handler) MountAuthenticatedAuth(r chi.Router) {
	r.Post("/password", s.HandleChangePassword)
	r.Post("/onboarding/complete", s.HandleCompleteOnboarding)
}

func (s *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	username := body.Username
	if username == "" {
		username = auth.DefaultUsername
	}

	_, ok, err := s.auth.VerifyPassword(r.Context(), username, body.Password)
	if err != nil || !ok {
		s.log.WithField("username", username).Warn("auth: login failed")
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := s.auth.IssueSession(username)
	if err != nil {
		s.log.WithField("username", username).Error("auth: failed to issue session")
		WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	s.setSessionCookie(w, token)
	s.log.WithField("username", username).Info("auth: login ok")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"using_default":       s.auth.UsingDefaultPassword(r.Context()),
		"onboarding_complete": s.auth.OnboardingComplete(r.Context()),
	})
}

// HandleToken implements the OAuth2 Resource Owner Password Credentials flow
// for Scalar API docs. It accepts application/x-www-form-urlencoded with
// grant_type=password, username, and password.
// Returns {"access_token": "<jwt>", "token_type": "Bearer", "expires_in": <seconds>}.
func (s *Handler) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeTokenError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}

	grantType := r.Form.Get("grant_type")
	username := r.Form.Get("username")
	password := r.Form.Get("password")

	if grantType != "password" {
		writeTokenError(w, http.StatusBadRequest, "unsupported_grant_type", "")
		return
	}
	if username == "" {
		username = auth.DefaultUsername
	}
	if password == "" {
		writeTokenError(w, http.StatusBadRequest, "invalid_request", "password is required")
		return
	}

	_, ok, err := s.auth.VerifyPassword(r.Context(), username, password)
	if err != nil || !ok {
		s.log.WithField("username", username).Warn("auth: token failed")
		writeTokenError(w, http.StatusUnauthorized, "invalid_grant", "")
		return
	}

	token, err := s.auth.IssueSession(username)
	if err != nil {
		s.log.WithField("username", username).Error("auth: failed to issue token")
		writeTokenError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	s.log.WithField("username", username).Info("auth: token issued")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(s.auth.TTL().Seconds()),
	})
}

// writeTokenError writes an RFC 6749 §5.2 error response.
func writeTokenError(w http.ResponseWriter, status int, code string, description string) {
	m := map[string]any{"error": code}
	if description != "" {
		m["error_description"] = description
	}
	writeJSON(w, status, m)
}

func (s *Handler) HandleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
	s.log.Info("auth: logout")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Handler) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	authed := false
	var username string
	if tok := sessionToken(r); tok != "" {
		if u, ok := s.auth.SessionUsername(tok); ok {
			authed = true
			username = u
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":       authed,
		"username":            username,
		"using_default":       s.auth.UsingDefaultPassword(r.Context()),
		"onboarding_complete": s.auth.OnboardingComplete(r.Context()),
	})
}

func (s *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.auth.SetPassword(r.Context(), auth.DefaultUsername, body.NewPassword); err != nil {
		s.log.WithError(err).Warn("auth: password change failed")
		WriteError(w, http.StatusBadRequest, sanitizeError(s.log, err, "password change failed"))
		return
	}
	s.log.Info("auth: password changed")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Handler) HandleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.CompleteOnboarding(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "onboarding completion failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// setSessionCookie writes the session cookie with the configured lifetime.
func (s *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(s.auth.TTL()),
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}
