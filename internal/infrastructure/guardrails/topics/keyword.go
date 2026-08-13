package topics

import (
	"strings"
	"unicode"
)

// keywordHit reports whether a normalized (lowercased) prompt mentions the
// given topic. The match is forgiving:
//
//   - Multi-word topics ("software engineering") match if all of their
//     significant tokens appear in any order within the same prompt.
//   - Single-word topics match on word boundaries.
//
// This is a substring + token check, not a full ngram model — fast,
// dependency-free, and matches the dominant case where the user names the
// topic literally somewhere in the prompt.
func keywordHit(promptLower string, topic string) bool {
	topic = strings.ToLower(strings.TrimSpace(topic))
	if topic == "" || promptLower == "" {
		return false
	}
	// Fast path: full substring match. Catches the common case where the
	// topic phrase appears verbatim ("can you help me with kubernetes?").
	if strings.Contains(promptLower, topic) {
		return true
	}
	// Token path: every significant token (>=3 chars) in the topic must be
	// present in the prompt as a whole word.
	tokens := tokenize(topic)
	if len(tokens) == 0 {
		return false
	}
	promptTokens := tokenSet(promptLower)
	for _, tok := range tokens {
		if len(tok) < 3 {
			continue
		}
		if !promptTokens[tok] {
			return false
		}
	}
	return true
}

func tokenize(s string) []string {
	out := make([]string, 0, 4)
	cur := make([]rune, 0, 16)
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return out
}

func tokenSet(s string) map[string]bool {
	toks := tokenize(s)
	out := make(map[string]bool, len(toks))
	for _, t := range toks {
		out[t] = true
	}
	return out
}
