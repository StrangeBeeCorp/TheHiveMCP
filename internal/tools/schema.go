package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
)

// SchemaConstraint carries the parts of a JSON Schema property that Go struct
// tags can no longer express.
//
// mcp-go v1.0.0 infers schemas with github.com/google/jsonschema-go, where the
// `jsonschema` tag is a plain description string rather than a `key=value`
// DSL — a tag shaped like `enum=a,enum=b` is rejected outright. Descriptions
// still come from `jsonschema_description`, so only enumerations and defaults
// need somewhere else to live.
type SchemaConstraint struct {
	// Enum is the closed set of permitted values.
	Enum []string
	// Default is the value the server applies when the caller omits the field.
	Default any
}

// WithInputSchemaConstraints infers the input schema from T and then applies
// constraints to named properties.
//
// The struct stays the single source of truth for property names, types,
// descriptions and requiredness; this only adds what inference cannot see, and
// drops the null inference puts in a slice or pointer's type (see
// dropNullFromTypes). A
// constraint naming a property that does not exist is a bug — it is logged, and
// TestToolSchemas_ConstraintsMatchTheStruct fails on it.
//
// It exists because mcp.WithInputSchema is silent on failure: it writes the
// inference error to stderr and returns without setting a schema, so a broken
// tag yields a tool advertising no parameters at all and nothing else reports
// it. See TestToolSchemas_AdvertiseTheirParameters.
func WithInputSchemaConstraints[T any](constraints map[string]SchemaConstraint) mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithInputSchema[T]()(tool)

		if len(tool.RawInputSchema) == 0 {
			slog.Error("Input schema inference produced nothing; the tool would advertise no parameters",
				"tool", tool.Name)

			return
		}

		raw, err := applyConstraints(tool.RawInputSchema, constraints, tool.Name)
		if err != nil {
			slog.Error("Failed to apply schema constraints", "tool", tool.Name, "error", err)

			return
		}

		tool.RawInputSchema = raw
	}
}

// applyConstraints decodes an inferred schema, drops null from its type unions,
// adds the enum and default keywords for the named properties, and re-encodes
// it, in that one decode. The enum and default it adds hold values, which
// dropNullFromTypes never descends into.
func applyConstraints(inferred json.RawMessage, constraints map[string]SchemaConstraint, toolName string) (json.RawMessage, error) {
	var schema map[string]any

	err := json.Unmarshal(inferred, &schema)
	if err != nil {
		return nil, fmt.Errorf("decode inferred schema: %w", err)
	}

	dropNullFromTypes(schema)

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("inferred schema has no properties object")
	}

	for name, constraint := range constraints {
		property, found := properties[name].(map[string]any)
		if !found {
			slog.Error("Constraint names a property the struct does not declare",
				"tool", toolName, "property", name)

			continue
		}

		constraint.applyTo(property)
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("re-encode schema: %w", err)
	}

	return raw, nil
}

// WithInputSchema infers the input schema from T, as mcp.WithInputSchema does,
// with every null dropped from its type unions. Tools use it in place of a bare
// mcp.WithInputSchema, whose ["null", T] unions TestToolSchemas_InputTypesAdmitNoNull
// rejects.
func WithInputSchema[T any]() mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithInputSchema[T]()(tool)

		if len(tool.RawInputSchema) == 0 {
			slog.Error("Input schema inference produced nothing; the tool would advertise no parameters",
				"tool", tool.Name)

			return
		}

		*tool = withNullFreeInputSchema(*tool)
	}
}

// WithNullFreeInputSchema drops null from the type unions of the input schema
// the options before it built, raw or structured. It is for a tool whose schema
// comes neither from WithInputSchema nor from WithInputSchemaConstraints, which
// both drop it already, and goes after the option that sets the schema.
func WithNullFreeInputSchema() mcp.ToolOption {
	return func(tool *mcp.Tool) {
		*tool = withNullFreeInputSchema(*tool)
	}
}

// withNullFreeInputSchema returns the tool with every type union in its input
// schema narrowed by dropNullFromTypes, whether that schema is raw or the
// structured InputSchema (mcp.WithString, WithArray, …).
//
// It runs while the tool is built, never at registration, so Definition()
// returns exactly the schema a client receives. It works on a fresh decoded
// copy, so the maps a structured schema may share beyond the tool are never
// touched. A schema with no null to drop is returned as it is, byte for byte.
func withNullFreeInputSchema(tool mcp.Tool) mcp.Tool {
	if len(tool.RawInputSchema) > 0 {
		narrowed, changed, err := narrowSchema(tool.RawInputSchema)
		if err != nil {
			slog.Error("Failed to narrow input schema; null left in its type unions", "tool", tool.Name, "error", err)

			return tool
		}

		if changed {
			tool.RawInputSchema = narrowed
		}

		return tool
	}

	encoded, err := json.Marshal(tool.InputSchema)
	if err != nil {
		slog.Error("Failed to encode input schema; null left in its type unions", "tool", tool.Name, "error", err)

		return tool
	}

	narrowed, changed, err := narrowSchema(encoded)
	if err != nil {
		slog.Error("Failed to narrow input schema; null left in its type unions", "tool", tool.Name, "error", err)

		return tool
	}

	if !changed {
		return tool
	}

	var inputSchema mcp.ToolInputSchema

	err = json.Unmarshal(narrowed, &inputSchema)
	if err != nil {
		slog.Error("Failed to decode narrowed input schema; null left in its type unions", "tool", tool.Name, "error", err)

		return tool
	}

	tool.InputSchema = inputSchema

	return tool
}

// narrowSchema decodes an encoded schema, drops null from its type unions, and
// re-encodes it. It reports whether it dropped any; when it did not, the
// returned schema is not meaningful.
func narrowSchema(encoded []byte) (json.RawMessage, bool, error) {
	var schema any

	err := json.Unmarshal(encoded, &schema)
	if err != nil {
		return nil, false, fmt.Errorf("decode input schema: %w", err)
	}

	if !dropNullFromTypes(schema) {
		return nil, false, nil
	}

	narrowed, err := json.Marshal(schema)
	if err != nil {
		return nil, false, fmt.Errorf("re-encode input schema: %w", err)
	}

	return narrowed, true, nil
}

// dropNullFromTypes narrows every ["null", T] type union to T, at any depth.
//
// jsonschema-go infers a slice or a pointer as ["null", T] because a nil one
// marshals to null. For an input that union says nothing useful — an omitted
// parameter and a null one decode to the same zero value — and it costs calls:
// a model reads it as "not quite an array" and sends the array JSON-encoded as a
// string, which the MCP gateway then rejects against this very schema.
//
// It descends only through the keywords that hold subschemas. default, enum,
// const and examples hold values, which may well carry a "type" field of their
// own, and are left as they are.
func dropNullFromTypes(node any) bool {
	schema, isSchema := node.(map[string]any)
	if !isSchema {
		return false
	}

	changed := false

	if types, isUnion := schema["type"].([]any); isUnion {
		narrowed, dropped := withoutNull(types)
		schema["type"] = narrowed
		changed = dropped
	}

	for _, keyword := range namedSubschemaKeywords {
		if named, ok := schema[keyword].(map[string]any); ok {
			for _, subschema := range named {
				changed = dropNullFromSubschemas(subschema) || changed
			}
		}
	}

	for _, keyword := range subschemaKeywords {
		changed = dropNullFromSubschemas(schema[keyword]) || changed
	}

	return changed
}

// dropNullFromSubschemas narrows a keyword's value, which is a single schema or,
// for allOf, anyOf, oneOf, prefixItems and a draft-4 items, a list of them, and
// reports whether it dropped any null.
func dropNullFromSubschemas(value any) bool {
	list, isList := value.([]any)
	if !isList {
		return dropNullFromTypes(value)
	}

	changed := false

	for _, subschema := range list {
		changed = dropNullFromTypes(subschema) || changed
	}

	return changed
}

// namedSubschemaKeywords map a name to a subschema. dependencies may also map a
// name to a list of property names, which dropNullFromSubschemas skips.
var namedSubschemaKeywords = []string{
	"properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependencies",
}

// subschemaKeywords hold a subschema, or a list of them.
//
// not and if are left out on purpose. Dropping null only narrows what a
// subschema accepts, which narrows the whole schema everywhere else, but not
// there: under not it widens it (null, refused, becomes accepted), and under if
// it moves null from then to else.
var subschemaKeywords = []string{
	"items", "prefixItems", "additionalItems", "additionalProperties", "unevaluatedItems",
	"unevaluatedProperties", "propertyNames", "contains", "then", "else",
	"allOf", "anyOf", "oneOf", "contentSchema",
}

// withoutNull drops "null" from a type union, and collapses a union left with a
// single type to that plain keyword. A union of null alone is returned as it
// is: emptied, it would read "type": [], which no value satisfies. So is a union
// holding no null, which has nothing to drop. It reports whether it dropped one.
func withoutNull(types []any) (any, bool) {
	narrowed := make([]any, 0, len(types))

	for _, name := range types {
		if name != "null" {
			narrowed = append(narrowed, name)
		}
	}

	switch {
	case len(narrowed) == 0, len(narrowed) == len(types):
		return types, false
	case len(narrowed) == 1:
		return narrowed[0], true
	default:
		return narrowed, true
	}
}

// applyTo writes the constraint's keywords onto one decoded schema property.
func (c SchemaConstraint) applyTo(property map[string]any) {
	if len(c.Enum) > 0 {
		property["enum"] = c.Enum
	}

	if c.Default != nil {
		property["default"] = c.Default
	}
}
