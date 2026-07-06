package utils

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// fakeHiveServer stands in for a real TheHive instance so expansion scoping can
// be exercised without Docker. Every SDK call (similarity queries and scope
// checks alike) hits POST /api/v1/query; this handler routes on the operation
// names in the request body.
//
//   - getAlert + similarCasesLight -> returns the alert's similar cases, flat:
//     {fieldID: ..., fieldSimilarObservableCount: N} like the *Light ops do.
//   - getCase  + filter        -> a scope check from GetEntityIDsInScope. Matches
//     (returns one row) only when the requested case id is in scopeIDs.
//
// scopeQueries counts how many scope checks were issued, so the test can assert
// whether similarity hits were re-scoped at all.
type fakeHiveServer struct {
	similarCases []map[string]any
	// similarCasesByParent, when set, returns different similar cases per parent
	// alert (keyed by the parent's idOrName). Falls back to similarCases when nil,
	// so the single-parent test is unaffected.
	similarCasesByParent map[string][]map[string]any
	scopeIDs             map[string]bool

	// GetScopedEntityIDsBatch fans the per-hit scope checks out concurrently, so
	// several handler goroutines update the counters at once. scopeQueries is
	// atomic so the test can read it (.Load) without locking; mu guards the
	// scopeCheckedIDs map, which an atomic can't cover.
	scopeQueries atomic.Int64
	mu           sync.Mutex
	// scopeCheckedIDs counts how many times each id was scope-checked. Cross-parent
	// batching dedups an id two parents share, so a shared hit is checked exactly
	// once even when several parents return it.
	scopeCheckedIDs map[string]int
}

func (f *fakeHiveServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var parsed struct {
		Query []map[string]any `json:"query"`
	}

	err := json.Unmarshal(body, &parsed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	names := operationNames(parsed.Query)

	w.Header().Set("Content-Type", "application/json")

	switch {
	// Parent scope check: getAlert -> filter (GetEntityIDsInScope).
	case names[0] == opGetAlert && slices.Contains(names, "filter"):
		idOrName, _ := parsed.Query[0]["idOrName"].(string)
		f.recordScopeCheck(idOrName)
		f.encodeScopeRows(w, f.inScopeSubset([]string{idOrName}))

	// Per-hit scope check: getCase -> filter(perm), one per similarity hit,
	// issued by GetScopedEntityIDsBatch. The batch fans these out concurrently
	// (the old single listCase _id-filter query was unreliable on real TheHive),
	// so the test sees one scope query PER hit, not one for the whole batch.
	case names[0] == opGetCase && slices.Contains(names, "filter"):
		idOrName, _ := parsed.Query[0]["idOrName"].(string)
		f.recordScopeCheck(idOrName)
		f.encodeScopeRows(w, f.inScopeSubset([]string{idOrName}))

	// Similarity query: getAlert -> similarCasesLight (the *Light op the MCP
	// now sends; the MCP-facing query name stays "similarCases").
	case names[0] == opGetAlert && slices.Contains(names, "similarCasesLight"):
		idOrName, _ := parsed.Query[0]["idOrName"].(string)

		encErr := json.NewEncoder(w).Encode(f.similarCasesFor(idOrName))
		if encErr != nil {
			http.Error(w, encErr.Error(), http.StatusInternalServerError)
		}

	default:
		http.Error(w, "unexpected query: "+strings.Join(names, ","), http.StatusInternalServerError)
	}
}

// recordScopeCheck bumps the total scope-query count and the per-id counter from
// concurrent get-by-id scope-check handlers. scopeQueries is atomic; the
// per-id map is guarded by mu.
func (f *fakeHiveServer) recordScopeCheck(id string) {
	f.scopeQueries.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.scopeCheckedIDs != nil {
		f.scopeCheckedIDs[id]++
	}
}

// similarCasesFor returns the similar cases for a given parent alert, using the
// per-parent map when configured and falling back to the shared similarCases.
func (f *fakeHiveServer) similarCasesFor(parentID string) []map[string]any {
	if f.similarCasesByParent != nil {
		return f.similarCasesByParent[parentID]
	}

	return f.similarCases
}

// inScopeSubset keeps only the ids the server considers in scope.
func (f *fakeHiveServer) inScopeSubset(ids []string) []string {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if f.scopeIDs[id] {
			kept = append(kept, id)
		}
	}

	return kept
}

// encodeScopeRows writes the in-scope ids back in TheHive's row shape.
func (f *fakeHiveServer) encodeScopeRows(w http.ResponseWriter, ids []string) {
	rows := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, map[string]any{fieldID: id})
	}

	encErr := json.NewEncoder(w).Encode(rows)
	if encErr != nil {
		http.Error(w, encErr.Error(), http.StatusInternalServerError)
	}
}

// operationNames extracts the _name of each operation in a query pipeline.
func operationNames(query []map[string]any) []string {
	names := make([]string, 0, len(query))
	for _, op := range query {
		if name, ok := op["_name"].(string); ok {
			names = append(names, name)
		}
	}

	return names
}

func newFakeHiveClient(t *testing.T, srv *httptest.Server) *thehive.APIClient {
	t.Helper()

	cfg := thehive.NewConfiguration()
	cfg.Host = strings.TrimPrefix(srv.URL, "http://")
	cfg.Scheme = "http"

	return thehive.NewAPIClient(cfg)
}

// startFakeHive boots the fake server, registers cleanup, and returns a context
// carrying a client wired to it — the four-line setup every scope test repeats.
func startFakeHive(t *testing.T, fake *fakeHiveServer) context.Context {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(srv.Close)
	client := newFakeHiveClient(t, srv)

	return context.WithValue(context.Background(), types.HiveClientCtxKey, client)
}

// tlpLTE2Filters is the analyst permission filter (tlp <= 2) shared by every
// scope test.
func tlpLTE2Filters() map[string]any {
	return map[string]any{
		opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2},
	}
}

// hitIDs extracts the _id of each hit under queryName on an expanded entity.
func hitIDs(t *testing.T, entity map[string]any, queryName string) []string {
	t.Helper()

	hits, ok := entity[queryName].([]map[string]any)
	require.True(t, ok, "%s must be present on the expanded entity", queryName)

	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		if id, ok := h[fieldID].(string); ok {
			ids = append(ids, id)
		}
	}

	return ids
}

// TestExpandSimilarityHitsAreScoped is the permission-bypass regression test.
//
// An analyst with tlp<=2 expands an in-scope alert with similarCases. TheHive's
// similarity engine returns two independent top-level cases: one TLP:AMBER (in
// scope) and one TLP:RED (out of scope). The out-of-scope case must NOT be
// surfaced, because a direct search would never return it.
//
// With the current code the similarity hits bypass scope re-checking entirely,
// so the TLP:RED case leaks through and this test fails.
func TestExpandSimilarityHitsAreScoped(t *testing.T) {
	const (
		inScopeCaseID = "~1000" // TLP:AMBER, analyst may see it
		outOfScopeID  = "~2000" // TLP:RED, analyst may NOT see it
		parentAlertID = "~500"
	)

	fake := &fakeHiveServer{
		// What getAlert->similarCasesLight returns: both cases, flat (the Light
		// ops emit entity fields and meta side by side, with no "case" wrapper).
		similarCases: []map[string]any{
			{
				fieldID:                     inScopeCaseID,
				fieldTitle:                  "In scope case",
				fieldTLP:                    2,
				fieldSimilarObservableCount: 3,
			},
			{
				fieldID:                     outOfScopeID,
				fieldTitle:                  "Secret RED case",
				fieldTLP:                    3,
				fieldSimilarObservableCount: 1,
			},
		},
		// Only the AMBER case is within the analyst's scope.
		scopeIDs: map[string]bool{
			inScopeCaseID: true,
			parentAlertID: true, // parent alert is in scope
		},
	}

	ctx := startFakeHive(t, fake)

	entities := []map[string]any{{fieldID: parentAlertID}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{querySimilarCases}, tlpLTE2Filters())
	require.NoError(t, err)
	require.Len(t, expanded, 1)

	gotIDs := hitIDs(t, expanded[0], querySimilarCases)

	require.Contains(t, gotIDs, inScopeCaseID, "the in-scope (TLP:AMBER) case must be returned")
	require.NotContains(t, gotIDs, outOfScopeID,
		"PERMISSION BYPASS: the out-of-scope (TLP:RED) similar case leaked through expansion")

	// Exactly 3 scope queries: 1 for the parent alert, and 1 per similarity hit
	// (2 hits) — the batch fans the proven get-by-ID check out concurrently
	// rather than issuing a single (unreliable) listCase _id-filter query. The
	// DL-5764 constraint is preserved as bounded *concurrency*, not a single
	// query: the N checks overlap so latency stays ~one round-trip. If the hits
	// were not re-scoped at all, only the parent check would fire (count 1) and
	// the RED case would leak.
	require.Equal(t, int64(3), fake.scopeQueries.Load(),
		"each similarity hit must be re-scoped (1 parent + 2 hits), and the RED hit must not leak")
}

// TestExpandSimilarityHitsScopedAcrossParentsAreBatched proves the scope checks
// are batched ACROSS parents, not once per parent (DL-5764, Finding 2).
//
// Two in-scope parent alerts are each expanded with similarCases. They share one
// similar case (sharedCaseID); parent A additionally surfaces a TLP:RED case and
// parent B an extra in-scope case. The cross-parent batch collects every hit _id
// into one set per target type, so the shared case is scope-checked EXACTLY ONCE
// — under the old per-parent code it would be checked once per parent (twice).
// That dedup is the load-bearing assertion: it fails on the per-parent code even
// though both produce the same surfaced result.
func TestExpandSimilarityHitsScopedAcrossParentsAreBatched(t *testing.T) {
	const (
		parentA      = "~500"
		parentB      = "~501"
		sharedCaseID = "~1000" // returned by BOTH parents, in scope
		onlyBCaseID  = "~1001" // returned only by parent B, in scope
		outOfScopeID = "~2000" // TLP:RED, returned by parent A, NOT in scope
	)

	fake := &fakeHiveServer{
		similarCasesByParent: map[string][]map[string]any{
			parentA: {
				{fieldID: sharedCaseID, fieldTitle: "Shared case", fieldTLP: 2, fieldSimilarObservableCount: 3},
				{fieldID: outOfScopeID, fieldTitle: "Secret RED case", fieldTLP: 3, fieldSimilarObservableCount: 1},
			},
			parentB: {
				{fieldID: sharedCaseID, fieldTitle: "Shared case", fieldTLP: 2, fieldSimilarObservableCount: 3},
				{fieldID: onlyBCaseID, fieldTitle: "Only B case", fieldTLP: 1, fieldSimilarObservableCount: 2},
			},
		},
		scopeIDs: map[string]bool{
			parentA:      true,
			parentB:      true,
			sharedCaseID: true,
			onlyBCaseID:  true,
			// outOfScopeID is deliberately absent — out of scope.
		},
		scopeCheckedIDs: map[string]int{},
	}

	ctx := startFakeHive(t, fake)

	entities := []map[string]any{{fieldID: parentA}, {fieldID: parentB}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{querySimilarCases}, tlpLTE2Filters())
	require.NoError(t, err)
	require.Len(t, expanded, 2)

	aIDs := hitIDs(t, expanded[0], querySimilarCases)
	require.Contains(t, aIDs, sharedCaseID, "parent A must keep the shared in-scope case")
	require.NotContains(t, aIDs, outOfScopeID,
		"PERMISSION BYPASS: the out-of-scope (TLP:RED) case leaked through parent A's expansion")

	bIDs := hitIDs(t, expanded[1], querySimilarCases)
	require.Contains(t, bIDs, sharedCaseID, "parent B must keep the shared in-scope case")
	require.Contains(t, bIDs, onlyBCaseID, "parent B must keep its own in-scope case")

	// Load-bearing cross-parent assertion: the shared case is scope-checked
	// exactly ONCE across both parents. Per-parent batching would check it twice.
	require.Equal(t, 1, fake.scopeCheckedIDs[sharedCaseID],
		"a hit shared by two parents must be scope-checked once, not once per parent")

	// Total = 2 parent (getAlert) checks + 3 DISTINCT case (getCase) checks
	// (sharedCaseID, onlyBCaseID, outOfScopeID). The naive per-parent approach
	// would check the shared case twice, totalling 6.
	require.Equal(t, int64(5), fake.scopeQueries.Load(),
		"scope checks must be batched across parents: 2 parents + 3 distinct hits, not 6")
}

// TestExpandIndependentNonSimilarityQueryIsScoped proves that re-scoping keys off
// the descriptor's declared ResultsAreIndependent property, NOT a hardcoded set
// of similarity query names. It registers a synthetic independent query whose
// name is NOT similar* and whose results are canned (no similarity engine), then
// asserts its out-of-scope hit is dropped exactly like a similar* hit would be.
//
// This is the regression guard for the old leak path: before Finding 6, scoping
// was gated on a hardcoded similarityQueries set, so any future independent query
// (e.g. linkedCases) would silently bypass the re-check and leak. Now the gate is
// a declared property, so this synthetic query is re-scoped purely because it set
// ResultsAreIndependent: true.
func TestExpandIndependentNonSimilarityQueryIsScoped(t *testing.T) {
	const (
		inScopeCaseID = "~1000"
		outOfScopeID  = "~2000"
		parentAlertID = "~500"
		queryName     = "linkedCasesTest" // deliberately NOT a similar* name
	)

	cannedHits := []map[string]any{
		{fieldID: inScopeCaseID, fieldTitle: "In scope linked case"},
		{fieldID: outOfScopeID, fieldTitle: "Out of scope linked case"},
	}

	// Inject a synthetic INDEPENDENT, non-similarity descriptor whose Func returns
	// canned hits without any network call. The per-hit scope checks (getCase ->
	// filter) still go through the fake server. Restored via defer.
	queryRegistry[types.EntityTypeAlert][queryName] = QueryDescriptor{
		Func: func(_ context.Context, _ *thehive.APIClient, _ string) ([]map[string]any, error) {
			// Return a fresh copy so the test's input is not mutated by projection.
			out := make([]map[string]any, len(cannedHits))
			for i, h := range cannedHits {
				cp := make(map[string]any, len(h))
				maps.Copy(cp, h)

				out[i] = cp
			}

			return out, nil
		},
		EntityType:            types.EntityTypeCase,
		ResultsAreIndependent: true,
	}
	defer delete(queryRegistry[types.EntityTypeAlert], queryName)

	fake := &fakeHiveServer{
		scopeIDs: map[string]bool{
			inScopeCaseID: true,
			parentAlertID: true,
		},
	}
	ctx := startFakeHive(t, fake)

	entities := []map[string]any{{fieldID: parentAlertID}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{queryName}, tlpLTE2Filters())
	require.NoError(t, err)
	require.Len(t, expanded, 1)

	gotIDs := hitIDs(t, expanded[0], queryName)
	require.Contains(t, gotIDs, inScopeCaseID, "the in-scope hit must be returned")
	require.NotContains(t, gotIDs, outOfScopeID,
		"PERMISSION BYPASS: an independent non-similarity hit leaked through expansion")

	// 1 parent check + 1 per hit (2 hits) = 3, identical to the similarity path —
	// proving the re-scope is driven by ResultsAreIndependent, not the query name.
	require.Equal(t, int64(3), fake.scopeQueries.Load(),
		"an independent query must be re-scoped per hit regardless of its name")
}

// TestExpandChildQueryIsNotRescoped is the companion to the test above: a query
// declared NOT independent (a child of an already-scoped parent) must fire ONLY
// the single parent scope check, never a per-hit re-check.
func TestExpandChildQueryIsNotRescoped(t *testing.T) {
	const (
		childID       = "~3000"
		parentAlertID = "~500"
		queryName     = "childObservablesTest"
	)

	queryRegistry[types.EntityTypeAlert][queryName] = QueryDescriptor{
		Func: func(_ context.Context, _ *thehive.APIClient, _ string) ([]map[string]any, error) {
			return []map[string]any{{fieldID: childID, fieldDataType: "ip"}}, nil
		},
		EntityType:            types.EntityTypeObservable,
		ResultsAreIndependent: false,
	}
	defer delete(queryRegistry[types.EntityTypeAlert], queryName)

	fake := &fakeHiveServer{
		scopeIDs: map[string]bool{parentAlertID: true},
	}
	ctx := startFakeHive(t, fake)

	entities := []map[string]any{{fieldID: parentAlertID}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{queryName}, tlpLTE2Filters())
	require.NoError(t, err)

	require.Len(t, hitIDs(t, expanded[0], queryName), 1,
		"child results pass through untouched, not re-scoped away")

	require.Equal(t, int64(1), fake.scopeQueries.Load(),
		"a child query must fire only the parent scope check, never a per-hit re-check")
}
