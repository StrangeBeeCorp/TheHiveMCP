package checks_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gatesdoc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

// repo writes the files into a temporary directory and returns it with their sorted names.
func repo(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()

	names := make([]string, 0, len(files))
	for name, content := range files {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))

		names = append(names, name)
	}

	sort.Strings(names)

	return root, names
}

func lines(n int) string {
	return strings.Repeat("x\n", n)
}

func messages(found []checks.Finding) string {
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.String()
	}

	return strings.Join(out, "\n")
}

const (
	makefile = "Makefile"
	bigGo    = "big.go"
	goneGo   = "gone.go"
	shrunkGo = "shrunk.go"
	smallGo  = "small.go"
)

var limits = checks.StructureLimits{
	MaxLines:           10,
	MaxCodeFilesPerDir: 5,
	CodeFiles:          []string{"*.go", makefile, "bin/*"},
	Caps:               map[string]int{bigGo: 20, shrunkGo: 30, smallGo: 15, goneGo: 40},
	Root:               []string{bigGo, shrunkGo, smallGo, goneGo, "long.go", "pkg", "README.md", "bin", makefile, "data.bin"},
}

func TestStructure(t *testing.T) {
	t.Parallel()
	root, files := repo(t, map[string]string{
		bigGo:             lines(20), // at its cap: fine
		shrunkGo:          lines(25), // under its cap: lower it
		smallGo:           lines(5),  // under the limit: remove the cap
		"long.go":         lines(11), // over the limit, no cap
		"pkg/a.go":        lines(1),
		"pkg/b.go":        lines(1),
		"pkg/c.go":        lines(1),
		"pkg/d.go":        lines(1),
		"pkg/e.go":        lines(1),
		"pkg/f.go":        lines(1), // a sixth code file in pkg/
		"pkg/a_test.go":   lines(1), // tests do not count
		"README.md":       lines(1000),
		"bin/tool":        lines(3),
		makefile:          lines(3),
		"data.bin":        "\x00\x01binary",
		"notes/latin1.go": "caf\xe9\n",
	})
	found, err := checks.Structure(root, files, limits)
	require.NoError(t, err)

	got := messages(found)
	assert.Contains(t, got, "shrunk.go: 25 lines, under its cap of 30: lower the cap to 25")
	assert.Contains(t, got, "small.go: 5 lines, within the limit of 10: remove its cap")
	assert.Contains(t, got, "long.go: 11 lines, over the limit of 10: split it")
	assert.Contains(t, got, "pkg/: 6 code files besides tests, over the limit of 5")
	assert.Contains(t, got, "gone.go: has a cap in gates/limits.yaml but no longer exists")
	assert.Contains(t, got, `notes/latin1.go: top-level entry "notes" is not in structure.root`)
	assert.Contains(t, got, "notes/latin1.go: not valid UTF-8")
	assert.NotContains(t, got, "big.go:", "a file at its cap is fine")
	assert.NotContains(t, got, "README.md", "Markdown is not code")
	assert.NotContains(t, got, "data.bin", "binary files are not checked for text")
	assert.Len(t, found, 7)
}

func TestStructureRefusesGrowthPastACap(t *testing.T) {
	t.Parallel()
	root, files := repo(t, map[string]string{bigGo: lines(21), shrunkGo: lines(30), smallGo: lines(15), goneGo: lines(40)})
	found, err := checks.Structure(root, files, limits)
	require.NoError(t, err)
	assert.Equal(t, "big.go: 21 lines, over its cap of 20: a cap may only go down", found[0].String())
	assert.Len(t, found, 1)
}

func TestPublicSafety(t *testing.T) {
	t.Parallel()

	home := "/Us" + "ers/" // split so that this file does not trip the gate it tests
	root, files := repo(t, map[string]string{
		"CLAUDE.local.md":             "notes",
		".claude/settings.local.json": "{}",
		".claude/settings.json":       "{}",
		".env.template":               "THEHIVE_URL=",
		"docs/how-to.md":              "run it from " + home + "alice/dev/repo\nor from " + home + "you/dev/repo\n",
		".github/workflows/ci.yml":    "GO_CACHE_DIR: /home/runner/.cache/x\n",
		"gates/limits.yaml":           home + "bob/ is named here on purpose\n",
	})
	rules := checks.SafetyRules{
		ForbiddenPaths: []string{".claude/settings.local.json", "CLAUDE.local.md", ".env.*"},
		AllowedPaths:   []string{".env.template"},
		HomeNames:      []string{"runner", "you"},
		SkipContent:    []string{"gates/limits.yaml"},
	}
	found, err := checks.PublicSafety(root, files, rules)
	require.NoError(t, err)

	got := messages(found)
	assert.Contains(t, got, ".claude/settings.local.json: a per-machine or secret-bearing file")
	assert.Contains(t, got, "CLAUDE.local.md: a per-machine or secret-bearing file")
	assert.Contains(t, got, `docs/how-to.md:1: personal home path "`+home+`alice/"`)
	assert.Len(t, found, 3, "templates, placeholders, the CI runner and skipped files pass")
}

const docsRegistry = `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites:
  - {id: everything, paths: [gates/**]}
gates:
  - {id: lint, runs: lint, cadence: [ci], suites: ["*"], budget_s: 1, blocks: x}
  - {id: docs, builtin: docs, cadence: [stop], suites: ["*"], budget_s: 1, blocks: x}
`

func docsRepo(t *testing.T, agents, ci, page string) (string, []string, *registry.Registry) {
	t.Helper()
	root, files := repo(t, map[string]string{
		"AGENTS.md":                     agents,
		".github/workflows/ci.yml":      ci,
		"internal/tools/tool.go":        "package tools\n",
		"internal/tools/schema.go":      "package tools\n",
		"internal/resources/facts/a.go": "package facts\n",
		"gates/registry.yaml":           docsRegistry,
	})
	reg, err := registry.Load(filepath.Join(root, "gates", "registry.yaml"))
	require.NoError(t, err)

	if page == "" {
		page = gatesdoc.Render(reg)
	}

	p := filepath.Join(root, gatesdoc.Path)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(page), 0o600))

	return root, append(files, gatesdoc.Path), reg
}

func TestDocsAcceptsTrueReferences(t *testing.T) {
	t.Parallel()

	agents := "- `internal/tools/` and `internal/tools/{tool,schema}.go`, `internal/resources/*/a.go`, `internal/tools/tool.go:12`.\n" +
		"- not paths: `make check-fast`, `internal/tools/<tool>/`, `facts/*.txt`, `/etc/hosts`, `FOO=bar`.\n" +
		"```text\ninternal/old/ inside a fence is not checked\n```\n"
	root, files, reg := docsRepo(t, agents, "run: bin/harness run --cadence ci --gate lint\n", "")
	found, err := checks.Docs(root, files, reg)
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestDocsFindsDrift(t *testing.T) {
	t.Parallel()

	agents := "- `internal/confirmation/` is gone, and so is `internal/tools/{tool,prompt}.go`.\n"
	root, files, reg := docsRepo(t, agents, "run: bin/harness run --gate ghost\n", "# stale\n")
	found, err := checks.Docs(root, files, reg)
	require.NoError(t, err)

	got := messages(found)
	assert.Contains(t, got, `AGENTS.md:1: names "internal/confirmation/", which does not exist`)
	assert.Contains(t, got, `AGENTS.md:1: names "internal/tools/prompt.go", which does not exist`)
	assert.Contains(t, got, "docs/reference/gates.md: does not match gates/registry.yaml: run make harness-docs")
	assert.Contains(t, got, `does not run gate "lint", whose cadence includes ci`)
	assert.Contains(t, got, `runs gate "ghost", which gates/registry.yaml does not define`)
	assert.Len(t, found, 5)
}

func TestTheRepositoryLimitsParse(t *testing.T) {
	t.Parallel()

	l, err := checks.LoadLimits(filepath.Join("..", "..", "..", "gates", "limits.yaml"))
	require.NoError(t, err)
	assert.Equal(t, 500, l.Structure.MaxLines)
	assert.NotEmpty(t, l.PublicSafety.ForbiddenPaths)
}
