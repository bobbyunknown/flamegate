// Package auth provides authentication services, token refreshing, and session management.
package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// CapabilityCaller invokes a capability (e.g. "oauth_refresh") on a provider or extension.
type CapabilityCaller interface {
	CallCapability(ctx context.Context, slug, capability string, args map[string]any) (map[string]any, error)
}

// AccountStore is the minimal repository interface needed by TokenRefresher.
type AccountStore interface {
	Get(ctx context.Context, id string) (schema.Account, error)
	Update(ctx context.Context, a schema.Account) error
}

// TokenRefresher coordinates proactive and reactive OAuth access token renewal.
type TokenRefresher struct {
	accounts AccountStore
	vault    *vault.Vault
	caller   CapabilityCaller
	log      *logrus.Entry
	mu       sync.Mutex // serializes refresh operations per account to prevent duplicate calls
}

// NewTokenRefresher builds a new TokenRefresher service.
func NewTokenRefresher(accounts AccountStore, v *vault.Vault, caller CapabilityCaller, log *logrus.Entry) *TokenRefresher {
	if log == nil {
		log = logrus.NewEntry(logrus.StandardLogger())
	}
	return &TokenRefresher{
		accounts: accounts,
		vault:    v,
		caller:   caller,
		log:      log.WithField("component", "token_refresher"),
	}
}

// EnsureValidToken checks whether an account's OAuth access token is expired or close to expiry.
// If it needs refresh, it rotates the token against the upstream provider and returns fresh credentials.
func (r *TokenRefresher) EnsureValidToken(ctx context.Context, acc schema.Account) (core.Credentials, schema.Account, error) {
	if acc.AuthKind != "oauth" && acc.RefreshCiphertext == "" {
		creds, err := r.vault.Open(acc)
		return creds, acc, err
	}

	needsRefresh := false
	if acc.TokenExpiresAt == nil {
		// If expires_at is null but account has refresh_token, perform an initial refresh
		if acc.TokenCiphertext == "" {
			needsRefresh = true
		}
	} else if time.Now().After(acc.TokenExpiresAt.Add(-5 * time.Minute)) {
		needsRefresh = true
	}

	if !needsRefresh {
		creds, err := r.vault.Open(acc)
		return creds, acc, err
	}

	return r.RefreshToken(ctx, acc)
}

// RefreshToken forcefully exchanges the stored refresh_token for a fresh access_token,
// updates the database and vault, and returns the newly decrypted credentials.
func (r *TokenRefresher) RefreshToken(ctx context.Context, acc schema.Account) (core.Credentials, schema.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Re-fetch latest account state from repo in case another goroutine just completed a refresh.
	if r.accounts != nil && acc.ID != "" {
		if freshAcc, err := r.accounts.Get(ctx, acc.ID); err == nil && freshAcc.ID != "" {
			acc = freshAcc
			if acc.TokenExpiresAt != nil && time.Now().Before(acc.TokenExpiresAt.Add(-5*time.Minute)) {
				creds, err := r.vault.Open(acc)
				return creds, acc, err
			}
		}
	}

	refreshToken, err := r.vault.OpenRefreshToken(acc)
	if err != nil || refreshToken == "" {
		r.log.WithFields(logrus.Fields{
			"account_id": acc.ID,
			"provider":   acc.Provider,
		}).Warn("no refresh token found to rotate access token")
		creds, oerr := r.vault.Open(acc)
		if oerr != nil {
			return creds, acc, fmt.Errorf("no refresh token and vault open failed: %w", oerr)
		}
		return creds, acc, nil
	}

	slug := acc.Provider
	r.log.WithFields(logrus.Fields{
		"account_id": acc.ID,
		"provider":   slug,
	}).Info("refreshing OAuth access token via provider capability")

	if r.caller == nil {
		creds, _ := r.vault.Open(acc)
		return creds, acc, fmt.Errorf("capability caller not available for provider %s", slug)
	}

	res, err := r.caller.CallCapability(ctx, slug, "oauth_refresh", map[string]any{
		"refresh_token": refreshToken,
	})
	if err != nil {
		r.log.WithError(err).WithFields(logrus.Fields{
			"account_id": acc.ID,
			"provider":   slug,
		}).Warn("oauth_refresh capability execution failed")
		creds, _ := r.vault.Open(acc)
		return creds, acc, fmt.Errorf("oauth_refresh %s: %w", slug, err)
	}

	if res == nil {
		creds, _ := r.vault.Open(acc)
		return creds, acc, fmt.Errorf("oauth_refresh %s: returned empty result", slug)
	}

	if errMsg, ok := res["error"].(string); ok && errMsg != "" {
		r.log.WithFields(logrus.Fields{
			"account_id": acc.ID,
			"provider":   slug,
			"error":      errMsg,
		}).Warn("oauth_refresh returned error response")
		creds, _ := r.vault.Open(acc)
		return creds, acc, fmt.Errorf("oauth_refresh %s: %s", slug, errMsg)
	}

	accessToken, _ := res["access_token"].(string)
	if accessToken == "" {
		creds, _ := r.vault.Open(acc)
		return creds, acc, fmt.Errorf("oauth_refresh %s: no access_token in response", slug)
	}

	newRefreshToken, _ := res["refresh_token"].(string)
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}

	var expiresAt *time.Time
	if expVal, ok := res["expires_at"]; ok && expVal != nil {
		expiresAt = parseOAuthExpiry(expVal)
	} else if expIn, ok := res["expires_in"]; ok && expIn != nil {
		expiresAt = parseOAuthExpiresIn(expIn)
	} else {
		t := time.Now().Add(1 * time.Hour)
		expiresAt = &t
	}

	if err := r.vault.Seal(&acc, vault.NewSecret{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    expiresAt,
	}); err != nil {
		creds, _ := r.vault.Open(acc)
		return creds, acc, fmt.Errorf("seal refreshed token: %w", err)
	}

	if r.accounts != nil {
		if err := r.accounts.Update(ctx, acc); err != nil {
			r.log.WithError(err).Warn("failed to persist refreshed account token to database")
		}
	}

	creds, err := r.vault.Open(acc)
	if err != nil {
		return creds, acc, fmt.Errorf("open refreshed credentials: %w", err)
	}

	r.log.WithFields(logrus.Fields{
		"account_id": acc.ID,
		"provider":   slug,
		"expires_at": expiresAt,
	}).Info("OAuth access token rotated and persisted successfully")

	return creds, acc, nil
}

func parseOAuthExpiry(v any) *time.Time {
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return &t
		}
	case time.Time:
		return &val
	case *time.Time:
		return val
	}
	return nil
}

func parseOAuthExpiresIn(v any) *time.Time {
	var sec int64
	switch val := v.(type) {
	case float64:
		sec = int64(val)
	case float32:
		sec = int64(val)
	case int:
		sec = int64(val)
	case int64:
		sec = val
	case string:
		if s, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			sec = s
		}
	}
	if sec <= 0 {
		sec = 3600
	}
	t := time.Now().Add(time.Duration(sec) * time.Second)
	return &t
}
