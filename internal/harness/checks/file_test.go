package checks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
)

const (
	formatted   = "package pkg\n\nfunc F() {}\n"
	unformatted = "package pkg\nfunc  F( ) {}\n"
	broken      = "package pkg\nfunc F( {\n"
)

func oneFile(t *testing.T, builtin, rel, content string) string {
	t.Helper()

	lim := checks.Limits{
		Structure:    limits,
		PublicSafety: checks.SafetyRules{ForbiddenPaths: []string{"*.key"}, HomeNames: []string{"example"}},
	}

	found, err := checks.OneFile(builtin, rel, []byte(content), lim)
	require.NoError(t, err)

	return messages(found)
}

func TestOneFileJudgesTheEditedFileAlone(t *testing.T) {
	t.Parallel()

	assert.Contains(t, oneFile(t, "structure", "newdir/x.go", formatted), `top-level entry "newdir"`)
	assert.Contains(t, oneFile(t, "structure", "long.go", lines(11)), "over the limit of 10")
	assert.Contains(t, oneFile(t, "structure", bigGo, lines(21)), "over its cap of 20")
	assert.Empty(t, oneFile(t, "structure", shrunkGo, lines(12)), "lowering a cap is not the editing agent's call")

	assert.Contains(t, oneFile(t, "go-source", "pkg/a.go", broken), "does not parse")
	assert.Contains(t, oneFile(t, "go-source", "pkg/a.go", unformatted), "gofmt would rewrite it")
	assert.Empty(t, oneFile(t, "go-source", "pkg/a.go", formatted))
	assert.Empty(t, oneFile(t, "go-source", "README.md", unformatted), "only Go source is parsed")

	assert.Contains(t, oneFile(t, "public-safety", "pkg/id.key", "x"), "must not be in the repository")
	assert.Contains(t, oneFile(t, "public-safety", "pkg/a.go", "// see /Users/"+"jdoe/notes\n"), "personal home path")

	_, err := checks.OneFile("docs", "README.md", nil, checks.Limits{})
	assert.Error(t, err, "a check that needs the whole tree says so")
}

func TestGoSourceReadsEveryGoFile(t *testing.T) {
	t.Parallel()

	found, err := checks.GoSource(checks.MemTree{"a.go": []byte(formatted), "b.go": []byte(unformatted), "c.md": []byte(unformatted)})
	require.NoError(t, err)
	assert.Equal(t, "b.go: gofmt would rewrite it: run make fmt", messages(found))
}

func TestLimitsNeedARedRoundBound(t *testing.T) {
	t.Parallel()

	_, err := checks.ParseLimits([]byte("hooks:\n  max_red_rounds: 0\n"), "limits.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_red_rounds")

	lim, err := checks.ParseLimits([]byte("hooks:\n  max_red_rounds: 6\n"), "limits.yaml")
	require.NoError(t, err)
	assert.Equal(t, 6, lim.Hooks.MaxRedRounds)
}

// A negative max_kb would switch the size check off without a word: it is refused.
func TestLimitsRefuseANegativeFileSize(t *testing.T) {
	t.Parallel()

	_, err := checks.ParseLimits([]byte("structure:\n  max_kb: -1\nhooks:\n  max_red_rounds: 6\n"), "limits.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_kb")
}
