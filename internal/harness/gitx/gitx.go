// Package gitx asks git which files exist and which changed, for the harness's gate selection
// and its file checks.
package gitx

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// nameOnly lists paths, not changes, and noRenames lists both paths of a rename.
const (
	nameOnly  = "--name-only"
	noRenames = "--no-renames"
)

// diffNames is a git diff that lists the paths it changes, NUL-separated.
func diffNames(args ...string) []string {
	return append([]string{"diff", "-z", nameOnly, noRenames}, args...)
}

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

// Staged lists the paths the next commit changes, deletions included, and both paths of a rename.
func Staged(ctx context.Context, root string) ([]string, error) {
	out, err := run(ctx, root, diffNames("--cached")...)
	if err != nil {
		return nil, err
	}

	return split(out), nil
}

// ChangedSince lists the paths that differ between the merge base of base and HEAD and the
// working tree: what the branch has changed so far, committed or not, untracked files included.
// A rename lists both paths, so the suite that owned the old one is selected too.
func ChangedSince(ctx context.Context, root, base string) ([]string, error) {
	mb, err := run(ctx, root, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}

	diff, err := run(ctx, root, diffNames(strings.TrimSpace(mb))...)
	if err != nil {
		return nil, err
	}

	untracked, err := run(ctx, root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	return split(diff, untracked), nil
}

// Uncommitted lists the paths the working tree and the index change against HEAD, and the
// untracked files that are not ignored: the work not committed yet.
func Uncommitted(ctx context.Context, root string) ([]string, error) {
	var outs []string

	for _, args := range [][]string{
		diffNames(),
		diffNames("--cached"),
		{"ls-files", "-z", "--others", "--exclude-standard"},
	} {
		out, err := run(ctx, root, args...)
		if err != nil {
			return nil, err
		}

		outs = append(outs, out)
	}

	return split(outs...), nil
}

// Committed lists the paths a commit changed against its first parent: a merge's are what it
// brought in, a root commit's every file it added.
func Committed(ctx context.Context, root, rev string) ([]string, error) {
	args := []string{"diff-tree", "-z", "-r", nameOnly, noRenames, "--no-commit-id", "--root", rev}
	if parent := Resolve(ctx, root, rev+"^1"); parent != "" {
		args = diffNames(parent, rev)
	}

	out, err := run(ctx, root, args...)
	if err != nil {
		return nil, err
	}

	return split(out), nil
}

// Indexed is a file as the index holds it: what the next commit carries.
func Indexed(ctx context.Context, root, path string) ([]byte, error) {
	out, err := run(ctx, root, "show", ":"+path)
	return []byte(out), err
}

// Remote is the ref a remote-tracking name like origin/main names, written out
// (refs/remotes/origin/main) when the remote has it: a tag or a branch spelt origin/main would
// otherwise stand in for it, since git looks names up in refs/tags and refs/heads first. Any
// other ref is returned as it is.
func Remote(ctx context.Context, root, ref string) string {
	full := "refs/remotes/" + ref
	if strings.HasPrefix(ref, "refs/") || ref == "HEAD" || !HasRef(ctx, root, full) {
		return ref
	}

	return full
}

// MergeBase is the merge base of ref and HEAD, or "" when there is none.
func MergeBase(ctx context.Context, root, ref string) string {
	out, err := run(ctx, root, "merge-base", ref, "HEAD")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(out)
}

// HasRef reports whether the ref resolves to a commit.
func HasRef(ctx context.Context, root, ref string) bool {
	_, err := run(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// indexFields is the number of fields of an `ls-files --stage` entry before the tab, and of a
// `cat-file --batch` header.
const indexFields = 3

// Index returns the content of every regular file in the index: exactly what the next commit
// carries, whatever the working tree holds. Symbolic links and submodules are left out.
func Index(ctx context.Context, root string) (map[string][]byte, error) {
	listing, err := run(ctx, root, "ls-files", "-z", "--stage")
	if err != nil {
		return nil, err
	}

	var names, blobs []string

	for entry := range strings.SplitSeq(listing, "\x00") {
		// "<mode> <blob> <stage>\t<path>"
		meta, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)

		if !ok || len(fields) != indexFields || !strings.HasPrefix(fields[0], "100") {
			continue
		}

		names = append(names, name)
		blobs = append(blobs, fields[1])
	}

	return readBlobs(ctx, root, names, blobs)
}

// readBlobs reads the blobs in one `cat-file --batch` process.
func readBlobs(ctx context.Context, root string, names, blobs []string) (map[string][]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch") // #nosec G204 -- a fixed git subcommand
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(blobs, "\n") + "\n")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch: %w", err)
	}

	files := make(map[string][]byte, len(names))
	r := bufio.NewReader(bytes.NewReader(out))

	for _, name := range names {
		content, err := nextBlob(r)
		if err != nil {
			return nil, fmt.Errorf("read %s from the index: %w", name, err)
		}

		files[name] = content
	}

	return files, nil
}

// nextBlob reads one "<blob> <type> <size>\n<content>\n" record of `cat-file --batch`.
func nextBlob(r *bufio.Reader) ([]byte, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}

	fields := strings.Fields(header)
	if len(fields) != indexFields {
		return nil, fmt.Errorf("unexpected header %q", strings.TrimSpace(header))
	}

	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("size in %q: %w", strings.TrimSpace(header), err)
	}

	content := make([]byte, size+1) // the content, then a newline

	_, err = io.ReadFull(r, content)
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}

	return content[:size], nil
}
