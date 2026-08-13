package guardrails

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// retentionStore is the subset of persistence.GuardrailLogRepo the sweeper needs.
type retentionStore interface {
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// RetentionSweeper periodically deletes audit log rows older than the configured
// retention window. It's intentionally minimal: one ticker, one DELETE per
// fire. Self-hosted deployments often run on SQLite where running this in the
// hot path would contend with audit inserts, so we keep the cadence loose
// (hourly by default).
type RetentionSweeper struct {
	store     retentionStore
	log       *logrus.Logger
	retention time.Duration
	interval  time.Duration

	stopCh chan struct{}
	doneCh chan struct{}
}

// RetentionConfig configures the sweeper. Retention <= 0 disables it. When
// Interval is zero, the sweeper runs hourly.
type RetentionConfig struct {
	Retention time.Duration
	Interval  time.Duration
}

// NewRetentionSweeper builds (but does not start) the sweeper. Call Start to
// begin sweeping. Returns nil when retention is disabled — callers should
// nil-check before starting/stopping.
func NewRetentionSweeper(s retentionStore, log *logrus.Logger, cfg RetentionConfig) *RetentionSweeper {
	if cfg.Retention <= 0 || s == nil {
		return nil
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Hour
	}
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &RetentionSweeper{
		store:     s,
		log:       log,
		retention: cfg.Retention,
		interval:  cfg.Interval,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start launches the background sweeper.
func (r *RetentionSweeper) Start() {
	if r == nil {
		return
	}
	go r.run()
}

// Stop signals the sweeper to exit; safe to call multiple times.
func (r *RetentionSweeper) Stop(deadline time.Duration) {
	if r == nil {
		return
	}
	close(r.stopCh)
	select {
	case <-r.doneCh:
	case <-time.After(deadline):
		r.log.Warn("guardrails retention sweeper drain timed out")
	}
}

// Retention returns the configured retention window.
func (r *RetentionSweeper) Retention() time.Duration {
	if r == nil {
		return 0
	}
	return r.retention
}

func (r *RetentionSweeper) run() {
	defer close(r.doneCh)

	// Run once immediately so a freshly-started server with a stale DB doesn't
	// wait an hour for first cleanup.
	r.sweep()

	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-t.C:
			r.sweep()
		}
	}
}

func (r *RetentionSweeper) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cutoff := time.Now().UTC().Add(-r.retention)
	n, err := r.store.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		r.log.Warn("guardrails retention sweep failed", "err", err)
		return
	}
	if n > 0 {
		r.log.Info("guardrails retention sweep", "deleted", n, "cutoff", cutoff.Format(time.RFC3339))
	}
}
