package shared

// Capability is a discrete model feature used by the routing layer to prevent
// silent quality downgrades. When a request requires a capability, the
// dispatcher will only fall back to models that also advertise it.
type Capability string

const (
	CapToolCalling      Capability = "tool_calling"
	CapVision           Capability = "vision"
	CapAudioInput       Capability = "audio_input"
	CapVideoInput       Capability = "video_input"
	CapDocumentInput    Capability = "document_input"
	CapImageOutput      Capability = "image_output"
	CapAudioOutput      Capability = "audio_output"
	CapWebSearch        Capability = "web_search"
	CapReasoning        Capability = "reasoning"
	CapStructuredOutput Capability = "structured_output"
	CapLongContext      Capability = "long_context" // >= 200k context window
	CapStreaming        Capability = "streaming"
)

// CapabilitySet is a set of capabilities with convenient set operations.
type CapabilitySet map[Capability]struct{}

// NewCapabilitySet builds a set from the given capabilities.
func NewCapabilitySet(caps ...Capability) CapabilitySet {
	s := make(CapabilitySet, len(caps))
	for _, c := range caps {
		s[c] = struct{}{}
	}
	return s
}

// Add inserts a capability.
func (s CapabilitySet) Add(c Capability) { s[c] = struct{}{} }

// Has reports whether c is present.
func (s CapabilitySet) Has(c Capability) bool {
	_, ok := s[c]
	return ok
}

// Satisfies reports whether s contains every capability in required. An empty
// requirement set is always satisfied.
func (s CapabilitySet) Satisfies(required CapabilitySet) bool {
	for c := range required {
		if !s.Has(c) {
			return false
		}
	}
	return true
}

// Slice returns the capabilities as a slice (unordered).
func (s CapabilitySet) Slice() []Capability {
	out := make([]Capability, 0, len(s))
	for c := range s {
		out = append(out, c)
	}
	return out
}
