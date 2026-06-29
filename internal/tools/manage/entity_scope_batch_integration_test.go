package manage_test

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
)

// TestGetScopedEntityIDsBatchHonorsIDFilter is the empirical proof that the
// DL-5764 batch scope check is sound against a REAL TheHive instance.
//
// GetScopedEntityIDsBatch re-scopes similarity hits by fanning out one
// concurrent get-by-ID check per hit, applying the MCP permission filters to
// each fetched entity:
//
//	getCase / getAlert -> filter(permFilters)   // one per hit, concurrently
//
// It deliberately does NOT use a single list query that combines the IDs with
// an _or of _id equality filters, because (as GetEntityIDsInScope documents)
// "TheHive does not support _id equality filters on list operations for every
// entity type" — that shortcut proved unreliable against a real instance. The
// only prior coverage was a fake server echoing IDs back, so nothing proved the
// concurrent batch path agrees with the proven get-by-ID path against real
// TheHive. Two failure modes are both catastrophic for a TLP:RED data-leak path:
//
//   - FALSE DENIAL: a get-by-ID check wrongly returns nothing for an in-scope
//     hit, silently dropping every in-scope similarity hit.
//   - LEAK: the scope check fails to apply permFilters and returns everything,
//     marking out-of-scope entities in scope.
//
// This test distinguishes correct filtering, false denial, and over-match
// (leak), and cross-checks the batch result against the proven get-by-ID path.
func TestGetScopedEntityIDsBatchHonorsIDFilter(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authCtx := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	ctx := context.WithValue(authCtx, types.HiveClientCtxKey, hiveClient)

	// permFilters mirrors a representative production filter (analyst.yaml uses
	// tlp<=2). An entity with tlp=3 (TLP:RED) is out of scope.
	permFilters := map[string]interface{}{
		"_lte": map[string]interface{}{"_field": "tlp", "_value": 2},
	}

	// GetScopedEntityIDsBatch is only ever called with case (similarCases) and
	// alert (similarAlerts), so both getCase and getAlert must be proven.
	t.Run("case", func(t *testing.T) {
		ids := createScopedCases(t, hiveClient, authCtx)
		assertBatchHonorsIDFilter(t, ctx, types.EntityTypeCase, ids, permFilters)
	})

	t.Run("alert", func(t *testing.T) {
		ids := createScopedAlerts(t, hiveClient, authCtx)
		assertBatchHonorsIDFilter(t, ctx, types.EntityTypeAlert, ids, permFilters)
	})
}

// scopedIDs holds three entity IDs with known scope membership.
type scopedIDs struct {
	inScope  string // tlp=2, passes permFilters
	outScope string // tlp=3 (TLP:RED), fails permFilters — the leak canary
	decoy    string // tlp=2, in scope but NOT passed to the batch — over-match canary
}

// createScopedCases creates three cases with the TLPs scopedIDs documents.
func createScopedCases(t *testing.T, c *thehive.APIClient, authCtx context.Context) scopedIDs {
	t.Helper()
	mk := func(title string, tlp int32) string {
		in := testutils.MockInputCase()
		in.Title = title
		in.Tlp = thehive.PtrInt32(tlp)
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

// createScopedAlerts creates three alerts with the TLPs scopedIDs documents.
func createScopedAlerts(t *testing.T, c *thehive.APIClient, authCtx context.Context) scopedIDs {
	t.Helper()
	mk := func(title, sourceRef string, tlp int32) string {
		in := testutils.MockInputAlert()
		in.Title = title
		in.SourceRef = sourceRef
		in.Tlp = thehive.PtrInt32(tlp)
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

// assertBatchHonorsIDFilter drives GetScopedEntityIDsBatch against real TheHive
// and asserts it filters correctly, naming whichever failure mode it catches.
func assertBatchHonorsIDFilter(
	t *testing.T,
	ctx context.Context,
	entityType string,
	ids scopedIDs,
	permFilters map[string]interface{},
) {
	t.Helper()

	// Sanity floor: with no filters every requested ID is trivially in scope and
	// no query is made. Proves the IDs are well-formed for this TheHive version
	// and the short-circuit path works.
	noFilter, err := utils.GetScopedEntityIDsBatch(ctx, entityType, []string{ids.inScope}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{ids.inScope: true}, noFilter)

	// The call under test. decoy is deliberately NOT requested: if it comes back
	// in scope, the batch scoped an entity it was never asked about (over-match
	// -> leak class).
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

	// Differential oracle: the batch path must agree with the proven get-by-ID
	// path on the exact same inputs. If they disagree on real TheHive, the batch
	// path is unsound — and `want` pins the correct answer from the trusted path
	// regardless of how the batch path failed.
	want, err := utils.GetEntityIDsInScope(ctx, entityType, requested, permFilters)
	require.NoError(t, err)
	require.Equal(t, want, inScope,
		"batch get-by-ID result must match the proven get-by-ID result for %s", entityType)
}
