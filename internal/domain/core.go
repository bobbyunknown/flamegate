// Package core is a compatibility re-export shim.
// Remove once all callers import domain/shared and domain/provider directly.
package core

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/domain/provider"
	"github.com/bobbyunknown/flamegate/internal/domain/shared"
)

// Type aliases — shared
type Capability = shared.Capability
type CapabilitySet = shared.CapabilitySet
type ErrorKind = shared.ErrorKind
type ProviderError = shared.ProviderError
type Role = shared.Role
type PartType = shared.PartType
type ContentPart = shared.ContentPart
type MediaPayload = shared.MediaPayload
type ToolCall = shared.ToolCall
type ToolResult = shared.ToolResult
type Message = shared.Message
type Dialect = shared.Dialect
type ChatRequest = shared.ChatRequest
type ReasoningConfig = shared.ReasoningConfig
type Tool = shared.Tool
type RequestMetadata = shared.RequestMetadata
type FinishReason = shared.FinishReason
type Usage = shared.Usage
type ChatResponse = shared.ChatResponse
type ChunkType = shared.ChunkType
type StreamChunk = shared.StreamChunk
type ServiceKind = shared.ServiceKind

// Type aliases — provider
type StreamConfig = provider.StreamConfig
type Connector = provider.Connector
type Credentials = provider.Credentials
type Validator = provider.Validator
type MediaConnector = provider.MediaConnector
type ImageConnector = provider.ImageConnector
type TranscriptionConnector = provider.TranscriptionConnector
type SpeechConnector = provider.SpeechConnector
type SearchConnector = provider.SearchConnector
type FetchConnector = provider.FetchConnector
type DirectStreamable = provider.DirectStreamable
type EmbeddingRequest = provider.EmbeddingRequest
type EmbeddingResponse = provider.EmbeddingResponse
type ImageRequest = provider.ImageRequest
type ImageData = provider.ImageData
type ImageResponse = provider.ImageResponse
type TranscriptionRequest = provider.TranscriptionRequest
type TranscriptionResponse = provider.TranscriptionResponse
type SpeechRequest = provider.SpeechRequest
type SpeechResponse = provider.SpeechResponse
type SearchRequest = provider.SearchRequest
type SearchResult = provider.SearchResult
type SearchResponse = provider.SearchResponse
type FetchRequest = provider.FetchRequest
type FetchResponse = provider.FetchResponse

// Const aliases — shared
const (
	ErrAuth = shared.ErrAuth
	ErrRateLimit = shared.ErrRateLimit
	ErrQuotaExhausted = shared.ErrQuotaExhausted
	ErrUpstream = shared.ErrUpstream
	ErrTimeout = shared.ErrTimeout
	ErrBadRequest = shared.ErrBadRequest
	ErrCapability = shared.ErrCapability
	ErrBudgetBlocked = shared.ErrBudgetBlocked
	ErrPolicyBlocked = shared.ErrPolicyBlocked
	ErrInternal = shared.ErrInternal
	RoleSystem = shared.RoleSystem
	RoleUser = shared.RoleUser
	RoleAssistant = shared.RoleAssistant
	RoleTool = shared.RoleTool
	PartText = shared.PartText
	PartImage = shared.PartImage
	PartAudio = shared.PartAudio
	PartToolCall = shared.PartToolCall
	PartToolResult = shared.PartToolResult
	PartThinking = shared.PartThinking
	DialectOpenAI = shared.DialectOpenAI
	DialectOpenAIResponses = shared.DialectOpenAIResponses
	DialectAnthropic = shared.DialectAnthropic
	DialectGemini = shared.DialectGemini
	DialectOllama = shared.DialectOllama
	DialectKiro = shared.DialectKiro
	DialectGeminiCLI = shared.DialectGeminiCLI
	DialectVertex = shared.DialectVertex
	DialectCursor = shared.DialectCursor
	DialectAntigravity = shared.DialectAntigravity
	DialectCommandCode = shared.DialectCommandCode
	DialectQoder = shared.DialectQoder
	DialectMimoFree = shared.DialectMimoFree
	DialectWebCookie = shared.DialectWebCookie
	FinishStop = shared.FinishStop
	FinishLength = shared.FinishLength
	FinishToolCalls = shared.FinishToolCalls
	FinishError = shared.FinishError
	FinishFilter = shared.FinishFilter
	ChunkText = shared.ChunkText
	ChunkThinking = shared.ChunkThinking
	ChunkToolCall = shared.ChunkToolCall
	ChunkUsage = shared.ChunkUsage
	ChunkFinish = shared.ChunkFinish
	ChunkError = shared.ChunkError
	ChunkPing = shared.ChunkPing
	CapToolCalling = shared.CapToolCalling
	CapVision = shared.CapVision
	CapAudioInput = shared.CapAudioInput
	CapVideoInput = shared.CapVideoInput
	CapDocumentInput = shared.CapDocumentInput
	CapImageOutput = shared.CapImageOutput
	CapAudioOutput = shared.CapAudioOutput
	CapWebSearch = shared.CapWebSearch
	CapReasoning = shared.CapReasoning
	CapStructuredOutput = shared.CapStructuredOutput
	CapLongContext = shared.CapLongContext
	CapStreaming = shared.CapStreaming
	ServiceLLM = shared.ServiceLLM
	ServiceEmbedding = shared.ServiceEmbedding
	ServiceImage = shared.ServiceImage
	ServiceSTT = shared.ServiceSTT
	ServiceTTS = shared.ServiceTTS
	ServiceSearch = shared.ServiceSearch
	ServiceFetch = shared.ServiceFetch
)

// Sentinel error re-exports — shared
var (
	ErrNotFound     = shared.ErrNotFound
	ErrConflict     = shared.ErrConflict
	ErrValidation   = shared.ErrValidation
	ErrUnauthorized = shared.ErrUnauthorized
	ErrForbidden    = shared.ErrForbidden
)

// Func wrappers — shared
func NewCapabilitySet(caps ...Capability) CapabilitySet { return shared.NewCapabilitySet(caps...) }
func AsProviderError(err error) *ProviderError { return shared.AsProviderError(err) }
func NewProviderError(kind ErrorKind, msg string) *ProviderError { return shared.NewProviderError(kind, msg) }
func AllServiceKinds() []ServiceKind { return shared.AllServiceKinds() }
func ValidServiceKind(s ServiceKind) bool { return shared.ValidServiceKind(s) }
func HasServiceKind(kinds []ServiceKind, kind ServiceKind) bool { return shared.HasServiceKind(kinds, kind) }
func EstimateTokensFromChars(chars int) int { return shared.EstimateTokensFromChars(chars) }
func EstimatePromptTokens(req *ChatRequest) int { return shared.EstimatePromptTokens(req) }

// Func wrappers — provider
func WithProxy(ctx context.Context, creds Credentials) context.Context { return provider.WithProxy(ctx, creds) }
func ProxyFromContext(ctx context.Context) (Credentials, bool) { return provider.ProxyFromContext(ctx) }
