package capability

import (
	"regexp"
	"strings"
	"sync"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
)

// CatalogSource provides lookup of dynamic model specifications from a catalog service.
type CatalogSource interface {
	FindModel(provider, modelID string) (*catalog.ModelSpec, bool)
}

var (
	catalogMu           sync.RWMutex
	globalCatalogSource CatalogSource
)

// SetCatalogSource configures the global catalog source used for dynamic profile enrichment.
func SetCatalogSource(src CatalogSource) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	globalCatalogSource = src
}

func getCatalogSource() CatalogSource {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	return globalCatalogSource
}

// Profile is the resolved capability descriptor for a single model. It captures
// the input/output modalities, feature flags, thinking metadata, and token
// limits a model exposes. Consumers derive a core.CapabilitySet from it for the
// routing guard, and may read the numeric/thinking fields directly for token
// accounting or reasoning translation.
type Profile struct {
	// Input modalities.
	Vision     bool // reads images
	PDF        bool // reads PDF / documents
	AudioInput bool // reads audio
	VideoInput bool // reads video

	// Output modalities.
	ImageOutput bool // generates images
	AudioOutput bool // generates audio

	// Features.
	Search           bool // built-in web search / grounding
	Tools            bool // function / tool calling
	Reasoning        bool // extended thinking / reasoning
	StructuredOutput bool // constrained / schema-guided output (json_schema)

	// ThinkingFormat is the reasoning wire format, only meaningful when
	// Reasoning is true. Empty means "derive from the transport dialect".
	// One of: openai, claude-adaptive, claude-budget, gemini-level,
	// gemini-budget, zai, qwen, deepseek, kimi, minimax, hunyuan, step.
	ThinkingFormat string
	// ThinkingCanDisable reports whether thinking can be turned off. When
	// false, callers clamp to the minimum budget instead of disabling.
	ThinkingCanDisable bool
	// ThinkingRange bounds the thinking budget for budget-style formats.
	// Nil means no clamp.
	ThinkingRange *ThinkingRange

	// Token limits.
	ContextWindow int
	MaxOutput     int
}

// ThinkingRange is an inclusive token budget window for budget-style thinking.
type ThinkingRange struct {
	Min int
	Max int
}

// defaultProfile is the safe floor every resolved profile is built on. Most
// modern chat models meet these limits, so unknown models resolve to a usable
// baseline rather than being treated as text-only.
func defaultProfile() Profile {
	return Profile{
		Tools:              true,
		ThinkingCanDisable: true,
		ContextWindow:      200000,
		MaxOutput:          64000,
	}
}

// caps is a partial override applied over a floor profile. Zero-valued fields
// inherit the floor. The two inverted flags express the uncommon case of
// disabling a feature the floor enables by default.
type caps struct {
	Vision      bool
	PDF         bool
	AudioInput  bool
	VideoInput  bool
	ImageOutput bool
	AudioOutput bool
	Search      bool
	Reasoning   bool
	Structured  bool

	ThinkingFormat string
	ThinkingRange  *ThinkingRange

	ContextWindow int
	MaxOutput     int

	// NoTools disables tool calling (the floor enables it).
	NoTools bool
	// ThinkingLocked marks reasoning that cannot be turned off (the floor
	// allows it).
	ThinkingLocked bool
}

// merge returns p with this override applied. Set boolean and non-zero scalar
// fields win over the floor; unset fields are left untouched.
func (c caps) merge(p Profile) Profile {
	if c.Vision {
		p.Vision = true
	}
	if c.PDF {
		p.PDF = true
	}
	if c.AudioInput {
		p.AudioInput = true
	}
	if c.VideoInput {
		p.VideoInput = true
	}
	if c.ImageOutput {
		p.ImageOutput = true
	}
	if c.AudioOutput {
		p.AudioOutput = true
	}
	if c.Search {
		p.Search = true
	}
	if c.Reasoning {
		p.Reasoning = true
	}
	if c.Structured {
		p.StructuredOutput = true
	}
	if c.ThinkingFormat != "" {
		p.ThinkingFormat = c.ThinkingFormat
	}
	if c.ThinkingRange != nil {
		p.ThinkingRange = c.ThinkingRange
	}
	if c.ContextWindow != 0 {
		p.ContextWindow = c.ContextWindow
	}
	if c.MaxOutput != 0 {
		p.MaxOutput = c.MaxOutput
	}
	if c.NoTools {
		p.Tools = false
	}
	if c.ThinkingLocked {
		p.ThinkingCanDisable = false
	}
	return p
}

// patternCaps binds a glob pattern (with "*" wildcards) to a capability
// override. Patterns are evaluated in declaration order, specific before
// generic, and the first match wins.
type patternCaps struct {
	pattern string
	caps    caps
}

// compiledPattern is a patternCaps with its glob pre-compiled to a regexp.
type compiledPattern struct {
	re   *regexp.Regexp
	caps caps
}

// compiledPatterns holds patternCapabilities compiled once at package load, so
// resolution never recompiles a regexp on the hot path.
var compiledPatterns = func() []compiledPattern {
	out := make([]compiledPattern, len(patternCapabilities))
	for i, p := range patternCapabilities {
		out[i] = compiledPattern{re: globToRegexp(p.pattern), caps: p.caps}
	}
	return out
}()

// globToRegexp compiles a "*"-glob into an anchored, case-insensitive regexp.
// Literal segments are escaped so only "*" acts as a wildcard.
func globToRegexp(pattern string) *regexp.Regexp {
	parts := strings.Split(pattern, "*")
	for i, s := range parts {
		parts[i] = regexp.QuoteMeta(s)
	}
	return regexp.MustCompile("(?i)^" + strings.Join(parts, ".*") + "$")
}

// ResolveProfile resolves the full capability profile for a model using a
// multi-step fallback chain, each step merged over defaultProfile so the result
// is always complete:
//
//  1. provider-specific override (providerCapabilities[provider][model])
//  2. canonical exact id (modelCapabilities), vendor prefix stripped
//  3. glob pattern match (patternCapabilities), first match wins
//  4. dynamic catalog enrichment (globalCatalogSource.FindModel)
//  5. floor (defaultProfile)
//
// The provider argument is optional; pass "" when the upstream provider is
// unknown.
func ResolveProfile(provider, model string) Profile {
	p := defaultProfile()
	if model == "" {
		return p
	}

	base := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		base = model[i+1:]
	}

	// 1. Provider-specific override, keyed by the full model id.
	matched := false
	if provider != "" {
		if byModel, ok := providerCapabilities[provider]; ok {
			if c, ok := byModel[model]; ok {
				p = c.merge(p)
				matched = true
			}
		}
	}

	// 2. Canonical exact id. Strip any vendor prefix
	// ("anthropic/claude-opus-4.7" -> "claude-opus-4.7") before lookup.
	if !matched {
		if c, ok := modelCapabilities[base]; ok {
			p = c.merge(p)
			matched = true
		} else if c, ok := modelCapabilities[model]; ok {
			p = c.merge(p)
			matched = true
		}
	}

	// 3. Glob pattern fallback.
	if !matched {
		for _, cp := range compiledPatterns {
			if cp.re.MatchString(base) || cp.re.MatchString(model) {
				p = cp.caps.merge(p)
				matched = true
				break
			}
		}
	}

	// 4. Dynamic Catalog Enrichment via globalCatalogSource.
	if src := getCatalogSource(); src != nil {
		if spec, ok := src.FindModel(provider, model); ok && spec != nil {
			if spec.Limits.Context > 0 {
				p.ContextWindow = spec.Limits.Context
			}
			if spec.Limits.Output > 0 {
				p.MaxOutput = spec.Limits.Output
			}
			if spec.HasVision() {
				p.Vision = true
			}
			if spec.HasPDF() {
				p.PDF = true
			}
			if spec.HasAudioInput() {
				p.AudioInput = true
			}
			if spec.HasVideoInput() {
				p.VideoInput = true
			}
			if spec.HasTools() {
				p.Tools = true
			}
			if spec.Reasoning {
				p.Reasoning = true
			}
			// CRITICAL PRESERVATION: DO NOT overwrite ThinkingFormat,
			// ThinkingCanDisable, ThinkingLocked, or ThinkingRange if already
			// set from core capability tables.
		}
	}

	// 5. Floor.
	return p
}
