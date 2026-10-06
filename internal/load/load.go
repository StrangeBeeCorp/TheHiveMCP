// Package load measures the HTTP transport under many clients at once: it runs the server as its
// HTTP users do, against a TheHive, and drives it with concurrent MCP clients, each with its own
// session. A load run makes a fixed number of calls as fast as the server answers; a soak run
// keeps a steady rate for a while and samples the server's memory. gates/load.yaml holds each
// run's shape and budgets, and baselines/load/ what they measured when the budgets were set.
package load

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// callTimeout bounds one tool call: a call that takes longer is an error.
	callTimeout = 30 * time.Second
	// warmup is the share of a soak run its memory growth is not measured over: the server's
	// caches and pools fill first.
	warmup = 0.2
	// minGrowthSamples is how many samples after the warmup a slope needs to mean anything.
	minGrowthSamples = 3
	// resultPerm is a result file's mode.
	resultPerm = 0o600
	// The percentiles a run reports.
	p50, p95, p99 = 0.50, 0.95, 0.99
	// kibPerMiB converts /proc's kB to MiB.
	kibPerMiB = 1024
	// The modes of a run.
	modeLoad = "load"
	modeSoak = "soak"
)

// ErrNoCall means no call of the run succeeded: there is nothing to measure.
var ErrNoCall = errors.New("no call succeeded")

// Target is the TheHive the server talks to, and what its clients send it.
type Target struct {
	URL, Key, Org string
}

// Config is the shape of one run.
type Config struct {
	Clients int `yaml:"clients"`
	// Calls is each client's number of calls, in a load run.
	Calls int `yaml:"calls"`
	// Duration makes a soak run: each client calls at Rate per second for this long.
	Duration time.Duration `yaml:"duration"`
	Rate     float64       `yaml:"rate"`
	// Sample is how often a run samples the server's memory.
	Sample time.Duration `yaml:"sample"`
}

// Result is what a run measured.
type Result struct {
	Run     string  `json:"run"`
	Mode    string  `json:"mode"`
	Clients int     `json:"clients"`
	Calls   int     `json:"calls"`
	Errors  int     `json:"errors"`
	Seconds float64 `json:"seconds"`
	PerSec  float64 `json:"calls_per_second"`
	P50ms   float64 `json:"p50_ms"`
	P95ms   float64 `json:"p95_ms"`
	P99ms   float64 `json:"p99_ms"`
	// RSSMiB is the server's resident memory at each sample.
	RSSMiB []float64 `json:"rss_mib,omitempty"`
	// GrowthMiBPerHour is the slope of the server's memory after the warmup, in a soak run.
	GrowthMiBPerHour float64   `json:"growth_mib_per_hour,omitempty"`
	FirstError       string    `json:"first_error,omitempty"`
	Recorded         time.Time `json:"recorded"`
}

// MaxRSS is the largest memory sample, in MiB.
func (r Result) MaxRSS() float64 {
	if len(r.RSSMiB) == 0 {
		return 0
	}

	return slices.Max(r.RSSMiB)
}

// Summary is the result in one line.
func (r Result) Summary() string {
	s := fmt.Sprintf("%s (%s): %d clients, %d calls in %.1f s (%.0f/s), %d errors, p50 %.1f ms, p95 %.1f ms, p99 %.1f ms",
		r.Run, r.Mode, r.Clients, r.Calls, r.Seconds, r.PerSec, r.Errors, r.P50ms, r.P95ms, r.P99ms)

	if len(r.RSSMiB) > 0 {
		s += fmt.Sprintf(", memory %.1f to %.1f MiB (at most %.1f)", r.RSSMiB[0], r.RSSMiB[len(r.RSSMiB)-1], r.MaxRSS())
	}

	if r.Mode == modeSoak {
		s += fmt.Sprintf(", growth %.1f MiB/h", r.GrowthMiBPerHour)
	}

	if r.FirstError != "" {
		s += "; first error: " + r.FirstError
	}

	return s
}

// Write saves the result as JSON.
func (r Result) Write(p string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the result: %w", err)
	}

	err = os.WriteFile(p, append(data, '\n'), resultPerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}

	return nil
}

// ReadResult reads a result, as Write saved it.
func ReadResult(p string) (Result, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- a baseline of this checkout
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", p, err)
	}

	var r Result

	err = json.Unmarshal(data, &r)
	if err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", p, err)
	}

	return r, nil
}

// Measure starts the server bin against the target, drives it as cfg says, and stops it.
func Measure(ctx context.Context, name string, cfg Config, bin string, target Target, log io.Writer) (Result, error) {
	srv, err := start(ctx, bin, target.URL)
	if err != nil {
		return Result{}, err
	}
	defer srv.stop()

	_, _ = fmt.Fprintf(log, "load: %s: the server listens on %s, TheHive at %s\n", name, srv.url, target.URL)

	r, err := drive(ctx, cfg, srv.url, srv.pid(), target)
	r.Run = name

	if err != nil {
		return r, fmt.Errorf("%w\nserver log:\n%s", err, srv.logs.String())
	}

	return r, nil
}

// tally collects the calls' outcomes from every client.
type tally struct {
	mu        sync.Mutex
	latencies []time.Duration
	errors    int
	first     string
}

func (t *tally) record(d time.Duration, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err == nil {
		t.latencies = append(t.latencies, d)

		return
	}

	t.errors++

	if t.first == "" {
		t.first = err.Error()
	}
}

// drive runs the clients, each with its own session, and measures them.
func drive(ctx context.Context, cfg Config, url string, pid int, target Target) (Result, error) {
	runCtx := ctx

	if cfg.Duration > 0 {
		var cancel context.CancelFunc

		runCtx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
	}

	samples := sample(runCtx, pid, cfg.Sample)
	began := time.Now()

	var (
		calls tally
		wg    sync.WaitGroup
	)

	for i := range cfg.Clients {
		wg.Go(func() { session(runCtx, cfg, url, target, i, &calls) })
	}

	wg.Wait()

	elapsed := time.Since(began)
	rss := samples()

	r := Result{
		Mode: modeLoad, Clients: cfg.Clients, Calls: len(calls.latencies) + calls.errors, Errors: calls.errors,
		Seconds: elapsed.Seconds(), PerSec: float64(len(calls.latencies)) / elapsed.Seconds(),
		P50ms: percentile(calls.latencies, p50), P95ms: percentile(calls.latencies, p95), P99ms: percentile(calls.latencies, p99),
		RSSMiB: rss, FirstError: calls.first, Recorded: time.Now().UTC().Truncate(time.Second),
	}

	if cfg.Duration > 0 {
		r.Mode = modeSoak
		r.GrowthMiBPerHour = growth(rss, cfg.Sample)
	}

	if len(calls.latencies) == 0 {
		return r, fmt.Errorf("%w: %s", ErrNoCall, calls.first)
	}

	return r, nil
}

// session is one client: its own MCP session, initialised, then its calls. A soak run's calls
// end with the run, which is no error.
func session(ctx context.Context, cfg Config, url string, target Target, n int, calls *tally) {
	headers := map[string]string{"Authorization": "Bearer " + target.Key}
	if target.Org != "" {
		headers["X-TheHive-Org"] = target.Org
	}

	c, err := client.NewStreamableHttpClient(url, transport.WithHTTPHeaders(headers))
	if err != nil {
		calls.record(0, fmt.Errorf("client %d: %w", n, err))

		return
	}
	defer func() { _ = c.Close() }()

	err = c.Start(ctx)
	if err == nil {
		_, err = c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "load", Version: "1"},
		}})
	}

	if err != nil {
		calls.record(0, fmt.Errorf("client %d initialise: %w", n, err))

		return
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = "search-entities"
	request.Params.Arguments = map[string]any{"entity-type": "alert", "limit": stubAlerts}

	var tick <-chan time.Time

	if cfg.Duration > 0 {
		t := time.NewTicker(time.Duration(float64(time.Second) / cfg.Rate))
		defer t.Stop()

		tick = t.C
	}

	for i := 0; cfg.Duration > 0 || i < cfg.Calls; i++ {
		if tick != nil {
			select {
			case <-ctx.Done():
				return
			case <-tick:
			}
		}

		d, err := call(ctx, c, request)
		if ctx.Err() != nil && cfg.Duration > 0 {
			return
		}

		calls.record(d, err)
	}
}

// call makes one tool call: a call the tool answers with an error counts as an error.
func call(ctx context.Context, c *client.Client, request mcp.CallToolRequest) (time.Duration, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	began := time.Now()
	result, err := c.CallTool(callCtx, request)
	d := time.Since(began)

	if err != nil {
		return d, fmt.Errorf("search-entities: %w", err)
	}

	if result.IsError {
		return d, fmt.Errorf("search-entities answered an error: %v", result.Content)
	}

	return d, nil
}

// percentile is the latency at q (0 to 1), in milliseconds: the nearest rank.
func percentile(d []time.Duration, q float64) float64 {
	if len(d) == 0 {
		return 0
	}

	s := slices.Clone(d)
	slices.Sort(s)

	i := min(len(s)-1, int(q*float64(len(s))))

	return float64(s[i].Microseconds()) / 1000
}

// sample samples the resident memory of pid every interval until ctx ends; the function it
// returns stops it, takes a last sample, and gives the samples, in MiB.
func sample(ctx context.Context, pid int, every time.Duration) func() []float64 {
	var (
		mu      sync.Mutex
		samples []float64
	)

	take := func() {
		mib, ok := rss(pid)
		if ok {
			mu.Lock()

			samples = append(samples, mib)
			mu.Unlock()
		}
	}

	done, stopped := make(chan struct{}), make(chan struct{})

	go func() {
		defer close(stopped)

		take()

		t := time.NewTicker(every)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-t.C:
				take()
			}
		}
	}()

	return func() []float64 {
		close(done)
		<-stopped
		take()

		mu.Lock()
		defer mu.Unlock()

		return samples
	}
}

// rss reads a process's resident memory from /proc: the run is made in the Linux Go image.
func rss(pid int) (float64, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status") // #nosec G304 -- the server this run started
	if err != nil {
		return 0, false
	}

	return parseRSS(string(data))
}

// parseRSS reads VmRSS from a /proc status file, in MiB.
func parseRSS(status string) (float64, bool) {
	for line := range strings.SplitSeq(status, "\n") {
		v, ok := strings.CutPrefix(line, "VmRSS:")
		if !ok {
			continue
		}

		kb, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")), 64)
		if err != nil {
			return 0, false
		}

		return kb / kibPerMiB, true
	}

	return 0, false
}

// growth is the least-squares slope of the samples after the warmup, in MiB per hour.
func growth(samples []float64, every time.Duration) float64 {
	s := samples[int(warmup*float64(len(samples))):]
	if len(s) < minGrowthSamples {
		return 0
	}

	var sx, sy, sxx, sxy float64

	for i, y := range s {
		x := float64(i) * every.Hours()
		sx, sy, sxx, sxy = sx+x, sy+y, sxx+x*x, sxy+x*y
	}

	n := float64(len(s))

	return (n*sxy - sx*sy) / (n*sxx - sx*sx)
}
