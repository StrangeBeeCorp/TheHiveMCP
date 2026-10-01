package evidence

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// surfacePrefix is the snapshot of what the model reads: a change there is model-facing.
const surfacePrefix = "baselines/surface/"

// Flag is a protected part of the repository the change touches, with the paths it touches.
type Flag struct {
	Class string
	Paths []string
}

// Flags groups the changed paths by the protected class they belong to (classOf decides), in a
// stable order.
func Flags(changed []string, classOf func(string) (string, bool)) []Flag {
	byClass := map[string][]string{}

	for _, p := range changed {
		if class, ok := classOf(p); ok {
			byClass[class] = append(byClass[class], p)
		}
	}

	flags := make([]Flag, 0, len(byClass))
	for class, paths := range byClass {
		sort.Strings(paths)
		flags = append(flags, Flag{Class: class, Paths: paths})
	}

	sort.Slice(flags, func(i, j int) bool { return flags[i].Class < flags[j].Class })

	return flags
}

// ModelFacing reports whether the change touches what the model reads: the surface snapshot,
// unless the change creates it.
func (ev Evidence) ModelFacing() bool {
	if ev.FirstSnapshot {
		return false
	}

	for _, p := range ev.Changed {
		if strings.HasPrefix(p, surfacePrefix) {
			return true
		}
	}

	return false
}

var marks = map[runner.Verdict]string{
	runner.Pass: "✅", runner.Fail: "❌", runner.Unavailable: "⚠️", runner.Skipped: "⏭️", runner.Incomplete: "⚠️",
}

// Markdown renders the evidence as the "Gate evidence" section of a pull request: the verdict at
// the exact commit, each gate, the protected parts the change touches, and whether it is
// model-facing.
func Markdown(ev Evidence, flags []Flag) string {
	var b strings.Builder

	took := ev.Finished.Sub(ev.Started).Round(time.Second)
	fmt.Fprintf(&b, "### Gate evidence\n\n`make pr` at `%s` (`%s`, compared with `%s`): **%s**, %d gates in %s.\n\n",
		short(ev.Head), ev.Branch, short(ev.Base), ev.Verdict, ran(ev.Results), took)

	b.WriteString("| Gate | Verdict | Time | Detail |\n|---|---|---|---|\n")

	for _, r := range ev.Results {
		fmt.Fprintf(&b, "| %s | %s %s | %.1f s | %s |\n", r.Gate, marks[r.Verdict], r.Verdict, r.Seconds, cell(r.Summary))
	}

	if len(flags) > 0 {
		b.WriteString("\n> [!CAUTION]\n> **This change touches what only a person may change.** Read these files closely:\n")

		for _, f := range flags {
			fmt.Fprintf(&b, "> - %s: %s\n", f.Class, codeList(f.Paths))
		}
	}

	switch {
	case ev.Base == "":
		b.WriteString("\n**Model-facing: unknown.** There was no base to compare with, so the changed paths are not known: " +
			"check `baselines/surface/` and the protected parts by hand.\n")
	case ev.ModelFacing():
		b.WriteString("\n**Model-facing: yes.** `baselines/surface/` changed: tick the model-facing box of the release impact.\n")
	case ev.FirstSnapshot:
		b.WriteString("\n**Model-facing: no.** The base had no `baselines/surface/`: this change creates the first snapshot of " +
			"what the server already advertised.\n")
	default:
		b.WriteString("\n**Model-facing: no.** `baselines/surface/` did not change.\n")
	}

	return b.String()
}

func ran(results []runner.Result) int {
	n := 0

	for _, r := range results {
		if r.Verdict != runner.Skipped {
			n++
		}
	}

	return n
}

// cell makes a summary fit a table cell: one line, no pipe, at most 120 characters.
// colours are a terminal's colour and style codes, which a gate's summary may carry.
var colours = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func cell(s string) string {
	const width = 120

	s = colours.ReplaceAllString(s, "")
	s = strings.ReplaceAll(strings.SplitN(s, "\n", 2)[0], "|", "\\|")

	if r := []rune(s); len(r) > width {
		s = string(r[:width-1]) + "…"
	}

	return s
}

func codeList(paths []string) string {
	const shown = 6

	quoted := make([]string, 0, shown+1)

	for i, p := range paths {
		if i == shown {
			quoted = append(quoted, fmt.Sprintf("and %d more", len(paths)-shown))
			break
		}

		quoted = append(quoted, "`"+p+"`")
	}

	return strings.Join(quoted, ", ")
}
