package change

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const shortSHA = 7

func short(sha string) string {
	return sha[:min(len(sha), shortSHA)]
}

// sha is a full commit id, as make pr records its base.
var sha = regexp.MustCompile(`^[0-9a-f]{40}$`)

// heading is the title of the section.
const heading = "### Change metrics\n\n"

// Markdown renders the metrics as the "Change metrics" section of a pull request.
func Markdown(m Metrics) string {
	var b strings.Builder

	against := m.Against
	if sha.MatchString(against) {
		against = short(against)
	}

	fmt.Fprintf(&b, heading+"The change from `%s` (its merge base with `%s`) to `%s`.\n\n", short(m.From), against, short(m.Head))
	b.WriteString("| Metric | This change |\n|---|---|\n")
	fmt.Fprintf(&b, "| Diff coverage | %s |\n", coverageCell(m.Coverage))
	fmt.Fprintf(&b, "| `//nolint` added | %d |\n", m.Markers.Nolint)
	fmt.Fprintf(&b, "| TODO comments added | %d |\n", m.Markers.TODO)
	fmt.Fprintf(&b, "| Dependencies added | %s |\n\n", depsCell(m.Deps))

	if len(m.Moves) == 0 {
		b.WriteString("No code file it lengthens ends within a quarter of its size limit.\n\n")
		return b.String()
	}

	b.WriteString("Code files it lengthens to within a quarter of their size limit (the cap in `gates/limits.yaml`, else `max_lines`):\n\n")
	b.WriteString("| File | Base | Branch | Limit |\n|---|---:|---:|---:|\n")

	for _, mv := range m.Moves {
		before := "new"
		if !mv.New {
			before = strconv.Itoa(mv.Before)
		}

		fmt.Fprintf(&b, "| `%s` | %s | %d | %d |\n", mv.Path, before, mv.After, mv.Limit)
	}

	b.WriteString("\n")

	return b.String()
}

// Unmeasured is the section when the change could not be measured: it says why.
func Unmeasured(err error) string {
	return heading + "Not measured: " + err.Error() + ".\n\n"
}

func coverageCell(c *Coverage) string {
	if c == nil {
		return "not measured: `make pr` kept no coverage profile for this commit"
	}

	var cell string

	if c.Statements == 0 {
		cell = "no changed statement in the profile"
	} else {
		cell = fmt.Sprintf("**%.1f %%**: the unit tests ran %d of the %d statements the change touches", c.Percent(), c.Covered, c.Statements)
	}

	if len(c.Unmeasured) > 0 {
		cell += fmt.Sprintf("; %d changed Go file(s) have no statement in the profile: %s", len(c.Unmeasured), codeList(c.Unmeasured))
	}

	return cell
}

func depsCell(deps []string) string {
	if len(deps) == 0 {
		return "none"
	}

	return codeList(deps)
}

func codeList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = "`" + it + "`"
	}

	return strings.Join(quoted, ", ")
}
