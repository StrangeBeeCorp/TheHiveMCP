package gitx

import (
	"context"
	"fmt"
	"os"
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

// noteIdentity signs the notes' own commits: they are the harness's record, not a person's work,
// and need no identity configured where they are written (a CI checkout has none).
var noteIdentity = []string{
	"GIT_AUTHOR_NAME=TheHiveMCP harness", "GIT_AUTHOR_EMAIL=harness@localhost",
	"GIT_COMMITTER_NAME=TheHiveMCP harness", "GIT_COMMITTER_EMAIL=harness@localhost",
}

// AddNote attaches data to commit as a git note under ref (refs/notes/...), replacing a note the
// commit had there.
func AddNote(ctx context.Context, root, ref, commit string, data []byte) error {
	cmd := exec.CommandContext(ctx, "git", "notes", "--ref", ref, "add", "-f", "-F", "-", commit) // #nosec G204 -- a fixed git subcommand on the harness's own ref
	cmd.Dir = root

	cmd.Env = append(os.Environ(), noteIdentity...)
	cmd.Stdin = strings.NewReader(string(data))

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git notes add: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// Note is the note commit carries under ref, or "" for none.
func Note(ctx context.Context, root, ref, commit string) string {
	out, err := run(ctx, root, "notes", "--ref", ref, "show", commit)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(out)
}

// PushRef pushes ref to the same name on remote.
func PushRef(ctx context.Context, root, remote, ref string) error {
	_, err := run(ctx, root, "push", "-q", remote, ref+":"+ref)

	return err
}

// Commit is one commit of a change. Author is its author's email and date, which a rebase keeps;
// PatchID is that of its own diff, which a rebase keeps unless it resolves a conflict in it.
type Commit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	PatchID string `json:"patch_id"`
}

// unitSep separates the fields of git log's format: no subject holds it.
const unitSep = "\x1f"

// Commits lists the commits of a revision range, "base..head", oldest first, merges left out. It
// is nil when the range is empty or git cannot read it.
func Commits(ctx context.Context, root, revs string) []Commit {
	log, err := run(ctx, root, "log", "--reverse", "--no-merges", "--format=%H"+unitSep+"%s"+unitSep+"%ae %aI", "--end-of-options", revs)
	log = strings.TrimSpace(log)

	if err != nil || log == "" {
		return nil
	}

	var commits []Commit

	for line := range strings.SplitSeq(log, "\n") {
		f := strings.Split(line, unitSep)
		if len(f) != 3 { //nolint:mnd // the format's three fields
			continue
		}

		commits = append(commits, Commit{SHA: f[0], Subject: f[1], Author: f[2], PatchID: PatchID(ctx, root, f[0]+"^.."+f[0])})
	}

	return commits
}
