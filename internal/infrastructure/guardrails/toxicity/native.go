package toxicity

import (
	"context"
	"regexp"
	"strings"
	"sync"
)

// nativeEngine is the default, offline toxicity scorer. It maintains a
// keyword catalog per category covering id + en, scores each category as
// (hit_count × per-term weight), and normalizes the result to 0–100.
//
// The catalog favors precision over recall: terms are unambiguous, common
// slurs and profanity. Borderline cases (sarcasm, reclaimed language,
// quotation) are out of scope — users who want richer behavior route via
// engine="openai".
type nativeEngine struct {
	categories map[string]*nativeCategory
	once       sync.Once
}

// nativeCategory is a compiled keyword catalog for one category.
type nativeCategory struct {
	Name    string
	Pattern *regexp.Regexp
	// Cap is the hit count at which the category is considered "saturated"
	// and maps to score 100. One severe hit is often enough on its own —
	// tune per-category below.
	Cap int
}

func newNativeEngine() *nativeEngine {
	e := &nativeEngine{}
	e.init()
	return e
}

func (e *nativeEngine) init() {
	e.once.Do(func() {
		e.categories = map[string]*nativeCategory{
			"profanity":   compileCategory("profanity", profanityTerms, 3),
			"hate":        compileCategory("hate", hateTerms, 1),
			"hate_speech": compileCategory("hate_speech", hateTerms, 1), // alias
			"harassment":  compileCategory("harassment", harassmentTerms, 2),
			"violence":    compileCategory("violence", violenceTerms, 1),
			"sexual":      compileCategory("sexual", sexualTerms, 2),
		}
	})
}

// Score implements engine.
func (e *nativeEngine) Score(_ context.Context, text string, allowed map[string]bool) ([]CategoryScore, error) {
	if text == "" {
		return nil, nil
	}
	// Match against lowercased text — patterns are written lowercase + use
	// word boundaries so this stays accurate while avoiding (?i) overhead.
	lower := strings.ToLower(text)

	out := make([]CategoryScore, 0, len(e.categories))
	seen := make(map[string]bool, len(e.categories))
	for name, cat := range e.categories {
		if allowed != nil && !allowed[name] {
			continue
		}
		// Avoid double-counting hate/hate_speech (they share a catalog).
		base := canonicalCategory(name)
		if seen[base] {
			continue
		}
		seen[base] = true

		hits := cat.Pattern.FindAllStringIndex(lower, -1)
		if len(hits) == 0 {
			continue
		}
		score := (len(hits) * 100) / cat.Cap
		if score > 100 {
			score = 100
		}
		out = append(out, CategoryScore{Category: base, Score: score})
	}
	return out, nil
}

// canonicalCategory collapses aliases (hate ↔ hate_speech) to a single name
// so audit log entities stay stable.
func canonicalCategory(name string) string {
	if name == "hate_speech" {
		return "hate"
	}
	return name
}

// compileCategory builds a single alternation regex from the catalog. Each
// term is wrapped in `\b` to avoid matching inside larger words (so "ass"
// in "assignment" does not count).
func compileCategory(name string, terms []string, cap int) *nativeCategory {
	if cap <= 0 {
		cap = 1
	}
	escaped := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		escaped = append(escaped, regexp.QuoteMeta(strings.ToLower(t)))
	}
	pat := `\b(?:` + strings.Join(escaped, "|") + `)\b`
	return &nativeCategory{
		Name:    name,
		Pattern: regexp.MustCompile(pat),
		Cap:     cap,
	}
}

// The keyword catalogs below are intentionally short and unambiguous. They
// are not meant to be exhaustive — users who need broader coverage should
// switch to engine="openai" which calls a maintained moderation model.

var profanityTerms = []string{
	// English
	"shit", "fuck", "fucking", "asshole", "bastard", "bitch", "dick", "piss",
	"damn it", "goddamn",
	// Indonesian
	"anjing", "bangsat", "babi", "kontol", "memek", "tolol", "goblok",
	"brengsek", "tai", "asu",
}

var hateTerms = []string{
	// English — generic slurs and explicit hate phrases. Kept tight.
	"kill all", "death to", "subhuman", "inferior race", "racial purity",
	"white power", "ethnic cleansing",
	// Indonesian
	"bunuh semua", "matikan semua", "ras rendah", "ras unggul",
	"pembersihan etnis",
}

var harassmentTerms = []string{
	// English
	"you are worthless", "kill yourself", "kys", "i hate you", "loser",
	"piece of shit", "waste of space", "stupid idiot",
	// Indonesian
	"bunuh diri saja", "matilah", "tidak berguna", "sampah masyarakat",
	"goblok banget", "idiot banget",
}

var violenceTerms = []string{
	// English
	"how to kill", "make a bomb", "build a bomb", "shoot up", "stab to death",
	"beat to death", "behead", "lynching",
	// Indonesian
	"cara membunuh", "buat bom", "membuat bom", "tembak mati", "tikam mati",
	"penggal kepala",
}

var sexualTerms = []string{
	// English — explicit-only; avoid ambiguous anatomical words.
	"explicit sex", "pornographic", "child porn", "underage sex",
	"rape fantasy",
	// Indonesian
	"porno anak", "seks anak", "pemerkosaan", "fantasi perkosaan",
}
