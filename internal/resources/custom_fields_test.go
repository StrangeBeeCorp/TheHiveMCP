package resources_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func customFieldsContext(t *testing.T, cortex *testutils.FakeCortex) context.Context {
	t.Helper()

	ctx := utils.AddPermissionsToContext(context.Background(), permissions.LoadAdminForTesting())

	return context.WithValue(ctx, types.HiveClientCtxKey, cortex.Client(t))
}

// TheHive keeps a field's options when its type changes, and lists an empty
// list for a field without any: the client refused both as "more than one
// schema", and the resource failed on every instance with such a field.
//
// spec: VER-002
func TestCustomFieldsResourceListsEveryKindOfOptionsTheHiveKeeps(t *testing.T) {
	t.Parallel()

	ctx := customFieldsContext(t, &testutils.FakeCortex{})

	contents, err := resources.GetAvailableCustomFields(ctx, mcp.ReadResourceRequest{})
	require.NoError(t, err)
	require.Len(t, contents, 1)

	text, ok := contents[0].(mcp.TextResourceContents)
	require.True(t, ok)

	var fields []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &fields))

	byName := map[string]map[string]any{}

	for _, field := range fields {
		name, _ := field["name"].(string)
		byName[name] = field
	}

	require.Len(t, byName, 4, "every recorded field is listed")

	external := byName["external-url"]
	require.NotNil(t, external)
	assert.Equal(t, "url", external["type"])
	assert.Empty(t, external["options"], "a field without options lists none")

	assert.Equal(t, "integer", byName["risk-score"]["type"])
	assert.Equal(t, []any{0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}, byName["risk-score"]["options"])

	assert.Equal(t, []any{"HR", "Sales"}, byName["department"]["options"])

	assert.Equal(t, "integer", byName["incident-class"]["type"])
	assert.Equal(t, []any{"Malware", "Intrusion", "Data Leak"}, byName["incident-class"]["options"],
		"an integer field holding string options keeps them")
}

// When TheHive refuses the list, the error says what TheHive said, not what Go
// prints for a pointer: the whole *http.Response (headers, addresses) was in it.
func TestCustomFieldsResourceErrorCarriesTheStatusAndTheHiveMessage(t *testing.T) {
	t.Parallel()

	refusals := map[string]struct {
		answer  testutils.Answer
		status  string
		message string
	}{
		"500": {testutils.Answer{Status: 500, Body: `{"type":"InternalError","message":"index unavailable"}`}, "500", "index unavailable"},
		"403": {testutils.Answer{Status: 403, Body: `{"type":"AuthorizationError","message":"Insufficient rights"}`}, "403", "Insufficient rights"},
	}

	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := customFieldsContext(t, &testutils.FakeCortex{CustomFieldsAnswer: refusal.answer})

			_, err := resources.GetAvailableCustomFields(ctx, mcp.ReadResourceRequest{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), refusal.status)
			assert.Contains(t, err.Error(), refusal.message)
			assert.NotContains(t, err.Error(), "&{", "the error must not print the *http.Response")
			assert.NotContains(t, err.Error(), "0xc", "the error must not print Go pointers")
			assert.NotContains(t, err.Error(), "map[", "the error must not print the response headers")
		})
	}
}
