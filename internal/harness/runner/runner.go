// Package runner runs gates and records what happened: one log per gate and a results.json that
// states each verdict. A limit (UNAVAILABLE) is reported, never counted as a pass: a run with one
// ends INCOMPLETE.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// Verdict is the outcome of one gate.
type Verdict string

// The verdicts. SKIPPED means the change did not select the gate. INCOMPLETE is the verdict of a
// run in which no gate failed but one could not run.
const (
	Pass        Verdict = "PASS"
	Fail        Verdict = "FAIL"
	Unavailable Verdict = "UNAVAILABLE"
	Skipped     Verdict = "SKIPPED"
	Incomplete  Verdict = "INCOMPLETE"
)

// ExitUnavailable is the exit code by which a gate command says it could not run here.
const ExitUnavailable = 75

const (
	logPerm  = 0o600
	dirPerm  = 0o750
	tailSize = 40
)

// Builtin runs a check built into the harness and returns its findings, one per line.
type Builtin func(ctx context.Context, name string) ([]string, error)

// Options configure a run.
type Options struct {
	Root     string
	Cadence  string
	LogDir   string
	Stream   bool // copy each command's output to Out as it runs
	Out      io.Writer
	Builtin  Builtin
	DockerUp func(ctx context.Context) bool
}

// Result is the record of one gate.
type Result struct {
	Gate    string  `json:"gate"`
	Verdict Verdict `json:"verdict"`
	Seconds float64 `json:"seconds"`
	Summary string  `json:"summary"`
	Log     string  `json:"log,omitempty"`
	// Streamed is true when the gate's output was copied to Out as it ran.
	Streamed bool `json:"-"`
	// Listing is true when the log is a built-in gate's findings, one per line: Print shows them
	// all, since nothing streamed them.
	Listing bool `json:"-"`
}

// Run runs the gates in order and returns one result per gate.
func Run(ctx context.Context, reg *registry.Registry, gates []registry.Gate, opts Options) ([]Result, error) {
	err := os.MkdirAll(opts.LogDir, dirPerm)
	if err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	dockerChecked, dockerUp := false, false

	results := make([]Result, 0, len(gates))
	for _, g := range gates {
		if g.Docker && !dockerChecked {
			dockerChecked, dockerUp = true, opts.DockerUp(ctx)
		}

		if g.Docker && !dockerUp {
			results = append(results, Result{Gate: g.ID, Verdict: Unavailable, Summary: "Docker is not running: start OrbStack and run again"})
			continue
		}

		results = append(results, runGate(ctx, reg, g, opts))
	}

	return results, nil
}

func runGate(ctx context.Context, reg *registry.Registry, g registry.Gate, opts Options) Result {
	start := time.Now()
	logPath := filepath.Join(opts.LogDir, g.ID+".log")

	ctx, cancel := context.WithTimeout(ctx, time.Duration(g.BudgetS)*time.Second)
	defer cancel()

	var r Result
	if g.Builtin != "" {
		r = runBuiltin(ctx, g, opts, logPath) // a builtin's findings go to its log, never to Out
	} else {
		r = runCommand(ctx, reg.Command(g, opts.Cadence), g, opts, logPath)
		r.Streamed = opts.Stream
	}

	// A run interrupted (Ctrl-C, a hook's timeout) stops its gates: an unfinished one is a limit,
	// not a failure of the change.
	if r.Verdict != Pass && errors.Is(ctx.Err(), context.Canceled) {
		r.Verdict, r.Summary = Unavailable, "interrupted before it finished"
	}

	r.Gate, r.Seconds = g.ID, time.Since(start).Seconds()

	return r
}

func runBuiltin(ctx context.Context, g registry.Gate, opts Options, logPath string) Result {
	findings, err := opts.Builtin(ctx, g.Builtin)
	if err != nil {
		return Result{Verdict: Fail, Summary: err.Error()}
	}

	if len(findings) == 0 {
		return Result{Verdict: Pass, Summary: "clean"}
	}

	err = os.WriteFile(logPath, []byte(strings.Join(findings, "\n")+"\n"), logPerm)
	if err != nil {
		return Result{Verdict: Fail, Summary: fmt.Sprintf("%d finding(s); the log could not be written: %v", len(findings), err)}
	}

	return Result{Verdict: Fail, Summary: fmt.Sprintf("%d finding(s): %s", len(findings), findings[0]), Log: logPath, Listing: true}
}

func runCommand(ctx context.Context, command string, g registry.Gate, opts Options, logPath string) Result {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logPerm) // #nosec G304 -- the path is built from the gate id
	if err != nil {
		return Result{Verdict: Fail, Summary: fmt.Sprintf("cannot write the log: %v", err)}
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, "sh", "-c", command) // #nosec G204 -- gate commands come from the reviewed registry
	cmd.Dir = opts.Root
	killGroupOnCancel(cmd)

	cmd.Env = append(os.Environ(), "HARNESS_CADENCE="+opts.Cadence, "HARNESS_GATE="+g.ID)

	var out io.Writer = logFile
	if opts.Stream {
		out = io.MultiWriter(logFile, opts.Out)
	}

	cmd.Stdout, cmd.Stderr = out, out
	_, _ = fmt.Fprintf(logFile, "$ %s\n", command)
	err = cmd.Run()

	var exit *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return Result{Verdict: Fail, Summary: fmt.Sprintf("over its %d s budget", g.BudgetS), Log: logPath}
	case err == nil:
		return Result{Verdict: Pass, Summary: lastLine(logPath), Log: logPath}
	case errors.As(err, &exit) && exit.ExitCode() == ExitUnavailable:
		return Result{Verdict: Unavailable, Summary: lastLine(logPath), Log: logPath}
	default:
		return Result{Verdict: Fail, Summary: lastLine(logPath), Log: logPath}
	}
}

// lastLine is the last non-empty line of the log, the usual one-line summary of a tool.
func lastLine(logPath string) string {
	lines := Tail(logPath, 1)
	if len(lines) == 0 {
		return ""
	}

	return lines[0]
}

// Tail returns the last n non-empty lines of a log, all of them when n is negative.
func Tail(logPath string, n int) []string {
	data, err := os.ReadFile(logPath) // #nosec G304 -- the path is a log this package wrote
	if err != nil {
		return nil
	}

	var lines []string

	for l := range strings.SplitSeq(string(data), "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}

	if n >= 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines
}

// Failed reports whether any gate failed. A limit is not a failure.
func Failed(results []Result) bool {
	for _, r := range results {
		if r.Verdict == Fail {
			return true
		}
	}

	return false
}

// Overall is the verdict of a run: FAIL when a gate failed, else INCOMPLETE when a gate could not
// run, else PASS. A skipped gate (not selected by the change) does not count.
func Overall(results []Result) Verdict {
	if Failed(results) {
		return Fail
	}

	for _, r := range results {
		if r.Verdict == Unavailable {
			return Incomplete
		}
	}

	return Pass
}

// Record is results.json: the run as a whole.
type Record struct {
	Cadence  string    `json:"cadence"`
	Head     string    `json:"head"`
	Suites   []string  `json:"suites"`
	Reason   string    `json:"reason,omitempty"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Verdict  Verdict   `json:"verdict"`
	Results  []Result  `json:"results"`
}

// Write saves the record as results.json in the log directory and returns its path.
func (rec Record) Write(logDir string) (string, error) {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode results: %w", err)
	}

	p := filepath.Join(logDir, "results.json")

	err = os.WriteFile(p, append(data, '\n'), logPerm)
	if err != nil {
		return "", fmt.Errorf("write results: %w", err)
	}

	return p, nil
}

// Print writes the human summary: one line per gate, then each failed gate's log, unless that
// output was already streamed: a built-in gate's findings in full, a command's last 40 lines.
func Print(w io.Writer, results []Result) {
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "  %-11s %-14s %6.1f s  %s\n", r.Verdict, r.Gate, r.Seconds, r.Summary)
	}

	for _, r := range results {
		if r.Verdict != Fail || r.Log == "" || r.Streamed {
			continue
		}

		if r.Listing {
			_, _ = fmt.Fprintf(w, "\n--- %s (%s) ---\n", r.Gate, r.Log)
			for _, l := range Tail(r.Log, -1) {
				_, _ = fmt.Fprintln(w, l)
			}

			continue
		}

		_, _ = fmt.Fprintf(w, "\n--- %s (last %d lines of %s) ---\n", r.Gate, tailSize, r.Log)
		for _, l := range Tail(r.Log, tailSize) {
			_, _ = fmt.Fprintln(w, l)
		}
	}
}
