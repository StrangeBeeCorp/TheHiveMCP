package bootstrap

import (
	"encoding/json"
	"reflect"
	"strings"
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
			paramEntityType: {entityAlert, entityCase, entityTask, entityObs, "procedure", "pattern", "case-template", "page"},
			"sort-order":    {"asc", "desc"},
		},
		toolExecuteAutomation: {
			paramOperation:  {"run-analyzer", "run-responder", "get-job-status", "get-action-status"},
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
