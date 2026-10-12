package checks

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
)

// homePath finds a personal home directory: /Users/<name>/, /home/<name>/ or C:\Users\<name>\.
var homePath = regexp.MustCompile(`(?:/Users/|/home/|[A-Za-z]:\\Users\\)([A-Za-z][A-Za-z0-9._-]*)[/\\]`)

// PublicSafety checks that nothing personal reaches this public repository: no per-machine or
// secret-bearing file (by path) and no personal home directory (by content). Secrets themselves
// are the secrets gate's job (gitleaks).
func PublicSafety(tree Tree, rules SafetyRules) ([]Finding, error) {
	var out []Finding

	for _, f := range tree.Files() {
		if matchesFile(rules.ForbiddenPaths, f) && !matchesFile(rules.AllowedPaths, f) {
			out = append(out, Finding{Path: f, Msg: "a per-machine or secret-bearing file must not be in the repository: add it to .gitignore"})
			continue
		}

		if matchesFile(rules.SkipContent, f) {
			continue
		}

		content, err := tree.Read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("public-safety: %w", err)
		}

		if isBinary(content) {
			continue
		}

		out = append(out, homePaths(f, content, rules.HomeNames)...)
	}

	return out, nil
}

func homePaths(f string, content []byte, allowed []string) []Finding {
	var out []Finding

	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), len(content)+1)

	for line := 1; sc.Scan(); line++ {
		for _, m := range homePath.FindAllStringSubmatch(sc.Text(), -1) {
			if !slices.Contains(allowed, m[1]) {
				out = append(out, Finding{Path: f, Line: line, Msg: fmt.Sprintf("personal home path %q: use a path relative to the repository or a placeholder", m[0])})
			}
		}
	}

	return out
}
