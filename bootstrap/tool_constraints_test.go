package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
)

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
