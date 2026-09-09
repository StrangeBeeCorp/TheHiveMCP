package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// fieldPlatforms is the Pattern attribute TheHive does not index.
const fieldPlatforms = "platforms"

// indexStandard / indexNone spell the expected rendering of the SDK enums.
const (
	indexStandard = string(thehive.ANYINDEXTYPE_STANDARD)
	indexNone     = string(thehive.ANYINDEXTYPE_NONE)
	fieldURL      = "url"
	fieldUser     = "user"
)

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

// PropertyDescription is a generated oneOf over eight variants, read
// reflectively so a ninth is not silently skipped. That also means an upstream
// rename would go unnoticed, so pin every variant against the real SDK types —
// with a populated IndexType, since leaving it zero exercises none of
// indexTypeString and lets a regression there pass.
//
// Note the fixtures use two different enum types: string and url carry
// AnyIndexType (which alone declares the fulltext values), the rest carry
// BasicIndexType. That divergence is why the reader goes through reflection
// rather than a type switch per variant.
func TestAttributeIndexType_ReadsEveryVariant(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		attribute thehive.PropertyDescription
		wantName  string
		wantIndex string
	}{
		"string": {
			attribute: thehive.StringPropertyDescriptionAsPropertyDescription(
				&thehive.StringPropertyDescription{Name: fieldPlatforms, IndexType: thehive.ANYINDEXTYPE_NONE, Type: "string"},
			),
			wantName:  fieldPlatforms,
			wantIndex: indexNone,
		},
		"boolean": {
			attribute: thehive.BooleanPropertyDescriptionAsPropertyDescription(
				&thehive.BooleanPropertyDescription{Name: "revoked", IndexType: thehive.BASICINDEXTYPE_STANDARD, Type: "boolean"},
			),
			wantName:  "revoked",
			wantIndex: indexStandard,
		},
		"date": {
			attribute: thehive.DatePropertyDescriptionAsPropertyDescription(
				&thehive.DatePropertyDescription{Name: "_createdAt", IndexType: thehive.BASICINDEXTYPE_STANDARD, Type: "date"},
			),
			wantName:  "_createdAt",
			wantIndex: indexStandard,
		},
		"enumeration": {
			attribute: thehive.EnumerationPropertyDescriptionAsPropertyDescription(
				&thehive.EnumerationPropertyDescription{Name: "status", IndexType: thehive.BASICINDEXTYPE_STANDARD, Type: "enumeration"},
			),
			wantName:  "status",
			wantIndex: indexStandard,
		},
		"float": {
			attribute: thehive.FloatPropertyDescriptionAsPropertyDescription(
				&thehive.FloatPropertyDescription{Name: "score", IndexType: thehive.BASICINDEXTYPE_STANDARD, Type: "float"},
			),
			wantName:  "score",
			wantIndex: indexStandard,
		},
		"integer": {
			attribute: thehive.IntegerPropertyDescriptionAsPropertyDescription(
				&thehive.IntegerPropertyDescription{Name: "severity", IndexType: thehive.BASICINDEXTYPE_STANDARD, Type: "integer"},
			),
			wantName:  "severity",
			wantIndex: indexStandard,
		},
		fieldURL: {
			attribute: thehive.UrlPropertyDescriptionAsPropertyDescription(
				&thehive.UrlPropertyDescription{Name: fieldURL, IndexType: thehive.ANYINDEXTYPE_STANDARD, Type: fieldURL},
			),
			wantName:  fieldURL,
			wantIndex: indexStandard,
		},
		fieldUser: {
			attribute: thehive.UserPropertyDescriptionAsPropertyDescription(
				&thehive.UserPropertyDescription{Name: "assignee", IndexType: thehive.BASICINDEXTYPE_STANDARD, Type: fieldUser},
			),
			wantName:  "assignee",
			wantIndex: indexStandard,
		},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotName, gotIndex, ok := attributeIndexType(testCase.attribute)

			require.True(t, ok, "every oneOf variant carries Name and IndexType")
			assert.Equal(t, testCase.wantName, gotName)
			assert.Equal(t, testCase.wantIndex, gotIndex, "indexTypeString must render the generated enum type")
		})
	}
}

// Every index type TheHive can report must land on a mode, or the field it
// describes goes unannotated and reads as "unknown" when it is in fact known.
func TestIndexTypeToFilterMode_CoversEveryDeclaredIndexType(t *testing.T) {
	t.Parallel()

	declared := []string{
		string(thehive.ANYINDEXTYPE_STANDARD),
		string(thehive.ANYINDEXTYPE_NONE),
		string(thehive.ANYINDEXTYPE_FULLTEXT),
		string(thehive.ANYINDEXTYPE_FULLTEXT_ONLY),
		string(thehive.BASICINDEXTYPE_STANDARD),
		string(thehive.BASICINDEXTYPE_NONE),
	}

	for _, indexType := range declared {
		assert.Contains(t, indexTypeToFilterMode, indexType,
			"index type %q is declared by the SDK but maps to no filter mode", indexType)
	}
}

func propertyKey(t *testing.T, properties map[string]any, name string) string {
	t.Helper()

	property, ok := properties[name].(map[string]any)
	require.True(t, ok, "property %s must be an object", name)

	mode, ok := property[filterableKey].(string)
	require.True(t, ok, "property %s must carry %s", name, filterableKey)

	return mode
}

// describe and the schemas disagree on shape: describe reports importDate flat
// while OutputAlert nests it under extraData, and reports computed.* dotted. A
// root-only walk reaches neither, which would miss every unindexed alert field.
func TestMergeFilterModes_ReachesNestedProperties(t *testing.T) {
	t.Parallel()

	schema := []byte(`{
		"type": "object",
		"properties": {
			"title": {"type": "string"},
			"extraData": {
				"type": "object",
				"properties": {
					"importDate": {"type": "integer"}
				}
			},
			"computed": {
				"type": "object",
				"properties": {
					"handlingDuration": {"type": "integer"}
				}
			},
			"actions": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {"objectId": {"type": "string"}}
				}
			}
		}
	}`)

	modes := map[string]string{
		"title":                     filterModeExact,
		"importDate":                filterModeNone, // described flat, nested here
		"computed.handlingDuration": filterModeNone, // described dotted
		"objectId":                  filterModeNone, // inside an array's items
	}

	merged, err := mergeFilterModes(schema, modes)
	require.NoError(t, err)

	var out map[string]any

	require.NoError(t, json.Unmarshal(merged, &out))

	properties, ok := out["properties"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, filterModeExact, propertyKey(t, properties, "title"))
	assert.Equal(t, filterModeNone, propertyKey(t, nested(t, properties, "extraData"), "importDate"))
	assert.Equal(t, filterModeNone, propertyKey(t, nested(t, properties, "computed"), "handlingDuration"))
	assert.Equal(t, filterModeNone, propertyKey(t, arrayItems(t, properties, "actions"), "objectId"))
}

// In HTTP mode each request may name its own TheHive and organisation, so a
// cache keyed on the entity alone would hand the first responder's schema shape
// to every other deployment.
func TestCacheKeyFor_ScopedToTheDeployment(t *testing.T) {
	t.Parallel()

	base := cacheKeyFor(context.Background(), "pattern")

	withURL := cacheKeyFor(
		context.WithValue(context.Background(), types.HiveURLCtxKey, "https://hive-a.example"),
		"pattern",
	)
	withOrg := cacheKeyFor(
		context.WithValue(context.Background(), types.HiveOrgCtxKey, "org-b"),
		"pattern",
	)

	assert.NotEqual(t, base, withURL, "a different TheHive must not reuse cached modes")
	assert.NotEqual(t, base, withOrg, "a different organisation must not reuse cached modes")
	assert.NotEqual(t, withURL, withOrg)
	assert.NotEqual(t, base, cacheKeyFor(context.Background(), "alert"), "entity still separates entries")
}

func nested(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()

	parent, ok := properties[name].(map[string]any)
	require.True(t, ok)

	child, ok := parent["properties"].(map[string]any)
	require.True(t, ok)

	return child
}

func arrayItems(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()

	parent, ok := properties[name].(map[string]any)
	require.True(t, ok)

	items, ok := parent["items"].(map[string]any)
	require.True(t, ok)

	child, ok := items["properties"].(map[string]any)
	require.True(t, ok)

	return child
}
