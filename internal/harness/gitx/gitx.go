// Package gitx asks git which files exist and which changed, for the harness's gate selection
// and its file checks.
package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed git subcommands; only refs and paths vary
	cmd.Dir = dir

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return string(out), nil
}

// split turns NUL-separated git output into sorted, unique paths.
func split(outs ...string) []string {
	seen := map[string]bool{}

	var paths []string

	for _, out := range outs {
		for p := range strings.SplitSeq(out, "\x00") {
			if p != "" && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}

	sort.Strings(paths)

	return paths
}

// Root is the top-level directory of the working tree that contains dir.
func Root(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

// Head is the commit checked out, or "" before the first commit.
func Head(ctx context.Context, root string) string {
	out, err := run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(out)
}

// Files lists the files the repository has or would have on the next `git add`: tracked files
// and untracked files that are not ignored.
func Files(ctx context.Context, root string) ([]string, error) {
	out, err := run(ctx, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	return split(out), nil
}

// Staged lists the paths the next commit changes, deletions included.
func Staged(ctx context.Context, root string) ([]string, error) {
	out, err := run(ctx, root, "diff", "-z", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}

	return split(out), nil
}

// ChangedSince lists the paths that differ between the merge base of base and HEAD and the
// working tree: what the branch has changed so far, committed or not, untracked files included.
func ChangedSince(ctx context.Context, root, base string) ([]string, error) {
	mb, err := run(ctx, root, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}

	diff, err := run(ctx, root, "diff", "-z", "--name-only", strings.TrimSpace(mb))
	if err != nil {
		return nil, err
	}

	untracked, err := run(ctx, root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	return split(diff, untracked), nil
}

// HasRef reports whether the ref resolves to a commit.
func HasRef(ctx context.Context, root, ref string) bool {
	_, err := run(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}
