package guardrails

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// auditStore is the slice of persistence.GuardrailLogRepo the audit writer needs.
// Defined as an interface for tests.
type auditStore interface {
	BatchInsert(ctx context.Context, entries []schema.GuardrailLog) error
}

// AuditWriter buffers detector decisions in memory and batch-inserts them
// into the guardrail_logs table. It runs in a background goroutine so the
// request path never blocks on a SQL write — audit is best-effort.
type AuditWriter struct {
	store auditStore
	log   *logrus.Logger
	hub   *LogHub

	buf chan schema.GuardrailLog

	flushInterval time.Duration
	flushSize     int

	dropped atomic.Int64

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	// flushReq is a non-blocking signal channel: a send on it wakes the
	// background goroutine to flush immediately. Used by FlushNow() so the
	// dashboard's test panel sees its row in audit logs without waiting for
	// the next tick. Buffered(1) so concurrent senders never block; once a
	// flush is queued we don't need a second one.
	flushReq chan struct{}
}

// AuditWriterConfig tunes the writer. Defaults: buffer 1024, flush every 1s
// or when 100 rows accumulate.
type AuditWriterConfig struct {
	BufferSize    int
	FlushInterval time.Duration
	FlushSize     int
	// Hub, if set, receives every successfully-flushed row so the gateway's
	// SSE stream can fan out audit events to dashboard clients.
	Hub *LogHub
}

// NewAuditWriter starts a background writer. The caller MUST call Stop on
// shutdown to drain remaining entries.
func NewAuditWriter(s auditStore, log *logrus.Logger, cfg AuditWriterConfig) *AuditWriter {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1024
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.FlushSize <= 0 {
		cfg.FlushSize = 100
	}
	if log == nil {
		log = logrus.StandardLogger()
	}
	w := &AuditWriter{
		store:         s,
		log:           log,
		hub:           cfg.Hub,
		buf:           make(chan schema.GuardrailLog, cfg.BufferSize),
		flushInterval: cfg.FlushInterval,
		flushSize:     cfg.FlushSize,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		flushReq:      make(chan struct{}, 1),
	}
	go w.run()
	return w
}

// Write enqueues a decision. It NEVER blocks — if the buffer is full, the row
// is dropped and a counter is incremented. The pipeline path stays fast.
func (w *AuditWriter) Write(ctx context.Context, e schema.GuardrailLog) {
	if w == nil || w.store == nil {
		return
	}
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	select {
	case w.buf <- e:
	default:
		w.dropped.Add(1)
	}
}

// FlushNow signals the background goroutine to flush whatever is currently
// buffered without waiting for the next tick. Used by the admin "test panel"
// so users see their row appear in the audit log immediately. The call is
// non-blocking — if a flush is already pending the signal is coalesced.
func (w *AuditWriter) FlushNow() {
	if w == nil {
		return
	}
	select {
	case w.flushReq <- struct{}{}:
	default:
	}
}

// Stop shuts down the writer and drains pending entries with a deadline.
func (w *AuditWriter) Stop(deadline time.Duration) {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stopCh) })
	select {
	case <-w.doneCh:
	case <-time.After(deadline):
		w.log.Warn("guardrails audit writer drain timed out", "pending", len(w.buf))
	}
}

// Dropped returns the count of rows lost to a full buffer since startup.
// Exposed for /api/system telemetry.
func (w *AuditWriter) Dropped() int64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

func (w *AuditWriter) run() {
	defer close(w.doneCh)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	batch := make([]schema.GuardrailLog, 0, w.flushSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.store.BatchInsert(ctx, batch); err != nil {
			w.log.Warn("guardrails audit batch insert failed", "err", err, "count", len(batch))
		} else if w.hub != nil {
			// Publish after successful insert so subscribers only see rows
			// that actually landed in the database.
			for _, e := range batch {
				w.hub.Publish(e)
			}
		}
		batch = batch[:0]
	}

	// drainPending pulls every immediately-ready row off the channel and
	// appends it to batch, flushing whenever flushSize is reached. Returns
	// once the channel has nothing else queued.
	drainPending := func() {
		for {
			select {
			case e := <-w.buf:
				batch = append(batch, e)
				if len(batch) >= w.flushSize {
					flush()
				}
			default:
				return
			}
		}
	}

	for {
		select {
		case <-w.stopCh:
			drainPending()
			flush()
			return
		case <-ticker.C:
			flush()
		case <-w.flushReq:
			drainPending()
			flush()
		case e := <-w.buf:
			batch = append(batch, e)
			if len(batch) >= w.flushSize {
				flush()
			}
		}
	}
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "gl_" + hex.EncodeToString(b[:])
}
