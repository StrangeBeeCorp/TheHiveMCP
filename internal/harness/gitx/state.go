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
