// Package capability provides modality stripping for requests whose resolved
// model profile cannot handle every input modality the request carries.
//
// The pipeline calls StripUnsupportedModalities on the canonical request before
// the connector translates it to the upstream wire format. Input modalities the
// model cannot read (vision, audio) are replaced with short text placeholders,
// so the upstream receives a valid request it can process rather than being
// rejected or sent bytes it cannot decode.
//
// Operating on the canonical core.ChatRequest (rather than each source dialect)
// means one code path covers every client format. Only hard input modalities
// are stripped; tool calls, tool results, text, and thinking blocks are always
// preserved — a model that lacks tool calling simply will not emit tool calls,
// and dropping tool history would break tool-result pairing.
package capability

import (
	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// Placeholder text substituted where a media part is removed. The current turn
// gets an explanatory message (the user just sent media this model can't read);
// earlier turns get a neutral note, since a fallback chain may route different
// turns to different models and history media should not imply the active model
// lacks a modality.
const (
	placeholderImageCurrent = "[image omitted: model has no vision support]"
	placeholderImagePrev    = "[previous image omitted from context]"
	placeholderAudioCurrent = "[audio omitted: model has no audio support]"
	placeholderAudioPrev    = "[previous audio omitted from context]"
)

// StripUnsupportedModalities replaces input-modality content parts the resolved
// model profile cannot handle with text placeholders. It mutates req in place
// and returns true if any part was stripped.
//
// Only hard input modalities (vision, audio) are stripped. Text, tool calls,
// tool results, and thinking blocks are always preserved. The placeholder for
// the final message (the current user turn) is explanatory; earlier turns use a
// neutral note.
func StripUnsupportedModalities(req *core.ChatRequest, provider, model string) bool {
	if req == nil || len(req.Messages) == 0 {
		return false
	}
	caps := OfProvider(provider, model)
	hasVision := caps.Has(core.CapVision)
	hasAudioInput := caps.Has(core.CapAudioInput)

	// Fast exit: the model supports every modality we would strip, so there is
	// nothing to do regardless of request content.
	if hasVision && hasAudioInput {
		return false
	}

	last := len(req.Messages) - 1
	stripped := false
	for i := range req.Messages {
		isCurrent := i == last
		msg := &req.Messages[i]
		for j := range msg.Content {
			p := &msg.Content[j]
			switch p.Type {
			case core.PartImage:
				if !hasVision {
					*p = core.ContentPart{Type: core.PartText, Text: imagePlaceholder(isCurrent)}
					stripped = true
				}
			case core.PartAudio:
				if !hasAudioInput {
					*p = core.ContentPart{Type: core.PartText, Text: audioPlaceholder(isCurrent)}
					stripped = true
				}
			}
		}
	}
	return stripped
}

func imagePlaceholder(current bool) string {
	if current {
		return placeholderImageCurrent
	}
	return placeholderImagePrev
}

func audioPlaceholder(current bool) string {
	if current {
		return placeholderAudioCurrent
	}
	return placeholderAudioPrev
}
