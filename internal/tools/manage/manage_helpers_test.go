package manage_test

import (
	"fmt"
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

// createCases creates count cases titled via titleFmt (given the 1-based index)
// and returns their IDs. Used by the batch merge tests.
func createCases(t *testing.T, hiveClient *thehive.APIClient, count int, titleFmt string) []string {
	t.Helper()

	caseIDs := make([]string, 0, count)

	for i := 1; i <= count; i++ {
		createdCase := createCase(t, hiveClient, fmt.Sprintf(titleFmt, i))
		caseIDs = append(caseIDs, createdCase.UnderscoreId)
	}

	return caseIDs
}

// createPageInCase creates a case titled caseTitle and a page inside it via the
// raw TheHive API, asserting both succeeded and returning the created page.
func createPageInCase(t *testing.T, hiveClient *thehive.APIClient, caseTitle string, page thehive.InputCreatePage) *thehive.OutputPage {
	t.Helper()

	createdCase := createCase(t, hiveClient, caseTitle)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdPage, _, err := hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(page).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdPage)

	return createdPage
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
