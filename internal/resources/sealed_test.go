package resources_test

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

// Other options TheHive keeps, a field TheHive 5.9 may add, and a field of a
// type that never had options: none may make the resource fail.
//
// spec: VER-002
func TestCustomFieldsResourceListsOtherOptionsAndFieldsItDoesNotKnow(t *testing.T) {
	t.Parallel()

	body := `[
{"_id":"~1","_type":"CustomField","_createdBy":"a","_createdAt":1,"name":"ratio","displayName":"Ratio","group":"default","description":"d","type":"float","options":[0.5,1.5],"mandatory":false,"extraData":{},"futureProperty":{"a":1}},
{"_id":"~2","_type":"CustomField","_createdBy":"a","_createdAt":1,"name":"seen","displayName":"Seen","group":"default","description":"d","type":"date","options":[],"mandatory":true,"extraData":{}},
{"_id":"~3","_type":"CustomField","_createdBy":"a","_createdAt":1,"name":"big","displayName":"Big","group":"default","description":"d","type":"integer","options":[100000,200000],"mandatory":false,"extraData":{}},
{"_id":"~4","_type":"CustomField","_createdBy":"a","_createdAt":1,"name":"flag","displayName":"Flag","group":"default","description":"d","type":"boolean","options":[],"mandatory":false,"extraData":{}}
]`

	ctx := customFieldsContext(t, &testutils.FakeCortex{CustomFieldsAnswer: testutils.Answer{Body: body}})

	contents, err := resources.GetAvailableCustomFields(ctx, mcp.ReadResourceRequest{})
	require.NoError(t, err)
	require.Len(t, contents, 1)

	text, ok := contents[0].(mcp.TextResourceContents)
	require.True(t, ok)

	var fields []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &fields))
	require.Len(t, fields, 4)

	byName := map[string]map[string]any{}

	for _, field := range fields {
		name, _ := field["name"].(string)
		byName[name] = field
	}

	assert.Equal(t, []any{0.5, 1.5}, byName["ratio"]["options"])
	assert.Equal(t, []any{100000.0, 200000.0}, byName["big"]["options"])
	assert.Empty(t, byName["seen"]["options"])
	assert.Empty(t, byName["flag"]["options"])
}

// A refusal that is not JSON still comes back as a status and a short cause.
func TestCustomFieldsResourceErrorOfAPageIsNotAPrintedResponse(t *testing.T) {
	t.Parallel()

	ctx := customFieldsContext(t, &testutils.FakeCortex{CustomFieldsAnswer: testutils.Answer{Status: 401, Body: `<html><body>Unauthorized</body></html>`}})

	_, err := resources.GetAvailableCustomFields(ctx, mcp.ReadResourceRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.NotContains(t, err.Error(), "&{")
	assert.NotContains(t, err.Error(), "0xc")
	assert.NotContains(t, err.Error(), "map[")
}
