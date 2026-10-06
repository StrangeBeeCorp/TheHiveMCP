package manage_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/manage"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// Every manage-entities operation puts entity-ids and target-id into a TheHive
// URL path, so an ID that could leave its path segment is refused before any
// call. IDs can also be names (a case template), so only the traversal
// shapes are refused, not everything but "~" + digits.
func TestManageRefusesIDsThatLeaveTheirPathSegment(t *testing.T) {
	t.Parallel()

	tool := manage.NewManageTool()
	deleteCase := func(ids ...string) error {
		return tool.ValidateParams(&manage.EntityParams{Operation: manage.OperationDelete, EntityType: types.EntityTypeCase, EntityIDs: ids})
	}
	applyTemplate := func(target string) error {
		return tool.ValidateParams(&manage.EntityParams{
			Operation: manage.OperationApplyTemplate, EntityType: types.EntityTypeCase, EntityIDs: []string{"~1"}, TargetID: target,
		})
	}

	mergeIntoCase := func(target string) error {
		return tool.ValidateParams(&manage.EntityParams{
			Operation: manage.OperationMerge, EntityType: types.EntityTypeAlert, EntityIDs: []string{"~1"}, TargetID: target,
		})
	}

	require.NoError(t, deleteCase("~123", "456"))
	require.NoError(t, mergeIntoCase("~42"))
	// apply-template sends the case IDs and the template name in its body, so
	// a template name may hold a slash.
	require.NoError(t, applyTemplate("Phishing Investigation"), "a template name with a space stays accepted")
	require.NoError(t, applyTemplate("IR/Phishing"), "a template name with a slash stays accepted")

	for _, bad := range []string{"", "..", ".", "~1/../../user", `~1\..\user`, "~1\n"} {
		require.ErrorContains(t, deleteCase("~123", bad), "invalid entity-ids entry", "%q", bad)
	}

	for _, bad := range []string{"..", "~1/../../user", `~1\..\user`} {
		require.ErrorContains(t, mergeIntoCase(bad), "invalid target-id", "%q", bad)
	}
}
