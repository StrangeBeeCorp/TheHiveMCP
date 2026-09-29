package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tools' structs only produce nullable properties today, so the advertised
// schemas cannot show a union nested in an array keyword. Pin it on a synthetic
// schema, so a nested struct or a union param added later is narrowed too.
func TestDropNullFromTypes_NarrowsUnionsInsideArrayKeywords(t *testing.T) {
	t.Parallel()

	var schema map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"ids": {"type": ["null", "array"], "items": {"type": "string"}},
			"either": {"anyOf": [{"type": ["null", "integer"]}, {"type": ["string", "boolean"]}]}
		}
	}`), &schema))

	dropNullFromTypes(schema)

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type": "object",
		"properties": {
			"ids": {"type": "array", "items": {"type": "string"}},
			"either": {"anyOf": [{"type": "integer"}, {"type": ["string", "boolean"]}]}
		}
	}`, string(encoded))
}
