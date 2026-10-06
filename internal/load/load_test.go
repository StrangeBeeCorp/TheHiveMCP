package load

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRun is the run of gates/load.yaml against the stub.
const stubRun = "stub"

func ms(n ...int) []time.Duration {
	d := make([]time.Duration, len(n))
	for i, v := range n {
		d[i] = time.Duration(v) * time.Millisecond
	}

	return d
}

// The percentiles are the nearest rank, whatever the calls' order.
func TestPercentilesAreTheNearestRank(t *testing.T) {
	t.Parallel()

	lat := ms(90, 10, 50, 30, 70, 20, 60, 40, 80, 100)
	assert.InDelta(t, 60.0, percentile(lat, 0.50), 0.001)
	assert.InDelta(t, 100.0, percentile(lat, 0.95), 0.001)
	assert.InDelta(t, 100.0, percentile(lat, 0.99), 0.001)
	assert.InDelta(t, 10.0, percentile(lat, 0), 0.001)
	assert.Zero(t, percentile(nil, 0.95), "no call, no latency")
	assert.Equal(t, ms(90, 10, 50, 30, 70, 20, 60, 40, 80, 100), lat, "the calls are not reordered")
}

// The memory growth is the slope after the warmup, per hour: a leak shows, a warmup does not.
func TestMemoryGrowthIsTheSlopeAfterTheWarmup(t *testing.T) {
	t.Parallel()

	// One sample a minute: a jump while warming up, then 1 MiB a minute.
	samples := []float64{10, 40, 40, 41, 42, 43, 44, 45, 46, 47}
	assert.InDelta(t, 60.0, growth(samples, time.Minute), 0.01)

	flat := []float64{10, 40, 40, 40, 40, 40, 40, 40, 40, 40}
	assert.InDelta(t, 0.0, growth(flat, time.Minute), 0.01, "a warmup is no leak")

	assert.Zero(t, growth([]float64{1, 2}, time.Minute), "too few samples to measure a slope")
	assert.InDelta(t, 60.0, growth([]float64{1, 2, 3}, time.Minute), 0.01, "three are enough")
}

// The memory is VmRSS from /proc, in MiB; anything else is no sample.
func TestTheMemoryIsReadFromProc(t *testing.T) {
	t.Parallel()

	mib, ok := parseRSS("Name:\tthehivemcp\nVmPeak:\t  999 kB\nVmRSS:\t   25600 kB\nThreads:\t9\n")
	require.True(t, ok)
	assert.InDelta(t, 25.0, mib, 0.001)

	_, ok = parseRSS("Name:\tthehivemcp\n")
	assert.False(t, ok, "a kernel thread has no VmRSS")

	_, ok = parseRSS("VmRSS:\tlots kB\n")
	assert.False(t, ok)

	_, ok = rss(-1)
	assert.False(t, ok, "no process, no sample")
}

// A result over any limit of its budget is reported, each limit once; a zero limit is not
// checked, but errors always are.
func TestABudgetReportsEachLimitExceeded(t *testing.T) {
	t.Parallel()

	r := Result{Run: stubRun, P95ms: 120, P99ms: 300, Errors: 2, FirstError: "EOF", PerSec: 90, RSSMiB: []float64{20, 140, 60}, GrowthMiBPerHour: 12}
	problems := Budget{P95ms: 100, P99ms: 250, MaxErrors: 1, MinPerSec: 100, MaxRSSMiB: 100, MaxGrowth: 10}.Check(r)
	assert.Equal(t, []string{
		"stub: p95 120.0 ms, over its budget of 100.0 ms",
		"stub: p99 300.0 ms, over its budget of 250.0 ms",
		"stub: memory 140.0 MiB, over its budget of 100.0 MiB",
		"stub: memory growth 12.0 MiB/h, over its budget of 10.0 MiB/h",
		"stub: 2 failed calls, 1 allowed (first: EOF)",
		"stub: 90 calls per second, under its budget of 100",
	}, problems)

	assert.Empty(t, Budget{P95ms: 120, P99ms: 300, MaxErrors: 2, MinPerSec: 90, MaxRSSMiB: 140, MaxGrowth: 12}.Check(r), "at the limit is within it")
	assert.Equal(t, []string{"stub: 2 failed calls, 0 allowed (first: EOF)"}, Budget{}.Check(r), "errors are checked even with no limit set")
}

// The repository's budgets load, each run with its shape, and a run is found by its name.
func TestTheRepositoryBudgetsLoad(t *testing.T) {
	t.Parallel()

	b, err := ReadBudgets(filepath.Join("..", "..", BudgetsPath))
	require.NoError(t, err)

	for _, name := range []string{stubRun, "soak", "thehive"} {
		r, err := b.Run(name)
		require.NoError(t, err, name)
		assert.Positive(t, r.Clients, name)
		assert.Positive(t, r.Budget.P95ms, name)
	}

	soak, err := b.Run("soak")
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, soak.Duration)
	assert.Positive(t, soak.Budget.MaxGrowth, "the soak holds the memory's growth")

	_, err = b.Run("nightly")
	require.ErrorIs(t, err, ErrBudgets)
}

// A run with no way to end, or an unknown field, is refused.
func TestIncompleteBudgetsAreRefused(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"no calls nor duration": "runs: {a: {clients: 1, sample: 1s, budget: {}}}\n",
		"a soak with no rate":   "runs: {a: {clients: 1, duration: 1m, sample: 1s, budget: {}}}\n",
		"no clients":            "runs: {a: {calls: 1, sample: 1s, budget: {}}}\n",
		"no sample interval":    "runs: {a: {clients: 1, calls: 1, budget: {}}}\n",
		"an unknown field":      "runs: {a: {clients: 1, calls: 1, sample: 1s, budget: {p90_ms: 1}}}\n",
	} {
		p := filepath.Join(t.TempDir(), "load.yaml")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))

		_, err := ReadBudgets(p)
		require.Error(t, err, name)
	}
}

// A result written as a baseline reads back, and the comparison gives each ratio.
func TestABaselineReadsBackAndCompares(t *testing.T) {
	t.Parallel()

	base := Result{Run: stubRun, P95ms: 50, P99ms: 60, PerSec: 700, RSSMiB: []float64{20, 25}, Recorded: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)}
	p := Baseline(t.TempDir(), stubRun)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, base.Write(p))

	got, err := ReadResult(p)
	require.NoError(t, err)
	assert.Equal(t, base, got)

	r := Result{Run: stubRun, P95ms: 100, P99ms: 60, PerSec: 350, RSSMiB: []float64{30}}
	assert.Equal(t, "stub against its baseline of 2026-10-06: p95 x2.00, p99 x1.00, throughput x0.50, memory x1.20", Compare(r, got))
	assert.Contains(t, Compare(r, Result{}), "p95 -", "no baseline value, no ratio")
}

// The stub answers the load's key only: the credential check, then a page of alerts.
func TestTheStubKnowsOneKey(t *testing.T) {
	t.Parallel()

	stub, target := NewStub()
	t.Cleanup(stub.Close)

	status := func(path, key string) int {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target.URL+path, strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+key)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		_ = resp.Body.Close()

		return resp.StatusCode
	}

	assert.Equal(t, http.StatusOK, status("/api/v1/user/current", target.Key))
	assert.Equal(t, http.StatusOK, status("/api/v1/query", target.Key))
	assert.Equal(t, http.StatusNotFound, status("/api/v1/case", target.Key))
	assert.Equal(t, http.StatusUnauthorized, status("/api/v1/query", "another-key"))
}

// A short run, end to end: the server built and run as its HTTP users do, the clients at once,
// every call answered. It builds the server, so it is not a -short test.
func TestAShortRunMeasuresTheServer(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the server: the integration suite and the load gate run it")
	}

	t.Parallel()

	bin, err := Build(t.Context(), filepath.Join("..", ".."), t.TempDir())
	require.NoError(t, err)

	stub, target := NewStub()
	t.Cleanup(stub.Close)

	var log strings.Builder

	r, err := Measure(t.Context(), "short", Config{Clients: 4, Calls: 5, Sample: 100 * time.Millisecond}, bin, target, &log)
	require.NoError(t, err, log.String())
	assert.Equal(t, 20, r.Calls)
	assert.Zero(t, r.Errors, r.FirstError)
	assert.Positive(t, r.P95ms)
	assert.NotEmpty(t, r.RSSMiB, "the server's memory is sampled")

	_, err = Measure(t.Context(), "short", Config{Clients: 1, Calls: 1, Sample: time.Second}, bin, Target{URL: target.URL, Key: "wrong"}, &log)
	require.ErrorIs(t, err, ErrNoCall, "a run where every call fails is no measurement")
}
