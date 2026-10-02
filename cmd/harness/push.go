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
	// mainBranch changes only when a person merges a pull request.
	mainBranch = "main"
	// personUsage is the help of --person, the flag only a person types.
	personUsage = "a person pushes: missing evidence or review only warn (the policy refuses it to Claude)"
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
// passed at HEAD (make pr) and the adversarial review must be ready for HEAD, or for the same
// change before a rebase, or the push is refused. A person who says so (--person, which the
// policy refuses to every Claude session) gets the same checks as warnings. A push keeps the
// review that let it ship (evidence.Keep), for that change's next rebase.
//
// This stops an agent's mistakes and a subagent's shortcuts, not a determined forger. The policy
// refuses the usual ways to write the evidence and the review state (the editor, redirects,
// copies, archives, sort -o, curl -o), not every program that can write a file, and a main
// session may run code it cannot read (python3). CI runs every gate again on the pull request,
// and a person merges it.
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

	shipped, err := e.shippable(ctx, out, gitx.Head(ctx, e.root), gitx.GitDir(ctx, e.root), !*person)
	if err != nil {
		return err
	}

	gitArgs := []string{"push", "-u", pushRemote, "HEAD:refs/heads/" + branch}
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

	if shipped == nil {
		return nil
	}

	err = evidence.Keep(e.root, *shipped)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	return nil
}

// shippable checks the evidence and the review of head (the review state is under gitDir) before
// a push: strict refuses, otherwise the problems are warnings. It returns the ready review that
// covers head, for the push to keep, or nil.
func (e env) shippable(ctx context.Context, out io.Writer, head, gitDir string, strict bool) (*evidence.Review, error) {
	ev, err := evidence.Read(e.root, head)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}

	review, covering, err := e.reviewOf(ctx, out, gitDir, head)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}

	gates := make([]string, 0, len(e.reg.At("pr")))
	for _, g := range e.reg.At("pr") {
		gates = append(gates, g.ID)
	}

	warnings, err := evidence.PushAllowed(head, ev, review, gates, strict)
	if err != nil {
		return nil, fmt.Errorf("push refused: %w (a person who means to push anyway passes --person)", err)
	}

	for _, w := range warnings {
		_, _ = fmt.Fprintln(out, "warning:", w)
	}

	return covering, nil
}

// reviewOf finds the review of head: the review state's when it judged head; else a ready
// review, the state's or one an earlier push kept, that judged the same change on an older base,
// which a rebase leaves as it was (gitx.PatchID). The first result is what PushAllowed judges;
// the second is the covering review as it was recorded, or nil when none covers head.
func (e env) reviewOf(ctx context.Context, out io.Writer, gitDir, head string) (*evidence.Review, *evidence.Review, error) {
	current, err := evidence.ReadReview(gitDir)
	if err != nil {
		return nil, nil, fmt.Errorf("find the review of %.7s: %w", head, err)
	}

	if current != nil && current.Head == head {
		if current.Ready() {
			return current, current, nil
		}

		return current, nil, nil
	}

	kept, err := evidence.Kept(e.root)
	if err != nil {
		return nil, nil, fmt.Errorf("find the review of %.7s: %w", head, err)
	}

	if current != nil {
		kept = append([]evidence.Review{*current}, kept...)
	}

	for _, r := range kept {
		if r.Ready() && e.sameChange(ctx, r, head) {
			_, _ = fmt.Fprintf(out, "the review of %.7s covers %.7s: the same change, rebased onto %s\n", r.Head, head, r.Base)

			found, rebased := r, r
			rebased.Head = head

			return &rebased, &found, nil
		}
	}

	return current, nil, nil
}

// sameChange reports whether head, compared with the review's base as it is now, makes the change
// the review read: from its merge base to the commit it judged.
func (e env) sameChange(ctx context.Context, r evidence.Review, head string) bool {
	if r.Base == "" || r.MergeBase == "" || r.Head == "" {
		return false
	}

	judged := gitx.PatchID(ctx, e.root, r.MergeBase+".."+r.Head)

	return judged != "" && judged == gitx.PatchID(ctx, e.root, r.Base+"..."+head)
}
