package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

// pushRemote is the one remote anything is pushed to. The public mirror is a person's to update.
const pushRemote = "origin"

// push pushes the branch checked out to origin, under its own name. It never pushes main or
// another remote: main changes when a person merges a pull request.
func (e env) push(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	force := fs.Bool("force", false, "push with --force-with-lease, after a rebase")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	branch := gitx.Branch(ctx, e.root)

	switch {
	case branch == "":
		return usageError{"HEAD is detached: check out the branch to push"}
	case branch == "main" || strings.HasPrefix(branch, "release/"):
		return usageError{branch + " changes only when a person merges a pull request: push a work branch"}
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
