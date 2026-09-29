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
// descriptions and requiredness; this only adds what inference cannot see. A
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

// applyConstraints decodes an inferred schema, adds the enum and default
// keywords for the named properties, and re-encodes it.
func applyConstraints(inferred json.RawMessage, constraints map[string]SchemaConstraint, toolName string) (json.RawMessage, error) {
	var schema map[string]any

	err := json.Unmarshal(inferred, &schema)
	if err != nil {
		return nil, fmt.Errorf("decode inferred schema: %w", err)
	}

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

	dropNullFromTypes(schema)

	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("re-encode schema: %w", err)
	}

	return raw, nil
}

// dropNullFromTypes narrows every ["null", T] type union to T, at any depth.
//
// jsonschema-go infers a slice or a pointer as ["null", T] because a nil one
// marshals to null. For an input that union says nothing useful — an omitted
// parameter and a null one decode to the same zero value — and it costs calls:
// a model reads it as "not quite an array" and sends the array JSON-encoded as a
// string, which the MCP gateway then rejects against this very schema.
func dropNullFromTypes(node any) {
	switch typed := node.(type) {
	case map[string]any:
		if types, isUnion := typed["type"].([]any); isUnion {
			typed["type"] = withoutNull(types)
		}

		for _, child := range typed {
			dropNullFromTypes(child)
		}
	case []any:
		for _, child := range typed {
			dropNullFromTypes(child)
		}
	}
}

// withoutNull drops "null" from a type union, and collapses a union left with a
// single type to that plain keyword.
func withoutNull(types []any) any {
	narrowed := make([]any, 0, len(types))

	for _, name := range types {
		if name != "null" {
			narrowed = append(narrowed, name)
		}
	}

	if len(narrowed) == 1 {
		return narrowed[0]
	}

	return narrowed
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
