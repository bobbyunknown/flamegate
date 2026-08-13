package pipeline

import (
	"context"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/shared/limits"
)

// This file extends the pipeline with the non-chat service kinds: embeddings,
// image generation, speech-to-text, text-to-speech, web search, and web fetch.
//
// Each method resolves an account for the requested provider/model via the
// dispatcher (reusing account selection, cooldowns, and credential opening),
// then type-asserts the connector to the matching capability interface and
// invokes it with fallback across the resolved attempts. Budget preflight runs
// the same way as chat; metering of media calls is best-effort (token usage is
// often unavailable for these endpoints).

// MediaOptions carries the resolved routing target for a media request.
type MediaOptions struct {
	// Targets is the ordered fallback chain (provider+model candidates).
	Targets []dispatch.Target
	// TenantID / ProjectID / APIKeyID scope budget and metering.
	TenantID  string
	ProjectID string
	APIKeyID  string
	Limits    limits.EffectiveLimits
}

// mediaAttempts resolves the ordered attempts for a media request and runs the
// budget preflight. Capability checks are skipped (media kinds aren't modeled
// in the chat capability matrix).
func (p *Pipeline) mediaAttempts(ctx context.Context, opts MediaOptions) ([]dispatch.Attempt, error) {
	if len(opts.Targets) == 0 {
		return nil, &core.ProviderError{Kind: core.ErrBadRequest, Message: "no routing targets resolved for model"}
	}
	if p.budget != nil {
		if err := p.budget.CheckOrError(ctx, budgetScope(opts)); err != nil {
			return nil, err
		}
	}
	return p.dispatcher.Plan(ctx, opts.TenantID, opts.Targets, core.NewCapabilitySet())
}

func (p *Pipeline) acquireMediaLimit(ctx context.Context, opts MediaOptions, estimatedTokens int64) (limits.ReleaseFunc, error) {
	if p.limiter == nil {
		return func(int64) {}, nil
	}
	provider, model := firstTarget(opts.Targets)
	release, decision, err := p.limiter.Acquire(ctx, limits.Request{
		TenantID:        opts.TenantID,
		ProjectID:       opts.ProjectID,
		APIKeyID:        opts.APIKeyID,
		Provider:        provider,
		Model:           model,
		EstimatedTokens: estimatedTokens,
		Limits:          opts.Limits,
	})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, rateLimitError(decision)
	}
	return release, nil
}

// Embeddings runs an embeddings request with fallback.
func (p *Pipeline) Embeddings(ctx context.Context, req *core.EmbeddingRequest, opts MediaOptions) (*core.EmbeddingResponse, string, error) {
	attempts, err := p.mediaAttempts(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	release, err := p.acquireMediaLimit(ctx, opts, estimateEmbeddingTokens(req))
	if err != nil {
		return nil, "", err
	}
	defer release(0)
	var lastErr error
	for _, a := range attempts {
		conn, ok := a.Conn.(core.MediaConnector)
		if !ok {
			lastErr = unsupported(a.Target.Provider, "embeddings")
			continue
		}
		r := *req
		r.Model = a.Target.Model
		resp, callErr := conn.Embeddings(ctx, &r, a.Creds)
		if callErr != nil {
			if !p.noteMediaFailure(ctx, a, callErr) {
				return nil, "", callErr
			}
			lastErr = callErr
			continue
		}
		return resp, a.Target.Provider, nil
	}
	return nil, "", orInternal(lastErr)
}

// GenerateImage runs an image-generation request with fallback.
func (p *Pipeline) GenerateImage(ctx context.Context, req *core.ImageRequest, opts MediaOptions) (*core.ImageResponse, string, error) {
	attempts, err := p.mediaAttempts(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	release, err := p.acquireMediaLimit(ctx, opts, 1)
	if err != nil {
		return nil, "", err
	}
	defer release(0)
	var lastErr error
	for _, a := range attempts {
		conn, ok := a.Conn.(core.ImageConnector)
		if !ok {
			lastErr = unsupported(a.Target.Provider, "image generation")
			continue
		}
		r := *req
		r.Model = a.Target.Model
		resp, callErr := conn.GenerateImage(ctx, &r, a.Creds)
		if callErr != nil {
			if !p.noteMediaFailure(ctx, a, callErr) {
				return nil, "", callErr
			}
			lastErr = callErr
			continue
		}
		return resp, a.Target.Provider, nil
	}
	return nil, "", orInternal(lastErr)
}

// Transcribe runs a speech-to-text request with fallback.
func (p *Pipeline) Transcribe(ctx context.Context, req *core.TranscriptionRequest, opts MediaOptions) (*core.TranscriptionResponse, string, error) {
	attempts, err := p.mediaAttempts(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	release, err := p.acquireMediaLimit(ctx, opts, 1)
	if err != nil {
		return nil, "", err
	}
	defer release(0)
	var lastErr error
	for _, a := range attempts {
		conn, ok := a.Conn.(core.TranscriptionConnector)
		if !ok {
			lastErr = unsupported(a.Target.Provider, "transcription")
			continue
		}
		r := *req
		r.Model = a.Target.Model
		resp, callErr := conn.Transcribe(ctx, &r, a.Creds)
		if callErr != nil {
			if !p.noteMediaFailure(ctx, a, callErr) {
				return nil, "", callErr
			}
			lastErr = callErr
			continue
		}
		return resp, a.Target.Provider, nil
	}
	return nil, "", orInternal(lastErr)
}

// Synthesize runs a text-to-speech request with fallback.
func (p *Pipeline) Synthesize(ctx context.Context, req *core.SpeechRequest, opts MediaOptions) (*core.SpeechResponse, string, error) {
	attempts, err := p.mediaAttempts(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	release, err := p.acquireMediaLimit(ctx, opts, int64((len(req.Input)+3)/4))
	if err != nil {
		return nil, "", err
	}
	defer release(0)
	var lastErr error
	for _, a := range attempts {
		conn, ok := a.Conn.(core.SpeechConnector)
		if !ok {
			lastErr = unsupported(a.Target.Provider, "speech synthesis")
			continue
		}
		r := *req
		r.Model = a.Target.Model
		resp, callErr := conn.Synthesize(ctx, &r, a.Creds)
		if callErr != nil {
			if !p.noteMediaFailure(ctx, a, callErr) {
				return nil, "", callErr
			}
			lastErr = callErr
			continue
		}
		return resp, a.Target.Provider, nil
	}
	return nil, "", orInternal(lastErr)
}

// Search runs a web-search request with fallback.
func (p *Pipeline) Search(ctx context.Context, req *core.SearchRequest, opts MediaOptions) (*core.SearchResponse, string, error) {
	attempts, err := p.mediaAttempts(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	release, err := p.acquireMediaLimit(ctx, opts, 1)
	if err != nil {
		return nil, "", err
	}
	defer release(0)
	var lastErr error
	for _, a := range attempts {
		conn, ok := a.Conn.(core.SearchConnector)
		if !ok {
			lastErr = unsupported(a.Target.Provider, "web search")
			continue
		}
		r := *req
		r.Model = a.Target.Model
		resp, callErr := conn.Search(ctx, &r, a.Creds)
		if callErr != nil {
			if !p.noteMediaFailure(ctx, a, callErr) {
				return nil, "", callErr
			}
			lastErr = callErr
			continue
		}
		return resp, a.Target.Provider, nil
	}
	return nil, "", orInternal(lastErr)
}

// Fetch runs a web-fetch request with fallback.
func (p *Pipeline) Fetch(ctx context.Context, req *core.FetchRequest, opts MediaOptions) (*core.FetchResponse, string, error) {
	attempts, err := p.mediaAttempts(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	release, err := p.acquireMediaLimit(ctx, opts, 1)
	if err != nil {
		return nil, "", err
	}
	defer release(0)
	var lastErr error
	for _, a := range attempts {
		conn, ok := a.Conn.(core.FetchConnector)
		if !ok {
			lastErr = unsupported(a.Target.Provider, "web fetch")
			continue
		}
		r := *req
		r.Model = a.Target.Model
		resp, callErr := conn.Fetch(ctx, &r, a.Creds)
		if callErr != nil {
			if !p.noteMediaFailure(ctx, a, callErr) {
				return nil, "", callErr
			}
			lastErr = callErr
			continue
		}
		return resp, a.Target.Provider, nil
	}
	return nil, "", orInternal(lastErr)
}

func estimateEmbeddingTokens(req *core.EmbeddingRequest) int64 {
	if req == nil {
		return 0
	}
	chars := 0
	for _, input := range req.Input {
		chars += len(input)
	}
	return int64((chars + 3) / 4)
}

// noteMediaFailure records a cooldown and reports whether the pipeline should
// keep trying the next attempt (true) or surface the error now (false).
func (p *Pipeline) noteMediaFailure(ctx context.Context, a dispatch.Attempt, err error) bool {
	pe := core.AsProviderError(err)
	p.dispatcher.NoteFailure(ctx, a.Account.ID, pe)
	if p.metrics != nil {
		p.metrics.RecordUpstreamError(a.Target.Provider, string(pe.Kind))
	}
	if !pe.Fallbackable() {
		return false
	}
	if p.metrics != nil {
		p.metrics.RecordFallback(string(pe.Kind))
	}
	p.log.Warn("media attempt failed, falling back",
		"provider", a.Target.Provider, "model", a.Target.Model, "kind", pe.Kind)
	return true
}

// unsupported builds a bad-request error for a provider that lacks a capability.
func unsupported(provider, capability string) error {
	return &core.ProviderError{Kind: core.ErrBadRequest, Provider: provider,
		Message: "provider " + provider + " does not support " + capability}
}

// orInternal returns err, or a generic internal error when nil.
func orInternal(err error) error {
	if err != nil {
		return err
	}
	return &core.ProviderError{Kind: core.ErrInternal, Message: "pipeline: no attempts executed"}
}

// budgetScope adapts media options to the budget engine's scope type.
func budgetScope(opts MediaOptions) budget.Scope {
	return budget.Scope{TenantID: opts.TenantID, ProjectID: opts.ProjectID, APIKeyID: opts.APIKeyID}
}
