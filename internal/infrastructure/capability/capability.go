// Package capability maps models to the features they support, so the
// dispatcher never silently falls back to a model that cannot honor the
// request (e.g. routing a tool-calling request to a model without tools, or a
// vision request to a text-only model).
//
// Resolution is profile-driven: ResolveProfile derives a full Profile for a
// model via a four-step fallback chain (provider override, exact id, glob
// pattern, floor). Of/OfProvider then project that Profile onto the discrete
// core.CapabilitySet the routing layer guards on. Unknown models resolve to a
// safe floor (tools + 200k context + streaming) rather than being treated as
// text-only.
package capability

import (
	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// longContextThreshold is the context-window size (tokens) at or above which a
// model is considered long-context.
const longContextThreshold = 200000

// CapabilitiesFromServiceKind maps a dashboard media-service kind (e.g.
// "imageToText", "tts") to the capability set it implies, so user-defined media
// models are classified by their modality rather than as text-only. The second
// result is false when the kind is unknown.
func CapabilitiesFromServiceKind(kind string) (core.CapabilitySet, bool) {
	c, ok := capabilitiesFromServiceKind(kind)
	if !ok {
		return nil, false
	}
	return profileSet(c.merge(defaultProfile())), true
}

// Of returns the capability set for a model id, with no provider context.
func Of(model string) core.CapabilitySet {
	return OfProvider("", model)
}

// OfProvider returns the capability set for a model resolved in the context of
// its upstream provider, allowing provider-specific overrides to apply. Pass an
// empty provider when it is unknown.
func OfProvider(provider, model string) core.CapabilitySet {
	return profileSet(ResolveProfile(provider, model))
}

// profileSet projects a resolved Profile onto the discrete capability set used
// by the routing guard. Streaming is granted to every model; long context is
// derived from the context-window threshold.
func profileSet(p Profile) core.CapabilitySet {
	set := core.NewCapabilitySet(core.CapStreaming)
	if p.Tools {
		set.Add(core.CapToolCalling)
	}
	if p.Vision {
		set.Add(core.CapVision)
	}
	if p.AudioInput {
		set.Add(core.CapAudioInput)
	}
	if p.VideoInput {
		set.Add(core.CapVideoInput)
	}
	if p.PDF {
		set.Add(core.CapDocumentInput)
	}
	if p.ImageOutput {
		set.Add(core.CapImageOutput)
	}
	if p.AudioOutput {
		set.Add(core.CapAudioOutput)
	}
	if p.Search {
		set.Add(core.CapWebSearch)
	}
	if p.Reasoning {
		set.Add(core.CapReasoning)
	}
	if p.StructuredOutput {
		set.Add(core.CapStructuredOutput)
	}
	if p.ContextWindow >= longContextThreshold {
		set.Add(core.CapLongContext)
	}
	return set
}

// Supports reports whether a model satisfies all required capabilities, with no
// provider context.
func Supports(model string, required core.CapabilitySet) bool {
	return Of(model).Satisfies(required)
}

// SupportsProvider reports whether a model, resolved in the context of its
// upstream provider, satisfies all required capabilities.
func SupportsProvider(provider, model string, required core.CapabilitySet) bool {
	return OfProvider(provider, model).Satisfies(required)
}

// Required infers the capabilities a request needs from its content, so the
// dispatcher can reject incapable fallback targets. It is conservative: it only
// flags capabilities that a target genuinely cannot fake, so a fallback never
// silently breaks the request.
//
// Tool calling and input modalities (vision, audio) are hard requirements: a
// model without them cannot honor the request at all. Structured output and
// reasoning are intentionally NOT flagged — they are adapted downstream rather
// than refused (json_schema degrades to json_object for providers without
// native support, and thinking is normalized per provider), so gating routing
// on them would reject targets that can in fact serve the request.
func Required(req *core.ChatRequest) core.CapabilitySet {
	set := core.NewCapabilitySet()
	if len(req.Tools) > 0 {
		set.Add(core.CapToolCalling)
	}
	if req.Stream {
		set.Add(core.CapStreaming)
	}
	for _, m := range req.Messages {
		for _, p := range m.Content {
			switch p.Type {
			case core.PartImage:
				set.Add(core.CapVision)
			case core.PartAudio:
				set.Add(core.CapAudioInput)
			}
		}
	}
	return set
}

// strippableCaps are capabilities whose content can be soft-degraded by
// StripUnsupportedModalities (replaced with text placeholders). The dispatch
// guard does not enforce these because the pipeline handles them gracefully.
var strippableCaps = map[core.Capability]struct{}{
	core.CapVision:     {},
	core.CapAudioInput: {},
}

// HardRequired returns only the non-strippable subset of Required —
// capabilities the pipeline cannot soft-degrade. The dispatcher enforces these
// as a hard guard; strippable modalities (vision, audio) are left to the
// pipeline's modality stripping, so a request with images is never hard-rejected
// just because a model profile lacks vision.
func HardRequired(req *core.ChatRequest) core.CapabilitySet {
	return NonStrippable(Required(req))
}

// NonStrippable returns the subset of a capability set that cannot be
// soft-degraded by modality stripping. The dispatch guard enforces only these;
// strippable modalities (vision, audio) are handled by the pipeline.
func NonStrippable(required core.CapabilitySet) core.CapabilitySet {
	hard := core.NewCapabilitySet()
	for c := range required {
		if _, strippable := strippableCaps[c]; !strippable {
			hard.Add(c)
		}
	}
	return hard
}
