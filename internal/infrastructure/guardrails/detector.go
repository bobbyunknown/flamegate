package guardrails

import (
	"context"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// Detector is the plugin contract every guardrail implements. New detectors
// drop into the Engine via Register; the rest of the system stays oblivious
// to which detectors are configured.
//
// Inbound runs against the request before dispatch. A Decision with
// Action == ActionMask MUST populate Mutated and MutatedField so the engine
// can apply the rewrite back to the request.
//
// Outbound runs against an LLM response. Detectors that only inspect input
// return nil from Outbound; engines short-circuit on nil.
type Detector interface {
	Name() string
	Inbound(ctx context.Context, in *InboundRequest, policy Policy) (*Decision, error)
	Outbound(ctx context.Context, out *OutboundResponse, policy Policy) (*Decision, error)
}

// InboundRequest is the canonical view of a prompt that detectors inspect.
// It is built once by the engine and shared across all detectors so each one
// avoids re-scanning the underlying ChatRequest.
type InboundRequest struct {
	// Source is a pointer to the live ChatRequest so detectors can rewrite
	// fields in place (after producing a Decision with Action == ActionMask).
	Source *core.ChatRequest

	// FlatText is the concatenated user-visible content (System + Messages)
	// with delimiters, used by text-based detectors so they don't have to walk
	// the message slice themselves.
	FlatText string
}

// OutboundResponse is the canonical view of an LLM completion handed to
// outbound detectors. For streaming, the engine assembles the response as
// chunks arrive — see Engine.ScanChunk.
type OutboundResponse struct {
	Source *core.ChatResponse

	// Text is the model's textual output (concatenated content parts).
	Text string

	// Streaming is true when the engine is inspecting accumulating chunks
	// rather than a finalized response. Detectors that need the full text to
	// be useful (e.g. coherence-level bias detection) may return nil for
	// streaming calls and act only on the final assembled text.
	Streaming bool
}
