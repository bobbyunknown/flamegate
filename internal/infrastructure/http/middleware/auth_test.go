package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
)

func newTestAuthSvc(t *testing.T) *auth.Service {
	t.Helper()
	ctx := context.Background()

	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	require.NoError(t, db.EnsureDefault())
	t.Cleanup(func() { _ = db.Close() })

	svc := auth.New(db.Users(), db.Settings(), "", time.Hour)
	_, err = svc.EnsureDefaults(ctx)
	require.NoError(t, err)

	return svc
}

func TestSessionAuth_BearerToken(t *testing.T) {
	svc := newTestAuthSvc(t)
	log := logrus.StandardLogger()
	mw := SessionAuth(svc, log)

	token, err := svc.IssueSession(auth.DefaultUsername)
	require.NoError(t, err)

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	t.Run("valid bearer", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.True(t, called, "next handler must be called")
	})

	t.Run("invalid bearer", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
		req.Header.Set("Authorization", "Bearer invalidtoken123")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.False(t, called, "next handler must NOT be called")
	})

	t.Run("no bearer no cookie", func(t *testing.T) {
		called =false
		req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.False(t, called)
	})
}

func TestSessionAuth_CookieFallback(t *testing.T) {
	svc := newTestAuthSvc(t)
	log := logrus.StandardLogger()

	token, err := svc.IssueSession(auth.DefaultUsername)
	require.NoError(t, err)

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := SessionAuth(svc, log)(next)

	t.Run("valid cookie", func(t *testing.T) {
		called =false
		req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.True(t, called)
	})

	t.Run("bearer takes precedence over cookie", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "invalid-cookie"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "valid bearer must succeed even with bad cookie")
		require.True(t, called)
	})
}
