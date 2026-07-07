package manage_test

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// DL-5764: GetScopedEntityIDsBatch fans out per-hit get-by-ID checks rather than
// one list query with _or of _id filters — _id equality is unsupported on list for
// some types (see GetEntityIDsInScope). On a TLP:RED path both failure modes are
// catastrophic: false denial (in-scope hit dropped) and leak (out-of-scope hit
// marked in-scope). Cross-checks against the proven get-by-ID path.
func TestGetScopedEntityIDsBatchHonorsIDFilter(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authCtx := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	ctx := context.WithValue(authCtx, types.HiveClientCtxKey, hiveClient)

	// Mirrors analyst.yaml (tlp<=2): tlp=3 (TLP:RED) is out of scope.
	permFilters := map[string]any{
		"_lte": map[string]any{"_field": testFieldTLP, "_value": 2},
	}

	// GetScopedEntityIDsBatch is only ever called with case (similarCases) and
	// alert (similarAlerts), so both getCase and getAlert must be proven.
	t.Run("case", func(t *testing.T) {
		ids := createScopedCases(authCtx, t, hiveClient)
		assertBatchHonorsIDFilter(ctx, t, types.EntityTypeCase, ids, permFilters)
	})

	t.Run("alert", func(t *testing.T) {
		ids := createScopedAlerts(authCtx, t, hiveClient)
		assertBatchHonorsIDFilter(ctx, t, types.EntityTypeAlert, ids, permFilters)
	})
}

type scopedIDs struct {
	inScope  string // tlp=2, passes permFilters
	outScope string // tlp=3 (TLP:RED), fails permFilters — the leak canary
	decoy    string // tlp=2, in scope but NOT passed to the batch — over-match canary
}

func createScopedCases(authCtx context.Context, t *testing.T, c *thehive.APIClient) scopedIDs {
	t.Helper()

	mk := func(title string, tlp int32) string {
		in := testutils.MockInputCase()
		in.Title = title
		in.Tlp = new(tlp)
		in.Tasks = nil
		created, _, err := c.CaseAPI.CreateCase(authCtx).InputCreateCase(*in).Execute()
		require.NoError(t, err)
		require.NotNil(t, created)

		return created.UnderscoreId
	}

	return scopedIDs{
		inScope:  mk("Batch scope in-scope case", 2),
		outScope: mk("Batch scope TLP:RED case", 3),
		decoy:    mk("Batch scope decoy case", 2),
	}
}

func createScopedAlerts(authCtx context.Context, t *testing.T, c *thehive.APIClient) scopedIDs {
	t.Helper()

	mk := func(title, sourceRef string, tlp int32) string {
		in := testutils.MockInputAlert()
		in.Title = title
		in.SourceRef = sourceRef
		in.Tlp = new(tlp)
		created, _, err := c.AlertAPI.CreateAlert(authCtx).InputCreateAlert(*in).Execute()
		require.NoError(t, err)
		require.NotNil(t, created)

		return created.UnderscoreId
	}

	return scopedIDs{
		inScope:  mk("Batch scope in-scope alert", "batch-scope-in", 2),
		outScope: mk("Batch scope TLP:RED alert", "batch-scope-red", 3),
		decoy:    mk("Batch scope decoy alert", "batch-scope-decoy", 2),
	}
}

func assertBatchHonorsIDFilter(
	ctx context.Context,
	t *testing.T,
	entityType string,
	ids scopedIDs,
	permFilters map[string]any,
) {
	t.Helper()

	// Sanity floor: nil filters short-circuit to in-scope, proving the IDs are
	// well-formed for this TheHive version.
	noFilter, err := utils.GetScopedEntityIDsBatch(ctx, entityType, []string{ids.inScope}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{ids.inScope: true}, noFilter)

	// decoy is deliberately NOT requested: if it comes back in scope, the batch
	// scoped an entity it was never asked about (over-match -> leak class).
	requested := []string{ids.inScope, ids.outScope}
	inScope, err := utils.GetScopedEntityIDsBatch(ctx, entityType, requested, permFilters)
	require.NoError(t, err)

	require.True(t, inScope[ids.inScope],
		"FALSE DENIAL: in-scope (tlp=2) hit dropped — the get-by-ID scope check (get%s -> filter(permFilters)) returned nothing for an entity that passes permFilters",
		entityType)

	require.False(t, inScope[ids.outScope],
		"LEAK / PERMISSION BYPASS: out-of-scope (TLP:RED, tlp=3) hit marked in-scope — the get%s scope check did not apply permFilters to the fetched entity",
		entityType)

	require.NotContains(t, inScope, ids.decoy,
		"LEAK (over-match): decoy (tlp=2, never requested) appeared — the batch returned an entity it was never asked to scope-check for %s",
		entityType)

	// Differential oracle: batch must agree with the proven get-by-ID path on the
	// same inputs; `want` pins the correct answer from the trusted path.
	want, err := utils.GetEntityIDsInScope(ctx, entityType, requested, permFilters)
	require.NoError(t, err)
	require.Equal(t, want, inScope,
		"batch get-by-ID result must match the proven get-by-ID result for %s", entityType)
}
