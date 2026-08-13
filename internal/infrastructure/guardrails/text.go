package guardrails

import core "github.com/bobbyunknown/flamegate/internal/domain"

// messageText returns the concatenated plain-text content of a message. It
// reuses core.Message.TextContent for the read side and pairs with
// setMessageText for the write side.
func messageText(m core.Message) string {
	return m.TextContent()
}

// setMessageText overwrites the text content of a message with the rewritten
// value. Non-text parts (images, tool calls, thinking) are preserved.
//
// When the message has at least one PartText part, only the FIRST PartText
// is replaced and the others are dropped — this matches the dominant case of
// a single text part per message and keeps downstream serialization simple.
// When the message has no PartText, we prepend one.
func setMessageText(m *core.Message, text string) {
	if m == nil {
		return
	}
	wrote := false
	out := m.Content[:0]
	for _, p := range m.Content {
		if p.Type == core.PartText {
			if !wrote {
				p.Text = text
				out = append(out, p)
				wrote = true
			}
			continue
		}
		out = append(out, p)
	}
	if !wrote {
		out = append([]core.ContentPart{{Type: core.PartText, Text: text}}, out...)
	}
	m.Content = out
}

// responseText returns the plain-text content of a chat response.
func responseText(r *core.ChatResponse) string {
	if r == nil {
		return ""
	}
	return r.Message.TextContent()
}

// setResponseText overwrites the response's text content with the masked
// value. Mirrors setMessageText semantics.
func setResponseText(r *core.ChatResponse, text string) {
	if r == nil {
		return
	}
	setMessageText(&r.Message, text)
}
