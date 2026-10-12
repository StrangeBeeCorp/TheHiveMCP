package checks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoFile reads a file of the repository, from this package's directory.
func repoFile(t *testing.T, rel string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", rel)) // #nosec G304 -- a fixed path of this repository
	require.NoError(t, err)

	return data
}

// literalFile is the file a managerFilePatterns entry names when it names one file, such as
// "/^Makefile$/" or "/^internal/harness/lint/images\.go$/"; ok is false for a pattern over many.
func literalFile(pattern string) (string, bool) {
	inner, found := strings.CutPrefix(pattern, "/^")
	if !found {
		return "", false
	}

	inner, found = strings.CutSuffix(inner, "$/")
	if !found {
		return "", false
	}

	inner = strings.ReplaceAll(inner, `\.`, ".")
	if strings.ContainsAny(inner, `\*+?()[]{}|`) {
		return "", false
	}

	return inner, true
}

// Each Renovate manager that tracks a version in one named file finds that version there: a
// renamed variable or an edited regex would otherwise leave the tool pinned, with no pull request
// and no error.
func TestEachRenovateManagerFindsTheVersionItTracks(t *testing.T) {
	t.Parallel()

	var config struct {
		Managers []struct {
			Description string   `json:"description"`
			Files       []string `json:"managerFilePatterns"`
			Match       []string `json:"matchStrings"`
		} `json:"customManagers"`
	}

	require.NoError(t, json.Unmarshal(repoFile(t, "renovate.json"), &config))

	checked := 0

	for _, m := range config.Managers {
		for _, pattern := range m.Files {
			file, ok := literalFile(pattern)
			if !ok {
				continue
			}

			content := repoFile(t, file)

			for _, ms := range m.Match {
				re, err := regexp.Compile(strings.ReplaceAll(ms, "(?<", "(?P<"))
				require.NoError(t, err, m.Description)

				match := re.FindSubmatch(content)
				require.NotNil(t, match, "%s: %q matches nothing in %s", m.Description, ms, file)
				assert.NotEmpty(t, match[re.SubexpIndex("currentValue")], "%s: no version in %s", m.Description, file)

				checked++
			}
		}
	}

	assert.GreaterOrEqual(t, checked, 5, "the Makefile's and images.go's managers were checked")
}
