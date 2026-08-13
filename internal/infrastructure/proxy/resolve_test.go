package proxy

import (
	"context"
	"errors"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// fakePoolSource is an in-memory PoolSource for tests.
type fakePoolSource struct {
	pool schema.ProxyPool
	err  error
	// gotID records the last id requested, to assert lookups do/don't happen.
	gotID string
}

func (f *fakePoolSource) Get(_ context.Context, id string) (schema.ProxyPool, error) {
	f.gotID = id
	if f.err != nil {
		return schema.ProxyPool{}, f.err
	}
	return f.pool, nil
}

func TestResolvePoolEmptyOrNone(t *testing.T) {
	for _, id := range []string{"", "__none__"} {
		src := &fakePoolSource{}
		creds := &core.Credentials{}
		if err := ResolvePool(context.Background(), src, id, creds); err != nil {
			t.Fatalf("ResolvePool(%q) unexpected error: %v", id, err)
		}
		if src.gotID != "" {
			t.Fatalf("ResolvePool(%q) should not query the pool source", id)
		}
		if creds.ProxyURL != "" || creds.RelayURL != "" {
			t.Fatalf("ResolvePool(%q) mutated creds: %+v", id, creds)
		}
	}
}

func TestResolvePoolInactive(t *testing.T) {
	src := &fakePoolSource{pool: schema.ProxyPool{
		Type:     "http",
		ProxyURL: "http://proxy.example:8080",
		IsActive: false,
	}}
	creds := &core.Credentials{}
	if err := ResolvePool(context.Background(), src, "pool-1", creds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Inactive pool means direct connection, no proxy config applied.
	if creds.ProxyURL != "" || creds.RelayURL != "" {
		t.Fatalf("inactive pool applied config: %+v", creds)
	}
}

func TestResolvePoolHTTP(t *testing.T) {
	src := &fakePoolSource{pool: schema.ProxyPool{
		Type:     "http",
		ProxyURL: "http://proxy.example:8080",
		NoProxy:  "localhost,127.0.0.1",
		Strict:   true,
		IsActive: true,
	}}
	creds := &core.Credentials{}
	if err := ResolvePool(context.Background(), src, "pool-1", creds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ProxyURL != "http://proxy.example:8080" {
		t.Fatalf("ProxyURL = %q, want http://proxy.example:8080", creds.ProxyURL)
	}
	if creds.RelayURL != "" {
		t.Fatalf("RelayURL should be empty for http type, got %q", creds.RelayURL)
	}
	if creds.NoProxy != "localhost,127.0.0.1" {
		t.Fatalf("NoProxy = %q", creds.NoProxy)
	}
	if !creds.StrictProxy {
		t.Fatal("StrictProxy should be true")
	}
}

func TestResolvePoolRelayTypes(t *testing.T) {
	for _, typ := range []string{"vercel", "cloudflare", "deno"} {
		src := &fakePoolSource{pool: schema.ProxyPool{
			Type:     typ,
			ProxyURL: "https://relay.example/proxy",
			IsActive: true,
		}}
		creds := &core.Credentials{}
		if err := ResolvePool(context.Background(), src, "pool-1", creds); err != nil {
			t.Fatalf("type %s: unexpected error: %v", typ, err)
		}
		if creds.RelayURL != "https://relay.example/proxy" {
			t.Fatalf("type %s: RelayURL = %q, want relay url", typ, creds.RelayURL)
		}
		if creds.ProxyURL != "" {
			t.Fatalf("type %s: ProxyURL should be empty, got %q", typ, creds.ProxyURL)
		}
	}
}

func TestResolvePoolLookupError(t *testing.T) {
	wantErr := errors.New("db down")
	src := &fakePoolSource{err: wantErr}
	creds := &core.Credentials{}
	err := ResolvePool(context.Background(), src, "pool-1", creds)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error does not wrap source error: %v", err)
	}
}
