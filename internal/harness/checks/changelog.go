package checks

import "strings"

// ChangelogPath is the changelog a change to what the model reads gets an entry in.
const ChangelogPath = changelogPath

// UnreleasedAdded reports whether the ## [Unreleased] section of after holds an entry that the
// section of before does not: a line of text, not a ### heading or a blank line.
func UnreleasedAdded(before, after string) bool {
	had := map[string]bool{}
	for _, line := range unreleased(before) {
		had[line] = true
	}

	for _, line := range unreleased(after) {
		if !had[line] {
			return true
		}
	}

	return false
}

// unreleased is the entries of the ## [Unreleased] section of a Keep a Changelog file.
func unreleased(text string) []string {
	var (
		lines []string
		in    bool
	)

	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "## "):
			in = strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), "[Unreleased]")
		case in && trimmed != "" && !strings.HasPrefix(trimmed, "#"):
			lines = append(lines, trimmed)
		}
	}

	return lines
}
