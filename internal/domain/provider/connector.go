package provider

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/bobbyunknown/flamegate/internal/domain/shared"
)

// StreamConfig carries per-request stream instrumentation. Connectors that
// support it wire OnFirstChunk into their SSE scanning so the pipeline can
// measure time-to-first-token (TTFT).
type StreamConfig struct {
	// StartedAt is the absolute time the upstream call was initiated (set by
	// the pipeline before calling Stream/StreamRaw). Connectors use it as the
	// TTFT reference point so the measurement includes HTTP connection setup
	// and header wait time. When zero, connectors fall back to their own
	// scan-start time (less accurate).
	StartedAt time.Time

	// OnFirstChunk, if non-nil, is called once when the first meaningful
	// chunk (text, thinking, or tool_call) arrives from the upstream. The
	// elapsed duration is measured from StartedAt (or scan start when zero).
	OnFirstChunk func(elapsed time.Duration)
}

// Connector is the contract every provider driver implements. The pipeline
// selects a connector + credentials, hands it a canonical ChatRequest, and the
// connector is responsible for dialect rendering, HTTP transport, and parsing
// the upstream response back into canonical form.
//
// Implementations must be safe for concurrent use: a single Connector value
// serves many in-flight requests.
type Connector interface {
	// ID returns the stable provider identifier (e.g. "openai", "anthropic").
	ID() string

	// Dialect reports the wire format this connector speaks upstream. The
	// transform layer uses it to decide whether translation is needed.
	Dialect() shared.Dialect

	// Chat performs a non-streaming completion.
	Chat(ctx context.Context, req *shared.ChatRequest, creds Credentials) (*shared.ChatResponse, error)

	// Stream performs a streaming completion, emitting canonical chunks on the
	// returned channel until it is closed. A terminal error is delivered as a
	// ChunkError followed by channel close, so callers select on ctx.Done and
	// range over the channel. cfg carries optional stream instrumentation; a
	// zero value is valid.
	Stream(ctx context.Context, req *shared.ChatRequest, creds Credentials, cfg StreamConfig) (<-chan shared.StreamChunk, error)
}

// Credentials carries the resolved secret material a connector needs for a
// single upstream call. The vault decrypts and supplies these; connectors must
// never persist them.
type Credentials struct {
	// AccountID identifies which provider account these belong to (metering).
	AccountID string
	// APIKey is set for key-based providers.
	APIKey string
	// AccessToken is set for OAuth/bearer providers.
	AccessToken string
	// BaseURL overrides the connector's default endpoint when non-empty.
	BaseURL string
	// Headers are extra headers to merge into the upstream request.
	Headers map[string]string
	// Extra holds provider-specific fields (project id, region, deployment...).
	Extra map[string]string

	// Proxy config (resolved from proxy pool at dispatch time).
	ProxyURL    string // HTTP/HTTPS/SOCKS proxy URL
	RelayURL    string // vercel/cloudflare/deno relay URL (mutually exclusive with ProxyURL)
	NoProxy     string // comma-separated bypass hosts
	StrictProxy bool   // fail request when proxy unreachable
}

// Validator is optionally implemented by connectors that can probe the upstream
// to confirm credentials are valid before persisting an account. The gateway
// calls Validate during account creation and on explicit connection-test
// requests.
type Validator interface {
	// Validate probes the upstream with the given credentials and returns nil
	// if they are accepted, or a descriptive error otherwise.
	Validate(ctx context.Context, creds Credentials) error
}

// MediaConnector is implemented by providers that support embeddings. It is
// optional; chat-only connectors omit it.
type MediaConnector interface {
	// Embeddings produces vector embeddings for the given inputs.
	Embeddings(ctx context.Context, req *EmbeddingRequest, creds Credentials) (*EmbeddingResponse, error)
}

// ImageConnector is implemented by providers that generate images.
type ImageConnector interface {
	GenerateImage(ctx context.Context, req *ImageRequest, creds Credentials) (*ImageResponse, error)
}

// TranscriptionConnector is implemented by providers that transcribe audio
// (speech-to-text).
type TranscriptionConnector interface {
	Transcribe(ctx context.Context, req *TranscriptionRequest, creds Credentials) (*TranscriptionResponse, error)
}

// SpeechConnector is implemented by providers that synthesize speech
// (text-to-speech).
type SpeechConnector interface {
	Synthesize(ctx context.Context, req *SpeechRequest, creds Credentials) (*SpeechResponse, error)
}

// SearchConnector is implemented by providers that run web searches.
type SearchConnector interface {
	Search(ctx context.Context, req *SearchRequest, creds Credentials) (*SearchResponse, error)
}

// FetchConnector is implemented by providers that fetch and extract URL
// content.
type FetchConnector interface {
	Fetch(ctx context.Context, req *FetchRequest, creds Credentials) (*FetchResponse, error)
}

// DirectStreamable is optionally implemented by connectors that can return the
// raw upstream SSE stream as an io.ReadCloser. The pipeline uses this for
// zero-copy same-dialect streaming: when the client dialect matches the
// upstream dialect and no tool-argument sanitization is needed, the raw bytes
// are piped directly to the client via io.Copy, bypassing all JSON parse and
// re-serialize overhead.
//
// Connectors that implement this MUST:
//   - Return a response whose body is valid SSE (data: lines with \n\n delimiters)
//   - Close the body when the stream ends or ctx is canceled
//   - Not start any goroutines — the caller owns the read loop
type DirectStreamable interface {
	// StreamRaw opens an SSE connection to the upstream and returns the raw
	// response body. The caller is responsible for closing body when done.
	// Headers contains the upstream response headers (for content-type, etc).
	StreamRaw(ctx context.Context, req *shared.ChatRequest, creds Credentials, cfg StreamConfig) (body io.ReadCloser, headers http.Header, err error)
}

// EmbeddingRequest is a canonical embeddings request.
type EmbeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// EmbeddingResponse holds the resulting vectors and token usage.
type EmbeddingResponse struct {
	Model   string       `json:"model"`
	Vectors [][]float32  `json:"vectors"`
	Usage   shared.Usage `json:"usage"`
}
