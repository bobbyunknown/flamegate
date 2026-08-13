package vault

import (
	"testing"
	"time"

	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	key, err := crypto.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	return New(sealer)
}

func TestSealOpenAPIKey(t *testing.T) {
	v := newTestVault(t)

	var acc schema.Account
	acc.ID = "acc-1"
	secret := NewSecret{
		APIKey:   "sk-super-secret-123",
		Metadata: map[string]string{"base_url": "https://api.example.com", "region": "us"},
	}
	if err := v.Seal(&acc, secret); err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Ciphertext must not contain the plaintext.
	if acc.SecretCiphertext == "" || acc.SecretCiphertext == secret.APIKey {
		t.Fatalf("secret not sealed: %q", acc.SecretCiphertext)
	}

	creds, err := v.Open(acc)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if creds.APIKey != secret.APIKey {
		t.Fatalf("APIKey = %q, want %q", creds.APIKey, secret.APIKey)
	}
	if creds.AccountID != "acc-1" {
		t.Fatalf("AccountID = %q, want acc-1", creds.AccountID)
	}
	if creds.BaseURL != "https://api.example.com" {
		t.Fatalf("BaseURL = %q, want https://api.example.com", creds.BaseURL)
	}
	// base_url is lifted out of Extra.
	if _, ok := creds.Extra["base_url"]; ok {
		t.Fatal("base_url should be removed from Extra")
	}
	if creds.Extra["region"] != "us" {
		t.Fatalf("Extra[region] = %q, want us", creds.Extra["region"])
	}
}

func TestSealOpenTokens(t *testing.T) {
	v := newTestVault(t)

	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	var acc schema.Account
	acc.ID = "acc-2"
	secret := NewSecret{
		AccessToken:  "access-tok",
		RefreshToken: "refresh-tok",
		ExpiresAt:    &exp,
	}
	if err := v.Seal(&acc, secret); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if acc.TokenExpiresAt == nil || !acc.TokenExpiresAt.Equal(exp) {
		t.Fatalf("TokenExpiresAt = %v, want %v", acc.TokenExpiresAt, exp)
	}

	creds, err := v.Open(acc)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if creds.AccessToken != "access-tok" {
		t.Fatalf("AccessToken = %q, want access-tok", creds.AccessToken)
	}

	refresh, err := v.OpenRefreshToken(acc)
	if err != nil {
		t.Fatalf("OpenRefreshToken: %v", err)
	}
	if refresh != "refresh-tok" {
		t.Fatalf("refresh = %q, want refresh-tok", refresh)
	}
}

func TestSealDefaultsMetadata(t *testing.T) {
	v := newTestVault(t)
	var acc schema.Account
	if err := v.Seal(&acc, NewSecret{APIKey: "k"}); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// Empty metadata is normalized to "{}" so Open can parse it.
	if acc.Metadata != "{}" {
		t.Fatalf("Metadata = %q, want {}", acc.Metadata)
	}
	if _, err := v.Open(acc); err != nil {
		t.Fatalf("Open with default metadata: %v", err)
	}
}

func TestOpenRefreshTokenMissing(t *testing.T) {
	v := newTestVault(t)
	var acc schema.Account
	acc.ID = "acc-3"
	if _, err := v.OpenRefreshToken(acc); err == nil {
		t.Fatal("expected error opening missing refresh token")
	}
}

func TestOpenWrongKeyFails(t *testing.T) {
	// Seal with one vault, try to open with a vault holding a different key.
	v1 := newTestVault(t)
	v2 := newTestVault(t)

	var acc schema.Account
	if err := v1.Seal(&acc, NewSecret{APIKey: "secret"}); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := v2.Open(acc); err == nil {
		t.Fatal("Open with wrong master key should fail")
	}
}

func TestOpenInvalidMetadata(t *testing.T) {
	v := newTestVault(t)
	acc := schema.Account{ID: "acc-4", Metadata: "{not-json"}
	if _, err := v.Open(acc); err == nil {
		t.Fatal("expected error for invalid metadata JSON")
	}
}

func TestSealerAccessor(t *testing.T) {
	v := newTestVault(t)
	if v.Sealer() == nil {
		t.Fatal("Sealer() returned nil")
	}
}
