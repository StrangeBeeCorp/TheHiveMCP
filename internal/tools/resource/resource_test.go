package resource_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestGetResourceCatalog(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "")

	requireResourceMeta(t, structuredData, "hive://catalog", "Resource Catalog", mimeJSON)

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	categoryNames := collectNames(t, data, fieldCategories, fieldName)

	require.Contains(t, categoryNames, "config")
	require.Contains(t, categoryNames, "schema")
	require.Contains(t, categoryNames, "metadata")
	require.Contains(t, categoryNames, "docs")
}

func TestGetResourceBrowseSchemaCategory(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "schema")

	require.Equal(t, "hive://schema", structuredData[argURI])

	resourceNames := collectNames(t, structuredData, fieldResources, fieldName)

	require.Contains(t, resourceNames, "Alert Output Schema")
	require.Contains(t, resourceNames, "Case Output Schema")
	require.Contains(t, resourceNames, "Task Output Schema")
	require.Contains(t, resourceNames, "Observable Output Schema")
}

func TestGetResourceFetchAlertSchema(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://schema/alert")

	requireResourceMeta(t, structuredData, "hive://schema/alert", "Alert Output Schema", mimeJSON)

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	fields, ok := data["properties"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, fields)

	fieldNames := make([]string, 0)
	for name := range fields {
		fieldNames = append(fieldNames, name)
	}

	require.Contains(t, fieldNames, "title")
	require.Contains(t, fieldNames, "severity")
	require.Contains(t, fieldNames, "type")
	require.Contains(t, fieldNames, "source")
}

func TestGetResourceFetchCurrentUser(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://config/current-user")

	requireResourceMeta(t, structuredData, "hive://config/current-user", "Current User", mimeJSON)

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	require.Contains(t, data, "login")
	require.Contains(t, data, "name")
	require.NotEmpty(t, data["login"])

	// The MCP client authenticates as this test's dedicated per-test-org user.
	require.Equal(t, testutils.TestUserLogin(t), data["login"])
}

func TestGetResourceFetchDocumentation(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://docs/entities/case")

	requireResourceMeta(t, structuredData, "hive://docs/entities/case", "Case Documentation", "text/plain")

	data, ok := structuredData["content"].(string)
	require.True(t, ok)
	require.NotEmpty(t, data)

	require.Contains(t, data, "A case is a structured entity used to track, investigate,")
}

func TestGetResourceBrowseMetadataCategory(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "metadata")

	require.Equal(t, "hive://metadata", structuredData[argURI])

	subcategoryNames := collectNames(t, structuredData, fieldSubcategories, fieldName)

	require.Contains(t, subcategoryNames, "entities")
	require.Contains(t, subcategoryNames, "automation")
	require.Contains(t, subcategoryNames, "organisation")
}

func TestGetResourceFetchCaseStatuses(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://metadata/entities/case/statuses")

	require.Equal(t, "hive://metadata/entities/case/statuses", structuredData[argURI])
	require.Equal(t, "Case Statuses", structuredData[fieldName])

	statusValues := collectNames(t, structuredData, fieldData, fieldValue)

	require.Contains(t, statusValues, "New")
	require.Contains(t, statusValues, "InProgress")
}

func TestGetResourceTrailingSlashEquivalence(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	noSlashData := getResourceStructured(t, mcpClient, "metadata/automation")
	withSlashData := getResourceStructured(t, mcpClient, "hive://metadata/automation/")

	require.Equal(t, "hive://metadata/automation", noSlashData[argURI])
	require.Equal(t, "hive://metadata/automation", withSlashData[argURI])
	require.Equal(t, noSlashData[fieldSubcategories], withSlashData[fieldSubcategories])

	// Compare resources as sets: order may vary.
	noSlashResources, ok := noSlashData[fieldResources].([]any)
	require.True(t, ok)
	withSlashResources, ok := withSlashData[fieldResources].([]any)
	require.True(t, ok)

	require.Len(t, withSlashResources, len(noSlashResources), "should have same number of resources")

	noSlashResourcesMap := make(map[string]any)
	withSlashResourcesMap := make(map[string]any)

	for _, res := range noSlashResources {
		resMap, ok := res.(map[string]any)
		require.True(t, ok)

		name, ok := resMap[fieldName].(string)
		require.True(t, ok)

		noSlashResourcesMap[name] = resMap
	}

	for _, res := range withSlashResources {
		resMap, ok := res.(map[string]any)
		require.True(t, ok)

		name, ok := resMap[fieldName].(string)
		require.True(t, ok)

		withSlashResourcesMap[name] = resMap
	}

	require.Equal(t, noSlashResourcesMap, withSlashResourcesMap, "resources should be equivalent regardless of order")
}

func TestGetResourceResourcesFieldBehavior(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	// Browse a category that has only subcategories (no direct resources)
	structuredData := getResourceStructured(t, mcpClient, "metadata")

	subcategories, exists := structuredData[fieldSubcategories]
	require.True(t, exists, "subcategories field should exist")
	require.NotNil(t, subcategories, "subcategories should not be null")

	subcategoriesList, ok := subcategories.([]any)
	require.True(t, ok)
	require.NotEmpty(t, subcategoriesList, "Should contain automation, entities, organisation subcategories")
}

// The whole point of the annotation is to be right about a live deployment, so
// it can only be verified against one: Pattern has 12 of 23 attributes at
// indexType none, and filtering on any of them returns zero rows instead of an
// error. Without the annotation a caller reads the schema, sees the field, and
// has no way to know.
//
// The expectations are read from the same describe endpoint the annotation is
// built from, rather than hard-coded, because indexType is a property of the
// deployment: hard-coding 5.6's answers made this fail on the 5.5 CI leg.
func TestGetResourcePatternSchemaMatchesDescribe(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(t)

	description, _, err := hiveClient.DescribeAPI.DescribeAModel(authContext, types.EntityTypePattern).Execute()
	if err != nil {
		// thehive4go cannot decode TheHive 5.5's describe: the generated model
		// requires `cardinality`, which 5.5 omits (it sends `values`/`labels`
		// instead). The annotation fails open there, so there is nothing to
		// assert — skip loudly rather than pretend this version is covered.
		t.Skipf("describe is undecodable on this TheHive, so no annotation is produced: %v", err)
	}

	wantModes := map[string]string{}

	for _, attribute := range description.Attributes {
		instance := attribute.GetActualInstance()
		if instance == nil {
			continue
		}

		name, indexType := describedAttribute(t, instance)

		switch indexType {
		case "standard", "fulltext":
			wantModes[name] = "exact"
		case "fulltextOnly":
			wantModes[name] = "fulltext"
		case "none":
			wantModes[name] = "no"
		}
	}

	require.NotEmpty(t, wantModes, "describe must report index types to compare against")

	structuredData := getResourceStructured(t, mcpClientForPattern(t), "hive://schema/pattern")

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	properties, ok := data["properties"].(map[string]any)
	require.True(t, ok)

	checked := 0

	for name, wantMode := range wantModes {
		property, isObject := properties[name].(map[string]any)
		if !isObject {
			continue // describe covers attributes the output schema does not list
		}

		require.Equal(t, wantMode, property["filterable"],
			"%s: the schema must report what describe says about it", name)

		checked++
	}

	require.NotZero(t, checked, "at least one described attribute must appear in the schema")

	// The unindexed fields that motivate this must be among them, whatever the
	// version: if TheHive stops describing them the annotation loses its point.
	for _, field := range []string{"platforms", "dataSources", "detection"} {
		require.Contains(t, wantModes, field, "describe should still report %s", field)
		require.Equal(t, "no", wantModes[field], "%s is expected to be unindexed", field)
	}

	require.Contains(t, data, "filterableLegend", "the annotation must explain its own key")
}

// describedAttribute reads the name and index type off one describe attribute,
// mirroring what the annotation does.
func describedAttribute(t *testing.T, instance any) (name, indexType string) {
	t.Helper()

	value := reflect.ValueOf(instance)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	return value.FieldByName("Name").String(), fmt.Sprintf("%v", value.FieldByName("IndexType").Interface())
}

// mcpClientForPattern is newResourceClient, named for what this test needs.
func mcpClientForPattern(t *testing.T) *client.Client {
	t.Helper()

	return newResourceClient(t)
}
