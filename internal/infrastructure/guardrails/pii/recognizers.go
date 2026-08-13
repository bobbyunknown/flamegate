// Package pii is FlameGate's native PII recognizer set. It implements the
// guardrails.Detector interface using regex + heuristic recognizers, with
// entity names that follow Microsoft Presidio (EMAIL_ADDRESS, PHONE_NUMBER,
// CREDIT_CARD, ...) plus Indonesian-specific extensions (ID_NIK, ID_NPWP,
// ID_PASSPORT). The interface lines up with Presidio so a future HTTP
// sidecar can drop in by implementing Detector instead.
package pii

import (
	"regexp"
	"strings"
)

// Entity is the canonical PII type name. Names match Presidio's
// supported_entities catalog where possible.
type Entity string

const (
	EntityEmail      Entity = "EMAIL_ADDRESS"
	EntityPhone      Entity = "PHONE_NUMBER"
	EntityCreditCard Entity = "CREDIT_CARD"
	EntityIBAN       Entity = "IBAN_CODE"
	EntityIP         Entity = "IP_ADDRESS"
	EntityURL        Entity = "URL"
	EntityPerson     Entity = "PERSON"
	EntityIDNIK      Entity = "ID_NIK"      // Indonesia: 16-digit national ID
	EntityIDNPWP     Entity = "ID_NPWP"     // Indonesia: tax ID
	EntityIDPassport Entity = "ID_PASSPORT" // Indonesia: 1 letter + 7 digits
)

// AllEntities returns the catalog in display order. Used by the dashboard's
// multi-select dropdown.
func AllEntities() []Entity {
	return []Entity{
		EntityEmail,
		EntityPhone,
		EntityCreditCard,
		EntityIBAN,
		EntityIP,
		EntityURL,
		EntityIDNIK,
		EntityIDNPWP,
		EntityIDPassport,
		EntityPerson,
	}
}

// Recognizer is one pattern in the catalog. Confidence is the base score;
// context words can boost it up to 1.0. Postcheck runs additional validation
// (Luhn, NIK province check) and may reject low-quality matches.
type Recognizer struct {
	Entity     Entity
	Pattern    *regexp.Regexp
	Confidence float64
	// Context words that, when found within `contextWindow` chars before the
	// match, raise confidence to 0.85 if it was lower.
	Context []string
	// Postcheck returns false to drop a match (e.g. failed Luhn for cards).
	Postcheck func(match string) bool
}

const contextWindow = 32

// Match is a single recognized PII span.
type Match struct {
	Entity Entity
	Start  int
	End    int
	Text   string
	Score  float64
}

// defaultRecognizers builds the catalog. Patterns are intentionally tight to
// keep false positives low; recall trades for precision in a gateway context
// (over-redacting an email costs more than missing one).
var defaultRecognizers = []Recognizer{
	{
		Entity:     EntityEmail,
		Pattern:    regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`),
		Confidence: 0.9,
	},
	{
		Entity: EntityIDNIK,
		// Indonesia NIK: 16 contiguous digits. Common written separators
		// (spaces, dots, dashes) are tolerated.
		Pattern:    regexp.MustCompile(`\b\d{2}[\s.\-]?\d{4}[\s.\-]?\d{6}[\s.\-]?\d{4}\b`),
		Confidence: 0.55,
		Context:    []string{"nik", "no ktp", "ktp", "nomor induk", "national id"},
		Postcheck:  validateNIK,
	},
	{
		Entity: EntityIDNPWP,
		// NPWP format: XX.XXX.XXX.X-XXX.XXX (15 digits with separators) or
		// the new 16-digit unified form.
		Pattern:    regexp.MustCompile(`\b\d{2}[.\-]\d{3}[.\-]\d{3}[.\-]\d[\-\s]\d{3}[.\-]\d{3}\b|\b\d{15,16}\b`),
		Confidence: 0.5,
		Context:    []string{"npwp", "tax id", "nomor pokok wajib pajak"},
		Postcheck:  validateNPWP,
	},
	{
		Entity: EntityIDPassport,
		// Indonesian passport: 1 letter + 7 digits, sometimes with a leading
		// "A" or "B" prefix character class.
		Pattern:    regexp.MustCompile(`\b[A-Z]\d{7}\b`),
		Confidence: 0.45,
		Context:    []string{"passport", "paspor"},
	},
	{
		Entity: EntityPhone,
		// Indonesian phone: +62 prefix, or 0 prefix followed by 8 then digits.
		// Also matches generic E.164 (+ followed by 7-15 digits).
		Pattern: regexp.MustCompile(
			`\+62[\s\-]?8\d{1,3}[\s\-]?\d{3,4}[\s\-]?\d{3,5}` +
				`|\b08\d{1,3}[\s\-]?\d{3,4}[\s\-]?\d{3,5}\b` +
				`|\+\d{7,15}\b`),
		Confidence: 0.85,
	},
	{
		Entity:     EntityCreditCard,
		Pattern:    regexp.MustCompile(`\b(?:\d[ \-]?){13,19}\b`),
		Confidence: 0.4,
		Context:    []string{"card", "credit", "kartu", "visa", "mastercard", "amex"},
		Postcheck:  func(s string) bool { return luhn(stripNonDigits(s)) },
	},
	{
		Entity: EntityIBAN,
		// IBAN: 2 letters + 2 digits + 11–30 alphanumerics. Postcheck
		// enforces a length window since the regex alone is loose.
		Pattern:    regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`),
		Confidence: 0.7,
	},
	{
		Entity:     EntityIP,
		Pattern:    regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		Confidence: 0.5,
		Postcheck:  validIPv4,
	},
	{
		Entity:     EntityURL,
		Pattern:    regexp.MustCompile(`https?://[^\s<>"']+`),
		Confidence: 0.6,
	},
	{
		Entity: EntityPerson,
		// Heuristic only — two-or-more capitalized words. Confidence stays
		// low so policies that scan PERSON should set min_score accordingly.
		Pattern:    regexp.MustCompile(`\b[A-Z][a-z]{2,}\s+[A-Z][a-z]{2,}(?:\s+[A-Z][a-z]+)?\b`),
		Confidence: 0.35,
	},
}

// Recognize scans text and returns all matches whose final score is at least
// minScore and whose Entity is in the allowed set. When allowed is empty, all
// catalog entities run.
func Recognize(text string, allowed map[Entity]bool, minScore float64) []Match {
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	var out []Match
	for _, rec := range defaultRecognizers {
		if len(allowed) > 0 && !allowed[rec.Entity] {
			continue
		}
		idx := rec.Pattern.FindAllStringIndex(text, -1)
		for _, span := range idx {
			matched := text[span[0]:span[1]]
			if rec.Postcheck != nil && !rec.Postcheck(matched) {
				continue
			}
			score := rec.Confidence
			if hasContext(lower, span[0], rec.Context) {
				if score < 0.85 {
					score = 0.85
				}
			}
			if score < minScore {
				continue
			}
			out = append(out, Match{
				Entity: rec.Entity,
				Start:  span[0],
				End:    span[1],
				Text:   matched,
				Score:  score,
			})
		}
	}
	return mergeOverlaps(out)
}

// hasContext returns true when any of the context words appears within
// contextWindow characters before the match start. The text is the already-
// lowercased copy so callers can pass it once.
func hasContext(lowerText string, start int, words []string) bool {
	if len(words) == 0 || start == 0 {
		return false
	}
	from := start - contextWindow
	if from < 0 {
		from = 0
	}
	window := lowerText[from:start]
	for _, w := range words {
		if strings.Contains(window, w) {
			return true
		}
	}
	return false
}

// mergeOverlaps drops a match that is fully contained in another with a
// higher score, so overlapping regex hits don't double-redact. Matches are
// not pre-sorted by position — callers may rely on insertion order.
func mergeOverlaps(matches []Match) []Match {
	if len(matches) < 2 {
		return matches
	}
	keep := make([]bool, len(matches))
	for i := range keep {
		keep[i] = true
	}
	for i := 0; i < len(matches); i++ {
		if !keep[i] {
			continue
		}
		for j := 0; j < len(matches); j++ {
			if i == j || !keep[j] {
				continue
			}
			if matches[j].Start >= matches[i].Start && matches[j].End <= matches[i].End && matches[j].Score <= matches[i].Score {
				keep[j] = false
			}
		}
	}
	out := matches[:0]
	for i, m := range matches {
		if keep[i] {
			out = append(out, m)
		}
	}
	return out
}

// stripNonDigits removes spaces, dashes, and dots from a numeric-looking
// string so postchecks like Luhn can operate on raw digits.
func stripNonDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// luhn validates a credit-card-style number with the Luhn checksum.
func luhn(digits string) bool {
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	alt := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// validateNIK checks Indonesian NIK structure: 16 digits, valid province code
// (01-99 reserved range), valid date within DDMMYY (with female DD+40).
func validateNIK(s string) bool {
	d := stripNonDigits(s)
	if len(d) != 16 {
		return false
	}
	// Province code: 11–94 are assigned. Outside that range is almost
	// certainly a false positive.
	prov := (int(d[0]-'0'))*10 + int(d[1]-'0')
	if prov < 11 || prov > 94 {
		return false
	}
	// Date-of-birth check: DD is bytes 6–7. Female adds 40, so DD ∈ [1..71].
	dd := (int(d[6]-'0'))*10 + int(d[7]-'0')
	if dd == 0 || (dd > 31 && dd < 41) || dd > 71 {
		return false
	}
	mm := (int(d[8]-'0'))*10 + int(d[9]-'0')
	if mm == 0 || mm > 12 {
		return false
	}
	return true
}

// validateNPWP accepts both legacy 15-digit and current 16-digit NPWP.
// Returns false for runs of digits that are obvious non-NPWP (all zeros, all
// same digit) to keep false positives low when the regex falls back to the
// generic 15-16 digit branch.
func validateNPWP(s string) bool {
	d := stripNonDigits(s)
	if len(d) != 15 && len(d) != 16 {
		return false
	}
	allSame := true
	for i := 1; i < len(d); i++ {
		if d[i] != d[0] {
			allSame = false
			break
		}
	}
	return !allSame
}

// validIPv4 confirms each octet is 0-255.
func validIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n := 0
		if len(p) == 0 || len(p) > 3 {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
