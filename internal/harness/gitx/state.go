package gitx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Branch is the branch checked out, or "" when HEAD is detached.
func Branch(ctx context.Context, root string) string {
	out, err := run(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || strings.TrimSpace(out) == "HEAD" {
		return ""
	}

	return strings.TrimSpace(out)
}

// MainCheckout is the main working tree of the repository root belongs to: root itself, unless
// root is a linked worktree.
func MainCheckout(ctx context.Context, root string) string {
	out, err := run(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return root
	}

	return filepath.Dir(strings.TrimSpace(out))
}

// Ignored reports whether git ignores the path (relative to root): it never reaches a commit.
func Ignored(ctx context.Context, root, rel string) bool {
	_, err := run(ctx, root, "check-ignore", "-q", "--", rel)
	return err == nil
}

// Contents are the checkout's Files, each with the sha256 of its content. A tracked file deleted
// from the tree is not among them.
func Contents(ctx context.Context, root string) (map[string]string, error) {
	paths, err := Files(ctx, root)
	if err != nil {
		return nil, err
	}

	files := map[string]string{}

	for _, p := range paths {
		content, err := os.ReadFile(filepath.Join(root, p)) // #nosec G304 -- a file of this checkout
		if err != nil {
			continue
		}

		sum := sha256.Sum256(content)
		files[p] = hex.EncodeToString(sum[:])
	}

	return files, nil
}

// Fingerprint identifies what the gates would read: HEAD, the diff from it (staged and not), and
// the untracked files with their content. An equal fingerprint means nothing changed since.
func Fingerprint(ctx context.Context, root string) (string, error) {
	h := sha256.New()
	_, _ = io.WriteString(h, Head(ctx, root)+"\x00")

	diff, err := run(ctx, root, "diff", "HEAD", "--binary")
	if err != nil {
		return "", err
	}

	_, _ = io.WriteString(h, diff)

	out, err := run(ctx, root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}

	for _, p := range split(out) {
		content, err := os.ReadFile(filepath.Join(root, p)) // #nosec G304 -- an untracked file of this checkout
		if err != nil {
			continue
		}

		_, _ = fmt.Fprintf(h, "\x00%s\x00%x", p, sha256.Sum256(content))
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// How many changed files and commits a Summary shows.
const (
	summaryLines = 30
	logLines     = 8
)

// Summary is a short account of the checkout for an agent that resumes work: the changed files
// and the last commits.
func Summary(ctx context.Context, root string) string {
	status, _ := run(ctx, root, "status", "--short")
	log, _ := run(ctx, root, "log", "--oneline", fmt.Sprintf("-%d", logLines))

	return "Working tree:\n" + firstLines(status, summaryLines) + "\nRecent commits:\n" + firstLines(log, logLines)
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append(lines[:n], fmt.Sprintf("… %d more", len(lines)-n))
	}

	return strings.Join(lines, "\n")
}

// Clean reports whether the working tree has no change, staged or not, and no untracked file.
func Clean(ctx context.Context, root string) (bool, error) {
	out, err := run(ctx, root, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(out) == "", nil
}

// GitDir is the git directory of the checkout: .git, or the worktree's under it.
func GitDir(ctx context.Context, root string) string {
	out, err := run(ctx, root, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return filepath.Join(root, ".git")
	}

	return strings.TrimSpace(out)
}

// Resolve is the commit a ref names, or "" when it names none.
func Resolve(ctx context.Context, root, ref string) string {
	if ref == "" {
		return ""
	}

	out, err := run(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(out)
}
