package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A union of null alone has no other type to fall back on. Emptying it to
// "type": [] would make a schema that matches no value at all, so it is kept as
// it is: TestToolSchemas_InputTypesAdmitNoNull then flags it, as it should.
func TestDropNullFromTypes_KeepsANullOnlyUnion(t *testing.T) {
	t.Parallel()

	var schema map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{"type": ["null"]}`), &schema))

	dropNullFromTypes(schema)

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type": ["null"]}`, string(encoded), `input {"type": ["null"]}`)
}

// default, enum, const and examples hold values, not schemas: a default filter
// payload whose own "type" field reads ["null", "string"] is data the caller
// gets back, and must reach the wire unchanged. A property that happens to be
// named "default" is still a schema, and is narrowed.
func TestDropNullFromTypes_LeavesValueKeywordsUntouched(t *testing.T) {
	t.Parallel()

	var schema map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"default": {"type": ["null", "array"]},
			"payload": {
				"type": "object",
				"default": {"type": ["null", "string"]},
				"const": {"type": ["null", "string"]},
				"enum": [{"type": ["null", "string"]}],
				"examples": [{"type": ["null", "string"]}]
			}
		}
	}`), &schema))

	dropNullFromTypes(schema)

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type": "object",
		"properties": {
			"default": {"type": "array"},
			"payload": {
				"type": "object",
				"default": {"type": ["null", "string"]},
				"const": {"type": ["null", "string"]},
				"enum": [{"type": ["null", "string"]}],
				"examples": [{"type": ["null", "string"]}]
			}
		}
	}`, string(encoded))
}

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
