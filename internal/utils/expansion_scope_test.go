package utils

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
)

// fakeHiveServer replaces a real TheHive (no Docker). Every SDK call hits POST
// /api/v1/query; handle routes on the operation names in the body. scopeQueries
// counts scope checks so tests can assert hits were re-scoped.
type fakeHiveServer struct {
	similarCases []map[string]interface{}
	// Per-parent similar cases (keyed by parent idOrName); falls back to similarCases when nil.
	similarCasesByParent map[string][]map[string]interface{}
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
		Query []map[string]interface{} `json:"query"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	names := operationNames(parsed.Query)
	w.Header().Set("Content-Type", "application/json")

	switch {
	// Parent scope check (GetEntityIDsInScope).
	case names[0] == "getAlert" && slices.Contains(names, "filter"):
		idOrName, _ := parsed.Query[0]["idOrName"].(string)
		f.recordScopeCheck(idOrName)
		f.encodeScopeRows(w, f.inScopeSubset([]string{idOrName}))

	// Per-hit scope check, fanned out concurrently one per hit (a single listCase
	// _id-filter query was unreliable on real TheHive) — tests see one query per hit.
	case names[0] == "getCase" && slices.Contains(names, "filter"):
		idOrName, _ := parsed.Query[0]["idOrName"].(string)
		f.recordScopeCheck(idOrName)
		f.encodeScopeRows(w, f.inScopeSubset([]string{idOrName}))

	// The MCP sends the *Light op; its MCP-facing query name stays "similarCases".
	case names[0] == "getAlert" && slices.Contains(names, "similarCasesLight"):
		idOrName, _ := parsed.Query[0]["idOrName"].(string)
		_ = json.NewEncoder(w).Encode(f.similarCasesFor(idOrName))

	default:
		http.Error(w, "unexpected query: "+strings.Join(names, ","), http.StatusInternalServerError)
	}
}

func (f *fakeHiveServer) recordScopeCheck(id string) {
	f.scopeQueries.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scopeCheckedIDs != nil {
		f.scopeCheckedIDs[id]++
	}
}

func (f *fakeHiveServer) similarCasesFor(parentID string) []map[string]interface{} {
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
	rows := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, map[string]interface{}{"_id": id})
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func operationNames(query []map[string]interface{}) []string {
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
func tlpLTE2Filters() map[string]interface{} {
	return map[string]interface{}{
		"_lte": map[string]interface{}{"_field": "tlp", "_value": 2},
	}
}

func hitIDs(t *testing.T, entity map[string]interface{}, queryName string) []string {
	t.Helper()
	hits, ok := entity[queryName].([]map[string]interface{})
	require.True(t, ok, "%s must be present on the expanded entity", queryName)
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		if id, ok := h["_id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// Permission-bypass regression: an analyst (tlp<=2) expands an in-scope alert with
// similarCases; the engine returns a TLP:AMBER (in scope) and a TLP:RED (out of
// scope) case. The RED case must not surface — a direct search would never return it.
func TestExpandSimilarityHitsAreScoped(t *testing.T) {
	const (
		inScopeCaseID = "~1000" // TLP:AMBER, analyst may see it
		outOfScopeID  = "~2000" // TLP:RED, analyst may NOT see it
		parentAlertID = "~500"
	)

	fake := &fakeHiveServer{
		// Both cases, flat like the *Light op (no "case" wrapper).
		similarCases: []map[string]interface{}{
			{
				"_id":                    inScopeCaseID,
				"title":                  "In scope case",
				"tlp":                    2,
				"similarObservableCount": 3,
			},
			{
				"_id":                    outOfScopeID,
				"title":                  "Secret RED case",
				"tlp":                    3,
				"similarObservableCount": 1,
			},
		},
		scopeIDs: map[string]bool{
			inScopeCaseID: true,
			parentAlertID: true,
		},
	}

	ctx := startFakeHive(t, fake)

	entities := []map[string]interface{}{{"_id": parentAlertID}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{"similarCases"}, tlpLTE2Filters())
	require.NoError(t, err)
	require.Len(t, expanded, 1)

	gotIDs := hitIDs(t, expanded[0], "similarCases")

	require.Contains(t, gotIDs, inScopeCaseID, "the in-scope (TLP:AMBER) case must be returned")
	require.NotContains(t, gotIDs, outOfScopeID,
		"PERMISSION BYPASS: the out-of-scope (TLP:RED) similar case leaked through expansion")

	// 3 scope queries: 1 parent + 1 per hit. DL-5764's constraint is kept as bounded
	// concurrency (overlapping checks, ~one round-trip), not a single unreliable
	// listCase _id-filter query. No re-scoping would fire only the parent (count 1).
	require.Equal(t, int64(3), fake.scopeQueries.Load(),
		"each similarity hit must be re-scoped (1 parent + 2 hits), and the RED hit must not leak")
}

// Scope checks are batched ACROSS parents, not once per parent (DL-5764, Finding 2).
// Two parents share one similar case; the cross-parent batch collects hit _ids into
// one set per type, so the shared case is scope-checked EXACTLY ONCE (twice under the
// old per-parent code). That dedup is the load-bearing assertion — both paths produce
// the same surfaced result, so only the check count distinguishes them.
func TestExpandSimilarityHitsScopedAcrossParentsAreBatched(t *testing.T) {
	const (
		parentA      = "~500"
		parentB      = "~501"
		sharedCaseID = "~1000" // returned by BOTH parents, in scope
		onlyBCaseID  = "~1001" // returned only by parent B, in scope
		outOfScopeID = "~2000" // TLP:RED, returned by parent A, NOT in scope
	)

	fake := &fakeHiveServer{
		similarCasesByParent: map[string][]map[string]interface{}{
			parentA: {
				{"_id": sharedCaseID, "title": "Shared case", "tlp": 2, "similarObservableCount": 3},
				{"_id": outOfScopeID, "title": "Secret RED case", "tlp": 3, "similarObservableCount": 1},
			},
			parentB: {
				{"_id": sharedCaseID, "title": "Shared case", "tlp": 2, "similarObservableCount": 3},
				{"_id": onlyBCaseID, "title": "Only B case", "tlp": 1, "similarObservableCount": 2},
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

	entities := []map[string]interface{}{{"_id": parentA}, {"_id": parentB}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{"similarCases"}, tlpLTE2Filters())
	require.NoError(t, err)
	require.Len(t, expanded, 2)

	aIDs := hitIDs(t, expanded[0], "similarCases")
	require.Contains(t, aIDs, sharedCaseID, "parent A must keep the shared in-scope case")
	require.NotContains(t, aIDs, outOfScopeID,
		"PERMISSION BYPASS: the out-of-scope (TLP:RED) case leaked through parent A's expansion")

	bIDs := hitIDs(t, expanded[1], "similarCases")
	require.Contains(t, bIDs, sharedCaseID, "parent B must keep the shared in-scope case")
	require.Contains(t, bIDs, onlyBCaseID, "parent B must keep its own in-scope case")

	// Load-bearing cross-parent assertion: the shared case is scope-checked
	// exactly ONCE across both parents. Per-parent batching would check it twice.
	require.Equal(t, 1, fake.scopeCheckedIDs[sharedCaseID],
		"a hit shared by two parents must be scope-checked once, not once per parent")

	// 2 parent checks + 3 distinct case checks = 5. Per-parent would check the
	// shared case twice, totalling 6.
	require.Equal(t, int64(5), fake.scopeQueries.Load(),
		"scope checks must be batched across parents: 2 parents + 3 distinct hits, not 6")
}

// Re-scoping keys off the descriptor's ResultsAreIndependent, NOT a hardcoded set of
// similarity names (Finding 6: that set would let a future independent query like
// linkedCases silently bypass the re-check and leak). A synthetic independent query
// with a non-similar* name gets its out-of-scope hit dropped, purely because it set
// ResultsAreIndependent: true.
func TestExpandIndependentNonSimilarityQueryIsScoped(t *testing.T) {
	const (
		inScopeCaseID = "~1000"
		outOfScopeID  = "~2000"
		parentAlertID = "~500"
		queryName     = "linkedCasesTest" // deliberately NOT a similar* name
	)

	cannedHits := []map[string]interface{}{
		{"_id": inScopeCaseID, "title": "In scope linked case"},
		{"_id": outOfScopeID, "title": "Out of scope linked case"},
	}

	// Synthetic independent, non-similarity descriptor: Func returns canned hits with
	// no network call; per-hit scope checks still hit the fake server. Restored via defer.
	queryRegistry[types.EntityTypeAlert][queryName] = QueryDescriptor{
		Func: func(_ context.Context, _ *thehive.APIClient, _ string) ([]map[string]interface{}, error) {
			// Fresh copy so the test's input is not mutated by projection.
			out := make([]map[string]interface{}, len(cannedHits))
			for i, h := range cannedHits {
				cp := make(map[string]interface{}, len(h))
				for k, v := range h {
					cp[k] = v
				}
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

	entities := []map[string]interface{}{{"_id": parentAlertID}}

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

// Companion to the test above: a query declared NOT independent (a child of an
// already-scoped parent) fires ONLY the parent scope check, never a per-hit re-check.
func TestExpandChildQueryIsNotRescoped(t *testing.T) {
	const (
		childID       = "~3000"
		parentAlertID = "~500"
		queryName     = "childObservablesTest"
	)

	queryRegistry[types.EntityTypeAlert][queryName] = QueryDescriptor{
		Func: func(_ context.Context, _ *thehive.APIClient, _ string) ([]map[string]interface{}, error) {
			return []map[string]interface{}{{"_id": childID, "dataType": "ip"}}, nil
		},
		EntityType:            types.EntityTypeObservable,
		ResultsAreIndependent: false,
	}
	defer delete(queryRegistry[types.EntityTypeAlert], queryName)

	fake := &fakeHiveServer{
		scopeIDs: map[string]bool{parentAlertID: true},
	}
	ctx := startFakeHive(t, fake)

	entities := []map[string]interface{}{{"_id": parentAlertID}}

	expanded, err := ExpandEntitiesWithQueries(ctx, types.EntityTypeAlert,
		entities, []string{queryName}, tlpLTE2Filters())
	require.NoError(t, err)

	require.Len(t, hitIDs(t, expanded[0], queryName), 1,
		"child results pass through untouched, not re-scoped away")

	require.Equal(t, int64(1), fake.scopeQueries.Load(),
		"a child query must fire only the parent scope check, never a per-hit re-check")
}
