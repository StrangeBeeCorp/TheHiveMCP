package bootstrap

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/resource"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// A tool that advertises an output schema its results violate is worse than a
// tool advertising none: the MCP spec makes structuredContent conforming to the
// declared outputSchema mandatory, so a client is entitled to reject every
// successful call — while the handler has already run, and any write it made is
// already durable. The caller cannot then tell "rejected" from "applied but
// unrepresentable", and a retry duplicates the write.
//
// This is not hypothetical. Three of the four tools return union types that
// Unwrap() flattens to the active variant, while advertising the *wrapper*
// struct via mcp.WithOutputSchema. mcp-go v0.43.1 generated those schemas with
// AllowAdditionalProperties:true, so the hoisted variant keys were tolerated.
// mcp-go v1.0.0 infers with github.com/google/jsonschema-go, which emits
// additionalProperties:false — turning a tolerated mismatch into a violation on
// every success path, for get-resource, manage-entities and execute-automation
// alike. Every unit and integration test still passed, because tests assert on
// the payload the code builds and never validate it against the schema the tool
// advertises.
//
// So validate one against the other. Reflection enumerates the variants, so a
// newly added one is covered without touching this test.
func TestToolSchemas_OutputMatchesAdvertisedSchema(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		definition mcp.Tool
		result     any
		// untrusted mirrors the tool's HasUntrustedData, since boundary-tag
		// wrapping runs over the payload before it reaches the client.
		untrusted bool
	}{
		toolGetResource: {
			definition: resource.NewResourceTool(nil).Definition(),
			result:     resource.GetResourceResult{},
			untrusted:  true,
		},
		toolManageEntities: {
			definition: manage.NewManageTool().Definition(),
			result:     manage.EntityResult{},
			untrusted:  true,
		},
		toolExecuteAutomation: {
			definition: execute_automation.NewExecuteAutomationTool().Definition(),
			result:     execute_automation.ExecuteAutomationResult{},
			untrusted:  true,
		},
		toolSearchEntities: {
			definition: search.NewSearchTool().Definition(),
			result:     search.EntitiesResult{},
			untrusted:  true,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			compiled := compileAdvertisedOutputSchema(t, testCase.definition)

			for _, variant := range resultVariants(t, testCase.result) {
				t.Run(variant.name, func(t *testing.T) {
					processed, err := utils.ProcessDatesRecursive(variant.value, testCase.untrusted)
					require.NoError(t, err, "the server pipeline must be able to process this result")

					require.NoError(t, compiled.Validate(jsonRoundTrip(t, processed)),
						"%s returns a %s result that violates its own advertised output schema",
						name, variant.name)
				})
			}
		})
	}
}

// compileAdvertisedOutputSchema compiles exactly what the tool puts on the
// wire, whether it advertises a raw schema or a generated one.
func compileAdvertisedOutputSchema(t *testing.T, definition mcp.Tool) *jsonschema.Schema {
	t.Helper()

	raw := definition.RawOutputSchema
	if len(raw) == 0 {
		require.NotEmpty(t, definition.OutputSchema.Type,
			"tool advertises no output schema at all; inference failed silently")

		marshalled, err := json.Marshal(definition.OutputSchema)
		require.NoError(t, err)

		raw = marshalled
	}

	var document any

	require.NoError(t, json.Unmarshal(raw, &document))

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("output.json", document))

	compiled, err := compiler.Compile("output.json")
	require.NoError(t, err, "the advertised output schema must be a compilable JSON Schema")

	return compiled
}

// namedVariant is one populated result shape to validate.
type namedVariant struct {
	name  string
	value any
}

// resultVariants returns one populated result per union variant, or the single
// populated result itself when the type is not a union. Every variant is filled
// with non-zero values so `required` keywords are exercised rather than skipped
// by omitempty.
func resultVariants(t *testing.T, result any) []namedVariant {
	t.Helper()

	resultType := reflect.TypeOf(result)
	require.Equal(t, reflect.Struct, resultType.Kind())

	variants := make([]namedVariant, 0, resultType.NumField())

	for i := range resultType.NumField() {
		field := resultType.Field(i)
		if !field.IsExported() || field.Type.Kind() != reflect.Pointer {
			continue
		}

		populated := reflect.New(resultType).Elem()
		populated.Field(i).Set(fill(field.Type, 0))
		variants = append(variants, namedVariant{name: field.Name, value: populated.Interface()})
	}

	if len(variants) == 0 {
		return []namedVariant{{name: "whole", value: fill(resultType, 0).Interface()}}
	}

	return variants
}

// maxFillDepth bounds fill against self-referential types.
const maxFillDepth = 6

// fill builds a non-zero value of t, recursing into structs, pointers, slices
// and maps.
func fill(t reflect.Type, depth int) reflect.Value {
	if depth > maxFillDepth {
		return reflect.Zero(t)
	}

	if t == reflect.TypeFor[time.Time]() {
		return reflect.ValueOf(time.Now().UTC())
	}

	switch t.Kind() {
	case reflect.Pointer:
		pointer := reflect.New(t.Elem())
		pointer.Elem().Set(fill(t.Elem(), depth+1))

		return pointer
	case reflect.Struct:
		value := reflect.New(t).Elem()

		for i := range t.NumField() {
			if t.Field(i).IsExported() {
				value.Field(i).Set(fill(t.Field(i).Type, depth+1))
			}
		}

		return value
	case reflect.Slice:
		slice := reflect.MakeSlice(t, 1, 1)
		slice.Index(0).Set(fill(t.Elem(), depth+1))

		return slice
	case reflect.Map:
		m := reflect.MakeMap(t)
		m.SetMapIndex(fill(t.Key(), depth+1), fill(t.Elem(), depth+1))

		return m
	case reflect.String:
		return reflect.ValueOf("x").Convert(t)
	case reflect.Bool:
		return reflect.ValueOf(true).Convert(t)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(1)).Convert(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(uint64(1)).Convert(t)
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(1.5).Convert(t)
	case reflect.Interface:
		return reflect.ValueOf(map[string]any{"x": "y"})
	default:
		return reflect.Zero(t)
	}
}

// jsonRoundTrip renders a processed result the way the transport does, so
// validation sees the JSON the client sees.
func jsonRoundTrip(t *testing.T, value any) any {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	var decoded any

	require.NoError(t, json.Unmarshal(encoded, &decoded))

	return decoded
}
