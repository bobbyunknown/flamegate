package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
)

func newTestAuthHandler(t *testing.T) *Handler {
	t.Helper()

	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	require.NoError(t, db.EnsureDefault())
	t.Cleanup(func() { _ = db.Close() })

	svc := auth.New(db.Users(), db.Settings(), "", time.Hour)
	_, err = svc.EnsureDefaults(t.Context())
	require.NoError(t, err)

	return New(Deps{
		Config:   config.Default(),
		Auth:     svc,
		Settings: db.Settings(),
	})
}

func postForm(t *testing.T, handler http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHandleToken_ValidPassword(t *testing.T) {
	h := newTestAuthHandler(t)
	rec := postForm(t, h.Handler(), "/api/auth/token", url.Values{
		"grant_type": {"password"},
		"username":   {"admin"},
		"password":   {auth.DefaultPassword},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.NotEmpty(t, body["access_token"], "access_token must be non-empty")
	require.Equal(t, "Bearer", body["token_type"])
	require.Equal(t, float64(3600), body["expires_in"]) // 1 hour as set in test
}

func TestHandleToken_InvalidPassword(t *testing.T) {
	h := newTestAuthHandler(t)
	rec := postForm(t, h.Handler(), "/api/auth/token", url.Values{
		"grant_type": {"password"},
		"username":   {"admin"},
		"password":   {"wrong"},
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "invalid_grant", body["error"])
}

func TestHandleToken_UnsupportedGrantType(t *testing.T) {
	h := newTestAuthHandler(t)
	rec := postForm(t, h.Handler(), "/api/auth/token", url.Values{
		"grant_type": {"client_credentials"},
		"username":   {"admin"},
		"password":   {auth.DefaultPassword},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "unsupported_grant_type", body["error"])
}

func TestHandleToken_MissingPassword(t *testing.T) {
	h := newTestAuthHandler(t)
	rec := postForm(t, h.Handler(), "/api/auth/token", url.Values{
		"grant_type": {"password"},
		"username":   {"admin"},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "invalid_request", body["error"])
	require.Equal(t, "password is required", body["error_description"])
}

func TestHandleToken_IssuedTokenVerifies(t *testing.T) {
	h := newTestAuthHandler(t)
	rec := postForm(t, h.Handler(), "/api/auth/token", url.Values{
		"grant_type": {"password"},
		"password":   {auth.DefaultPassword},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	token, ok := body["access_token"].(string)
	require.True(t, ok)

	require.True(t, h.Auth().VerifySession(token), "issued token must pass VerifySession")
}
