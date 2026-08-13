package limits

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type Limiter interface {
	Acquire(ctx context.Context, req Request) (ReleaseFunc, Decision, error)
}

type ReleaseFunc func(actualTokens int64)

type Request struct {
	TenantID        string
	ProjectID       string
	APIKeyID        string
	Provider        string
	Model           string
	EstimatedTokens int64
	Limits          EffectiveLimits
}

type EffectiveLimits struct {
	RPM         int64
	TPM         int64
	Concurrency int64
}

type Decision struct {
	Allowed    bool
	Reason     string
	RetryAfter time.Duration
	LimitKind  string
	Scope      string
	Limit      int64
	Remaining  int64
	Reset      time.Time
}

type Resolver struct {
	cfg   config.LimitsConfig
	plans *persistence.PlanRepo
}

func NewResolver(cfg config.LimitsConfig, plans *persistence.PlanRepo) *Resolver {
	return &Resolver{cfg: cfg, plans: plans}
}

func (r *Resolver) Resolve(ctx context.Context, key schema.APIKey) (EffectiveLimits, error) {
	if key.PlanID != "" && r.plans != nil {
		plan, err := r.plans.Get(ctx, key.PlanID)
		if err != nil {
			return EffectiveLimits{}, err
		}
		return EffectiveLimits{
			RPM:         plan.RPMLimit,
			TPM:         plan.TPMLimit,
			Concurrency: plan.ConcurrencyLimit,
		}, nil
	}
	return EffectiveLimits{
		RPM:         r.cfg.DefaultRPM,
		TPM:         r.cfg.DefaultTPM,
		Concurrency: r.cfg.DefaultConcurrency,
	}, nil
}

type MemoryConfig struct {
	Enabled         bool
	Window          time.Duration
	CleanupInterval time.Duration
	Now             func() time.Time
}

type bucket struct {
	count   int64
	expires time.Time
}

type Memory struct {
	enabled atomic.Bool
	window  time.Duration
	now     func() time.Time

	mu          sync.Mutex
	buckets     map[string]bucket
	concurrency map[string]int64
}

func NewMemory(cfg MemoryConfig) *Memory {
	window := cfg.Window
	if window <= 0 {
		window = time.Minute
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	m := &Memory{
		window:      window,
		now:         now,
		buckets:     make(map[string]bucket),
		concurrency: make(map[string]int64),
	}
	m.enabled.Store(cfg.Enabled)
	if cfg.CleanupInterval > 0 {
		go m.cleanupLoop(cfg.CleanupInterval)
	}
	return m
}

func (m *Memory) SetEnabled(enabled bool) {
	m.enabled.Store(enabled)
}

func (m *Memory) Enabled() bool {
	return m.enabled.Load()
}

func (m *Memory) Acquire(ctx context.Context, req Request) (ReleaseFunc, Decision, error) {
	if ctx.Err() != nil {
		return nil, Decision{}, ctx.Err()
	}
	if !m.enabled.Load() {
		return noopRelease, Decision{Allowed: true}, nil
	}
	if req.APIKeyID == "" {
		return noopRelease, Decision{Allowed: true}, nil
	}
	if req.EstimatedTokens < 0 {
		req.EstimatedTokens = 0
	}

	key := "key:" + req.APIKeyID
	concurrencyKey := key + ":concurrency"
	now := m.now()
	windowStart := now.Truncate(m.window)
	reset := windowStart.Add(m.window)
	var concurrencyInc bool

	m.mu.Lock()
	defer m.mu.Unlock()

	if req.Limits.Concurrency > 0 {
		current := m.concurrency[concurrencyKey]
		if current >= req.Limits.Concurrency {
			return noopRelease, denied("concurrency", "key", req.Limits.Concurrency, 0, time.Until(reset), reset), nil
		}
		m.concurrency[concurrencyKey] = current + 1
		concurrencyInc = true
	}

	if req.Limits.RPM > 0 {
		bucketKey := fmt.Sprintf("%s:rpm:%d", key, windowStart.Unix())
		b := m.bucket(bucketKey, reset, now)
		if b.count+1 > req.Limits.RPM {
			if concurrencyInc {
				m.decrementLocked(concurrencyKey)
			}
			remaining := req.Limits.RPM - b.count
			if remaining < 0 {
				remaining = 0
			}
			return noopRelease, denied("rpm", "key", req.Limits.RPM, remaining, time.Until(reset), reset), nil
		}
		b.count++
		m.buckets[bucketKey] = b
	}

	if req.Limits.TPM > 0 {
		tokens := req.EstimatedTokens
		if tokens <= 0 {
			tokens = 1
		}
		bucketKey := fmt.Sprintf("%s:tpm:%d", key, windowStart.Unix())
		b := m.bucket(bucketKey, reset, now)
		if b.count+tokens > req.Limits.TPM {
			if concurrencyInc {
				m.decrementLocked(concurrencyKey)
			}
			remaining := req.Limits.TPM - b.count
			if remaining < 0 {
				remaining = 0
			}
			return noopRelease, denied("tpm", "key", req.Limits.TPM, remaining, time.Until(reset), reset), nil
		}
		b.count += tokens
		m.buckets[bucketKey] = b
	}

	var once sync.Once
	release := func(actualTokens int64) {
		once.Do(func() {
			if !concurrencyInc {
				return
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			m.decrementLocked(concurrencyKey)
		})
	}
	return release, Decision{Allowed: true}, nil
}

func (m *Memory) bucket(key string, reset time.Time, now time.Time) bucket {
	b := m.buckets[key]
	if b.expires.IsZero() || !b.expires.After(now) {
		return bucket{expires: reset}
	}
	return b
}

func (m *Memory) decrementLocked(key string) {
	current := m.concurrency[key]
	if current <= 1 {
		delete(m.concurrency, key)
		return
	}
	m.concurrency[key] = current - 1
}

func (m *Memory) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		m.cleanup()
	}
}

func (m *Memory) cleanup() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, b := range m.buckets {
		if !b.expires.After(now) {
			delete(m.buckets, k)
		}
	}
}

func denied(kind, scope string, limit, remaining int64, retryAfter time.Duration, reset time.Time) Decision {
	if retryAfter < 0 {
		retryAfter = 0
	}
	return Decision{
		Allowed:    false,
		Reason:     fmt.Sprintf("rate limit exceeded: %s %s limit", scope, kind),
		RetryAfter: retryAfter,
		LimitKind:  kind,
		Scope:      scope,
		Limit:      limit,
		Remaining:  remaining,
		Reset:      reset,
	}
}

func noopRelease(int64) {}
