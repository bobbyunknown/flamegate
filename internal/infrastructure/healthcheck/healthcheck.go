package healthcheck

import (
	"context"
	"sync"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"

	"github.com/google/uuid"

	"github.com/sirupsen/logrus"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// ConnectorSource resolves provider connectors.
type ConnectorSource interface {
	Get(provider string) (core.Connector, error)
}

// Config controls the background health checker.
type Config struct {
	Enabled              bool
	Interval             time.Duration
	Timeout              time.Duration
	MaxParallel          int
	FailureThreshold     int
	SuccessThreshold     int
	RecentModelWindow    time.Duration
	MaxModelsPerProvider int
}

// Checker probes configured provider accounts and records account/model health.
type Checker struct {
	cfg      Config
	log      *logrus.Logger
	accounts *persistence.AccountRepo
	health   *persistence.HealthRepo
	conns    ConnectorSource
	vault    *vault.Vault
}

// New builds a Checker.
func New(cfg Config, log *logrus.Logger, accounts *persistence.AccountRepo, health *persistence.HealthRepo, conns ConnectorSource, vault *vault.Vault) *Checker {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 8
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 1
	}
	if cfg.RecentModelWindow <= 0 {
		cfg.RecentModelWindow = 24 * time.Hour
	}
	if cfg.MaxModelsPerProvider <= 0 {
		cfg.MaxModelsPerProvider = 8
	}
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &Checker{cfg: cfg, log: log, accounts: accounts, health: health, conns: conns, vault: vault}
}

// Run starts periodic probes until ctx is canceled.
func (c *Checker) Run(ctx context.Context, tenantID string) {
	if c == nil || !c.cfg.Enabled {
		return
	}
	c.CheckOnce(ctx, tenantID)
	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CheckOnce(ctx, tenantID)
		}
	}
}

// CheckOnce probes all enabled accounts for a tenant.
func (c *Checker) CheckOnce(ctx context.Context, tenantID string) {
	if c == nil || c.accounts == nil || c.health == nil || c.conns == nil || c.vault == nil {
		return
	}
	accounts, err := c.accounts.ListByTenant(ctx, tenantID)
	if err != nil {
		c.log.Warn("health check: list accounts failed", "err", err)
		return
	}
	recent, _ := c.health.RecentAccountModels(ctx, tenantID, time.Now().Add(-c.cfg.RecentModelWindow), len(accounts)*c.cfg.MaxModelsPerProvider)

	modelsByAccount := map[string][]string{}
	for _, h := range recent {
		if len(modelsByAccount[h.AccountID]) >= c.cfg.MaxModelsPerProvider {
			continue
		}
		modelsByAccount[h.AccountID] = appendUnique(modelsByAccount[h.AccountID], h.Model)
	}

	sem := make(chan struct{}, c.cfg.MaxParallel)
	var wg sync.WaitGroup
	for _, acc := range accounts {
		if acc.Disabled || acc.NeedsReconnect {
			continue
		}
		models := modelsByAccount[acc.ID]
		if len(models) == 0 {
			models = []string{"__all__"}
		}
		for _, model := range models {
			acc := acc
			model := model
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				c.probe(ctx, acc, model)
			}()
		}
	}
	wg.Wait()
}

func (c *Checker) probe(ctx context.Context, acc schema.Account, model string) {
	conn, err := c.conns.Get(acc.Provider)
	if err != nil {
		c.record(ctx, acc, model, 0, err)
		return
	}
	creds, err := c.vault.Open(acc)
	if err != nil {
		c.record(ctx, acc, model, 0, err)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	start := time.Now()
	if model == "__all__" {
		if v, ok := conn.(core.Validator); ok {
			err = v.Validate(probeCtx, creds)
		} else {
			err = nil
		}
	} else {
		max := 8
		req := &core.ChatRequest{
			Model: model,
			Messages: []core.Message{{
				Role: core.RoleUser,
				Content: []core.ContentPart{{
					Type: core.PartText,
					Text: "ping",
				}},
			}},
			MaxTokens: &max,
			Metadata: core.RequestMetadata{
				TenantID: acc.TenantID,
				Provider: acc.Provider,
			},
		}
		_, err = conn.Chat(probeCtx, req, creds)
	}
	c.record(ctx, acc, model, int(time.Since(start).Milliseconds()), err)
}

func (c *Checker) record(ctx context.Context, acc schema.Account, model string, latencyMS int, probeErr error) {
	now := time.Now()
	prev, err := c.health.Get(ctx, acc.ID, model)
	if err != nil && err != schema.ErrNotFound {
		c.log.Debug("health check: read previous state failed", "account", acc.ID, "model", model, "err", err)
	}
	h := prev
	if h.ID == "" {
		h.ID = uuid.NewString()
		h.TenantID = acc.TenantID
		h.AccountID = acc.ID
		h.Provider = acc.Provider
		h.Model = model
	}
	h.LatencyMS = latencyMS
	h.LastCheckedAt = now
	h.UpdatedAt = now
	if probeErr == nil {
		h.ConsecutiveSuccesses++
		h.ConsecutiveFailures = 0
		h.LastError = ""
		h.LastOKAt = &now
		if h.ConsecutiveSuccesses >= c.cfg.SuccessThreshold {
			h.Status = "healthy"
		} else if h.Status == "" {
			h.Status = "degraded"
		}
	} else {
		h.ConsecutiveFailures++
		h.ConsecutiveSuccesses = 0
		h.LastError = probeErr.Error()
		if h.ConsecutiveFailures >= c.cfg.FailureThreshold {
			h.Status = "unhealthy"
		} else {
			h.Status = "degraded"
		}
	}
	if err := c.health.Upsert(ctx, h); err != nil {
		c.log.Warn("health check: upsert failed", "account", acc.ID, "model", model, "err", err)
	}
}

func appendUnique(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}
