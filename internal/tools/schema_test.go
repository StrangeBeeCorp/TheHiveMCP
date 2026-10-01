package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputProperty returns, re-encoded, one property of the raw input schema tool
// advertises.
func inputProperty(t *testing.T, tool mcp.Tool, name string) string {
	t.Helper()

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(tool.RawInputSchema, &schema), "tool %q has an undecodable input schema", tool.Name)
	require.Contains(t, schema.Properties, name, "tool %q advertises no property %q", tool.Name, name)

	return string(schema.Properties[name])
}

// A []string field is inferred as ["null", "array"]; WithInputSchema, which
// get-resource uses in place of a bare mcp.WithInputSchema, drops the null.
func TestWithInputSchema_DropsNullFromAnInferredSchema(t *testing.T) {
	t.Parallel()

	type params struct {
		IDs []string `json:"ids,omitempty"`
	}

	tool := mcp.NewTool("inferred", WithInputSchema[params]())

	assert.JSONEq(t, `{"type": "array", "items": {"type": "string"}}`, inputProperty(t, tool, "ids"),
		"[]string field `ids`, inferred as [\"null\", \"array\"]")
}

// Nested collections put the inferred union below items and
// additionalProperties, where a narrowing that stopped at the property would
// leave it.
func TestWithInputSchema_DropsNullBelowItemsAndAdditionalProperties(t *testing.T) {
	t.Parallel()

	type params struct {
		Matrix [][]string          `json:"matrix,omitempty"`
		Index  map[string][]string `json:"index,omitempty"`
	}

	tool := mcp.NewTool("nested", WithInputSchema[params]())

	assert.JSONEq(t, `{"type": "array", "items": {"type": "array", "items": {"type": "string"}}}`,
		inputProperty(t, tool, "matrix"), "[][]string field `matrix`")
	assert.JSONEq(t, `{"type": "object", "additionalProperties": {"type": "array", "items": {"type": "string"}}}`,
		inputProperty(t, tool, "index"), "map[string][]string field `index`")
}

// WithInputSchemaConstraints drops null in the same pass that adds the
// constraints, and before it adds them: a default is a value, so a default
// payload with a "type" field of its own reaches the wire unchanged.
func TestWithInputSchemaConstraints_DropsNullButLeavesConstraintValues(t *testing.T) {
	t.Parallel()

	type params struct {
		IDs    []string       `json:"ids,omitempty"`
		Filter map[string]any `json:"filter,omitempty"`
	}

	var payload any

	require.NoError(t, json.Unmarshal([]byte(`{"type": ["null", "string"]}`), &payload))

	tool := mcp.NewTool("constrained", WithInputSchemaConstraints[params](map[string]SchemaConstraint{
		"filter": {Default: payload},
	}))

	assert.JSONEq(t, `{"type": "array", "items": {"type": "string"}}`, inputProperty(t, tool, "ids"),
		"[]string field `ids` beside a constrained field")

	var filter struct {
		Default json.RawMessage `json:"default"`
	}

	require.NoError(t, json.Unmarshal([]byte(inputProperty(t, tool, "filter")), &filter), "property `filter` decodes")
	assert.JSONEq(t, `{"type": ["null", "string"]}`, string(filter.Default), "default payload of `filter`")
}

// A schema with no null union to drop reaches the wire byte for byte: decoding
// and re-encoding it would sort its keys, losing the property order
// jsonschema-go keeps from the struct, for no change in meaning. A union
// holding no null has nothing to drop either.
func TestWithNullFreeInputSchema_LeavesASchemaWithoutNullUntouched(t *testing.T) {
	t.Parallel()

	raw := `{"type": "object", "properties": {"zeta": {"type": "string"}, "alpha": {"type": ["string", "boolean"]}}}`

	tool := mcp.NewTool("no-null", mcp.WithRawInputSchema([]byte(raw)), WithNullFreeInputSchema())

	assert.Equal(t, raw, string(tool.RawInputSchema),
		`schema with properties zeta then alpha, and a ["string", "boolean"] union holding no null`)
}

// A schema without a top-level properties object, which applyConstraints
// refuses, is narrowed all the same.
func TestWithNullFreeInputSchema_DropsNullFromASchemaWithoutProperties(t *testing.T) {
	t.Parallel()

	raw := `{"type": "object", "$defs": {"ids": {"type": ["null", "array"]}}}`

	tool := mcp.NewTool("no-properties", mcp.WithRawInputSchema([]byte(raw)), WithNullFreeInputSchema())

	assert.JSONEq(t, `{"type": "object", "$defs": {"ids": {"type": "array"}}}`, string(tool.RawInputSchema),
		`schema with no properties, ["null", "array"] only under $defs.ids`)
}

// A tool built from the structured InputSchema, as the mcp.WithString /
// WithArray options build it, rather than from a raw one, is narrowed all the
// same. Its maps may be shared beyond the tool, and must keep their contents.
func TestWithNullFreeInputSchema_DropsNullFromAStructuredSchema(t *testing.T) {
	t.Parallel()

	const original = `{
		"type": "object",
		"properties": {
			"ids": {"type": ["null", "array"], "items": {"type": "string"}},
			"filter": {"$ref": "#/$defs/filter"}
		},
		"$defs": {"filter": {"type": ["null", "object"]}},
		"required": []
	}`

	structured := mcp.Tool{Name: "structured"}
	require.NoError(t, json.Unmarshal([]byte(original), &structured.InputSchema), "structured schema fixture")

	shared := structured.InputSchema

	WithNullFreeInputSchema()(&structured)

	var wire struct {
		InputSchema json.RawMessage `json:"inputSchema"`
	}

	encoded, err := json.Marshal(structured)
	require.NoError(t, err, "tool %q does not encode", structured.Name)
	require.NoError(t, json.Unmarshal(encoded, &wire), "tool %q encodes to undecodable JSON", structured.Name)

	assert.JSONEq(t, `{
		"type": "object",
		"properties": {
			"ids": {"type": "array", "items": {"type": "string"}},
			"filter": {"$ref": "#/$defs/filter"}
		},
		"$defs": {"filter": {"type": "object"}},
		"required": []
	}`, string(wire.InputSchema), `structured schema with ["null", T] in properties.ids and $defs.filter`)

	before, err := json.Marshal(shared)
	require.NoError(t, err, "tool %q's original structured schema does not encode", structured.Name)
	assert.JSONEq(t, original, string(before), "the maps of the original structured schema after narrowing")
}

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
	}`, string(encoded), `["null", T] held in default, const, enum and examples, beside a property named "default"`)
}

// Dropping null narrows what a subschema accepts. Under not, that widens what
// the schema accepts: not {"type": ["null", "string"]} refuses null, and
// narrowed to not {"type": "string"} it would let null through. An if condition
// that stops matching null would switch null from then to else. Both are left
// as they are; then, which only ever applies, is narrowed like any subschema.
func TestDropNullFromTypes_LeavesNotAndIfConditionsUntouched(t *testing.T) {
	t.Parallel()

	var schema map[string]any

	require.NoError(t, json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"refused": {"not": {"type": ["null", "string"]}},
			"conditional": {
				"if": {"type": ["null", "string"]},
				"then": {"type": ["null", "string"], "maxLength": 3}
			}
		}
	}`), &schema))

	dropNullFromTypes(schema)

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type": "object",
		"properties": {
			"refused": {"not": {"type": ["null", "string"]}},
			"conditional": {
				"if": {"type": ["null", "string"]},
				"then": {"type": "string", "maxLength": 3}
			}
		}
	}`, string(encoded), `["null", "string"] under not, under an if condition, and under then`)
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
	}`, string(encoded), `["null", "array"] beside items, and ["null", "integer"] inside anyOf`)
}
