package manage_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/client"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

// manageStructured calls manage-entities with the given arguments, asserts the
// tool did not report an error, and returns the structured content as a map.
func manageStructured(t *testing.T, c *client.Client, args map[string]any) map[string]any {
	t.Helper()

	result := testutils.CallToolOK(t, c, testToolName, args)

	return testutils.StructuredData(t, result)
}

// createCase creates a case with the given title via the raw TheHive API and
// asserts it succeeded, returning the created case for use as a parent.
func createCase(t *testing.T, hiveClient *thehive.APIClient, title string) *thehive.OutputCase {
	t.Helper()

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = title

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	return createdCase
}

// createAlert creates an alert with the given title and source reference via
// the raw TheHive API and asserts it succeeded.
func createAlert(t *testing.T, hiveClient *thehive.APIClient, title, sourceRef string) *thehive.OutputAlert {
	t.Helper()

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testAlert := testutils.MockInputAlert()
	testAlert.Title = title
	testAlert.SourceRef = sourceRef

	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	return createdAlert
}
