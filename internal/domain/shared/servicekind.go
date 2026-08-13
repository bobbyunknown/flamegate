package shared

// ServiceKind identifies a capability category that a provider or model serves.
//
// FlameGate started as a chat (+embedding) router; to reach feature parity with
// multi-modal gateways every provider/model now advertises one or more service
// kinds. The gateway exposes per-kind model discovery (GET /v1/models/<kind>)
// and routes each request to a connector that implements the matching kind.
type ServiceKind string

const (
	// ServiceLLM is chat / text completion (the default kind).
	ServiceLLM ServiceKind = "llm"
	// ServiceEmbedding produces vector embeddings.
	ServiceEmbedding ServiceKind = "embedding"
	// ServiceImage generates images from a prompt.
	ServiceImage ServiceKind = "image"
	// ServiceSTT transcribes audio to text (speech-to-text).
	ServiceSTT ServiceKind = "stt"
	// ServiceTTS synthesizes speech from text (text-to-speech).
	ServiceTTS ServiceKind = "tts"
	// ServiceSearch runs a web search and returns results.
	ServiceSearch ServiceKind = "search"
	// ServiceFetch retrieves and extracts the content of a URL.
	ServiceFetch ServiceKind = "fetch"
)

// AllServiceKinds lists every supported service kind in a stable order.
func AllServiceKinds() []ServiceKind {
	return []ServiceKind{
		ServiceLLM, ServiceEmbedding, ServiceImage,
		ServiceSTT, ServiceTTS, ServiceSearch, ServiceFetch,
	}
}

// ValidServiceKind reports whether s is a recognized service kind.
func ValidServiceKind(s ServiceKind) bool {
	switch s {
	case ServiceLLM, ServiceEmbedding, ServiceImage, ServiceSTT, ServiceTTS, ServiceSearch, ServiceFetch:
		return true
	default:
		return false
	}
}

// HasServiceKind reports whether kind is present in kinds. An empty kinds slice
// is treated as LLM-only, matching the provider catalog default.
func HasServiceKind(kinds []ServiceKind, kind ServiceKind) bool {
	if len(kinds) == 0 {
		return kind == ServiceLLM
	}
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}
