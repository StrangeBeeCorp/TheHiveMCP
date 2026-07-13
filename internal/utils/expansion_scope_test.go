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

// fakeHiveServer replaces a real TheHive (no Docker). Every SDK call hits POST
// /api/v1/query; handle routes on the operation names in the body. scopeQueries
// counts scope queries (round-trips) so tests can assert hits were re-scoped and
// that a batch collapses to one query.
type fakeHiveServer struct {
	similarCases []map[string]any
	// Per-parent similar cases (keyed by parent idOrName); falls back to similarCases when nil.
	similarCasesByParent map[string][]map[string]any
	scopeIDs             map[string]bool

	// Concurrent handler goroutines update these: scopeQueries atomic; mu guards
	// scopeCheckedIDs.
	scopeQueries atomic.Int64
	mu           sync.Mutex
	// Per-id scope-check count, for asserting cross-parent dedup (a hit two parents
	// return is checked once).
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
	// Scope check: one list query per type with an _in over the _id set (parent
	// verification and the batched per-hit re-check both take this path). Each hit
	// counts as one check; a batch of N ids is a single query. Routes both listAlert
	// (parent) and listCase (case hits).
	case (names[0] == opListAlert || names[0] == opListCase) && slices.Contains(names, "filter"):
		ids := scopeInValues(parsed.Query)
		f.recordScopeCheck(ids)
		f.encodeScopeRows(w, f.inScopeSubset(ids))

	// The MCP sends the *Light op; its MCP-facing query name stays "similarCases".
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

// scopeInValues extracts the _in._values ids from a scope filter operation shaped
// filter(_and[ permFilters, _in{_field:_id, _values:[...]} ]).
func scopeInValues(query []map[string]any) []string {
	for _, op := range query {
		if op[opNameKey] != opFilter {
			continue
		}

		and, _ := op[opAnd].([]any)
		if ids := inValuesFromAnd(and); ids != nil {
			return ids
		}
	}

	return nil
}

// inValuesFromAnd returns the string _in._values from the first _and clause that
// carries one, or nil.
func inValuesFromAnd(and []any) []string {
	for _, clause := range and {
		clauseMap, _ := clause.(map[string]any)
		in, _ := clauseMap[opIn].(map[string]any)

		values, ok := in[keyValues].([]any)
		if !ok {
			continue
		}

		ids := make([]string, 0, len(values))
		for _, v := range values {
			if s, ok := v.(string); ok {
				ids = append(ids, s)
			}
		}

		return ids
	}

	return nil
}

// recordScopeCheck counts one scope query (round-trip) and, per id in that
// query's _in set, one per-id check — so scopeQueries collapses a batch to 1
// while scopeCheckedIDs still proves cross-parent dedup (a shared hit lands in a
// single query, counted once).
func (f *fakeHiveServer) recordScopeCheck(ids []string) {
	f.scopeQueries.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.scopeCheckedIDs != nil {
		for _, id := range ids {
			f.scopeCheckedIDs[id]++
		}
	}
}

func (f *fakeHiveServer) similarCasesFor(parentID string) []map[string]any {
	if f.similarCasesByParent != nil {
		return f.similarCasesByParent[parentID]
	}

	return f.similarCases
}

func (f *fakeHiveServer) inScopeSubset(ids []string) []string {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if f.scopeIDs[id] {
			kept = append(kept, id)
		}
	}

	return kept
}

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

// startFakeHive returns a context carrying a client wired to the fake server.
func startFakeHive(t *testing.T, fake *fakeHiveServer) context.Context {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(srv.Close)
	client := newFakeHiveClient(t, srv)

	return context.WithValue(context.Background(), types.HiveClientCtxKey, client)
}

// tlpLTE2Filters is the analyst permission filter shared by every scope test.
func tlpLTE2Filters() map[string]any {
	return map[string]any{
		opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2},
	}
}

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

// Permission-bypass regression: an analyst (tlp<=2) expands an in-scope alert with
// similarCases; the engine returns a TLP:AMBER (in scope) and a TLP:RED (out of
// scope) case. The RED case must not surface — a direct search would never return it.
func TestExpandSimilarityHitsAreScoped(t *testing.T) {
	t.Parallel()

	const (
		inScopeCaseID = "~1000" // TLP:AMBER, analyst may see it
		outOfScopeID  = "~2000" // TLP:RED, analyst may NOT see it
		parentAlertID = "~500"
	)

	fake := &fakeHiveServer{
		// Both cases, flat like the *Light op (no "case" wrapper).
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
		scopeIDs: map[string]bool{
			inScopeCaseID: true,
			parentAlertID: true,
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

	// 2 scope queries: 1 parent + 1 batched hit re-check (both hits collapse into a
	// single listCase _in query, DL-5764). No re-scoping would fire only the parent
	// (count 1).
	require.Equal(t, int64(2), fake.scopeQueries.Load(),
		"the similarity hits must be re-scoped in one batched query (1 parent + 1 hit batch), and the RED hit must not leak")
}

// Scope checks are batched ACROSS parents, not once per parent (DL-5764, Finding 2).
// Two parents share one similar case; the cross-parent batch collects hit _ids into
// one set per type, so the shared case is scope-checked EXACTLY ONCE (twice under the
// old per-parent code). That dedup is the load-bearing assertion — both paths produce
// the same surfaced result, so only the check count distinguishes them.
func TestExpandSimilarityHitsScopedAcrossParentsAreBatched(t *testing.T) {
	t.Parallel()

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

	// 2 queries total: 1 parent query (both parents collapse into one listAlert _in
	// batch) + 1 hit query (the 3 distinct case hits collapse into one listCase _in
	// batch). The load-bearing assertion above is scopeCheckedIDs[sharedCaseID]==1;
	// this count guards that neither parents nor hits fan back out per item.
	require.Equal(t, int64(2), fake.scopeQueries.Load(),
		"scope checks must be batched: 1 parent batch + 1 hit batch, not fanned out per parent/hit")
}

// Re-scoping keys off the descriptor's ResultsAreIndependent, NOT a hardcoded set of
// similarity names (Finding 6: that set would let a future independent query like
// linkedCases silently bypass the re-check and leak). A synthetic independent query
// with a non-similar* name gets its out-of-scope hit dropped, purely because it set
// ResultsAreIndependent: true.
// Not parallel: mutates the package-global queryRegistry (restored via defer),
// which would race sibling parallel tests. Kept serial deliberately.
//
//nolint:paralleltest // mutates the global queryRegistry
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

	// Synthetic independent, non-similarity descriptor: Func returns canned hits with
	// no network call; per-hit scope checks still hit the fake server. Restored via defer.
	queryRegistry[types.EntityTypeAlert][queryName] = QueryDescriptor{
		Func: func(_ context.Context, _ *thehive.APIClient, _ string) ([]map[string]any, error) {
			// Fresh copy so the test's input is not mutated by projection.
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

	// 1 parent query + 1 batched hit query, identical to the similarity path —
	// proving the re-scope is driven by ResultsAreIndependent, not the query name.
	require.Equal(t, int64(2), fake.scopeQueries.Load(),
		"an independent query must be re-scoped regardless of its name")
}

// Companion to the test above: a query declared NOT independent (a child of an
// already-scoped parent) fires ONLY the parent scope check, never a per-hit re-check.
//
// Not parallel: mutates the package-global queryRegistry (restored via defer),
// which would race sibling parallel tests. Kept serial deliberately.
//
//nolint:paralleltest // mutates the global queryRegistry
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
