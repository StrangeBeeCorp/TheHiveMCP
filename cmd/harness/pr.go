package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// pr runs the pr cadence's gates at the exact commit checked out, records the evidence, and on a
// pass pushes the branch (make pr). The tree must be clean: the evidence is for a commit.
func (e env) pr(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("pr", flag.ContinueOnError)

	var scope scopeFlags
	scope.register(fs)
	push := fs.Bool("push", true, "push the branch when the gates pass")
	force := fs.Bool("force", false, "push with --force-with-lease, after a rebase")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	clean, err := gitx.Clean(ctx, e.root)
	if err != nil {
		return fmt.Errorf("read the working tree: %w", err)
	}

	if !clean {
		return usageError{"commit first: the evidence is for a commit, and the working tree has changes"}
	}

	branch, err := pushTarget(gitx.Branch(ctx, e.root))
	if err != nil {
		return err
	}

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return err
	}

	e.changed, e.base = sel.Paths, sel.Base

	rec, err := e.runGates(ctx, "pr", e.reg.At("pr"), sel, runFlags{stream: os.Getenv("CI") != ""}, out)
	if err != nil {
		return err
	}

	ev := evidence.Evidence{
		Head: rec.Head, Base: gitx.Resolve(ctx, e.root, sel.Base), Branch: branch, Verdict: rec.Verdict,
		Started: rec.Started, Finished: rec.Finished, Suites: rec.Suites, Changed: sel.Paths, Results: rec.Results,
	}

	p, err := ev.Write(e.root)
	if err != nil {
		return fmt.Errorf("record the evidence: %w", err)
	}

	_, _ = fmt.Fprintf(out, "evidence: %s\n", p)

	switch rec.Verdict {
	case runner.Fail:
		return errFailed
	case runner.Incomplete:
		return errIncomplete
	}

	if !*push {
		return nil
	}

	var pushArgs []string
	if *force {
		pushArgs = append(pushArgs, "--force")
	}

	return e.push(ctx, pushArgs, out)
}

// showEvidence prints the evidence of HEAD: as the pull request's Gate evidence section with
// --markdown, else as JSON.
func (e env) showEvidence(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("evidence", flag.ContinueOnError)
	md := fs.Bool("markdown", false, "print the Gate evidence section of the pull request")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	head := gitx.Head(ctx, e.root)

	ev, err := evidence.Read(e.root, head)
	if err != nil {
		return fmt.Errorf("evidence: %w", err)
	}

	if ev == nil {
		return usageError{"no evidence for " + head + ": run make pr"}
	}

	if *md {
		_, _ = io.WriteString(out, evidence.Markdown(*ev, evidence.Flags(ev.Changed, classOf)))
		return nil
	}

	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the evidence: %w", err)
	}

	_, _ = fmt.Fprintln(out, string(data))

	return nil
}

// classOf names the protected class of a path, for the red flags of the pull request.
func classOf(p string) (string, bool) {
	class, ok := policy.ClassOf(p)
	return string(class), ok
}

// reviewEffort prints the effort of the adversarial review the change needs: the highest of the
// selected suites' (gates/registry.yaml). The review skill runs it (review_effort_cmd).
func (e env) reviewEffort(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("review-effort", flag.ContinueOnError)

	var scope scopeFlags
	scope.register(fs)

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, e.reg.ReviewEffort(sel))

	return nil
}
