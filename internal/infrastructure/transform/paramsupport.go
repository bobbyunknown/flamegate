package transform

import "strings"

// Param support rules strip request parameters a given model rejects upstream
// (HTTP 400). They are config-driven so a new quirk is one entry here rather
// than scattered deletes across codecs and connectors.

// modelRejectsTemperature reports whether a model rejects the temperature
// parameter. Anthropic's claude-opus-4 series deprecated it and returns a 400
// when it is present, regardless of which connector fronts the model.
func modelRejectsTemperature(model string) bool {
	return strings.Contains(strings.ToLower(model), "claude-opus-4")
}
