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
// passed at HEAD (make pr) and the adversarial review must be ready for HEAD. A person at a
// terminal gets the same checks as warnings; anyone else, an agent first, is refused.
//
// This stops an agent's mistakes and a subagent's shortcuts, not a person's main session: that
// session may run code the policy cannot read (python3), and so could write any file. CI runs
// every gate again on the pull request, and a person merges it.
func (e env) push(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	force := fs.Bool("force", false, "push with --force-with-lease, after a rebase")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	branch, err := pushTarget(gitx.Branch(ctx, e.root))
	if err != nil {
		return err
	}

	err = e.shippable(ctx, out)
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

// shippable checks the evidence and the review of HEAD before a push.
func (e env) shippable(ctx context.Context, out io.Writer) error {
	head := gitx.Head(ctx, e.root)

	ev, err := evidence.Read(e.root, head)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	review, err := evidence.ReadReview(gitx.GitDir(ctx, e.root))
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	gates := make([]string, 0, len(e.reg.At("pr")))
	for _, g := range e.reg.At("pr") {
		gates = append(gates, g.ID)
	}

	warnings, err := evidence.PushAllowed(head, ev, review, gates, !aPersonAtATerminal())
	if err != nil {
		return fmt.Errorf("push refused: %w", err)
	}

	for _, w := range warnings {
		_, _ = fmt.Fprintln(out, "warning:", w)
	}

	return nil
}

// aPersonAtATerminal reports whether a person runs the push: stdin is a terminal, and the process
// does not run under Claude Code (which sets CLAUDECODE, and gives its commands no terminal).
// The policy denies clearing CLAUDECODE, so an agent needs both to pass for a person.
func aPersonAtATerminal() bool {
	if os.Getenv("CLAUDECODE") != "" {
		return false
	}

	info, err := os.Stdin.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
