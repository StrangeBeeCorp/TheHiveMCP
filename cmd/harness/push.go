package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

const (
	// pushRemote is the one remote anything is pushed to. The public mirror is a person's to update.
	pushRemote = "origin"
	// gitPush is git's subcommand.
	gitPush = "push"
	// mainBranch changes only when a person merges a pull request.
	mainBranch = "main"
	// personUsage is the help of --person, the flag only a person types.
	personUsage = "a person pushes: missing evidence or review only warn (Claude Code's permission rules refuse it to Claude)"
)

// pushTarget is the branch a push may update: never main or a release branch, which change only
// when a person merges a pull request, and never a detached HEAD.
func pushTarget(branch string) (string, error) {
	switch {
	case branch == "":
		return "", usageError{"HEAD is detached: check out the branch to push"}
	case branch == mainBranch || strings.HasPrefix(branch, "release/"):
		return "", usageError{branch + " changes only when a person merges a pull request: push a work branch"}
	}

	return branch, nil
}

// push pushes the branch checked out to origin, under its own name. The pr gates must have
// passed at HEAD (make pr) and the adversarial review must be ready for HEAD, or have read the
// branch's commits before a rebase, or the push is refused. A person who says so (--person, which the
// permission rules of .claude/settings.json refuse to Claude in the form it writes) gets the same
// checks as warnings. A push keeps the review that let it ship (evidence.Keep), for the branch's
// next rebase.
//
// This stops mistakes and shortcuts, not a determined forger: the policy refuses an edit of the
// evidence or the review state, not a command that writes them. CI runs every gate again on the
// pull request, and a person merges it.
func (e env) push(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	force := fs.Bool("force", false, "push with --force-with-lease, after a rebase")
	person := fs.Bool("person", false, personUsage)

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	branch, err := pushTarget(gitx.Branch(ctx, e.root))
	if err != nil {
		return err
	}

	head := gitx.Head(ctx, e.root)

	shipped, err := e.shippable(ctx, out, head, gitx.GitDir(ctx, e.root), !*person)
	if err != nil {
		return err
	}

	gitArgs := []string{gitPush, "-u", pushRemote, "HEAD:refs/heads/" + branch}
	if *force {
		gitArgs = append(gitArgs, "--force-with-lease")
	}

	cmd := exec.CommandContext(ctx, "git", gitArgs...) // #nosec G204 -- fixed remote; the branch is the one checked out
	cmd.Dir = e.root
	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	err = e.keep(head, shipped)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	// This clone's notes ref goes with the branch: make pr's timings, for the dashboard, merged with
	// what origin holds under it. A notes push that fails is said; the branch is pushed all the same.
	if ref := evidence.NotesRef(ctx, e.root); ref != "" && gitx.Note(ctx, e.root, ref, head) != "" {
		err = gitx.PushNotes(ctx, e.root, pushRemote, ref)
		if err != nil {
			_, _ = fmt.Fprintln(out, "warning: the timings' notes are not pushed:", err)
		}
	}

	return nil
}

// shippable checks the evidence and the review of head (the review state is under gitDir) before
// a push: strict refuses, otherwise the problems are warnings. It returns the ready review that
// covers head, for the push to keep and record, or none.
func (e env) shippable(ctx context.Context, out io.Writer, head, gitDir string, strict bool) (covering, error) {
	ev, err := evidence.Read(e.root, head)
	if err != nil {
		return covering{}, fmt.Errorf("push: %w", err)
	}

	review, covers, err := e.reviewOf(ctx, out, gitDir, head)
	if err != nil {
		return covering{}, fmt.Errorf("push: %w", err)
	}

	gates := make([]string, 0, len(e.reg.At("pr")))
	for _, g := range e.reg.At("pr") {
		gates = append(gates, g.ID)
	}

	warnings, err := evidence.PushAllowed(head, ev, review, gates, strict)
	if err != nil {
		return covering{}, fmt.Errorf("push refused: %w (a person who means to push anyway passes --person)", err)
	}

	for _, w := range warnings {
		_, _ = fmt.Fprintln(out, "warning:", w)
	}

	return covers, nil
}

// keep records, once head is pushed, the review that let it ship, for the next rebase, and how it
// covers head in head's evidence, for the pull request's Gate evidence.
func (e env) keep(head string, shipped covering) error {
	if shipped.review == nil {
		return nil
	}

	err := evidence.Keep(e.root, *shipped.review)
	if err != nil {
		return fmt.Errorf("keep the review: %w", err)
	}

	if shipped.coverage == nil {
		return nil
	}

	ev, err := evidence.Read(e.root, head)
	if err != nil {
		return fmt.Errorf("record the review's coverage: %w", err)
	}

	// A person's push may have no evidence to record the coverage in.
	if ev == nil {
		return nil
	}

	ev.Coverage = shipped.coverage

	_, err = ev.Write(e.root)
	if err != nil {
		return fmt.Errorf("record the review's coverage: %w", err)
	}

	return nil
}

// covering is the ready review that lets head ship: as it was recorded, for the push to keep, and
// how it read the branch when it judged an older commit (nil: it judged head itself).
type covering struct {
	review   *evidence.Review
	coverage *evidence.Coverage
}

// reviewOf finds the review of head: the review state's when it judged head; else the ready
// review, the state's or one an earlier push kept, that leaves the fewest of the branch's commits
// unread (evidence.Cover, Coverage.Closer). A rebase asks for no new review: the commits it changed, and those added
// since, are listed for whoever merges. The first result is what PushAllowed judges.
func (e env) reviewOf(ctx context.Context, out io.Writer, gitDir, head string) (*evidence.Review, covering, error) {
	current, err := evidence.ReadReview(gitDir)
	if err != nil {
		return nil, covering{}, fmt.Errorf("find the review of %.7s: %w", head, err)
	}

	if current != nil && current.Head == head {
		if current.Ready() {
			return current, covering{review: current}, nil
		}

		return current, covering{}, nil
	}

	kept, err := evidence.Kept(e.root)
	if err != nil {
		return nil, covering{}, fmt.Errorf("find the review of %.7s: %w", head, err)
	}

	if current != nil {
		kept = append([]evidence.Review{*current}, kept...)
	}

	var best covering

	for _, r := range kept {
		c, ok := e.coverage(ctx, r, head)
		if ok && (best.coverage == nil || c.Closer(*best.coverage)) {
			found := r
			best = covering{review: &found, coverage: &c}
		}
	}

	if best.review == nil {
		return current, covering{}, nil
	}

	c := best.coverage
	_, _ = fmt.Fprintf(out, "the review of %.7s covers %.7s, which it did not judge: %d commit(s) as it read them, %d changed by the rebase, %d added since and not reviewed\n",
		best.review.Head, head, c.Replayed, len(c.Rebased), len(c.Added))

	for _, k := range c.Rebased {
		_, _ = fmt.Fprintf(out, "  changed by the rebase: %.7s %s\n", k.SHA, k.Subject)
	}

	for _, k := range c.Added {
		_, _ = fmt.Fprintf(out, "  added since the review: %.7s %s\n", k.SHA, k.Subject)
	}

	rebased := *best.review
	rebased.Head = head

	return &rebased, best, nil
}

// coverage pairs the commits a ready review read, from its merge base to the commit it judged,
// with head's commits since the review's base as it is now; false when the review is not ready,
// is incomplete, or read none of them.
func (e env) coverage(ctx context.Context, r evidence.Review, head string) (evidence.Coverage, bool) {
	if !r.Ready() || r.Base == "" || r.MergeBase == "" || r.Head == "" {
		return evidence.Coverage{}, false
	}

	c := evidence.Cover(gitx.Commits(ctx, e.root, r.MergeBase+".."+r.Head), gitx.Commits(ctx, e.root, gitx.Remote(ctx, e.root, r.Base)+".."+head))
	c.Review = r.Head

	return c, c.Covers()
}
