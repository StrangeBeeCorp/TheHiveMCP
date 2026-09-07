package bootstrap

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
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
			paramEntityType: {entityAlert, entityCase, entityTask, entityObs, "procedure", "pattern", "case-template", "page", "job", "action"},
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

// constraintsUnderTest pairs each tool with the constraint map its definition
// applies, so the tests can check the maps themselves rather than only the
// handful of values another test happens to spell out.
func constraintsUnderTest() map[string]map[string]tools.SchemaConstraint {
	return map[string]map[string]tools.SchemaConstraint{
		toolManageEntities:    manage.EntityParamConstraints,
		toolSearchEntities:    search.EntitiesParamConstraints,
		toolExecuteAutomation: execute_automation.ExecuteAutomationParamConstraints,
	}
}

// A constraint naming a property the struct does not declare is silently
// skipped: WithInputSchemaConstraints logs it and moves on, so a typo or a
// renamed field would quietly drop an enum or a default from the wire schema
// while every other test still passed.
//
// This is the drift guard that schema.go's documentation promises.
func TestToolSchemas_ConstraintsMatchTheStruct(t *testing.T) {
	t.Parallel()

	subjects := toolsUnderTest()

	for name, constraints := range constraintsUnderTest() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			properties := propertiesOf(t, subjects[name].tool)

			for property := range constraints {
				assert.Contains(t, properties, property,
					"tool %q constrains %q, which its parameters struct does not declare", name, property)
			}
		})
	}
}

// Every constraint must reach the advertised schema. TestToolSchemas_Enumerations
// pins the values callers depend on most, but only for the keys it lists; this
// asserts that no constraint of either kind is dropped — the defaults for
// sort-by, sort-order and limit included.
func TestToolSchemas_ConstraintsReachTheWire(t *testing.T) {
	t.Parallel()

	subjects := toolsUnderTest()

	for name, constraints := range constraintsUnderTest() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			properties := propertiesOf(t, subjects[name].tool)

			for propertyName, constraint := range constraints {
				property, ok := properties[propertyName].(map[string]any)
				require.True(t, ok, "tool %q has no property %q", name, propertyName)

				if len(constraint.Enum) > 0 {
					advertised, hasEnum := property["enum"].([]any)
					require.True(t, hasEnum, "tool %q parameter %q advertises no enum", name, propertyName)
					assert.Len(t, advertised, len(constraint.Enum),
						"tool %q parameter %q advertises a different number of permitted values", name, propertyName)
				}

				if constraint.Default != nil {
					// Presence only: this test reads its expectation from the
					// constraint, so it cannot judge the value. The values are
					// pinned independently in TestToolSchemas_DefaultsSurviveInference.
					assert.Contains(t, property, "default",
						"tool %q parameter %q advertises no default, but one is constrained", name, propertyName)
				}
			}
		})
	}
}

// The advertised defaults, written out rather than read back from the
// constraint maps.
//
// TestToolSchemas_ConstraintsReachTheWire can only prove a default is present:
// it takes its expectation from the same map it is checking, so changing the
// constraint changes both sides and a wrong value passes. These are the values
// callers actually receive, so they are stated here independently.
func TestToolSchemas_DefaultsSurviveInference(t *testing.T) {
	t.Parallel()

	expected := map[string]map[string]any{
		toolSearchEntities: {
			// sort-by is absent on purpose: its applied default is
			// per-entity-type, so advertising one value would be wrong for job
			// and action. TestSearchEntitiesSortByAdvertisesNoDefault pins that.
			"sort-order": "desc",
			"limit":      float64(10),
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

				assert.Equal(t, want, property["default"],
					"tool %q parameter %q advertises the wrong default", name, field)
			}
		})
	}
}

// The advertised entity-type enum must match exactly what the handler accepts.
//
// These were two hand-maintained lists — the enum in search.EntitiesParamConstraints
// and the slice ValidateParams checks — and adding an entity type to one without
// the other fails in whichever direction the drift went: a type accepted by the
// handler but absent from the enum is invisible to clients, and a type in the
// enum the handler rejects is advertised as supported and errors on use. Both
// now read from search.ValidEntityTypes; this asserts the wire schema agrees
// with the code path that enforces it, whatever they are built from.
func TestSearchEntityTypes_AdvertisedEnumMatchesValidation(t *testing.T) {
	t.Parallel()

	searchTool := search.NewSearchTool()
	properties := propertiesOf(t, searchTool.Definition())

	property, ok := properties[paramEntityType].(map[string]any)
	require.True(t, ok, "search-entities has no %q property", paramEntityType)

	raw, ok := property["enum"].([]any)
	require.True(t, ok, "search-entities parameter %q advertises no enum", paramEntityType)

	advertised := make([]string, 0, len(raw))

	for _, value := range raw {
		str, isString := value.(string)
		require.True(t, isString, "enum value %v is not a string", value)

		advertised = append(advertised, str)
	}

	assert.ElementsMatch(t, search.ValidEntityTypes, advertised,
		"the advertised entity-type enum and the types ValidateParams accepts have drifted")

	// Every advertised value must survive validation, and validation must apply
	// a sort default for it — a type reachable from the schema but missing a
	// default sorts on a column TheHive may not have.
	for _, entityType := range advertised {
		params := search.EntitiesParams{EntityType: entityType}

		err := searchTool.ValidateParams(&params)
		require.NoError(t, err, "advertised entity-type %q is rejected by ValidateParams", entityType)
		assert.NotEmpty(t, params.SortBy, "advertised entity-type %q gets no default sort field", entityType)
	}
}

// sort-by must advertise no default.
//
// The applied default depends on the entity type (types.DefaultSortField gives
// startDate for job and action, _createdAt otherwise), so a single advertised
// value would be a false claim for two of the ten types. It is also actively
// harmful: a client that materializes schema defaults would send _createdAt
// explicitly, and ValidateParams only applies the per-type default when the
// caller omits the field. The rule is documented in the parameter description.
func TestSearchEntitiesSortByAdvertisesNoDefault(t *testing.T) {
	t.Parallel()

	properties := propertiesOf(t, search.NewSearchTool().Definition())

	property, ok := properties["sort-by"].(map[string]any)
	require.True(t, ok, "search-entities has no sort-by property")

	assert.NotContains(t, property, "default",
		"sort-by advertises a default, but the applied default is per-entity-type")

	description, _ := property["description"].(string)
	assert.Contains(t, description, "startDate",
		"sort-by advertises no default, so its description must state the per-type rule")
}
