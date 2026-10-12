package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/change"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

// pr runs the pr cadence's gates at the exact commit checked out, records the evidence, and on a
// pass pushes the branch (make pr). The tree must be clean: the evidence is for a commit. The
// change is always the branch against origin/main (every suite when there is none): no flag or
// $HARNESS_BASE narrows what a pr verdict covers.
func (e env) pr(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("pr", flag.ContinueOnError)

	scope := scopeFlags{base: defaultBase}
	push := fs.Bool("push", true, "push the branch when the gates pass")
	force := fs.Bool("force", false, "push with --force-with-lease, after a rebase")
	person := fs.Bool("person", false, personUsage)

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

	head := gitx.Head(ctx, e.root)

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return err
	}

	e.changed, e.base = sel.Paths, sel.Base
	e.clearProfile(out)

	rec, err := e.runGates(ctx, "pr", e.reg.At("pr"), sel, runFlags{stream: os.Getenv("CI") != ""}, out)
	if err != nil {
		return err
	}

	clean, err = gitx.Clean(ctx, e.root)
	if err != nil {
		return fmt.Errorf("read the working tree: %w", err)
	}

	err = unchanged(head, gitx.Head(ctx, e.root), clean)
	if err != nil {
		return err
	}

	ev := evidence.Evidence{
		Head: head, Base: gitx.Resolve(ctx, e.root, sel.Base), Branch: branch, Verdict: rec.Verdict,
		Started: rec.Started, Finished: rec.Finished, Suites: rec.Suites, Changed: sel.Paths, Results: rec.Results,
		FirstSnapshot: firstSnapshot(ctx, e.root, sel.Base),
	}

	err = e.record(ctx, ev, out)
	if err != nil {
		return err
	}

	err = verdictError(rec.Verdict)
	if err != nil {
		return err
	}

	if !*push {
		return nil
	}

	var pushArgs []string
	if *force {
		pushArgs = append(pushArgs, "--force")
	}

	if *person {
		pushArgs = append(pushArgs, "--person")
	}

	return e.push(ctx, pushArgs, out)
}

// clearProfile removes the coverage profile a previous run left, so that the one record keeps is
// this run's. A profile it cannot remove is said: the change metrics may then read a stale one.
func (e env) clearProfile(out io.Writer) {
	err := evidence.ClearProfile(e.root)
	if err != nil {
		_, _ = fmt.Fprintln(out, "warning: the change metrics may read an earlier run's coverage:", err)
	}
}

// record writes the evidence of the run, keeps the coverage profile it wrote beside it for the
// pull request's change metrics, and notes its gate times. Only the evidence must be written.
func (e env) record(ctx context.Context, ev evidence.Evidence, out io.Writer) error {
	p, err := ev.Write(e.root)
	if err != nil {
		return fmt.Errorf("record the evidence: %w", err)
	}

	_, _ = fmt.Fprintf(out, "evidence: %s\n", p)

	_, err = evidence.KeepProfile(e.root, ev.Head)
	if err != nil {
		_, _ = fmt.Fprintln(out, "warning: the change metrics will have no diff coverage:", err)
	}

	e.note(ctx, ev, out)

	return nil
}

// note records the run's gate times on its commit, under this machine's notes ref, which the push
// takes along: the repository's dashboard charts them by machine. A note that cannot be written is
// said, never a failure of the run.
func (e env) note(ctx context.Context, ev evidence.Evidence, out io.Writer) {
	ref := evidence.NotesRef(ctx, e.root)
	if ref == "" {
		_, _ = fmt.Fprintln(out, "warning: the run's timings are not noted: this clone has no notes id, and none could be kept")

		return
	}

	data, err := ev.Note(evidence.ThisMachine(ctx))
	if err == nil {
		err = gitx.AddNote(ctx, e.root, ref, ev.Head, data)
	}

	if err != nil {
		_, _ = fmt.Fprintln(out, "warning: the run's timings are not noted:", err)
	}
}

// firstSnapshot reports whether the change creates the snapshot of what the model reads: the base
// has none. Without a base, nothing is known.
func firstSnapshot(ctx context.Context, root, base string) bool {
	return base != "" && !gitx.HasPath(ctx, root, base, checks.SurfaceSummary)
}

// errChangedDuringRun means the gates did not run on the commit HEAD names now.
var errChangedDuringRun = errors.New("the commit or the working tree changed while the gates ran: commit, then run make pr again")

// unchanged checks that HEAD is still the commit the gates started on and the tree still clean
// once they have run: the evidence must describe the commit they ran on.
func unchanged(before, after string, clean bool) error {
	if after != before {
		return fmt.Errorf("%w (HEAD was %s, it is now %s)", errChangedDuringRun, before, after)
	}

	if !clean {
		return fmt.Errorf("%w (the working tree has changes)", errChangedDuringRun)
	}

	return nil
}

// showEvidence prints the evidence of HEAD: as the pull request's Gate evidence section with
// --markdown, else as JSON.
func (e env) showEvidence(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("evidence", flag.ContinueOnError)
	md := fs.Bool("markdown", false, "print the Gate evidence section of the pull request")
	against := fs.String("base", "", "flag what the pull request changes against this ref, its base (a stacked pull request's is another branch)")

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

	if *against != "" {
		if !gitx.HasRef(ctx, e.root, *against) {
			return usageError{"evidence: no " + *against + " to compare with"}
		}

		changed, err := gitx.ChangedSince(ctx, e.root, *against)
		if err != nil {
			return fmt.Errorf("evidence: list the changes since %s: %w", *against, err)
		}

		ev.Changed, ev.FirstSnapshot, ev.Against = changed, firstSnapshot(ctx, e.root, *against), *against
	}

	if *md {
		_, _ = io.WriteString(out, evidence.Markdown(*ev, evidence.Flags(ev.Changed, classOf)))
		e.changeMetrics(ctx, cmp.Or(*against, ev.Base), head, out)

		return nil
	}

	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the evidence: %w", err)
	}

	_, _ = fmt.Fprintln(out, string(data))

	return nil
}

// changeMetrics prints the Change metrics section: what the change since against does, measured
// on its merge base and on head, with the coverage profile make pr kept for head. With no base,
// there is no change to measure. A change it cannot measure is said in the section, never a
// failure: the Gate evidence above it is already printed, and stays whole.
func (e env) changeMetrics(ctx context.Context, against, head string, out io.Writer) {
	if against == "" {
		return
	}

	profile, err := evidence.Profile(e.root, head)

	var m change.Metrics
	if err == nil {
		m, err = change.Measure(ctx, e.root, against, profile, e.limits.Structure)
	}

	section := change.Markdown(m)
	if err != nil {
		section = change.Unmeasured(err)
	}

	_, _ = io.WriteString(out, "\n"+section)
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
