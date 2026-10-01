package bench_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/bench"
)

const (
	call = "BenchmarkSearchEntitiesCall"
	wrap = "BenchmarkWrapSearchPage"
	runs = `goos: linux
BenchmarkSearchEntitiesCall-15    	      78	   4811331 ns/op	 2360418 B/op	   20384 allocs/op
BenchmarkSearchEntitiesCall-15    	      81	   4773148 ns/op	 2373763 B/op	   20376 allocs/op
BenchmarkSearchEntitiesCall-15    	      91	   5208072 ns/op	 2379229 B/op	   20376 allocs/op
BenchmarkWrapSearchPage-15      	    2356	    185904 ns/op	  148166 B/op	    3403 allocs/op
BenchmarkWrapSearchPage-15      	    1928	    178911 ns/op	  148146 B/op	    3403 allocs/op
BenchmarkNoMem      	    1000	    10 ns/op
PASS
ok  	example.com/m/bootstrap	1.996s
`
)

var budgets = bench.Budgets{Packages: []string{"./x"}, Count: 5, Benchtime: "300ms", Slower: 1.5, MoreAllocs: 0.05}

func TestBenchmarkRunsAreReadAsMedians(t *testing.T) {
	t.Parallel()

	results := bench.Parse(runs)

	assert.Equal(t, bench.Result{Runs: 3, Ns: 4811331, Bytes: 2373763, Allocs: 20376}, results[call])
	assert.Equal(t, bench.Result{Runs: 2, Ns: 182407.5, Bytes: 148156, Allocs: 3403}, results[wrap], "an even count: the mean of the middle two")
	assert.Equal(t, bench.Result{Runs: 1, Ns: 10}, results["BenchmarkNoMem"])
	assert.Len(t, results, 3)
}

func TestAHeadSlowerOrAllocatingMoreThanTheBaseFails(t *testing.T) {
	t.Parallel()

	base := map[string]bench.Result{wrap: {Ns: 180000, Allocs: 3403}, call: {Ns: 4800000, Allocs: 20376}}

	same := map[string]bench.Result{wrap: {Ns: 250000, Allocs: 3403}, call: {Ns: 4900000, Allocs: 20390}}
	assert.Empty(t, bench.Compare(base, same, budgets), "noise within the budgets")

	worse := map[string]bench.Result{wrap: {Ns: 300000, Allocs: 3800}, call: {Ns: 4800000, Allocs: 20376}}
	problems := bench.Compare(base, worse, budgets)
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0], wrap+": 300000 ns/op, 1.67x the base's 180000 (budget 1.50x)")
	assert.Contains(t, problems[1], wrap+": allocates 3800 times per op, the base 3403 (budget +170)")

	assert.Empty(t, bench.Compare(nil, worse, budgets), "no base, no budget: nothing to hold it to")
}

func TestAbsoluteBudgetsHoldWithoutABase(t *testing.T) {
	t.Parallel()

	b := budgets
	b.Max = map[string]bench.Max{call: {Ns: 20000000, Allocs: 22500}, "BenchmarkGone": {Ns: 1}}

	problems := bench.Compare(nil, map[string]bench.Result{call: {Ns: 25000000, Allocs: 23000}}, b)
	require.Len(t, problems, 3)
	assert.Contains(t, problems[0], "BenchmarkGone: has a budget in gates/bench.yaml but did not run")
	assert.Contains(t, problems[1], call+": 25000000 ns/op, over its budget of 20000000")
	assert.Contains(t, problems[2], call+": allocates 23000 times per op, over its budget of 22500")
}

func TestTheRepositoryBudgetsLoad(t *testing.T) {
	t.Parallel()

	b, err := bench.Load(filepath.Join("..", "..", "..", "gates", "bench.yaml"))
	require.NoError(t, err)
	assert.Contains(t, b.Args(), "-benchmem")
	assert.Contains(t, b.Max, call, "the time the server adds to a TheHive call has a budget")

	bad := filepath.Join(t.TempDir(), "bench.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("packages: [./x]\ncount: 0\n"), 0o600))

	_, err = bench.Load(bad)
	require.ErrorIs(t, err, bench.ErrBudgets)
}

func TestTheTableShowsTheBaseWhenItHasTheBenchmark(t *testing.T) {
	t.Parallel()

	rows := bench.Table(map[string]bench.Result{wrap: {Ns: 100, Allocs: 3}}, map[string]bench.Result{wrap: {Ns: 150, Allocs: 3}, call: {Ns: 1}})
	require.Len(t, rows, 2)
	assert.NotContains(t, rows[0], "base")
	assert.Contains(t, rows[1], "(1.50x)")
}
