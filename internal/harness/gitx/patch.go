package gitx

import (
	"context"
	"os/exec"
	"strings"
)

// PatchID names a change by its content, whatever commit it sits on: the patch id of the diff of a
// revision range, "from..to", or "base...head" from their merge base, whitespace included (an
// indent changes YAML, a tab a Makefile). A rebase onto a newer base keeps it, unless the new base
// changes the lines the diff shows around the change. It is "" when git cannot tell (a commit
// missing) or the range changes nothing.
func PatchID(ctx context.Context, root, revs string) string {
	diff, err := run(ctx, root, "diff", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv", "--end-of-options", revs)
	if err != nil || diff == "" {
		return ""
	}

	cmd := exec.CommandContext(ctx, "git", "patch-id", "--verbatim") // #nosec G204 -- a fixed git subcommand
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(diff)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	id, _, _ := strings.Cut(strings.TrimSpace(string(out)), " ")

	return id
}
