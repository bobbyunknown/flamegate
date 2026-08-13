package limits

import (
	"context"
	"testing"
	"time"
)

func TestMemoryRPMAllowUnderLimit(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", Limits: EffectiveLimits{RPM: 2}}
	for i := 0; i < 2; i++ {
		release, decision, err := lim.Acquire(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Allowed {
			t.Fatalf("request %d denied: %+v", i, decision)
		}
		release(0)
	}
}

func TestMemoryRPMDenyOverLimit(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", Limits: EffectiveLimits{RPM: 1}}
	release, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("first denied: %+v", decision)
	}
	release(0)

	_, decision, err = lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.LimitKind != "rpm" {
		t.Fatalf("expected rpm deny, got %+v", decision)
	}
}

func TestMemoryTPMDenyOverLimit(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", EstimatedTokens: 11, Limits: EffectiveLimits{TPM: 10}}
	_, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.LimitKind != "tpm" {
		t.Fatalf("expected tpm deny, got %+v", decision)
	}
}

func TestMemoryConcurrencyDenyAndRelease(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", Limits: EffectiveLimits{Concurrency: 1}}
	release, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("first denied: %+v", decision)
	}

	_, decision, err = lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.LimitKind != "concurrency" {
		t.Fatalf("expected concurrency deny, got %+v", decision)
	}

	release(0)
	release(0) // idempotent

	release2, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allow after release, got %+v", decision)
	}
	release2(0)
}

func TestMemoryExpiredWindowResets(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", Limits: EffectiveLimits{RPM: 1}}
	release, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("first denied: %+v", decision)
	}
	release(0)

	now = now.Add(time.Minute)

	release, decision, err = lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allow after window reset, got %+v", decision)
	}
	release(0)
}

func TestMemoryUnlimitedZeroMeansNoDeny(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", EstimatedTokens: 1_000_000, Limits: EffectiveLimits{}}
	for i := 0; i < 100; i++ {
		release, decision, err := lim.Acquire(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Allowed {
			t.Fatalf("request %d denied: %+v", i, decision)
		}
		release(0)
	}
}

func TestMemoryDeniedTPMRollsBackConcurrency(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	lim := NewMemory(MemoryConfig{Enabled: true, Window: time.Minute, Now: func() time.Time { return now }})

	req := Request{APIKeyID: "key1", EstimatedTokens: 20, Limits: EffectiveLimits{Concurrency: 1, TPM: 10}}
	_, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.LimitKind != "tpm" {
		t.Fatalf("expected tpm deny, got %+v", decision)
	}

	req.EstimatedTokens = 1
	release, decision, err := lim.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatalf("concurrency was not rolled back: %+v", decision)
	}
	release(0)
}
