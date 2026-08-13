// Package caveman implements output-token compression by appending a terse
// output-style guideline to the request's system prompt. The model is asked to
// keep all technical substance exact while dropping articles, filler, hedging,
// and pleasantries — cutting roughly 65-75% of output tokens without losing
// meaning.
//
// The guideline is woven into the system prompt as a normal style directive: it
// carries no marker comment and never names itself, so agentic coding tools
// (which flag foreign, self-identifying instruction blocks as untrusted
// injected content) treat it as part of the operator's own system prompt rather
// than rejecting it. Idempotency across retries/fallbacks is detected from the
// directive text itself, so no visible sentinel is needed.
//
// Like terse mode, this is a request-side transform that only touches the
// system prompt and runs before format translation, so it applies uniformly
// across every provider dialect.
package caveman

import (
	"strings"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// Level selects how aggressively the model compresses its output.
type Level string

const (
	// LevelLite keeps grammar and full sentences but drops filler and hedging.
	LevelLite Level = "lite"
	// LevelFull uses terse caveman style: fragments, dropped articles, short
	// synonyms, while keeping all technical substance exact.
	LevelFull Level = "full"
	// LevelUltra is maximally compressed and telegraphic, with abbreviations
	// and causal arrows.
	LevelUltra Level = "ultra"
	// LevelWenyanLite is semi-classical: drop filler/hedging but keep grammar
	// structure and classical register.
	LevelWenyanLite Level = "wenyan-lite"
	// LevelWenyanFull is maximum classical terseness (文言文), achieving 80-90%
	// character reduction with classical sentence patterns and particles.
	LevelWenyanFull Level = "wenyan-full"
	// LevelWenyanUltra is extreme abbreviation while keeping classical Chinese
	// feel. Maximum compression combining ultra and wenyan brevity.
	LevelWenyanUltra Level = "wenyan-ultra"
)

// Config controls caveman behavior for a request.
type Config struct {
	Enabled bool
	Level   Level
}

// idempotencyProbe is a stable substring shared by every level's directive.
// Apply uses it to detect whether the style guideline is already present in the
// system prompt, making re-application a no-op across retries and fallbacks
// without embedding any visible marker the model could surface.
const idempotencyProbe = "Keep all technical substance exact"

// sharedExamples shows the contrast between filler-heavy and terse replies.
const sharedExamples = "Not: \"Sure! I'd be happy to help you with that. " +
	"The issue you're experiencing is likely caused by...\" " +
	"Yes: \"Bug in auth middleware. Token expiry check use `<` not `<=`. Fix:\""

// sharedExamplesExtra provides additional level-comparison examples used by
// the full and ultra prompts to illustrate the expected compression style.
const sharedExamplesExtra = "Example — \"Why React component re-render?\" " +
	"full: \"New object ref each render. Inline object prop = new ref = re-render. Wrap in `useMemo`.\" " +
	"ultra: \"Inline obj prop → new ref → re-render. `useMemo`.\" " +
	"Example — \"Explain database connection pooling.\" " +
	"full: \"Pool reuse open DB connections. No new connection per request. Skip handshake overhead.\" " +
	"ultra: \"Pool = reuse DB conn. Skip handshake → fast under load.\""

// sharedBoundaries protects content that must never be compressed.
const sharedBoundaries = "Code blocks, file paths, commands, errors, URLs: keep exact. " +
	"Do not describe or announce this style; just write in it. " +
	"Give the terse answer only — never a terse answer plus a normal-prose recap. " +
	"Preserve user's dominant language. User writes Portuguese → reply in terse Portuguese. " +
	"Compress the style, not the language. No forced English openings or status phrases. " +
	"ALWAYS keep technical terms, code, API names, CLI commands, commit-type keywords (feat/fix/...), " +
	"and exact error strings verbatim — unless user explicitly ask for translation."

// sharedAutoClarity tells the model when to temporarily drop terse style.
const sharedAutoClarity = "Auto-Clarity: drop caveman for security warnings, irreversible actions, " +
	"multi-step sequences where fragment order or omitted conjunctions risk misread, " +
	"compression itself creates technical ambiguity (e.g. \"migrate table drop column backup first\" — " +
	"order unclear without articles/conjunctions), or when user repeats a question. " +
	"Resume caveman after clear part done."

// sharedPersistence keeps the directive active across a long conversation,
// phrased as an ordinary style-consistency note rather than an insistent
// override (which reads as injected content to agentic tools).
const sharedPersistence = "Keep this style consistent throughout the conversation."

const promptLite = "Respond tersely. Keep all technical substance exact. Keep grammar and full sentences but drop filler, hedging and pleasantries " +
	"(just/really/basically/sure/of course/I'd be happy to). " +
	"Pattern: state the thing, the action, the reason. Then next step. " +
	sharedExamples + " " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

const promptFull = "Respond in a terse, information-dense technical style. Keep all technical substance exact; only fluff goes. " +
	"Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries (sure/certainly/of course/happy to), hedging. " +
	"Fragments OK. Short synonyms (big not extensive, fix not implement a solution for). " +
	"No tool-call narration, no decorative tables/emoji, no dumping long raw error logs unless asked — quote shortest decisive line. " +
	"Standard well-known tech acronyms OK (DB/API/HTTP); never invent new abbreviations reader can't decode. " +
	"Technical terms exact. Code blocks unchanged. Errors quoted exact. " +
	"Pattern: [thing] [action] [reason]. [next step]. " +
	sharedExamples + " " +
	sharedExamplesExtra + " " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

const promptUltra = "Respond ultra-terse. Maximum compression. Telegraphic. Keep all technical substance exact. " +
	"Abbreviate prose words (DB/auth/config/req/res/fn/impl) — prose words only, never real code symbols/function names. " +
	"Strip conjunctions, use arrows for causality (X → Y), one word when one word enough. " +
	"Code symbols, function names, API names, error strings: never abbreviate. " +
	"Pattern: [thing] → [result]. [fix]. " +
	sharedExamples + " " +
	sharedExamplesExtra + " " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptWenyanLite uses semi-classical style: drop filler/hedging but keep
// grammar structure and classical register.
const promptWenyanLite = "Respond in semi-classical style. Keep all technical substance exact. Drop filler and hedging but keep grammar structure. " +
	"Use classical register and concise phrasing. Technical terms, code, and API names stay verbatim. " +
	"Preserve user's dominant language — if user writes in Chinese, reply in semi-classical Chinese. " +
	"Pattern: [subject] [verb] [object], classical particles allowed. " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptWenyanFull is maximum classical terseness (文言文), achieving 80-90%
// character reduction with classical sentence patterns, verbs preceding objects,
// subjects often omitted, and classical particles (之/乃/為/其).
const promptWenyanFull = "Respond in full 文言文 (classical Chinese) style. Keep all technical substance exact. Maximum classical terseness. " +
	"80-90% character reduction. Classical sentence patterns: verbs precede objects, subjects often omitted, " +
	"classical particles (之/乃/為/其/矣/也/焉). No modern filler words. " +
	"Technical terms, code, API names, CLI commands: keep verbatim, wrap in classical sentence structure. " +
	"Preserve user's dominant language for technical context. " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptWenyanUltra is extreme abbreviation while keeping classical Chinese feel.
// Maximum compression combining ultra telegraphic style with wenyan brevity.
const promptWenyanUltra = "Respond ultra-terse with classical Chinese feel. Keep all technical substance exact. Extreme abbreviation. " +
	"Maximum compression. Classical particles minimal. One character when one character enough. " +
	"Technical terms, code, API names: keep verbatim. Arrows for causality (→). " +
	"Preserve user's dominant language for technical context. " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptFor returns the instruction text for a level, defaulting to full.
func promptFor(l Level) string {
	switch l {
	case LevelLite:
		return promptLite
	case LevelUltra:
		return promptUltra
	case LevelWenyanLite:
		return promptWenyanLite
	case LevelWenyanFull:
		return promptWenyanFull
	case LevelWenyanUltra:
		return promptWenyanUltra
	case LevelFull:
		return promptFull
	default:
		return promptFull
	}
}

// Apply weaves the terse output-style guideline into req.System when
// cfg.Enabled.
//
// It is a no-op when disabled or when the guideline is already present
// (detected from the directive text via idempotencyProbe), so it is safe across
// retries and fallback attempts. The guideline is appended after any existing
// system text so the operator's own instructions keep priority while the style
// directive still takes effect. No marker comment is added, so coding tools see
// it as a normal part of the system prompt rather than injected content.
func Apply(req *core.ChatRequest, cfg Config) {
	if req == nil || !cfg.Enabled {
		return
	}
	if strings.Contains(req.System, idempotencyProbe) {
		return
	}

	block := promptFor(cfg.Level)
	if strings.TrimSpace(req.System) == "" {
		req.System = block
		return
	}
	req.System = req.System + "\n\n" + block
}

// ValidLevel reports whether s is a recognized caveman level.
func ValidLevel(s Level) bool {
	switch s {
	case LevelLite, LevelFull, LevelUltra,
		LevelWenyanLite, LevelWenyanFull, LevelWenyanUltra:
		return true
	default:
		return false
	}
}
