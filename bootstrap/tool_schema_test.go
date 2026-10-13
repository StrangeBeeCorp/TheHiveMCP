package bootstrap

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/resource"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
)

// A tool that advertises no parameters is worse than a missing tool: the model
// is told it takes none, calls it empty, and gets a validation error it cannot
// diagnose. mcp.WithInputSchema fails that way silently — on an inference error
// it writes to stderr and returns without setting a schema — so nothing but an
// assertion on the advertised schema catches it.
//
// This is not hypothetical. Upgrading to mcp-go v1.0.0 moved inference to
// github.com/google/jsonschema-go, which rejects the `jsonschema:"enum=..."`
// tag syntax the params structs used. Three of the four tools began advertising
// zero properties. Every unit and integration test still passed, because tests
// call tools with arguments they build themselves and never read the schema.
// Tool names, and the parameter names the enumeration test pins.
const (
	toolManageEntities    = "manage-entities"
	toolSearchEntities    = "search-entities"
	toolExecuteAutomation = "execute-automation"
	toolGetResource       = "get-resource"

	paramOperation  = "operation"
	paramEntityType = "entity-type"

	entityAlert = "alert"
	entityCase  = "case"
	entityTask  = "task"
	entityObs   = "observable"

	typeArray  = "array"
	typeObject = "object"
)

func advertisedSchema(t *testing.T, tool mcp.Tool) map[string]any {
	t.Helper()

	raw := tool.RawInputSchema
	if len(raw) == 0 {
		marshalled, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)

		raw = marshalled
	}

	var schema map[string]any
	require.NoError(t, json.Unmarshal(raw, &schema), "tool %q has an undecodable input schema", tool.Name)

	return schema
}

func propertiesOf(t *testing.T, tool mcp.Tool) map[string]any {
	t.Helper()

	properties, ok := advertisedSchema(t, tool)["properties"].(map[string]any)
	require.True(t, ok, "tool %q advertises no properties object", tool.Name)

	return properties
}

// jsonFieldNames returns the wire names of T's fields, which are exactly the
// property names the schema must advertise.
func jsonFieldNames(t *testing.T, params any) []string {
	t.Helper()

	typ := reflect.TypeOf(params)
	names := make([]string, 0, typ.NumField())

	for field := range typ.Fields() {
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		names = append(names, strings.Split(tag, ",")[0])
	}

	return names
}

// toolsUnderTest pairs each registered tool with the struct its schema is
// inferred from, so drift between the two fails here.
func toolsUnderTest() map[string]struct {
	tool   mcp.Tool
	params any
} {
	return map[string]struct {
		tool   mcp.Tool
		params any
	}{
		toolManageEntities:    {manage.NewManageTool().Definition(), manage.EntityParams{}},
		toolSearchEntities:    {search.NewSearchTool().Definition(), search.EntitiesParams{}},
		toolExecuteAutomation: {execute_automation.NewExecuteAutomationTool().Definition(), execute_automation.ExecuteAutomationParams{}},
		toolGetResource:       {resource.NewResourceTool(nil).Definition(), resource.GetResourceParams{}},
	}
}

// Every field of the params struct must appear in the advertised schema.
func TestToolSchemas_AdvertiseTheirParameters(t *testing.T) {
	t.Parallel()

	for name, subject := range toolsUnderTest() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			properties := propertiesOf(t, subject.tool)
			assert.NotEmpty(t, properties, "tool %q advertises an empty schema; clients cannot call it", name)

			for _, field := range jsonFieldNames(t, subject.params) {
				assert.Contains(t, properties, field,
					"tool %q does not advertise parameter %q", name, field)
			}
		})
	}
}

// Descriptions come from `jsonschema_description`, which survived the v1.0.0
// inference change. Assert it, since losing them would be as quiet as losing
// the properties was.
func TestToolSchemas_ParametersKeepTheirDescriptions(t *testing.T) {
	t.Parallel()

	for name, subject := range toolsUnderTest() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for field, raw := range propertiesOf(t, subject.tool) {
				property, ok := raw.(map[string]any)
				require.True(t, ok, "property %q is not an object", field)

				description, _ := property["description"].(string)
				assert.NotEmpty(t, description, "tool %q parameter %q has no description", name, field)
			}
		})
	}
}

// The enumerations moved out of struct tags into explicit constraints, so a
// constraint naming a field that no longer exists would silently do nothing.
// Pin the closed sets that callers actually depend on.
func TestToolSchemas_EnumerationsSurviveInference(t *testing.T) {
	t.Parallel()

	expected := map[string]map[string][]string{
		toolManageEntities: {
			paramOperation:  {"create", "update", "delete", "comment", "promote", "merge", "apply-template"},
			paramEntityType: {entityCase, entityAlert, entityTask, entityObs, "procedure", "case-template", "page"},
		},
		toolSearchEntities: {
			paramEntityType: {entityAlert, entityCase, entityTask, entityObs, "procedure", "pattern", "case-template", "page", "knowledge-base", "job", "action"},
			"sort-order":    {"asc", "desc"},
		},
		toolExecuteAutomation: {
			paramOperation:  {"run-analyzer", "run-responder", "get-job-status", "get-action-status", "wait-for-jobs"},
			paramEntityType: {entityCase, entityAlert, entityTask, entityObs},
		},
	}

	subjects := toolsUnderTest()

	for name, fields := range expected {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			properties := propertiesOf(t, subjects[name].tool)

			for field, want := range fields {
				property, ok := properties[field].(map[string]any)
				require.True(t, ok, "tool %q has no property %q", name, field)

				raw, ok := property["enum"].([]any)
				require.True(t, ok, "tool %q parameter %q advertises no enum", name, field)

				got := make([]string, 0, len(raw))

				for _, value := range raw {
					str, isString := value.(string)
					require.True(t, isString, "enum value %v is not a string", value)

					got = append(got, str)
				}

				assert.ElementsMatch(t, want, got,
					"tool %q parameter %q advertises the wrong permitted values", name, field)
			}
		})
	}
}

// Required-ness is inferred from the absence of `omitempty`. Pin the fields
// callers must always send, so a stray `omitempty` cannot quietly relax them.
func TestToolSchemas_RequiredParameters(t *testing.T) {
	t.Parallel()

	expected := map[string][]string{
		toolManageEntities:    {paramOperation, paramEntityType},
		toolSearchEntities:    {paramEntityType},
		toolExecuteAutomation: {paramOperation},
	}

	subjects := toolsUnderTest()

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw, _ := advertisedSchema(t, subjects[name].tool)["required"].([]any)

			got := make([]string, 0, len(raw))

			for _, value := range raw {
				str, isString := value.(string)
				require.True(t, isString, "required entry %v is not a string", value)

				got = append(got, str)
			}

			assert.ElementsMatch(t, want, got, "tool %q advertises the wrong required parameters", name)
		})
	}
}

// An input type must not offer null. jsonschema-go infers a slice or a pointer
// as ["null", T], because a nil one marshals to null — true of what the server
// emits, meaningless for what a caller sends, since an omitted parameter and a
// null one decode to the same zero value. Advertised to a model, the union reads
// as "not quite an array": measured on muse-spark 1.3, search-entities'
// extra-columns came in as the JSON-encoded STRING '["description", …]' on every
// one of 59 alerts, and the MCP gateway, which validates arguments against this
// schema, rejected each call with "is not of type 'null', 'array'".
//
// It sweeps the tools the server actually registered, not toolsUnderTest, so a
// tool added later is covered without anyone remembering to list it. Nothing
// rewrites a schema at registration, so this is also what catches a tool built
// with a bare mcp.WithInputSchema instead of tools.WithInputSchema. It flags
// every form a nullable input takes, not only the ["null", T] union
// tools.WithInputSchema drops: a "type": "null" or a null anyOf branch has to be
// written by hand, and is refused just the same.
func TestToolSchemas_InputTypesAdmitNoNull(t *testing.T) {
	t.Parallel()

	registered := registeredTools()
	require.NotEmpty(t, registered, "the server registered no tools")

	for name, tool := range registered {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, nullableTypePaths(advertisedSchema(t, tool), "$"),
				"tool %q advertises input types that admit null", name)
		})
	}
}

// nullableTypePaths is the oracle of TestToolSchemas_InputTypesAdmitNoNull, so
// it is pinned on its own: it must see every way a schema can admit null, and
// nothing that does not.
func TestNullableTypePaths(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		schema string
		want   []string
	}{
		"type union holding null": {
			schema: `{"type": ["null", "array"]}`,
			want:   []string{"$"},
		},
		"type null alone": {
			schema: `{"properties": {"a": {"type": "null"}}}`,
			want:   []string{"$.properties.a"},
		},
		"null branch of anyOf": {
			schema: `{"properties": {"a": {"anyOf": [{"type": "null"}, {"type": "array"}]}}}`,
			want:   []string{"$.properties.a.anyOf[0]"},
		},
		"null below items and $defs": {
			schema: `{"items": {"type": ["null", "string"]}, "$defs": {"d": {"type": ["null", "object"]}}}`,
			want:   []string{"$.items", "$.$defs.d"},
		},
		"null held in value keywords": {
			schema: `{"properties": {"a": {
				"type": "object",
				"default": {"type": ["null", "string"]},
				"const": {"type": "null"},
				"enum": [{"type": ["null", "string"]}],
				"examples": [{"type": "null"}]
			}}}`,
			want: nil,
		},
		"null under not refuses it": {
			schema: `{"properties": {"a": {"not": {"type": ["null", "string"]}}}}`,
			want:   nil,
		},
		"null in an if condition, and in then": {
			schema: `{"properties": {"a": {"if": {"type": ["null", "string"]}, "then": {"type": ["null", "string"]}}}}`,
			want:   []string{"$.properties.a.then"},
		},
		"a property named default is a schema": {
			schema: `{"properties": {"default": {"type": ["null", "array"]}}}`,
			want:   []string{"$.properties.default"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var schema any

			require.NoError(t, json.Unmarshal([]byte(tc.schema), &schema), "fixture %q", name)
			assert.ElementsMatch(t, tc.want, nullableTypePaths(schema, "$"), "schema %s", tc.schema)
		})
	}
}

// Every test built on toolsUnderTest reads the schema from Definition(). That is
// only sound if registration advertises that very schema, rather than a
// rewritten one no such test sees.
func TestToolSchemas_DefinitionIsWhatTheServerAdvertises(t *testing.T) {
	t.Parallel()

	registered := registeredTools()

	for name, subject := range toolsUnderTest() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Contains(t, registered, name, "tool %q is not registered", name)
			assert.Equal(t, advertisedSchema(t, subject.tool), advertisedSchema(t, registered[name]),
				"tool %q: Definition() schema vs the one the server advertises", name)
		})
	}
}

// Dropping null must leave the type the struct declares, as a plain keyword: an
// array parameter reads "array", not "null" or ["array"]. The map parameters
// never carried a union, but are pinned alongside so the collections a caller
// most often sends JSON-encoded as a string all have their type stated here.
func TestToolSchemas_CollectionParametersAdvertiseAPlainType(t *testing.T) {
	t.Parallel()

	expected := map[string]map[string]string{
		toolManageEntities: {
			"entity-ids":  typeArray,
			"entity-data": typeObject,
		},
		toolSearchEntities: {
			"extra-columns":      typeArray,
			"extra-data":         typeArray,
			"additional-queries": typeArray,
			"filters":            typeObject,
		},
		toolExecuteAutomation: {
			"parameters": typeObject,
		},
	}

	registered := registeredTools()

	for name, parameters := range expected {
		require.Contains(t, registered, name, "tool %q is not registered", name)

		properties := propertiesOf(t, registered[name])

		for parameter, want := range parameters {
			property, ok := properties[parameter].(map[string]any)
			require.True(t, ok, "tool %q has no property %q", name, parameter)
			assert.Equal(t, want, property["type"], "tool %q parameter %q", name, parameter)
		}
	}
}

// registeredTools returns the tools as the server advertises them, after every
// step registration applies to their definitions. The server is built once, as
// building it registers every tool and resource; callers only read the map.
var registeredTools = sync.OnceValue(func() map[string]mcp.Tool {
	registered := make(map[string]mcp.Tool)

	for name, serverTool := range GetMCPServerAndRegisterTools().ListTools() {
		registered[name] = serverTool.Tool
	}

	return registered
})

// nullableTypePaths lists the paths of the subschemas whose type admits null —
// "null" alone, or a union holding it, which also covers a null branch of
// anyOf / oneOf — at any depth (properties, items, $defs, …).
//
// It walks only the keywords holding subschemas, listed here from the JSON
// Schema spec rather than taken from the production code it checks. It skips
// default, enum, const and examples, which hold values, and not and the if
// condition, where a null type refuses null or only picks a branch.
func nullableTypePaths(node any, path string) []string {
	schema, isSchema := node.(map[string]any)
	if !isSchema {
		return nil
	}

	var found []string

	switch types := schema["type"].(type) {
	case string:
		if types == "null" {
			found = append(found, path)
		}
	case []any:
		if slices.Contains(types, any("null")) {
			found = append(found, path)
		}
	}

	for _, keyword := range []string{
		"properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependencies",
	} {
		named, _ := schema[keyword].(map[string]any)
		for name, subschema := range named {
			found = append(found, nullableTypePaths(subschema, path+"."+keyword+"."+name)...)
		}
	}

	for _, keyword := range []string{
		"items", "prefixItems", "additionalItems", "additionalProperties", "unevaluatedItems",
		"unevaluatedProperties", "propertyNames", "contains", "then", "else",
		"allOf", "anyOf", "oneOf", "contentSchema",
	} {
		list, isList := schema[keyword].([]any)
		if !isList {
			found = append(found, nullableTypePaths(schema[keyword], path+"."+keyword)...)

			continue
		}

		for index, subschema := range list {
			found = append(found, nullableTypePaths(subschema, path+"."+keyword+"["+strconv.Itoa(index)+"]")...)
		}
	}

	return found
}
