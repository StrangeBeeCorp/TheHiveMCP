// Package gatesdoc renders docs/reference/gates.md from the gate registry. The output is laid out
// the way prettier formats Markdown tables, so the fmt gate leaves the generated file alone.
package gatesdoc

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// Path is where the generated page lives, relative to the repository root.
const Path = "docs/reference/gates.md"

// Render returns the generated page.
func Render(reg *registry.Registry) string {
	var b strings.Builder
	b.WriteString("# Gates\n\n")
	b.WriteString("<!-- Generated from gates/registry.yaml by `make harness-docs`. Do not edit: the docs gate fails when this page drifts. -->\n\n")
	b.WriteString(Wrap("A gate is one check with a verdict. The changed paths select suites, and a gate runs at its cadences when one of its suites is "+
		"selected (`*`: whatever changed). A path that no suite claims selects every suite. Run one gate with `make gate ID=<gate>`. "+
		"To take a change through the gates, see [Work with the harness](../how-to/work-with-the-harness.md); to act on a vulnerability, see "+
		"[Handle a CVE](../how-to/handle-a-cve.md).") + "\n\n")
	b.WriteString(cadenceTable())
	b.WriteString("\n## Every gate\n\n")

	rows := make([][]string, 0, len(reg.Gates))
	for _, g := range reg.Gates {
		rows = append(rows, []string{code(g.ID), strings.Join(g.Cadence, ", "), codes(g.Suites), what(g), fmt.Sprintf("%d s", g.BudgetS), oneLine(g.Blocks)})
	}

	b.WriteString(Table([]string{"Gate", "Cadence", "Suites", "Runs", "Budget", "Blocks when"}, rows))
	b.WriteString("\n## Suites\n\n")

	rows = rows[:0]
	for _, s := range reg.Suites {
		rows = append(rows, []string{code(s.ID), oneLine(s.About), codes(s.Paths), codes(s.Implies)})
	}

	b.WriteString(Table([]string{"Suite", "Covers", "Paths", "Implies"}, rows))

	return b.String()
}

func cadenceTable() string {
	when := map[string]string{
		"edit":    "an agent edits a file (Claude Code hook)",
		"stop":    "an agent stops, or `make check-fast`",
		"commit":  "the git pre-commit hook, `make pre-commit`, on the staged files",
		"pr":      "before a pull request is opened",
		"ci":      "GitHub Actions, on every pull request and every push to `main`",
		"nightly": "the nightly run",
	}

	rows := make([][]string, 0, len(registry.Cadences))
	for _, c := range registry.Cadences {
		rows = append(rows, []string{code(c), when[c]})
	}

	return Table([]string{"Cadence", "When"}, rows)
}

func what(g registry.Gate) string {
	if g.Builtin != "" {
		return "built into the harness"
	}

	return code(g.Runs)
}

func code(s string) string {
	if s == "" {
		return ""
	}

	return "`" + s + "`"
}

func codes(items []string) string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = code(s)
	}

	return strings.Join(out, " ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// width is the line width prettier wraps prose at in this repository (.prettierrc.json).
const width = 160

// Wrap fills a paragraph greedily up to the prettier width, never breaking inside a code span.
func Wrap(text string) string {
	var lines []string

	line := ""

	for _, w := range words(text) {
		switch {
		case line == "":
			line = w
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(w) <= width:
			line += " " + w
		default:
			lines = append(lines, line)
			line = w
		}
	}

	if line != "" {
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// words splits on spaces outside code spans and link texts, as prettier keeps both whole.
func words(text string) []string {
	var (
		out []string
		cur strings.Builder
	)

	inCode, inLink := false, false

	for _, r := range text {
		switch {
		case r == '[' && !inCode:
			inLink = true

			cur.WriteRune(r)
		case r == ']' && !inCode:
			inLink = false

			cur.WriteRune(r)
		case r == '`':
			inCode = !inCode

			cur.WriteRune(r)
		case r == ' ' && !inCode && !inLink:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}

	if cur.Len() > 0 {
		out = append(out, cur.String())
	}

	return out
}

// Table renders a Markdown table with every column padded to its widest cell, as prettier does.
func Table(header []string, rows [][]string) string {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = max(utf8.RuneCountInString(h), len("---"))
	}

	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(escape(c)))
		}
	}

	var b strings.Builder

	line := func(cells []string) {
		b.WriteString("|")

		for i, c := range cells {
			c = escape(c)
			b.WriteString(" " + c + strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c)) + " |")
		}

		b.WriteString("\n")
	}
	line(header)

	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = strings.Repeat("-", widths[i])
	}

	line(sep)

	for _, r := range rows {
		line(r)
	}

	return b.String()
}

func escape(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
