package wasm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionSchema_DefaultAccountKey(t *testing.T) {
	schemaJSON := `{
		"slug": "kiro",
		"name": "Kiro",
		"version": "1.0.0",
		"default_account_key": "kiro-prod",
		"entrypoints": {"chat": "invoke"}
	}`

	var schema ExtensionSchema
	err := json.Unmarshal([]byte(schemaJSON), &schema)
	require.NoError(t, err)
	assert.Equal(t, "kiro", schema.Slug)
	assert.Equal(t, "kiro-prod", schema.DefaultAccountKey)
}

func TestExtensionSchema_DefaultAccountKey_Empty(t *testing.T) {
	schemaJSON := `{
		"slug": "mimo",
		"name": "Mimo",
		"version": "1.0.0",
		"entrypoints": {"chat": "invoke"}
	}`

	var schema ExtensionSchema
	err := json.Unmarshal([]byte(schemaJSON), &schema)
	require.NoError(t, err)
	assert.Empty(t, schema.DefaultAccountKey)
}
