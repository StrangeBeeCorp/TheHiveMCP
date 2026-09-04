package manage_test

import (
	"strconv"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// DL-5764 regression: the manage by-ID scope check must accept a BARE entity id
// (a plain case number, not the ~-prefixed internal _id), because the manage
// tool schema advertises name/number-style ids (e.g. case-template names, bare
// case numbers) and the old get-by-idOrName path resolved them.
//
// The new single-query scope path resolves scope with
//
//	listCase -> filter(_and[ permFilters, _in{_field:_id, _values:ids} ])
//
// and _in{_id} on a list op matches ONLY the ~-prefixed internal id form. A bare
// number never matches, so an in-scope case addressed by its bare number is
// wrongly denied (fail-closed functional regression). This test drives the manage
// tool with the bare case number and asserts the in-scope update SUCCEEDS.
//
// It fails on the current _in-only code and passes once the manage path tolerates
// bare ids again.
func TestManageScopeUpdateAcceptsBareID(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)
	authContext := testutils.GetAuthContext(t)

	// In-scope (tlp=2). Number is the bare case number the LLM may legitimately
	// supply; UnderscoreId is the ~-prefixed internal id.
	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Bare-id in scope case", 2)
	bareID := strconv.Itoa(int(inScopeCase.Number))
	require.NotEqual(t, inScopeCase.UnderscoreId, bareID,
		"the bare number must differ from the ~-prefixed _id, or the test proves nothing")

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{bareID},
				testArgEntityData: map[string]any{testFieldTitle: "Title changed via bare id"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.False(t, result.IsError,
		"FALSE DENIAL: an in-scope case addressed by its bare number was wrongly denied — "+
			"the _in{_id} scope check does not match bare ids, only ~-prefixed ones")

	fetchedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, inScopeCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Title changed via bare id", fetchedCase.Title,
		"the update addressed by bare id must have been applied")
}
