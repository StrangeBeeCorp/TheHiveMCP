package resources

import (
	"encoding/json"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldPlatforms is the Pattern attribute TheHive does not index.
const fieldPlatforms = "platforms"

// A schema that lists every returned field reads as the filter vocabulary, so
// the fields TheHive does not index have to be marked — otherwise a caller
// filters on them and gets zero rows, which is indistinguishable from a
// genuine empty result.
func TestMergeFilterModes_MarksEachDescribedProperty(t *testing.T) {
	t.Parallel()

	schema := []byte(`{
		"type": "object",
		"properties": {
			"name":        {"type": "string"},
			"description": {"type": "string"},
			"platforms":   {"type": "array"},
			"unknownToTheHive": {"type": "string"}
		}
	}`)

	modes := map[string]string{
		keyName:        filterModeExact,
		"description":  filterModeFulltext,
		fieldPlatforms: filterModeNone,
	}

	merged, err := mergeFilterModes(schema, modes)
	require.NoError(t, err)

	var out map[string]any

	require.NoError(t, json.Unmarshal(merged, &out))

	properties, ok := out["properties"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, filterModeExact, propertyKey(t, properties, keyName))
	assert.Equal(t, filterModeFulltext, propertyKey(t, properties, "description"))
	assert.Equal(t, filterModeNone, propertyKey(t, properties, fieldPlatforms))

	// A field describe says nothing about is left alone: marking it
	// unfilterable would trade one wrong answer for another.
	unknown, isObject := properties["unknownToTheHive"].(map[string]any)
	require.True(t, isObject)
	assert.NotContains(t, unknown, filterableKey)

	// The legend travels with the annotation so the key is self-explaining.
	assert.Contains(t, out, filterableKey+"Legend")
}

// Annotating nothing must not add a legend explaining a key that is absent.
func TestMergeFilterModes_NoMatchesLeavesSchemaAlone(t *testing.T) {
	t.Parallel()

	schema := []byte(`{"properties":{"somethingElse":{"type":"string"}}}`)

	merged, err := mergeFilterModes(schema, map[string]string{keyName: filterModeExact})
	require.NoError(t, err)

	assert.JSONEq(t, string(schema), string(merged))
}

func TestMergeFilterModes_SchemaWithoutPropertiesIsUntouched(t *testing.T) {
	t.Parallel()

	schema := []byte(`{"type":"string"}`)

	merged, err := mergeFilterModes(schema, map[string]string{keyName: filterModeExact})
	require.NoError(t, err)

	assert.JSONEq(t, string(schema), string(merged))
}

// PropertyDescription is a generated oneOf over eight variants. Reading it
// reflectively is what keeps a ninth variant from being silently skipped, but
// it also means a rename upstream would go unnoticed — so pin it against the
// real SDK types rather than a fake.
func TestAttributeIndexType_ReadsEveryVariant(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		attribute thehive.PropertyDescription
		wantName  string
		wantIndex string
	}{
		"string": {
			attribute: thehive.StringPropertyDescriptionAsPropertyDescription(
				&thehive.StringPropertyDescription{Name: fieldPlatforms, Type: "string"},
			),
			wantName: fieldPlatforms,
		},
		"boolean": {
			attribute: thehive.BooleanPropertyDescriptionAsPropertyDescription(
				&thehive.BooleanPropertyDescription{Name: "revoked", Type: "boolean"},
			),
			wantName: "revoked",
		},
		"date": {
			attribute: thehive.DatePropertyDescriptionAsPropertyDescription(
				&thehive.DatePropertyDescription{Name: "_createdAt", Type: "date"},
			),
			wantName: "_createdAt",
		},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotName, _, ok := attributeIndexType(testCase.attribute)

			require.True(t, ok, "every oneOf variant carries Name and IndexType")
			assert.Equal(t, testCase.wantName, gotName)
		})
	}
}

// An empty wrapper has no active variant; it must be skipped, not panic.
func TestAttributeIndexType_EmptyWrapperIsSkipped(t *testing.T) {
	t.Parallel()

	_, _, ok := attributeIndexType(thehive.PropertyDescription{})

	assert.False(t, ok)
}

func propertyKey(t *testing.T, properties map[string]any, name string) string {
	t.Helper()

	property, ok := properties[name].(map[string]any)
	require.True(t, ok, "property %s must be an object", name)

	mode, ok := property[filterableKey].(string)
	require.True(t, ok, "property %s must carry %s", name, filterableKey)

	return mode
}
