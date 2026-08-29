package wasm

import (
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain/provider"
)

func TestCredResponseFrom_PrefersAccessToken(t *testing.T) {
	// OAuth account: vault stores the WorkOS token in AccessToken.
	cred := credResponseFrom(provider.Credentials{
		AccessToken: "workos:oauth-token",
	})
	if cred.APIKey != "workos:oauth-token" {
		t.Fatalf("APIKey = %q, want workos:oauth-token", cred.APIKey)
	}
}

func TestCredResponseFrom_APIKeyFallback(t *testing.T) {
	// BYOK account: sk_-style key in APIKey, no AccessToken.
	cred := credResponseFrom(provider.Credentials{
		APIKey: "sk_test",
	})
	if cred.APIKey != "sk_test" {
		t.Fatalf("APIKey = %q, want sk_test", cred.APIKey)
	}
	if cred.BaseURL != "" {
		t.Fatalf("BaseURL = %q, want empty", cred.BaseURL)
	}
}

func TestCredResponseFrom_BaseURLForwarded(t *testing.T) {
	cred := credResponseFrom(provider.Credentials{
		APIKey:  "k",
		BaseURL: "https://custom.example",
	})
	if cred.BaseURL != "https://custom.example" {
		t.Fatalf("BaseURL = %q, want custom url", cred.BaseURL)
	}
}