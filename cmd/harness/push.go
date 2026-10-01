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
// passed at HEAD (make pr) and the adversarial review must be ready for HEAD, or the push is
// refused. A person who says so (--person, which the policy refuses to every Claude session) gets
// the same checks as warnings.
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

	err = e.shippable(out, gitx.Head(ctx, e.root), gitx.GitDir(ctx, e.root), !*person)
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

	return nil
}

// shippable checks the evidence and the review of head (the review state is under gitDir) before
// a push: strict refuses, otherwise the problems are warnings.
func (e env) shippable(out io.Writer, head, gitDir string, strict bool) error {
	ev, err := evidence.Read(e.root, head)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	review, err := evidence.ReadReview(gitDir)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	gates := make([]string, 0, len(e.reg.At("pr")))
	for _, g := range e.reg.At("pr") {
		gates = append(gates, g.ID)
	}

	warnings, err := evidence.PushAllowed(head, ev, review, gates, strict)
	if err != nil {
		return fmt.Errorf("push refused: %w (a person who means to push anyway passes --person)", err)
	}

	for _, w := range warnings {
		_, _ = fmt.Fprintln(out, "warning:", w)
	}

	return nil
}
