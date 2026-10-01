package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// integrationGate starts a TheHive stack: its canaries run where nothing else holds the ports.
const integrationGate = "integration"

// canaryOutcome is what became of one canary.
type canaryOutcome struct {
	id, gate string
	verdict  runner.Verdict
	note     string
}

// canaries applies each known-bad patch of gates/canaries/ to HEAD, in a scratch worktree, and
// runs the gate it names: that gate must fail. A canary that passes means the check stopped
// working; one that no longer applies must be rewritten.
func (e env) canaries(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("canaries", flag.ContinueOnError)

	var only gateList
	fs.Var(&only, "id", "run only this canary (repeatable)")
	withIntegration := fs.Bool("integration", os.Getenv("CI") != "", "run the canaries of the integration gate (default in CI)")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	list, err := checks.LoadCanaries(checks.DirTree{Root: e.root}, e.reg)
	if err != nil {
		return fmt.Errorf("load the canaries: %w", err)
	}

	r := canaryRun{env: e, out: out, baseline: map[string]int{}}

	var outcomes []canaryOutcome

	for _, c := range list {
		if len(only) > 0 && !slices.Contains(only, c.ID) {
			continue
		}

		if c.Gate == integrationGate && !*withIntegration {
			outcomes = append(outcomes, canaryOutcome{c.ID, c.Gate, runner.Skipped, "needs its own TheHive stack: --integration"})
			continue
		}

		_, _ = fmt.Fprintf(out, "canary %s: %s must fail (%s)\n", c.ID, c.Gate, c.Why)
		outcomes = append(outcomes, r.canary(ctx, c))
	}

	if r.stack {
		r.teardown(ctx)
	}

	return report(outcomes, out)
}

// canaryRun runs canaries, and each gate once without a patch: a gate that fails on HEAD alone
// proves nothing when it fails with the patch.
type canaryRun struct {
	env

	out      io.Writer
	baseline map[string]int
	stack    bool
	// matched tells whether the last patched run's gate log matched the canary's expect.
	matched bool
}

func (r *canaryRun) canary(ctx context.Context, c checks.Canary) canaryOutcome {
	o := canaryOutcome{id: c.ID, gate: c.Gate}
	key := c.Gate + " " + c.Run

	if _, done := r.baseline[key]; !done {
		code, err := r.runIn(ctx, c, false)
		if err != nil {
			o.verdict, o.note = runner.Unavailable, err.Error()
			return o
		}

		r.baseline[key] = code
	}

	if code := r.baseline[key]; code != 0 {
		o.verdict, o.note = runner.Unavailable, fmt.Sprintf("the gate fails on HEAD alone (exit %d): the canary proves nothing until it passes", code)
		return o
	}

	started := time.Now()

	code, err := r.runIn(ctx, c, true)

	switch {
	case err != nil:
		o.verdict, o.note = runner.Fail, err.Error()
	case code == 0:
		o.verdict, o.note = runner.Fail, "ESCAPED: the gate passed with the patch applied"
	case code == 1 && !r.matched:
		o.verdict, o.note = runner.Unavailable, fmt.Sprintf("the gate failed, but its log has no %q: it failed for another reason", c.Expect)
	case code == 1:
		o.verdict, o.note = runner.Pass, fmt.Sprintf("caught in %.0f s", time.Since(started).Seconds())
	default:
		o.verdict, o.note = runner.Unavailable, fmt.Sprintf("the gate could not run (exit %d)", code)
	}

	_, _ = fmt.Fprintf(r.out, "  %s %s\n", o.verdict, o.note)

	return o
}

// runIn runs the canary's gate in a scratch checkout of HEAD, its patch applied or not, and
// returns the gate's exit code.
func (r *canaryRun) runIn(ctx context.Context, c checks.Canary, patched bool) (int, error) {
	dir, remove, err := gitx.Scratch(ctx, r.root, "HEAD")
	if err != nil {
		return 0, fmt.Errorf("no scratch worktree: %w", err)
	}
	defer remove()

	if patched {
		err = gitx.Apply(ctx, dir, filepath.Join(r.root, "gates", "canaries", c.Patch))
		if err != nil {
			return 0, errors.New("the patch no longer applies to HEAD: rewrite it")
		}
	}

	if c.Gate == integrationGate {
		r.stack = true
	}

	cmd := exec.CommandContext(ctx, filepath.Join(dir, "bin", "harness"), "run", "--cadence", "nightly", "--gate", c.Gate, "--force") // #nosec G204 -- the scratch checkout's own launcher; the gate id comes from the manifest
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard

	// The change under test is the patch alone: compared with HEAD, not with the branch's base.
	cmd.Env = append(os.Environ(), "RUN="+c.Run, "HARNESS_BASE=HEAD")

	err = cmd.Run()

	if patched {
		r.matched = logMatches(dir, c)
	}

	var exit *exec.ExitError

	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		return exit.ExitCode(), nil
	}

	return 0, fmt.Errorf("the gate could not start: %w", err)
}

// logMatches reports whether the log the gate wrote in the scratch checkout matches the canary's
// expect.
func logMatches(dir string, c checks.Canary) bool {
	logs, _ := filepath.Glob(filepath.Join(dir, ".cache", "harness", "runs", "nightly-*", c.Gate+".log"))

	re, err := regexp.Compile(c.Expect)
	if err != nil {
		return false
	}

	for _, p := range logs {
		data, err := os.ReadFile(p) // #nosec G304 -- a gate log of the scratch checkout
		if err == nil && re.Match(data) {
			return true
		}
	}

	return false
}

// teardown removes the TheHive stack the integration canaries started.
func (r *canaryRun) teardown(ctx context.Context) {
	dir, remove, err := gitx.Scratch(ctx, r.root, "HEAD")
	if err != nil {
		return
	}
	defer remove()

	cmd := exec.CommandContext(ctx, "make", "test-integration-down") // #nosec G204 -- a fixed make target
	cmd.Dir = dir
	_ = cmd.Run()
}

func report(outcomes []canaryOutcome, out io.Writer) error {
	failed, incomplete := 0, 0

	_, _ = fmt.Fprintln(out)

	for _, o := range outcomes {
		_, _ = fmt.Fprintf(out, "%-12s %-24s %-12s %s\n", o.verdict, o.id, o.gate, o.note)

		switch o.verdict {
		case runner.Fail:
			failed++
		case runner.Unavailable, runner.Skipped:
			incomplete++
		}
	}

	switch {
	case failed > 0:
		return errFailed
	case incomplete > 0:
		return errIncomplete
	}

	return nil
}
