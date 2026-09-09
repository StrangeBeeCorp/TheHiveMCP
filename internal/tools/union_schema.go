package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// WithUnionOutputSchema advertises the output schema of a union result type
// whose Unwrap() flattens it via utils.UnwrapUnion.
//
// A union result serializes as its single active variant, hoisted to the top
// level — never as the wrapper struct. mcp.WithOutputSchema[T]() therefore
// advertises a shape the handler never sends: it permits only the wrapper's own
// field names and, since mcp-go v1.0.0, closes the object with
// additionalProperties:false. Every successful result then violates the tool's
// own declared contract, because each of its keys belongs to the variant rather
// than to the wrapper. Under mcp-go v0.43.1 this passed unnoticed — it set
// AllowAdditionalProperties:true, so the hoisted keys were tolerated.
//
// So describe what Unwrap() actually produces: anyOf over the variant schemas.
// anyOf rather than oneOf because oneOf demands exactly one match and would
// reject a payload satisfying two variants whose field sets overlap.
//
// Branches are lifted from the schema mcp-go itself generates for the wrapper,
// rather than reflected here, so each variant keeps mcp-go's own inference —
// including its fallback for the `jsonschema:"enum=..."` tag syntax that
// github.com/google/jsonschema-go rejects outright.
func WithUnionOutputSchema[T any]() mcp.ToolOption {
	return outputSchemaOption[T](true)
}

// WithResultOutputSchema advertises the output schema of a result type that
// serializes as itself. It exists so that even a non-union result gets the
// date-field retyping every tool's payload undergoes; see retypeDateFields.
func WithResultOutputSchema[T any]() mcp.ToolOption {
	return outputSchemaOption[T](false)
}

// errUnionSchema marks an output schema that could not be derived.
var errUnionSchema = errors.New("output schema")

// outputSchemaOption generates T's schema through mcp-go, rewrites it to match
// what the serialization pipeline actually emits, and advertises the result.
//
// TestToolSchemas_OutputMatchesAdvertisedSchema validates real handler results
// against what this advertises; that assertion, not these comments, is what
// stops schema and payload drifting apart again.
func outputSchemaOption[T any](union bool) mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithOutputSchema[T]()(tool)

		raw, err := rewriteOutputSchema[T](tool.OutputSchema, union)
		if err != nil {
			slog.Error("Failed to build output schema; the tool would advertise a shape it never sends",
				"tool", tool.Name, "error", err)

			return
		}

		// OutputSchema and RawOutputSchema are mutually exclusive: mcp.Tool's
		// MarshalJSON errors when both are set, which would strand the tool out
		// of every tools/list response.
		tool.OutputSchema = mcp.ToolOutputSchema{}
		tool.RawOutputSchema = raw
	}
}

// rewriteOutputSchema turns a generated schema into one describing the emitted
// payload: anyOf over the variants for a union, and date fields retyped in
// either case.
func rewriteOutputSchema[T any](generated mcp.ToolOutputSchema, union bool) (json.RawMessage, error) {
	// WithOutputSchema is silent on inference failure: it writes the error to
	// stderr and returns without setting a schema. An empty Type is that.
	if generated.Type == "" {
		return nil, fmt.Errorf("%w: nothing inferred for %T", errUnionSchema, *new(T))
	}

	encoded, err := json.Marshal(generated)
	if err != nil {
		return nil, fmt.Errorf("re-encode inferred schema for %T: %w", *new(T), err)
	}

	var document map[string]any

	err = json.Unmarshal(encoded, &document)
	if err != nil {
		return nil, fmt.Errorf("decode inferred schema for %T: %w", *new(T), err)
	}

	if union {
		document, err = liftVariants[T](document)
		if err != nil {
			return nil, err
		}
	}

	retypeDateFields(document)

	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal output schema for %T: %w", *new(T), err)
	}

	return raw, nil
}

// liftVariants replaces a wrapper schema with anyOf over its variant schemas.
func liftVariants[T any](document map[string]any) (map[string]any, error) {
	properties, ok := document["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: inferred schema for %T has no properties object", errUnionSchema, *new(T))
	}

	names := unionVariantFields(reflect.TypeFor[T]())
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: %T declares no pointer variants", errUnionSchema, *new(T))
	}

	variants := make([]any, 0, len(names))

	for _, name := range names {
		variant, found := properties[name]
		if !found {
			return nil, fmt.Errorf("%w: variant %q absent from the inferred schema for %T",
				errUnionSchema, name, *new(T))
		}

		variants = append(variants, variant)
	}

	// type:object alongside anyOf, not anyOf alone. structuredContent is always
	// an object, and the MCP schema for a tool types outputSchema as an object
	// schema — the TypeScript SDK pins `type` to the literal "object", so a
	// bare anyOf fails to parse and takes the whole tools/list response with
	// it, leaving a client showing no tools at all rather than one bad tool.
	// The keyword is also true: every variant is an object.
	lifted := map[string]any{"type": "object", "anyOf": variants}

	// $defs live at the document root, so a lifted branch carrying a $ref would
	// dangle without them.
	if defs, hasDefs := document["$defs"]; hasDefs {
		lifted["$defs"] = defs
	}

	return lifted, nil
}

// retypeDateFields declares every date-named property a string, at any depth.
//
// utils.ProcessDatesRecursive rewrites those fields from an epoch integer to a
// formatted string on the way out, so a schema inferred from the Go struct
// declares `integer` for a value the client only ever sees as a string. Null
// stays permitted: a nil pointer date serializes as null.
func retypeDateFields(node any) {
	switch typed := node.(type) {
	case map[string]any:
		retypeSchemaNode(typed)
	case []any:
		for _, child := range typed {
			retypeDateFields(child)
		}
	}
}

// retypeSchemaNode retypes one schema object's date properties and recurses
// through everything else it holds ($defs, items, anyOf, …).
func retypeSchemaNode(node map[string]any) {
	properties, ok := node["properties"].(map[string]any)
	if ok {
		retypeDateProperties(properties)
	}

	for key, child := range node {
		if key != "properties" {
			retypeDateFields(child)
		}
	}
}

// retypeDateProperties rewrites the date-named entries of one properties map.
func retypeDateProperties(properties map[string]any) {
	for name, property := range properties {
		schema, isSchema := property.(map[string]any)
		if isSchema && utils.IsDateField(name) {
			schema["type"] = []string{"null", "string"}
			delete(schema, "format")

			continue
		}

		retypeDateFields(property)
	}
}

// unionVariantFields returns the JSON names of t's exported pointer fields, in
// declaration order — the fields utils.UnwrapUnion picks the active variant
// from. Deriving the schema and the payload from the same rule is what keeps
// them in agreement.
func unionVariantFields(t reflect.Type) []string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	names := make([]string, 0, t.NumField())

	for field := range t.Fields() {
		if !field.IsExported() || field.Type.Kind() != reflect.Pointer {
			continue
		}

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" {
			name = field.Name
		}

		names = append(names, name)
	}

	return names
}
