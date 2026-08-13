package transform

import (
	"strings"

	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"
)

// Gemini's generateContent API accepts only a restricted subset of JSON Schema
// for functionDeclarations[].parameters. Tool schemas authored for OpenAI /
// Anthropic (especially MCP-sourced ones) routinely carry keywords Gemini
// rejects with HTTP 400 ("Invalid JSON payload" / "Unknown name"). The cleaner
// below normalizes a schema into the supported subset, and sanitizeGeminiName
// brings function names into Gemini's allowed character set.

// geminiUnsupportedSchemaKeys are JSON Schema keywords Gemini does not accept
// on a functionDeclaration parameter schema. They are stripped recursively at
// every level (object properties, array items, nested schemas).
var geminiUnsupportedSchemaKeys = map[string]struct{}{
	// Size/range constraints.
	"minLength": {}, "maxLength": {}, "exclusiveMinimum": {}, "exclusiveMaximum": {},
	"minItems": {}, "maxItems": {}, "minProperties": {}, "maxProperties": {},
	"minimum": {}, "maximum": {}, "multipleOf": {}, "pattern": {}, "format": {},
	// Values the API rejects in validated mode.
	"default": {}, "examples": {}, "example": {},
	// Meta keywords.
	"$schema": {}, "$defs": {}, "definitions": {}, "const": {}, "$ref": {}, "$comment": {}, "$id": {},
	// Annotations.
	"deprecated": {}, "readOnly": {}, "writeOnly": {}, "enumDescriptions": {},
	// Object validation keywords.
	"additionalProperties": {}, "propertyNames": {}, "patternProperties": {},
	"unevaluatedProperties": {}, "unevaluatedItems": {},
	// Composition keywords handled separately (flattened/merged before strip).
	"anyOf": {}, "oneOf": {}, "allOf": {}, "not": {},
	// Dependency keywords.
	"dependencies": {}, "dependentSchemas": {}, "dependentRequired": {},
	// Other unsupported keywords.
	"title": {}, "optional": {}, "if": {}, "then": {}, "else": {},
	"contentMediaType": {}, "contentEncoding": {}, "contentSchema": {},
	// UI/styling properties leaking in from editor tools (not JSON Schema).
	"cornerRadius": {}, "fillColor": {}, "fontFamily": {}, "fontSize": {}, "fontWeight": {},
	"gap": {}, "padding": {}, "strokeColor": {}, "strokeThickness": {}, "textColor": {},
}

// cleanGeminiToolSchema normalizes a raw JSON Schema into the subset Gemini's
// functionDeclaration parameters accept. It returns a JSON object ready to send
// upstream; on any decode failure it falls back to an empty object schema so a
// tool is never sent with a malformed schema.
func cleanGeminiToolSchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		out, _ := json.Marshal(emptyObjectSchema())
		return out
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		out, _ := json.Marshal(emptyObjectSchema())
		return out
	}

	// Phase 1: convert and prepare.
	convertConstToEnum(schema)
	convertEnumValuesToStrings(schema)

	// Phase 2: flatten composition keywords into a single schema.
	mergeAllOf(schema)
	schema = flattenAnyOfOneOf(schema)
	flattenTypeArrays(schema)

	// Phase 3: infer missing object type, then strip unsupported keywords.
	ensureObjectType(schema)
	schema = removeUnsupportedSchemaKeys(schema)

	// Phase 4: drop required entries that no longer reference a property, then
	// add a placeholder property for empty object schemas (Gemini rejects an
	// object schema with no properties).
	cleanupRequired(schema)
	addSchemaPlaceholders(schema)

	out, err := json.Marshal(schema)
	if err != nil {
		fallback, _ := json.Marshal(emptyObjectSchema())
		return fallback
	}
	return out
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "Brief explanation of why you are calling this tool",
			},
		},
		"required": []any{"reason"},
	}
}

// convertConstToEnum rewrites {const: X} as {enum: [X]} since Gemini supports
// enum but not const.
func convertConstToEnum(node any) {
	switch v := node.(type) {
	case map[string]any:
		if c, ok := v["const"]; ok {
			if _, hasEnum := v["enum"]; !hasEnum {
				v["enum"] = []any{c}
			}
			delete(v, "const")
		}
		for _, child := range v {
			convertConstToEnum(child)
		}
	case []any:
		for _, child := range v {
			convertConstToEnum(child)
		}
	}
}

// convertEnumValuesToStrings coerces every enum value to a string and forces
// type:"string" when an enum is present (Gemini requires both).
func convertEnumValuesToStrings(node any) {
	switch v := node.(type) {
	case map[string]any:
		if enum, ok := v["enum"].([]any); ok {
			for i, e := range enum {
				enum[i] = stringifyEnumValue(e)
			}
			v["enum"] = enum
			if _, hasType := v["type"]; !hasType {
				v["type"] = "string"
			}
		}
		for _, child := range v {
			convertEnumValuesToStrings(child)
		}
	case []any:
		for _, child := range v {
			convertEnumValuesToStrings(child)
		}
	}
}

func stringifyEnumValue(v any) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.Trim(string(b), `"`)
	}
}

// mergeAllOf collapses an allOf list into the parent schema by combining
// properties and required entries.
func mergeAllOf(node any) {
	switch v := node.(type) {
	case map[string]any:
		if list, ok := v["allOf"].([]any); ok {
			mergedProps := map[string]any{}
			var mergedRequired []any
			for _, item := range list {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if props, ok := m["properties"].(map[string]any); ok {
					for k, val := range props {
						mergedProps[k] = val
					}
				}
				if req, ok := m["required"].([]any); ok {
					mergedRequired = append(mergedRequired, req...)
				}
			}
			delete(v, "allOf")
			if len(mergedProps) > 0 {
				existing, _ := v["properties"].(map[string]any)
				if existing == nil {
					existing = map[string]any{}
				}
				for k, val := range mergedProps {
					existing[k] = val
				}
				v["properties"] = existing
			}
			if len(mergedRequired) > 0 {
				existing, _ := v["required"].([]any)
				v["required"] = append(existing, mergedRequired...)
			}
		}
		for _, child := range v {
			mergeAllOf(child)
		}
	case []any:
		for _, child := range v {
			mergeAllOf(child)
		}
	}
}

// flattenAnyOfOneOf replaces an anyOf/oneOf node with its best non-null branch
// (the most structurally specific schema). Returns the possibly-replaced node.
func flattenAnyOfOneOf(node any) any {
	switch v := node.(type) {
	case map[string]any:
		for _, key := range []string{"anyOf", "oneOf"} {
			if list, ok := v[key].([]any); ok && len(list) > 0 {
				if best, ok := selectBestSchema(list); ok {
					delete(v, key)
					if bestMap, ok := best.(map[string]any); ok {
						for k, val := range bestMap {
							if _, exists := v[k]; !exists {
								v[k] = val
							}
						}
					}
				} else {
					delete(v, key)
				}
			}
		}
		for k, child := range v {
			v[k] = flattenAnyOfOneOf(child)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = flattenAnyOfOneOf(child)
		}
		return v
	default:
		return node
	}
}

// selectBestSchema picks the most specific non-null branch from a list of
// schemas (objects > arrays > scalars).
func selectBestSchema(list []any) (any, bool) {
	bestScore := -1
	var best any
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := m["type"].(string); ok && t == "null" {
			continue
		}
		score := 1
		if _, ok := m["properties"]; ok {
			score = 3
		} else if t, _ := m["type"].(string); t == "object" {
			score = 3
		} else if _, ok := m["items"]; ok {
			score = 2
		} else if t, _ := m["type"].(string); t == "array" {
			score = 2
		}
		if score > bestScore {
			bestScore = score
			best = item
		}
	}
	return best, best != nil
}

// flattenTypeArrays collapses a type array like ["string","null"] into a single
// non-null type string.
func flattenTypeArrays(node any) {
	switch v := node.(type) {
	case map[string]any:
		if types, ok := v["type"].([]any); ok {
			chosen := "string"
			for _, t := range types {
				if s, ok := t.(string); ok && s != "null" {
					chosen = s
					break
				}
			}
			v["type"] = chosen
		}
		for _, child := range v {
			flattenTypeArrays(child)
		}
	case []any:
		for _, child := range v {
			flattenTypeArrays(child)
		}
	}
}

// ensureObjectType sets type:"object" on any node that declares properties but
// omits a type (Gemini requires the explicit type).
func ensureObjectType(node any) {
	switch v := node.(type) {
	case map[string]any:
		if _, hasProps := v["properties"]; hasProps {
			if _, hasType := v["type"]; !hasType {
				v["type"] = "object"
			}
		}
		for _, child := range v {
			ensureObjectType(child)
		}
	case []any:
		for _, child := range v {
			ensureObjectType(child)
		}
	}
}

// removeUnsupportedSchemaKeys recursively deletes unsupported keywords and any
// vendor-extension keys (x-* prefix). Returns the cleaned node.
func removeUnsupportedSchemaKeys(node any) any {
	switch v := node.(type) {
	case map[string]any:
		for key := range v {
			if _, bad := geminiUnsupportedSchemaKeys[key]; bad || strings.HasPrefix(key, "x-") {
				delete(v, key)
				continue
			}
		}
		for k, child := range v {
			v[k] = removeUnsupportedSchemaKeys(child)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = removeUnsupportedSchemaKeys(child)
		}
		return v
	default:
		return node
	}
}

// cleanupRequired drops required entries that no longer reference an existing
// property, and removes an empty required array entirely.
func cleanupRequired(node any) {
	switch v := node.(type) {
	case map[string]any:
		if req, ok := v["required"].([]any); ok {
			props, _ := v["properties"].(map[string]any)
			var valid []any
			for _, r := range req {
				name, ok := r.(string)
				if !ok {
					continue
				}
				if props != nil {
					if _, exists := props[name]; exists {
						valid = append(valid, name)
					}
				}
			}
			if len(valid) == 0 {
				delete(v, "required")
			} else {
				v["required"] = valid
			}
		}
		for _, child := range v {
			cleanupRequired(child)
		}
	case []any:
		for _, child := range v {
			cleanupRequired(child)
		}
	}
}

// addSchemaPlaceholders gives every empty object schema a single placeholder
// property, since Gemini rejects an object schema with no properties.
func addSchemaPlaceholders(node any) {
	switch v := node.(type) {
	case map[string]any:
		if t, _ := v["type"].(string); t == "object" {
			props, _ := v["properties"].(map[string]any)
			if len(props) == 0 {
				v["properties"] = map[string]any{
					"reason": map[string]any{
						"type":        "string",
						"description": "Brief explanation of why you are calling this tool",
					},
				}
				v["required"] = []any{"reason"}
			}
		}
		for _, child := range v {
			addSchemaPlaceholders(child)
		}
	case []any:
		for _, child := range v {
			addSchemaPlaceholders(child)
		}
	}
}

// sanitizeGeminiName brings a function name into Gemini's allowed set: it must
// start with a letter or underscore, contain only [a-zA-Z0-9_.:-], and be at
// most 64 characters. Invalid characters are replaced with '_'.
func sanitizeGeminiName(name string) string {
	if name == "" {
		return "_unknown"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '.' || r == ':' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	sanitized := b.String()
	if sanitized == "" {
		sanitized = "_unknown"
	}
	if c := sanitized[0]; (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' {
		sanitized = "_" + sanitized
	}
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}
	return sanitized
}
