package resource_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

const customFieldsURI = "hive://metadata/entities/custom-fields"

// createCustomField creates a custom field, deleted when the test ends.
func createCustomField(t *testing.T, name string, fieldType thehive.CustomFieldType, options []any) {
	t.Helper()

	hiveClient := testutils.NewTestClient(t)
	authContext := testutils.GetAuthContext(t)

	input := thehive.NewInputCustomField(name, "default", "created by an integration test", fieldType)
	input.Options = options

	created, httpResp, err := hiveClient.CustomFieldAPI.CreateCustomField(authContext).InputCustomField(*input).Execute()
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}

	require.NoError(t, err, "TheHive must accept the custom field %s", name)
	require.NotNil(t, created)

	t.Cleanup(func() {
		deleteResp, deleteErr := hiveClient.CustomFieldAPI.DeleteCustomField(authContext, created.UnderscoreId).Execute()
		if deleteResp != nil && deleteResp.Body != nil {
			_ = deleteResp.Body.Close()
		}

		assert.NoError(t, deleteErr, "cleanup of the custom field %s", name)
	})
}

// The custom fields of the instance, read through the MCP client, whatever
// options they hold: a string field without options (TheHive lists "options":
// []) and an integer one with options, both of which the client once refused.
// Custom fields are instance-wide: the shared devstack TheHive is read, never
// written.
//
// spec: VER-002
func TestCustomFieldsResourceReadsTheFieldsOfTheInstance(t *testing.T) { //nolint:paralleltest // integration test: testutils.Parallel
	testutils.Parallel(t)

	mcpClient := newResourceClient(t)

	var stringField, integerField string

	if !testutils.SharedMode() {
		suffix := time.Now().UnixNano()
		stringField = fmt.Sprintf("mcp-it-string-%d", suffix)
		integerField = fmt.Sprintf("mcp-it-integer-%d", suffix)

		createCustomField(t, stringField, thehive.CUSTOMFIELDTYPE_STRING, nil)
		createCustomField(t, integerField, thehive.CUSTOMFIELDTYPE_INTEGER, []any{1, 2, 3})
	}

	data := getResourceStructured(t, mcpClient, customFieldsURI)
	require.Equal(t, customFieldsURI, data[argURI])

	if testutils.SharedMode() {
		return
	}

	rows, ok := data[fieldData].([]any)
	require.True(t, ok, "the resource lists custom fields as an array")

	var foundString, foundInteger bool

	for _, row := range rows {
		field, isObject := row.(map[string]any)
		require.True(t, isObject)

		name := fmt.Sprint(field[fieldName])

		switch {
		case strings.Contains(name, stringField):
			foundString = true

			assert.Empty(t, field["options"], "a string field created without options lists none")
		case strings.Contains(name, integerField):
			foundInteger = true

			assert.Equal(t, []any{1.0, 2.0, 3.0}, field["options"])
		}
	}

	assert.True(t, foundString, "the string custom field %s is in the resource", stringField)
	assert.True(t, foundInteger, "the integer custom field %s is in the resource", integerField)
}
