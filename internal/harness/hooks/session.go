package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/policy"
)

var ticket = regexp.MustCompile(`DL-[0-9]+`)

// How much of a report or of BLOCKED.md the session start repeats.
const (
	reportLines  = 20
	blockedLines = 15
)

const rules = `The rules: every stop runs make check-fast (docs/reference/gates.md), and an agent does not stop red.
The Tester writes the tests and the Coder the code (.claude/agents/); the implement-task workflow runs their duel.
Ship with make pr: the pr gates at HEAD, recorded, then the push (after the review; never main); a person merges. The hooks deny what breaks
these rules, and say what to do instead.`

// hooksDir is where make dev-setup points git for its hooks: the commit gates.
const hooksDir = ".githooks"

// SessionStart tells the agent where it is: the checkout and its ticket, whether its commits run
// the commit gates, the last check-fast, a BLOCKED report, and what was saved before a compaction.
func SessionStart(ctx context.Context, root string, in Input) Output {
	var b strings.Builder

	branch := gitx.Branch(ctx, root)
	head := gitx.Head(ctx, root)

	kind := "the main checkout"
	if main := gitx.MainCheckout(ctx, root); filepath.Clean(main) != filepath.Clean(root) {
		kind = "a worktree of " + main
	}

	fmt.Fprintf(&b, "TheHiveMCP harness. Checkout %s (%s), branch %s at %.10s", root, kind, orNone(branch), head)

	if t := ticket.FindString(branch); t != "" {
		fmt.Fprintf(&b, ", ticket %s", t)
	}

	b.WriteString(".\n")

	if gitx.HooksPath(ctx, root) != hooksDir {
		b.WriteString("Commits here do not run the harness's commit gates (core.hooksPath is not " + hooksDir + "): run make dev-setup.\n")
	}

	st := load(root)

	switch {
	case st.Last == nil:
		b.WriteString("No check-fast has run in this checkout yet.\n")
	case st.Last.Red:
		fmt.Fprintf(&b, "The last check-fast (%s) was red:\n%s\n", st.Last.At.Format(time.DateTime), firstLines(st.Last.Report, reportLines))
	case st.Last.Incomplete:
		fmt.Fprintf(&b, "The last check-fast (%s) could not run every gate, so it is not a pass:\n%s\n",
			st.Last.At.Format(time.DateTime), firstLines(st.Last.Report, reportLines))
	case st.Last.Static:
		fmt.Fprintf(&b, "The last check-fast (%s) ran the static gates only, the Tester's, and they were green.\n", st.Last.At.Format(time.DateTime))
	default:
		fmt.Fprintf(&b, "The last check-fast (%s) was green.\n", st.Last.At.Format(time.DateTime))
	}

	blocked, err := os.ReadFile(filepath.Join(root, policy.BlockedFile)) // #nosec G304 -- this checkout's report
	if err == nil && len(strings.TrimSpace(string(blocked))) > 0 {
		fmt.Fprintf(&b, "This task is BLOCKED (%s):\n%s\n", policy.BlockedFile, firstLines(string(blocked), blockedLines))
	}

	if in.Source == "compact" && st.Progress != "" {
		fmt.Fprintf(&b, "Saved before the compaction:\n%s\n", st.Progress)
	}

	b.WriteString(rules)

	return jsonOutput(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     "SessionStart",
		"additionalContext": b.String(),
	}})
}

// PreCompact saves where the work stands, for SessionStart to give back after the compaction.
func PreCompact(ctx context.Context, root string) Output {
	progress := gitx.Summary(ctx, root)

	err := update(root, func(st *State) error {
		if st.Last != nil && st.Last.Red {
			progress += "\nRed at the last check-fast:\n" + firstLines(st.Last.Report, blockedLines)
		}

		st.Progress = progress

		return nil
	})
	if err != nil {
		return note("harness: could not save the progress before the compaction: " + err.Error())
	}

	return Output{}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}

	return s
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append(lines[:n], fmt.Sprintf("… %d more lines", len(lines)-n))
	}

	return strings.Join(lines, "\n")
}
