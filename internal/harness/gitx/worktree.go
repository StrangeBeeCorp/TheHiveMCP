package gitx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Scratch checks out rev in a new temporary worktree and returns its directory and its removal.
// It never touches the working tree of root.
func Scratch(ctx context.Context, root, rev string) (string, func(), error) {
	parent, err := os.MkdirTemp("", "harness-scratch-")
	if err != nil {
		return "", nil, fmt.Errorf("scratch directory: %w", err)
	}

	dir := filepath.Join(parent, "checkout")

	_, err = run(ctx, root, "worktree", "add", "--detach", "--quiet", dir, rev)
	if err != nil {
		_ = os.RemoveAll(parent)
		return "", nil, err
	}

	remove := func() {
		_, _ = run(context.WithoutCancel(ctx), root, "worktree", "remove", "--force", dir)
		_ = os.RemoveAll(parent)
	}

	return dir, remove, nil
}

// HasPath reports whether the file exists at the revision.
func HasPath(ctx context.Context, root, rev, path string) bool {
	_, err := run(ctx, root, "cat-file", "-e", rev+":"+path)
	return err == nil
}

// At is a file's content at the revision, and whether it exists there.
func At(ctx context.Context, root, rev, path string) ([]byte, bool) {
	out, err := run(ctx, root, "show", rev+":"+path)
	if err != nil {
		return nil, false
	}

	return []byte(out), true
}

// Apply applies a patch file to the working tree at dir.
func Apply(ctx context.Context, dir, patch string) error {
	_, err := run(ctx, dir, "apply", "--whitespace=nowarn", patch)
	return err
}
