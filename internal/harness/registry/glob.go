package registry

import (
	"path"
	"strings"
)

// Match reports whether the slash-separated path matches the pattern. Segments match as in
// path.Match, and a "**" segment matches any number of segments, none included: "docs/**"
// matches everything under docs/, "**/*_test.go" every test file, and "*.md" only Markdown
// files at the top level.
func Match(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			for i := 0; i <= len(name); i++ {
				if matchSegments(rest, name[i:]) {
					return true
				}
			}

			return false
		}

		if len(name) == 0 {
			return false
		}

		ok, err := path.Match(pat[0], name[0])
		if err != nil || !ok {
			return false
		}

		pat, name = pat[1:], name[1:]
	}

	return len(name) == 0
}

// MatchAny reports whether the path matches one of the patterns.
func MatchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if Match(p, name) {
			return true
		}
	}

	return false
}
