package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

type mockCapabilityCaller struct {
	res map[string]any
	err error
}

func (m *mockCapabilityCaller) CallCapability(ctx context.Context, slug, capability string, args map[string]any) (map[string]any, error) {
	return m.res, m.err
}

type mockAccountStore struct {
	acc schema.Account
}

func (m *mockAccountStore) Get(ctx context.Context, id string) (schema.Account, error) {
	return m.acc, nil
}
func (m *mockAccountStore) Update(ctx context.Context, a schema.Account) error {
	m.acc = a
	return nil
}

func TestTokenRefresher_EnsureValidToken(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, 32)
	copy(key, "test-master-key-0123456789012345")
	sealer, err := crypto.NewSealer(key)
	require.NoError(t, err)
	v := vault.New(sealer)

	var acc schema.Account
	acc.ID = "acc-1"
	acc.Provider = "antigravity"
	acc.AuthKind = "oauth"
	past := time.Now().Add(-1 * time.Hour)
	err = v.Seal(&acc, vault.NewSecret{
		AccessToken:  "old-access-token",
		RefreshToken: "valid-refresh-token",
		ExpiresAt:    &past,
	})
	require.NoError(t, err)

	store := &mockAccountStore{acc: acc}
	caller := &mockCapabilityCaller{
		res: map[string]any{
			"access_token": "new-fresh-access-token",
			"expires_in":   3600,
		},
	}

	refresher := auth.NewTokenRefresher(store, v, caller, nil)
	creds, updatedAcc, err := refresher.EnsureValidToken(ctx, acc)
	require.NoError(t, err)
	require.Equal(t, "new-fresh-access-token", creds.AccessToken)
	require.NotNil(t, updatedAcc.TokenExpiresAt)
	require.True(t, updatedAcc.TokenExpiresAt.After(time.Now()))
}
