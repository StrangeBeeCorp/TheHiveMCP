package main

// The nightly workflow's own commands: triage files the night's findings, next lists them, and
// eval-smoke runs TheHiveMCPEval's suite against main.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evalsmoke"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// blockedPath is where an agent that stayed red for its rounds writes why (internal/harness/hooks).
const blockedPath = ".cache/harness/BLOCKED.md"

// errNoGH means gh is not there to read or write the findings.
var errNoGH = errors.New("gh is needed for the nightly findings (install it and run gh auth login)")

// triageNight files the night's failures as GitHub issues labelled nightly, comments on those
// still failing and closes those that pass again (the nightly workflow's last job).
func (e env) triageNight(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("triage", flag.ContinueOnError)
	runs := fs.String("runs", "", "the directory of the night's artifacts, one subdirectory per shard")
	runURL := fs.String("run-url", "", "the nightly run's URL, quoted in each issue")
	expect := fs.String("expect", "", "the shards the night has, comma-separated: one with no result is a finding")
	dry := fs.Bool("dry-run", false, "print what would change, change nothing")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	if *runs == "" || *runURL == "" {
		return usageError{"triage: --runs and --run-url are needed"}
	}

	var shards []string
	if *expect != "" {
		shards = strings.Split(*expect, ",")
	}

	outcomes, err := triage.Read(*runs, shards)
	if err != nil {
		return fmt.Errorf("triage: %w", err)
	}

	gh := ghIssues{ctx: ctx, dir: e.root}

	open, err := gh.list()
	if err != nil {
		return err
	}

	actions := triage.Plan(outcomes, open, *runURL)
	for _, a := range actions {
		_, _ = fmt.Fprintln(out, describe(a))
	}

	if *dry {
		return nil
	}

	if slices.ContainsFunc(actions, func(a triage.Action) bool { return a.Kind == triage.Open }) {
		err = gh.label()
		if err != nil {
			return err
		}
	}

	for _, a := range actions {
		err = gh.apply(a)
		if err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(out, "%d finding(s) changed, %d open before this night\n", len(actions), len(open))

	return nil
}

func describe(a triage.Action) string {
	if a.Kind == triage.Open {
		return "open: " + a.Title
	}

	return fmt.Sprintf("%s #%d", a.Kind, a.Issue)
}

// next lists the open findings, the oldest first, after an agent's BLOCKED.md if there is one
// (make next: what to work on).
func (e env) next(ctx context.Context, out io.Writer) error {
	blocked, err := os.ReadFile(filepath.Join(e.root, blockedPath)) // #nosec G304 -- this checkout's own cache
	if err == nil {
		_, _ = fmt.Fprintf(out, "An agent is blocked (%s):\n\n%s\n", blockedPath, strings.TrimSpace(string(blocked)))
	}

	issues, err := ghIssues{ctx: ctx, dir: e.root}.listForNext()
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		_, _ = fmt.Fprintln(out, "No open nightly finding.")

		return nil
	}

	_, _ = fmt.Fprintf(out, "%d open nightly finding(s), the oldest first:\n", len(issues))

	for _, i := range issues {
		_, _ = fmt.Fprintf(out, "  #%d  %s  (since %s)  %s\n", i.Number, i.Title, i.CreatedAt.Format(time.DateOnly), i.URL)
	}

	return nil
}

// ghIssues reads and writes the nightly findings with gh, in the repository of dir.
type ghIssues struct {
	ctx context.Context //nolint:containedctx // one triage's calls share its context
	dir string
}

// listed is an issue as `gh issue list --json` prints it.
type listed struct {
	triage.Issue

	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

func (g ghIssues) list() ([]triage.Issue, error) {
	found, err := g.query("number,title,body")
	if err != nil {
		return nil, err
	}

	issues := make([]triage.Issue, 0, len(found))
	for _, l := range found {
		issues = append(issues, l.Issue)
	}

	return issues, nil
}

func (g ghIssues) listForNext() ([]listed, error) {
	found, err := g.query("number,title,url,createdAt")
	if err != nil {
		return nil, err
	}

	slices.SortFunc(found, func(a, b listed) int { return a.CreatedAt.Compare(b.CreatedAt) })

	return found, nil
}

func (g ghIssues) query(fields string) ([]listed, error) {
	data, err := g.run(nil, "issue", "list", "--label", triage.Label, "--state", "open", "--limit", "500", "--json", fields)
	if err != nil {
		return nil, err
	}

	var found []listed

	err = json.Unmarshal(data, &found)
	if err != nil {
		return nil, fmt.Errorf("read gh's issue list: %w", err)
	}

	return found, nil
}

// label makes sure the nightly label exists before an issue carries it.
func (g ghIssues) label() error {
	_, err := g.run(nil, "label", "create", triage.Label, "--force", "--color", "B60205", "--description", "A finding of the nightly run, filed and closed by bin/harness triage")

	return err
}

func (g ghIssues) apply(a triage.Action) error {
	var err error

	switch a.Kind {
	case triage.Open:
		_, err = g.run(strings.NewReader(a.Body), "issue", "create", "--title", a.Title, "--label", triage.Label, "--body-file", "-")
	case triage.Comment:
		_, err = g.run(strings.NewReader(a.Body), "issue", "comment", strconv.Itoa(a.Issue), "--body-file", "-")
	case triage.Close:
		_, err = g.run(nil, "issue", "close", strconv.Itoa(a.Issue), "--comment", a.Body)
	}

	return err
}

// run runs gh with the body on stdin, and returns its output.
func (g ghIssues) run(stdin io.Reader, args ...string) ([]byte, error) {
	_, err := exec.LookPath("gh")
	if err != nil {
		return nil, errNoGH
	}

	cmd := exec.CommandContext(g.ctx, "gh", args...) // #nosec G204 -- gh with the triage's own arguments; bodies go on stdin
	cmd.Dir = g.dir
	cmd.Stdin = stdin

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))
	}

	return data, nil
}

var (
	// errEvalSmoke means the night's smoke found a problem; they are printed.
	errEvalSmoke = errors.New("eval smoke")
	// errNoPrevious means there is no previous night to compare with: the first night, or none kept.
	errNoPrevious = errors.New("no previous night")
	// errNoEval means the run left no summary: there is nothing to read.
	errNoEval = errors.New("the run created no evaluation")
)

const (
	// evalLogLines is how much of eval-cli's output a failure shows.
	evalLogLines = 30
	// evalUsageExit is eval-cli's exit for a usage error, or CI without an OpenRouter key.
	evalUsageExit = 2
	// summaryName is the run summary's file, next to the night's result.
	summaryName = "summary.json"
)

// evalSmoke runs TheHiveMCPEval's suite against this checkout on gates/eval.yaml's model, its
// stack already up, and holds the result to the cap and to the previous night's pass rate (the
// eval-smoke gate, nightly). The result is written whatever it says, with whether it passed: the
// next night compares itself with the last night that did.
func (e env) evalSmoke(ctx context.Context, args []string, out io.Writer) error {
	set := flag.NewFlagSet("eval-smoke", flag.ContinueOnError)
	dir := set.String("eval", os.Getenv("THEHIVEMCP_EVAL"), "the TheHiveMCPEval checkout, its stack up ($THEHIVEMCP_EVAL)")
	outPath := set.String("out", "", "write the night's result as JSON to this file")
	previousPath := set.String("previous", "", "the last night that passed, to hold the pass rate to (absent: not compared)")

	err := set.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	config, err := evalsmoke.Load(filepath.Join(e.root, evalsmoke.Path))
	if err != nil {
		return fmt.Errorf("eval smoke: %w", err)
	}

	err = evalReady(*dir, runtime.GOOS)
	if err != nil {
		return err
	}

	err = config.Preflight()
	if err != nil {
		return fmt.Errorf("eval smoke: %w", err)
	}

	previous, err := readPrevious(*previousPath, out)
	if err != nil && !errors.Is(err, errNoPrevious) {
		return err
	}

	summaryPath, err := runSummaryPath(*outPath)
	if err != nil {
		return err
	}

	result, err := e.runEval(ctx, *dir, summaryPath, config, out)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, evalsmoke.Summary(result, previous))

	problems := config.Check(result, previous)
	for _, p := range problems {
		_, _ = fmt.Fprintln(out, p)
	}

	result.OK = len(problems) == 0

	if *outPath != "" {
		err = writeResult(result, *outPath)
		if err != nil {
			return err
		}
	}

	if !result.OK {
		return fmt.Errorf("%w: %d problem(s)", errEvalSmoke, len(problems))
	}

	return nil
}

// runSummaryPath is where eval-cli writes the run's summary: next to the night's result, which
// the nightly keeps, else in a directory of its own. It is absolute: eval-cli runs in its checkout.
func runSummaryPath(outPath string) (string, error) {
	dir := filepath.Dir(outPath)
	if outPath == "" {
		var err error

		dir, err = os.MkdirTemp("", "eval-smoke-")
		if err != nil {
			return "", fmt.Errorf("eval smoke: %w", err)
		}
	}

	err := os.MkdirAll(dir, dirPerm)
	if err == nil {
		dir, err = filepath.Abs(dir)
	}

	if err != nil {
		return "", fmt.Errorf("eval smoke: %w", err)
	}

	p := filepath.Join(dir, summaryName)

	// A summary left by an earlier run would be read as this one's.
	err = os.Remove(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("eval smoke: %w", err)
	}

	return p, nil
}

func writeResult(r evalsmoke.Result, p string) error {
	err := os.MkdirAll(filepath.Dir(p), dirPerm)
	if err == nil {
		err = r.Write(p)
	}

	if err != nil {
		return fmt.Errorf("eval smoke: %w", err)
	}

	return nil
}

// evalReady says whether the smoke can run here: a TheHiveMCPEval checkout, uv to run its CLI,
// and an OpenRouter key (on macOS, goos darwin, the eval CLI reads it from the keychain).
func evalReady(dir, goos string) error {
	if dir == "" {
		return fmt.Errorf("%w: no TheHiveMCPEval checkout (set THEHIVEMCP_EVAL)", errCannotRun)
	}

	_, err := os.Stat(filepath.Join(dir, "pyproject.toml")) // #nosec G703 -- the TheHiveMCPEval checkout the night was given
	if err != nil {
		return fmt.Errorf("%w: %s is no TheHiveMCPEval checkout: %w", errCannotRun, dir, err)
	}

	_, err = exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("%w: uv is needed to run the eval CLI", errCannotRun)
	}

	if os.Getenv("OPENROUTER_API_KEY") == "" && goos != "darwin" {
		return fmt.Errorf("%w: no OPENROUTER_API_KEY", errCannotRun)
	}

	return nil
}

// runEval runs the suite, then reads the summary eval-cli writes whatever the outcome: the
// evaluations the run created (one per phase, read then write) and their counts. The night is
// their sum, at the true cost of each.
func (e env) runEval(ctx context.Context, dir, summaryPath string, c evalsmoke.Config, out io.Writer) (evalsmoke.Result, error) {
	_, _ = fmt.Fprintf(out, "eval smoke: %s on %s, %d cases, about $%.3f (cap $%.2f)\n", c.Suite, c.Model, c.Cases, c.Estimate(), c.Cap)

	// eval-cli exits 100 when cases failed: the summary says how many passed, and its status
	// whether the run completed.
	runOut, runErr := evalCLI(ctx, dir, "eval", cmdRun, c.Suite, "--local", e.root, "--model", c.Model, "--summary-json", summaryPath)

	data, err := os.ReadFile(summaryPath) // #nosec G304 -- the summary this run asked eval-cli for
	if err != nil {
		return evalsmoke.Result{}, fmt.Errorf("eval smoke: %w: no run summary (%w)%s\n%s", errNoEval, err, runNote(runErr), lastLines(runOut, evalLogLines))
	}

	run, err := evalsmoke.ReadRun(data)
	if err != nil {
		return evalsmoke.Result{}, fmt.Errorf("eval smoke: %w", err)
	}

	// Exit 2 is a usage error, or CI without an OpenRouter key: the night could not run here.
	if run.ExitCode == evalUsageExit {
		return evalsmoke.Result{}, fmt.Errorf("%w: eval-cli refused the run (exit 2): %s", errCannotRun, lastLines(runOut, evalLogLines))
	}

	result, err := run.Result(c.Model)
	if err != nil {
		return evalsmoke.Result{}, fmt.Errorf("eval smoke: %w\n%s", err, lastLines(runOut, evalLogLines))
	}

	result.Suite = c.Suite

	for _, id := range run.EvalIDs {
		usd, ok := trueCost(ctx, dir, id, c.Model)
		result.CostUSD += usd
		result.CostUnmeasured = result.CostUnmeasured || !ok
	}

	_, _ = fmt.Fprintf(out, "evaluations: %s\n", strings.Join(run.EvalIDs, ", "))

	result.Head = gitx.Head(ctx, e.root)
	result.Recorded = time.Now().UTC().Truncate(time.Second)

	return result, nil
}

// runNote is how eval run ended, for an error that follows it.
func runNote(err error) string {
	if err == nil {
		return " (eval run exited 0)"
	}

	return " (" + err.Error() + ")"
}

// trueCost is one evaluation's true cost for the model, from LiteLLM's spend log: false when
// eval-cli could not attribute it, which the night's check reports.
func trueCost(ctx context.Context, dir, id, model string) (float64, bool) {
	cost, err := evalCLI(ctx, dir, "eval", "cost", id)
	if err != nil {
		return 0, false
	}

	return evalsmoke.TrueCost([]byte(cost), model)
}

// evalCLI runs `uv run eval-cli ARGS` in the TheHiveMCPEval checkout and returns its output.
func evalCLI(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "uv", append([]string{cmdRun, "eval-cli"}, args...)...) // #nosec G204 -- the eval CLI with the smoke's own arguments
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String() + stderr.String(), fmt.Errorf("eval-cli %s: %w: %s", strings.Join(args[:2], " "), err, lastLines(stderr.String(), evalLogLines))
	}

	return stdout.String(), nil
}

// readPrevious reads the last night that passed, which the nightly workflow downloads. None (the
// first night, or none kept) is errNoPrevious; a night that did not pass is no baseline either.
func readPrevious(p string, out io.Writer) (*evalsmoke.Result, error) {
	if p == "" {
		return nil, errNoPrevious
	}

	r, err := evalsmoke.Read(p)
	if errors.Is(err, fs.ErrNotExist) {
		_, _ = fmt.Fprintln(out, "no previous night to compare with:", p)

		return nil, errNoPrevious
	}

	if err != nil {
		return nil, fmt.Errorf("eval smoke: %w", err)
	}

	if !r.OK {
		_, _ = fmt.Fprintf(out, "the previous night (%s) did not pass: not compared with\n", r.Recorded.Format(time.DateOnly))

		return nil, errNoPrevious
	}

	return &r, nil
}
