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
		outcomes = append(outcomes, e.canary(ctx, c, out))
	}

	return report(outcomes, out)
}

func (e env) canary(ctx context.Context, c checks.Canary, out io.Writer) canaryOutcome {
	o := canaryOutcome{id: c.ID, gate: c.Gate}

	dir, remove, err := gitx.Scratch(ctx, e.root, "HEAD")
	if err != nil {
		o.verdict, o.note = runner.Unavailable, "no scratch worktree: "+err.Error()
		return o
	}
	defer remove()

	err = gitx.Apply(ctx, dir, filepath.Join(e.root, "gates", "canaries", c.Patch))
	if err != nil {
		o.verdict, o.note = runner.Fail, "the patch no longer applies to HEAD: rewrite it"
		return o
	}

	started := time.Now()
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "bin", "harness"), "run", "--cadence", "nightly", "--gate", c.Gate, "--force") // #nosec G204 -- the scratch checkout's own launcher; the gate id comes from the manifest
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard

	cmd.Env = append(os.Environ(), "RUN="+c.Run)

	err = cmd.Run()

	var exit *exec.ExitError

	switch {
	case err == nil:
		o.verdict, o.note = runner.Fail, "ESCAPED: the gate passed with the patch applied"
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		o.verdict, o.note = runner.Pass, fmt.Sprintf("caught in %.0f s", time.Since(started).Seconds())
	default:
		o.verdict, o.note = runner.Unavailable, "the gate could not run: "+err.Error()
	}

	_, _ = fmt.Fprintf(out, "  %s %s\n", o.verdict, o.note)

	return o
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
